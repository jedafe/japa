// redrive-intel-extract re-enqueues intel:extract for a source's raw
// documents that never produced a usable record — every intel_record for the
// doc is either absent or an empty stub (extracted='{}'). Use after wiring an
// AI provider for the "extraction" category: docs fetched while no provider
// was configured parked as empty review stubs and detect_changes will not
// re-dispatch them (content hash unchanged → DecisionUnchanged), so they need
// an explicit re-drive.
//
// Manifest docs (application/x-intel-manifest) are skipped — they are
// change-detection signal, not extractable content.
//
// Usage:
//
//	go run ./cmd/redrive-intel-extract -source <uuid> [-dry-run] [-limit N]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

func main() {
	var (
		sourceID = flag.String("source", "", "intel_sources.id to redrive (required)")
		dryRun   = flag.Bool("dry-run", false, "list docs that would be enqueued without enqueueing")
		limit    = flag.Int("limit", 0, "max docs to enqueue (0 = all)")
	)
	flag.Parse()

	if *sourceID == "" {
		slog.Error("-source flag is required")
		os.Exit(1)
	}

	ctx := context.Background()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	conn, err := pgx.Connect(ctx, resolveDatabaseURL())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// Content docs for the source with no successful record: no record at
	// all, or every record is an empty stub. Records with real extractions —
	// including off_topic and rejected ones — mean the doc was already
	// processed and must not be redriven.
	rows, err := conn.Query(ctx, `
		SELECT d.id, d.channel_id
		FROM intel_raw_documents d
		WHERE d.source_id = $1
		  AND (d.content_type IS NULL OR d.content_type <> 'application/x-intel-manifest')
		  AND NOT EXISTS (
		    SELECT 1 FROM intel_records r
		    WHERE r.raw_document_id = d.id
		      AND r.extracted <> '{}'::jsonb
		  )
		ORDER BY d.retrieved_at
	`, *sourceID)
	if err != nil {
		slog.Error("failed to query raw documents", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	type doc struct {
		id        string
		channelID *string
	}

	var docs []doc
	for rows.Next() {
		var d doc
		if err := rows.Scan(&d.id, &d.channelID); err != nil {
			slog.Warn("skipping row: scan error", "error", err)
			continue
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "error", err)
		os.Exit(1)
	}

	if len(docs) == 0 {
		slog.Info("no unprocessed documents found", "source_id", *sourceID)
		return
	}
	if *limit > 0 && len(docs) > *limit {
		docs = docs[:*limit]
	}

	slog.Info("documents to redrive", "count", len(docs), "source_id", *sourceID, "dry_run", *dryRun)
	if *dryRun {
		return
	}

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: resolveRedisAddr()})
	defer client.Close()

	enqueued, failed := 0, 0
	for _, d := range docs {
		p := map[string]string{
			"version":    "v1",
			"raw_doc_id": d.id,
		}
		if d.channelID != nil && *d.channelID != "" {
			p["channel_id"] = *d.channelID
		}

		payload, err := json.Marshal(p)
		if err != nil {
			slog.Warn("skipping: marshal payload", "raw_doc_id", d.id, "error", err)
			failed++
			continue
		}

		info, err := client.Enqueue(asynq.NewTask("intel:extract", payload), asynq.Queue("default"))
		if err != nil {
			slog.Warn("enqueue failed", "raw_doc_id", d.id, "error", err)
			failed++
			continue
		}
		slog.Info("enqueued", "raw_doc_id", d.id, "task_id", info.ID)
		enqueued++
	}

	fmt.Printf("\nRedrive complete: %d extract tasks enqueued, %d failed\n", enqueued, failed)
}

// resolveDatabaseURL reads DATABASE_URL from the environment, falling back to
// JC_DATABASE_URL then the .env file in the working directory.
func resolveDatabaseURL() string {
	for _, key := range []string{"DATABASE_URL", "JC_DATABASE_URL"} {
		if url := os.Getenv(key); url != "" {
			return url
		}
	}
	if url := readEnvFile("JC_DATABASE_URL"); url != "" {
		return url
	}

	slog.Error("no DATABASE_URL or JC_DATABASE_URL found")
	os.Exit(1)
	return ""
}

// resolveRedisAddr reads JC_VALKEY_ADDR (canonical) then JC_REDIS_ADDR
// (legacy alias) and the generic REDIS_ADDR/REDIS_URL fallbacks from the
// environment then .env, defaulting to localhost:6379.
func resolveRedisAddr() string {
	for _, key := range []string{"JC_VALKEY_ADDR", "JC_REDIS_ADDR", "REDIS_ADDR", "REDIS_URL"} {
		if addr := os.Getenv(key); addr != "" {
			return addr
		}
	}
	for _, key := range []string{"JC_VALKEY_ADDR", "JC_REDIS_ADDR"} {
		if addr := readEnvFile(key); addr != "" {
			return addr
		}
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
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return strings.TrimPrefix(strings.TrimSpace(line), prefix)
		}
	}
	return ""
}
