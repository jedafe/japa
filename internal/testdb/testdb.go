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
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/migrate"
)

// defaultTestDSN is used when JC_TEST_DATABASE_URL is unset. It targets the
// dedicated japa_test database on the same local Postgres instance the app
// and migrator use. The harness intentionally does NOT fall back to
// JC_DATABASE_URL: that variable is the application's DSN and legitimately
// points at the primary "japa" database in dev/prod, so reusing it would
// make the Connect guard refuse every integration test (and, were the guard
// absent, would seed/migrate real data). ADR-061 requires tests to target a
// dedicated test database by default; the only override is JC_TEST_DATABASE_URL.
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
// if set, otherwise the hardcoded japa_test default (ADR-061). It deliberately
// ignores JC_DATABASE_URL — that variable is the application's DSN and points
// at the primary "japa" database in dev/prod, so falling back to it would make
// Connect's primary-database guard refuse every integration test (and, sans
// guard, would risk seeding/migrating real data). The dedicated japa_test
// default is the safe baseline; JC_TEST_DATABASE_URL is the only escape hatch.
func resolveDSN() string {
	if dsn := os.Getenv("JC_TEST_DATABASE_URL"); dsn != "" {
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
// pending forward migration before returning. It reuses the shared
// forward-only apply logic from internal/migrate (ADR-061) so the harness
// and cmd/migrate can never drift.
func connectAndMigrate(ctx context.Context, dsn string) (*db.DB, error) {
	database, err := db.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to test database: %w", err)
	}

	if _, err := migrate.ApplyPending(ctx, database.Writer, migrate.FindMigrationsDir()); err != nil {
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

	_, err := database.Writer.Exec(ctx, `INSERT INTO tenants (id, name, slug) VALUES ($1, $2, $3)`, tenantID, "Test Tenant", slug)
	if err != nil {
		t.Fatalf("testdb: NewTenant: inserting tenant: %v", err)
	}

	groupID := mustUUID()

	_, err = database.Writer.Exec(ctx, `INSERT INTO user_groups (id, name, slug, is_system) VALUES ($1, $2, $3, $4, $5)`, groupID, tenantID, "Members", "members", true)
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
