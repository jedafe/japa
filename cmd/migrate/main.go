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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/migrate"
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

	migrationsDir := migrate.FindMigrationsDir()

	files, err := migrate.ListMigrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("listing migration files: %w", err)
	}

	if len(files) == 0 {
		slog.Info("no migration files found", "directory", migrationsDir)
		return nil
	}

	applied, err := migrate.ApplyPending(ctx, pool, migrationsDir)
	if err != nil {
		return err
	}

	for _, name := range applied {
		slog.Info("applied migration", "file", name)
	}

	slog.Info("migrations complete", "count", len(files))

	return nil
}
