// cmd/approve-queue — one-off dev tool: approves all review_required intel
// records and enqueues intel:promote for each, replicating the review
// approve flow (status transition + review log + asynq task) without
// needing an authenticated admin session.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("JC_DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "JC_DATABASE_URL is required")
		os.Exit(1)
	}
	redisAddr := os.Getenv("JC_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		panic(err)
	}
	defer conn.Close(ctx)

	// Dev reviewer identity — the synthetic Deleted User sentinel.
	reviewer := "ffffffff-0000-0000-0000-000000000001"

	rows, err := conn.Query(ctx,
		`SELECT id, entity_type FROM intel_records WHERE status = 'review_required'`)
	if err != nil {
		panic(err)
	}
	type rec struct {
		id   pgtype.UUID
		kind string
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.kind); err != nil {
			panic(err)
		}
		recs = append(recs, r)
	}
	rows.Close()
	fmt.Printf("found %d records awaiting review\n", len(recs))

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()

	approved, enqueued := 0, 0
	for _, r := range recs {
		tag, err := conn.Exec(ctx,
			`UPDATE intel_records SET status = 'approved'
			 WHERE id = $1 AND status = 'review_required'`, r.id)
		if err != nil {
			fmt.Printf("  %s update failed: %v\n", r.id.String(), err)
			continue
		}
		if tag.RowsAffected() == 0 {
			fmt.Printf("  %s skipped (no longer review_required)\n", r.id.String())
			continue
		}
		approved++

		note := "approved by reviewer (dev bulk-approve)"
		if _, err := conn.Exec(ctx,
			`INSERT INTO intel_review_log (intel_record_id, actor_user_id, action, field_changes, note)
			 VALUES ($1, $2::uuid, 'approve', '[]'::jsonb, $3)`,
			r.id, reviewer, note); err != nil {
			fmt.Printf("  %s review log failed: %v\n", r.id.String(), err)
		}

		payload, _ := json.Marshal(map[string]string{
			"record_id":        r.id.String(),
			"reviewer_user_id": reviewer,
		})
		if _, err := client.EnqueueContext(ctx, asynq.NewTask("intel:promote", payload)); err != nil {
			fmt.Printf("  %s enqueue failed: %v\n", r.id.String(), err)
			continue
		}
		enqueued++
		fmt.Printf("  %s (%s) approved + promote queued\n", r.id.String(), r.kind)
	}
	fmt.Printf("done: %d approved, %d promote tasks queued\n", approved, enqueued)
}
