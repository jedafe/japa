// Command requeue-universities re-enqueues space:auto_populate tasks for
// all university spaces that have no wiki_content yet. Run once after
// deploying the fixed Wikidata enrichment code.
// -slug re-enqueues a single space by slug (any level/type) — useful for
// backfilling logo/header images on existing geo and university spaces.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/worker"
)

func main() {
	slug := flag.String("slug", "", "requeue a single space by slug (any level/type)")
	types := flag.String("types", "", "comma-separated space-type slugs to backfill, e.g. country,state,city,university — selects spaces missing a logo or header")
	flag.Parse()

	dbURL := os.Getenv("JC_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("JC_DATABASE_URL not set")
	}

	redisAddr := os.Getenv("JC_VALKEY_ADDR")
	if redisAddr == "" {
		redisAddr = os.Getenv("JC_REDIS_ADDR")
	}

	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()

	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
		Err() error
	}

	switch {
	case *slug != "":
		rows, err = pool.Query(context.Background(), `
			SELECT id::text
			FROM spaces
			WHERE slug = $1
			  AND deleted_at IS NULL
		`, *slug)
	case *types != "":
		rows, err = pool.Query(context.Background(), `
			SELECT s.id::text
			FROM spaces s
			JOIN space_types st ON st.id = s.space_type_id
			WHERE st.slug = ANY($1)
			  AND s.deleted_at IS NULL
			  AND (s.logo_media_id IS NULL OR s.header_image_id IS NULL)
		`, strings.Split(*types, ","))
	default:
		rows, err = pool.Query(context.Background(), `
			SELECT id::text
			FROM spaces
			WHERE space_type_id = (SELECT id FROM space_types WHERE slug='university')
			  AND deleted_at IS NULL
			  AND (wiki_content IS NULL OR wiki_content = '{}'::jsonb)
		`)
	}

	if err != nil {
		log.Fatalf("querying spaces: %v", err)
	}

	defer rows.Close()

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr, DB: 0})
	defer client.Close()

	count := 0

	for rows.Next() {
		var spaceID string
		if err := rows.Scan(&spaceID); err != nil {
			log.Printf("scan error: %v", err)
			continue
		}

		// Payload shape matches the canonical space:auto_populate contract
		// (version marker + space_id); TaskOpts restores the registered
		// defaults (Unique dedup, 2m timeout, 10 retries) instead of the
		// ad-hoc 25-retry/60s opts that let rate-limited tasks cycle far
		// longer than designed.
		payload, _ := json.Marshal(map[string]string{
			"version":  "1",
			"space_id": spaceID,
		})

		_, err := client.Enqueue(
			asynq.NewTask(worker.TypeSpaceAutoPopulate, payload),
			worker.TaskOpts(worker.TypeSpaceAutoPopulate)...,
		)
		if err != nil {
			log.Printf("enqueue error for %s: %v", spaceID, err)
			continue
		}

		count++
		if count%5000 == 0 {
			log.Printf("enqueued %d...", count)
		}
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("rows error: %v", err)
	}

	fmt.Printf("Enqueued %d space:auto_populate tasks\n", count)
}
