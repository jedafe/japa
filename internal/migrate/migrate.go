// Package migrate contains the shared forward-only database migration
// runner used by cmd/migrate (the production migrator) and internal/testdb
// (the integration-test harness).
//
// ADR-061 requires the integration-test harness to reuse the migrator's
// forward-only apply logic rather than reimplement it, so the two paths
// can never drift. Both callers go through ApplyPending (or the individual
// primitives it composes), which is the single source of truth for:
//
//   - the schema_migrations tracking table shape,
//   - the forward-only file selection rule (apply "NNNNNN_name.sql" and
//     "NNNNNN_name.up.sql"; skip "NNNNNN_name.down.sql" rollbacks —
//     AUDIT-BE-003), and
//   - the per-migration transactional apply + record sequence.
//
// All functions are safe to call more than once against the same database:
// already-applied filenames are skipped, so re-running is idempotent.
package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureMigrationsTable creates the schema_migrations tracking table if it
// does not already exist. The table records the filename of every applied
// migration so re-running the migrator is safe.
func EnsureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
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

// GetAppliedMigrations returns the set of filenames already recorded as
// applied in schema_migrations.
func GetAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
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

// ApplyMigration executes a single migration's SQL in a transaction and
// records it as applied. If the SQL fails, the transaction rolls back and
// the filename is not recorded, so a retry will apply it again.
//
// The filename is re-checked inside the transaction before the body runs:
// ApplyPending snapshots the applied set once, so a filename registered
// mid-run (e.g. by 000000_register_renumbered_migrations.sql marking a
// renumbered file as applied) or by a concurrent migrator process must not
// execute its body again.
func ApplyMigration(ctx context.Context, pool *pgxpool.Pool, filename, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var alreadyApplied bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = $1)", filename).Scan(&alreadyApplied); err != nil {
		return fmt.Errorf("checking applied state: %w", err)
	}

	if alreadyApplied {
		return nil
	}

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

// ListMigrationFiles returns the full paths of all forward migration files
// in dir, sorted so they apply in order. Both "NNNNNN_name.sql" and
// "NNNNNN_name.up.sql" are forward migrations; "NNNNNN_name.down.sql"
// files are rollbacks and are excluded (AUDIT-BE-003).
func ListMigrationFiles(dir string) ([]string, error) {
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

		// Never apply teardown migrations forward. Both ".up.sql" and plain
		// "NNNNNN_name.sql" files are forward migrations; ".down.sql" files
		// are rollbacks and must be excluded here. Previously every ".sql"
		// file was globbed and applied in alphabetical order, so ".down.sql"
		// (which sorts before ".up.sql") ran forward — dropping/deleting
		// data and polluting schema_migrations. See AUDIT-BE-003.
		if strings.HasSuffix(entry.Name(), ".down.sql") {
			continue
		}

		files = append(files, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(files)

	return files, nil
}

// FindMigrationsDir locates the migrations directory relative to the
// current working directory, checking ./migrations, ../migrations, and
// ../../migrations so the runner resolves correctly whether the binary
// runs from the repo root, cmd/migrate, or internal/testdb.
func FindMigrationsDir() string {
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

// duplicatePrefixFiles returns a human-readable list of migration files that
// share the same numeric prefix (e.g. two files numbered 000958). The 000735
// baseline schema/seed pair is intentionally exempt — it is a designed pair,
// not an authoring collision.
func duplicatePrefixFiles(files []string) []string {
	seen := map[string]string{}

	var dups []string

	for _, f := range files {
		name := filepath.Base(f)
		prefix, _, ok := strings.Cut(name, "_")
		if !ok || prefix == "000735" {
			continue
		}

		if prev, dup := seen[prefix]; dup {
			dups = append(dups, fmt.Sprintf("%s (%s, %s)", prefix, prev, name))
			continue
		}

		seen[prefix] = name
	}

	return dups
}

// ApplyPending ensures the schema_migrations table exists, lists the
// forward migration files in dir, and applies every one not yet recorded,
// in alphabetical (i.e. numeric-prefix) order. It returns the filenames
// of the migrations it newly applied. Already-applied files are skipped,
// so this is safe to call more than once against the same database.
//
// This is the single forward-only apply path reused by cmd/migrate and
// the integration-test harness (ADR-061).
func ApplyPending(ctx context.Context, pool *pgxpool.Pool, dir string) ([]string, error) {
	if err := EnsureMigrationsTable(ctx, pool); err != nil {
		return nil, fmt.Errorf("ensuring schema_migrations table: %w", err)
	}

	files, err := ListMigrationFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("listing migration files: %w", err)
	}

	// Duplicate numeric prefixes are almost always an authoring accident:
	// apply order between same-numbered files falls back to lexical sort
	// rather than intent, and the filename-keyed tracker makes later renames
	// painful. Fail fast so the collision is caught at migrate time instead
	// of silently interleaving on fresh environments. The 000735 baseline
	// pair (schema + seed) is the one intentional exception.
	if dups := duplicatePrefixFiles(files); len(dups) > 0 {
		return nil, fmt.Errorf("duplicate migration number prefixes: %s", strings.Join(dups, ", "))
	}

	applied, err := GetAppliedMigrations(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("reading applied migrations: %w", err)
	}

	var newlyApplied []string

	for _, file := range files {
		name := filepath.Base(file)

		if applied[name] {
			continue
		}

		// #nosec G304 // migration files are loaded from a trusted, configured repo directory.
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading migration %s: %w", name, err)
		}

		if err := ApplyMigration(ctx, pool, name, string(content)); err != nil {
			return nil, fmt.Errorf("applying migration %s: %w", name, err)
		}

		newlyApplied = append(newlyApplied, name)
	}

	return newlyApplied, nil
}
