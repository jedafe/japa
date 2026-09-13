package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/worker"
)

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dbURL := resolveDatabaseURL()
	redisAddr := resolveRedisAddr()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT id
		FROM intel_sources
		WHERE status = 'active'
		  AND enabled = true
		  AND (description IS NULL OR description = ''
		       OR logo_url IS NULL OR logo_url = ''
		       OR keywords IS NULL OR keywords = '{}')
		ORDER BY created_at
	`)
	if err != nil {
		slog.Error("failed to query sources missing metadata", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	type sourceRow struct {
		ID       pgtype.UUID
		TenantID pgtype.UUID
	}

	var sources []sourceRow
	for rows.Next() {
		var s sourceRow
		if err := rows.Scan(&s.ID, &s.TenantID); err != nil {
			slog.Warn("skipping row: scan error", "error", err)
			continue
		}
		sources = append(sources, s)
	}

	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "error", err)
		os.Exit(1)
	}

	if len(sources) == 0 {
		slog.Info("no sources found with missing metadata, nothing to backfill")
		return
	}

	slog.Info("found sources with missing metadata", "count", len(sources))

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()

	enqueued := 0
	skipped := 0

	for _, s := range sources {
		payload, err := json.Marshal(map[string]string{
			"source_id": s.ID.String(),
			"tenant_id": s.TenantID.String(),
		})
		if err != nil {
			slog.Warn("skipping: failed to marshal payload",
				"source_id", s.ID.String(), "error", err)
			skipped++
			continue
		}

		task := asynq.NewTask(worker.TypeIntelSourceMetadata, payload)

		info, err := client.Enqueue(task, asynq.Queue("default"))
		if err != nil {
			slog.Warn("skipping: failed to enqueue",
				"source_id", s.ID.String(), "error", err)
			skipped++
			continue
		}

		slog.Info("enqueued source metadata task",
			"source_id", s.ID.String(),
			"tenant_id", s.TenantID.String(),
			"task_id", info.ID)

		enqueued++
	}

	slog.Info("backfill complete",
		"total_sources", len(sources),
		"enqueued", enqueued,
		"skipped", skipped)
	fmt.Printf("\nBackfill complete: %d metadata tasks enqueued, %d skipped\n", enqueued, skipped)
}

func resolveDatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		slog.Info("using DATABASE_URL from environment")
		return url
	}

	if url := os.Getenv("JC_DATABASE_URL"); url != "" {
		slog.Info("using JC_DATABASE_URL from environment")
		return url
	}

	if url := readEnvFile("JC_DATABASE_URL"); url != "" {
		slog.Info("using JC_DATABASE_URL from .env")
		return url
	}

	slog.Info("no database URL found, reading DB_PASSWORD from .env")

	password := readEnvFile("DB_PASSWORD")
	if password == "" {
		slog.Error("no DATABASE_URL, JC_DATABASE_URL, or DB_PASSWORD found")
		os.Exit(1)
	}

	url := fmt.Sprintf("postgres://japa:%s@localhost:5432/japa?sslmode=disable", password)
	slog.Info("constructed DATABASE_URL from .env", "host", "localhost", "db", "japa")
	return url
}

func resolveRedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}

	if addr := os.Getenv("REDIS_URL"); addr != "" {
		return addr
	}

	return "localhost:6379"
}

func readEnvFile(key string) string {
	data, err := os.ReadFile(".env")
	if err != nil {
		return ""
	}

	prefix := key + "="

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}

	return ""
}
