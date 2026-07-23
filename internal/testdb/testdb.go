//go:build integration

// Package testdb provides a shared real-Postgres integration-test harness
// for the JapaConnect backend (ADR-061). It connects to a dedicated
// japa_test database (never the primary japa database), applies all
// forward migrations once per test process, and exposes typed seed helpers
// that give each test its own tenant as a data-isolation boundary — since
// every store query filters by tenant_id (ADR-004), a fresh tenant per
// test can never see another test's rows, so tests stay parallel-safe with
// no truncation or teardown required.
//
// All files in this package carry the "integration" build tag and are
// excluded from the default `go test ./...` run. Execute them with:
//
//	make test-integration
//	go test -tags integration ./...
package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jedafe/japa/internal/db"
)

// defaultTestDSN is used when neither JC_TEST_DATABASE_URL nor
// JC_DATABASE_URL is set. It targets the dedicated japa_test database on
// the same local Postgres instance the app and migrator use.
const defaultTestDSN = "postgresql://japa:japa@localhost:5432/japa_test?sslmode=disable"

// primaryDatabaseName is the development/production database name. Connect
// refuses to run against it under any circumstance, to guarantee seeding
// and migrations never touch real data.
const primaryDatabaseName = "japa"

var (
	// setupOnce guards the single migration-apply pass per test process.
	setupOnce sync.Once

	// sharedDB is the process-wide pool every Connect call returns. It is
	// intentionally never closed by a per-test t.Cleanup: many packages'
	// test binaries call Connect and tests run with t.Parallel(), so
	// closing the shared pool when one test finishes would break every
	// other concurrently running test in the same process. The pool is
	// reclaimed when the test binary process exits.
	sharedDB *db.DB

	// setupErr is the migration/connection error from the one setup
	// attempt, if any. Every Connect call after a failed setup reports it
	// again rather than silently returning a nil/broken pool.
	setupErr error
)

// Connect returns a process-wide connection to the japa_test database. On
// the first call in a test binary it applies every pending forward
// migration (guarded by sync.Once); subsequent calls, including from other
// tests in the same package, reuse the same pool. Connect fails the test
// immediately unless it can positively confirm the resolved DSN targets a
// database other than the primary "japa" — any parse error, an empty name,
// or a name equal to "japa" all fail closed, so a misconfigured or
// adversarial DSN can never run destructive seeding against real data.
func Connect(t *testing.T) *db.DB {
	t.Helper()

	dsn := resolveDSN()

	name, err := resolvedDatabaseName(dsn)
	if err != nil || name == "" || name == primaryDatabaseName {
		t.Fatalf("testdb: refusing to run integration tests: could not confirm the resolved DSN "+
			"targets a database other than the primary %q (resolved database=%q, parse error=%v); "+
			"set JC_TEST_DATABASE_URL to a dedicated test database",
			primaryDatabaseName, name, err)
	}

	setupOnce.Do(func() {
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()

		sharedDB, setupErr = connectAndMigrate(ctx, dsn)
	})

	if setupErr != nil {
		t.Fatalf("testdb: harness setup failed: %v", setupErr)
	}

	return sharedDB
}

// resolveDSN picks the test database connection string: JC_TEST_DATABASE_URL
// first, then JC_DATABASE_URL, then the hardcoded japa_test default (ADR-061).
func resolveDSN() string {
	if dsn := os.Getenv("JC_TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}

	if dsn := os.Getenv("JC_DATABASE_URL"); dsn != "" {
		return dsn
	}

	return defaultTestDSN
}

// resolvedDatabaseName returns the database name pgx will actually connect
// to for dsn, parsed with pgxpool's own parser — the identical path db.New
// uses internally (see internal/db/pool.go's createPool, which calls
// pgxpool.ParseConfig then pgxpool.NewWithConfig). This must not diverge
// from the real connection logic: a hand-rolled net/url.Parse extraction
// disagrees with pgx on at least two DSN forms that a naive check would
// wrongly wave through —
//
//   - libpq keyword/value DSNs ("host=localhost dbname=japa ..."): the
//     whole string lands in a URL's Path, never matching "japa" by name,
//     while pgx correctly resolves Database="japa".
//   - a `?dbname=` query override on a postgresql:// URL
//     ("postgresql://.../japa_test?dbname=japa"): pgx's own parser lets the
//     dbname query parameter override the URL path's database, resolving
//     to "japa", while a Path-only check would still see "japa_test".
//
// Using pgx's parser here closes both gaps because the guard now sees
// exactly what the connection will see. A non-nil error means the DSN
// could not be parsed at all — callers must treat that as "unsafe", not
// "not the primary database".
func resolvedDatabaseName(dsn string) (string, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing dsn: %w", err)
	}

	return cfg.ConnConfig.Database, nil
}

// connectAndMigrate opens the writer/reader pools and applies every
// pending forward migration before returning.
func connectAndMigrate(ctx context.Context, dsn string) (*db.DB, error) {
	database, err := db.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to test database: %w", err)
	}

	if err := applyMigrations(ctx, database.Writer); err != nil {
		database.Close()
		return nil, fmt.Errorf("applying migrations: %w", err)
	}

	return database, nil
}

// NewTenant inserts a fresh tenant plus the minimal FK scaffolding a
// seeded user needs (a default "members" user_groups row) and returns the
// new tenant id. Callers should seed everything under the returned id: a
// fresh tenant per test is a complete data-isolation boundary (ADR-061 §3).
func NewTenant(t *testing.T, database *db.DB) pgtype.UUID {
	t.Helper()

	ctx := t.Context()
	tenantID := mustUUID()
	slug := "test-tenant-" + uuid.New().String()

	_, err := database.Writer.Exec(ctx,
		`INSERT INTO tenants (id, name, slug) VALUES ($1, $2, $3)`,
		tenantID, "Test Tenant", slug)
	if err != nil {
		t.Fatalf("testdb: NewTenant: inserting tenant: %v", err)
	}

	groupID := mustUUID()

	_, err = database.Writer.Exec(ctx,
		`INSERT INTO user_groups (id, tenant_id, name, slug, is_system) VALUES ($1, $2, $3, $4, $5)`,
		groupID, tenantID, "Members", "members", true)
	if err != nil {
		t.Fatalf("testdb: NewTenant: inserting default user_groups row: %v", err)
	}

	return tenantID
}

// mustUUID generates a fresh random UUID as a pgtype.UUID. Scan on a
// freshly generated uuid.New() string cannot fail; a panic here would
// indicate a pgtype library defect, not bad caller input.
func mustUUID() pgtype.UUID {
	var id pgtype.UUID

	if err := id.Scan(uuid.New().String()); err != nil {
		panic(fmt.Sprintf("testdb: impossible uuid scan failure: %v", err))
	}

	return id
}

// --- Migration apply logic -------------------------------------------------
//
// Duplicated from cmd/migrate/main.go's runMigrations/listMigrationFiles/
// applyMigration/findMigrationsDir rather than extracted into a shared
// package. cmd/migrate is a `package main` entrypoint that this task does
// not modify, and this change's file scope is internal/testdb plus
// .jeos/sections/27-devops-testing-qa.md only (see task-0-report.md for the
// extraction recommendation left to a follow-up). The forward-only rule —
// apply plain "NNNNNN_name.sql" and "NNNNNN_name.up.sql" files, skip
// "NNNNNN_name.down.sql" rollbacks — is copied verbatim (AUDIT-BE-003). If
// cmd/migrate's apply logic ever changes, this copy must be updated to
// match.

// applyMigrations ensures the schema_migrations tracking table exists and
// applies every forward migration file not yet recorded there, in
// alphabetical (i.e. numeric-prefix) order. Already-applied files are
// skipped, so this is safe to call once per test-binary process even
// though many test binaries share the same japa_test database.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("ensuring schema_migrations table: %w", err)
	}

	files, err := listMigrationFiles(findMigrationsDir())
	if err != nil {
		return fmt.Errorf("listing migration files: %w", err)
	}

	applied, err := getAppliedMigrations(ctx, pool)
	if err != nil {
		return fmt.Errorf("reading applied migrations: %w", err)
	}

	for _, file := range files {
		name := filepath.Base(file)

		if applied[name] {
			continue
		}

		content, err := os.ReadFile(file) //nolint:gosec // G304: migration files from trusted repo directory
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		if err := applyMigration(ctx, pool, name, string(content)); err != nil {
			return fmt.Errorf("applying migration %s: %w", name, err)
		}
	}

	return nil
}

// ensureMigrationsTable creates the schema_migrations table if it does not
// already exist.
func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename     text PRIMARY KEY,
			applied_at   timestamptz NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	return nil
}

// getAppliedMigrations returns the set of filenames already recorded as
// applied.
func getAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, "SELECT filename FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("querying schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)

	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, fmt.Errorf("scanning migration row: %w", err)
		}

		applied[filename] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating migration rows: %w", err)
	}

	return applied, nil
}

// applyMigration executes a single migration's SQL in a transaction and
// records it as applied. If the SQL fails, the transaction rolls back and
// the filename is not recorded, so a retry will apply it again.
func applyMigration(ctx context.Context, pool *pgxpool.Pool, filename, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("executing SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (filename) VALUES ($1)", filename); err != nil {
		return fmt.Errorf("recording migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// listMigrationFiles returns the full paths of all forward migration files
// in dir, sorted so they apply in order. Both "NNNNNN_name.sql" and
// "NNNNNN_name.up.sql" are forward migrations; "NNNNNN_name.down.sql"
// files are rollbacks and are excluded (AUDIT-BE-003).
func listMigrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var files []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		if strings.HasSuffix(entry.Name(), ".down.sql") {
			continue
		}

		files = append(files, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(files)

	return files, nil
}

// findMigrationsDir locates the migrations directory relative to the
// current working directory, mirroring cmd/migrate/main.go so it resolves
// correctly whether the test binary runs from the repo root, cmd/migrate,
// or internal/testdb.
func findMigrationsDir() string {
	candidates := []string{"migrations", "../migrations", "../../migrations"}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}

	// Fall back to the default — the error will surface when we try to
	// read the directory.
	return "migrations"
}
