// Command api is the main entry point for the JapaConnect HTTP API.
//
// It loads configuration from JC_ environment variables, creates a database
// connection pool, builds the Chi router with all routes and middleware,
// and starts an HTTP server. On SIGINT or SIGTERM it shuts down gracefully,
// letting in-flight requests complete before exiting.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	errtracking "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/server"
	"github.com/jedafe/japa/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application error", "error", err)
		os.Exit(1)
	}
}

// run contains the full application lifecycle so main stays thin and
// testable. It returns an error instead of calling os.Exit so callers
// can inspect the result.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Flush error tracking on any exit path. Deferred early so it runs
	// after config is loaded but before anything else — covers normal
	// shutdown, signal-triggered exit, and fatal errors. See AUDIT-BE-002.
	defer errtracking.Flush(2)

	slog.Info("starting JapaConnect API", "config", cfg.String())

	// Create the database pool. This pings the database on startup so
	// we fail fast if PostgreSQL is unreachable. Pool tuning comes from
	// JC_DB_* environment variables; zero values fall back to defaults.
	// When JC_DATABASE_READER_URL is set, reads go to the replica and
	// writes go to the primary (db.NewWithReplica). When it is empty,
	// both pools point at the primary (db.NewWithConfig).
	poolCfg := db.PoolConfig{
		MaxConns:        cfg.DBMaxConns,
		MinConns:        cfg.DBMinConns,
		MaxConnLifetime: time.Duration(cfg.DBMaxConnLifetimeMs) * time.Millisecond,
		MaxConnIdleTime: time.Duration(cfg.DBMaxConnIdleTimeMs) * time.Millisecond,
	}

	var database *db.DB

	if cfg.DatabaseReaderURL != "" {
		slog.Info("database: read replica configured", "reader_url", cfg.DatabaseReaderURL)
		database, err = db.NewWithReplica(context.Background(), cfg.DatabaseURL, cfg.DatabaseReaderURL, poolCfg)
	} else {
		database, err = db.NewWithConfig(context.Background(), cfg.DatabaseURL, poolCfg)
	}

	if err != nil {
		return fmt.Errorf("creating database pool: %w", err)
	}

	defer database.Close()

	// Build the HTTP server with the configured port.
	workerCfg := worker.DefaultConfig()
	router := server.NewServer(database, &cfg)
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start the Asynq background worker. The worker shares the same
	// Valkey/Redis instance used for caching. If the worker fails to start
	// we log the error but continue — the HTTP server remains functional.
	worker.RegisterDBPool(database)

	workerShutdown, workerErr := server.StartWorker(workerCfg)
	if workerErr != nil {
		slog.Error("failed to start Asynq worker", "error", workerErr)
	}

	// Start the Asynq scheduler for periodic tasks (scheduled publishing,
	// velocity score recalculation). Shares the same Redis connection.
	// Only ONE process may run the scheduler — when api replicas scale >1,
	// set JC_SCHEDULER_ENABLED=0 on the extras so periodic tasks don't
	// enqueue once per replica.
	schedulerEnabled := os.Getenv("JC_SCHEDULER_ENABLED") != "0"
	if schedulerEnabled {
		scheduler := worker.NewScheduler(workerCfg)
		go func() {
			slog.Info("asynq scheduler starting", "addr", worker.Addr(workerCfg))

			if err := scheduler.Run(); err != nil {
				slog.Error("asynq scheduler stopped", "error", err)
			}
		}()

		defer scheduler.Shutdown()
	}

	defer func() {
		if workerShutdown != nil {
			workerShutdown()
		}

		server.Shutdown()
	}()

	// JC_HTTP_ENABLED=0 runs the process as a worker-only node (no HTTP
	// listener) — pair with JC_SCHEDULER_ENABLED=0 on the api replicas for a
	// split deployment: fat worker pods for heavy jobs, thin api pods for HTTP.
	// Default stays all-in-one for single-node deploys.
	httpEnabled := os.Getenv("JC_HTTP_ENABLED") != "0"

	var errChan chan error
	if httpEnabled {
		// Start listening in a goroutine so we can handle shutdown signals.
		errChan = make(chan error, 1)
		go func() {
			slog.Info("HTTP server listening", "port", cfg.Port)

			if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errChan <- fmt.Errorf("HTTP server: %w", err)
			}

			close(errChan)
		}()
	} else {
		slog.Info("worker-only mode — HTTP listener disabled")
	}

	// Wait for SIGINT or SIGTERM to trigger graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining connections")
	}

	if httpEnabled {
		// Give in-flight requests up to 30 seconds to complete.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down HTTP server: %w", err)
		}
	}

	slog.Info("shutdown complete")

	return nil
}
