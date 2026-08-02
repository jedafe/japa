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
)

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dbURL := resolveDatabaseURL()
	redisAddr := resolveRedisAddr()

	// Connect to Postgres.
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// Fetch all published content items.
	rows, err := conn.Query(ctx, `
		SELECT c.id, c.author_id, c.tenant_id, COALESCE(c.content_type, 'blog_post')
		FROM contents c
		WHERE c.status = 'published'
		ORDER BY c.created_at
	`)
	if err != nil {
		slog.Error("failed to query published content", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	type contentRow struct {
		ContentID   pgtype.UUID
		AuthorID    pgtype.UUID
		TenantID    pgtype.UUID
		ContentType string
	}

	var items []contentRow
	for rows.Next() {
		var item contentRow
		if err := rows.Scan(&item.ContentID, &item.AuthorID, &item.TenantID, &item.ContentType); err != nil {
			slog.Warn("skipping row: scan error", "error", err)
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "error", err)
		os.Exit(1)
	}

	if len(items) == 0 {
		slog.Info("no published content found, nothing to backfill")
		return
	}

	slog.Info("found published content", "count", len(items))

	// Connect to Redis (Asynq).
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()

	enqueued := 0
	skipped := 0

	for _, item := range items {
		// Fan-out type "followers": distribute to author's followers.
		payload, err := json.Marshal(map[string]string{
			"content_id":   item.ContentID.String(),
			"tenant_id":    item.TenantID.String(),
			"author_id":    item.AuthorID.String(),
			"space_id":     pgtype.UUID{}.String(), // zero UUID — no space assignment
			"content_type": item.ContentType,
			"fan_out_type": "followers",
		})
		if err != nil {
			slog.Warn("skipping: failed to marshal payload",
				"content_id", item.ContentID.String(), "error", err)
			skipped++
			continue
		}

		task := asynq.NewTask("content:fan_out_to_feeds", payload)
		info, err := client.Enqueue(task, asynq.Queue("default"))
		if err != nil {
			slog.Warn("skipping: failed to enqueue",
				"content_id", item.ContentID.String(), "error", err)
			skipped++
			continue
		}

		slog.Info("enqueued fan-out task",
			"content_id", item.ContentID.String(),
			"author_id", item.AuthorID.String(),
			"tenant_id", item.TenantID.String(),
			"content_type", item.ContentType,
			"task_id", info.ID)
		enqueued++
	}

	slog.Info("backfill complete",
		"total_content", len(items),
		"enqueued", enqueued,
		"skipped", skipped)
	fmt.Printf("\nBackfill complete: %d fan-out tasks enqueued, %d skipped\n", enqueued, skipped)
}

// resolveDatabaseURL reads DATABASE_URL from the environment, falling back to
// reading DB_PASSWORD from a local .env file to construct a default URL.
func resolveDatabaseURL() string {
	// Priority: DATABASE_URL (env) > JC_DATABASE_URL (env) > JC_DATABASE_URL (.env) > construct from DB_PASSWORD
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

// resolveRedisAddr reads REDIS_ADDR from the environment, defaulting to
// localhost:6379.
func resolveRedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	if addr := os.Getenv("REDIS_URL"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

// readEnvFile reads a key=value pair from a .env file in the working directory.
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
