// Command migrate is the database migration runner for JapaConnect.
//
// It reads SQL files from the migrations/ directory, applies them in
// alphabetical order, and tracks which migrations have already been
// applied in a schema_migrations table. This ensures migrations are
// idempotent — running migrate twice does not re-apply already-applied
// files.
//
// Usage:
//
//	go run ./cmd/migrate
//
// The JC_DATABASE_URL environment variable must point at the target database.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/config"
)

func main() {
	if err := runMigrations(); err != nil {
		slog.Error("migration error", "error", err)
		os.Exit(1)
	}
}

// runMigrations is the full migration lifecycle, extracted from main so
// it is testable and returns errors instead of calling os.Exit.
func runMigrations() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("ensuring schema_migrations table: %w", err)
	}

	migrationsDir := findMigrationsDir()

	files, err := listMigrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("listing migration files: %w", err)
	}

	if len(files) == 0 {
		slog.Info("no migration files found", "directory", migrationsDir)
		return nil
	}

	applied, err := getAppliedMigrations(ctx, pool)
	if err != nil {
		return fmt.Errorf("reading applied migrations: %w", err)
	}

	for _, file := range files {
		name := filepath.Base(file)
		if _, alreadyApplied := applied[name]; alreadyApplied {
			slog.Info("skipping already-applied migration", "file", name)
			continue
		}

		content, err := os.ReadFile(file) //nolint:gosec // G304: migration files from trusted admin-configured directory
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		if err := applyMigration(ctx, pool, name, string(content)); err != nil {
			return fmt.Errorf("applying migration %s: %w", name, err)
		}

		slog.Info("applied migration", "file", name)
	}

	slog.Info("migrations complete", "count", len(files))

	return nil
}

// ensureMigrationsTable creates the schema_migrations table if it does not
// already exist. This table records the filename of every applied migration
// so re-running the migrator is safe.
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

// getAppliedMigrations returns a set of filenames that have already been
// applied to the database.
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
// the filename is not recorded — so re-running the migrator will retry.
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

// listMigrationFiles returns the full paths of all .sql files in the
// migrations directory, sorted alphabetically so they apply in order.
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

		// Never apply teardown migrations forward. Both ".up.sql" and plain
		// "NNNNNN_name.sql" files are forward migrations; ".down.sql" files are
		// rollbacks and must be excluded here. Previously every ".sql" file was
		// globbed and applied in alphabetical order, so ".down.sql" (which sorts
		// before ".up.sql") ran forward — dropping/deleting data and polluting
		// schema_migrations. See AUDIT-BE-003.
		if strings.HasSuffix(entry.Name(), ".down.sql") {
			continue
		}

		files = append(files, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(files)

	return files, nil
}

// findMigrationsDir locates the migrations directory relative to the
// current working directory. It checks for ./migrations first, then
// ../migrations (when running from a subdirectory), so the migrator
// works regardless of where it is invoked.
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
