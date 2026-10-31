// Command bulk-enqueue fans out seed/enrichment Asynq tasks over DB-selected
// spaces. Modes:
//
//	facts        — wiki:populate_facts for every country/state/city/education space
//	attractions  — wiki:seed_attractions for every country space
//	universities — space:auto_populate for education spaces missing media or wiki content
//
// Env contract matches cmd/requeue-universities (JC_DATABASE_URL,
// JC_VALKEY_ADDR / JC_REDIS_ADDR).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/worker"
)

func main() {
	mode := flag.String("mode", "", "facts | attractions | universities")
	limit := flag.Int("limit", 100, "per-parent attraction limit")
	dryRun := flag.Bool("dry-run", false, "print selected row count only")
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

	var query, taskType string
	var payloadFor func(spaceID string) map[string]any

	switch *mode {
	case "facts":
		taskType = worker.TypeWikiPopulateFacts
		query = `
			SELECT s.id::text FROM spaces s
			JOIN space_types st ON st.id = s.space_type_id
			WHERE s.deleted_at IS NULL
			  AND st.slug IN ('country','state','city','education')`
		payloadFor = func(id string) map[string]any {
			return map[string]any{"space_id": id}
		}
	case "attractions":
		taskType = worker.TypeWikiSeedAttractions
		query = `
			SELECT s.id::text FROM spaces s
			JOIN space_types st ON st.id = s.space_type_id
			WHERE s.deleted_at IS NULL AND st.slug = 'country'`
		payloadFor = func(id string) map[string]any {
			return map[string]any{"parent_space_id": id, "limit": *limit}
		}
	case "universities":
		taskType = worker.TypeSpaceAutoPopulate
		query = `
			SELECT s.id::text FROM spaces s
			JOIN space_types st ON st.id = s.space_type_id
			WHERE s.deleted_at IS NULL AND st.slug = 'education'
			  AND (s.logo_media_id IS NULL OR s.header_image_id IS NULL
			       OR s.wiki_content IS NULL OR s.wiki_content = '{}'::jsonb)`
		payloadFor = func(id string) map[string]any {
			return map[string]any{"version": "1", "space_id": id}
		}
	default:
		log.Fatal("-mode must be one of: facts, attractions, universities")
	}

	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		log.Fatalf("querying spaces: %v", err)
	}
	defer rows.Close()

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr, DB: 0})
	defer client.Close()

	count, errs := 0, 0
	for rows.Next() {
		var spaceID string
		if err := rows.Scan(&spaceID); err != nil {
			log.Printf("scan error: %v", err)
			continue
		}
		if *dryRun {
			count++
			continue
		}
		payload, _ := json.Marshal(payloadFor(spaceID))
		if _, err := client.Enqueue(asynq.NewTask(taskType, payload), worker.TaskOpts(taskType)...); err != nil {
			log.Printf("enqueue error for %s: %v", spaceID, err)
			errs++
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

	fmt.Printf("mode=%s task=%s processed=%d errors=%d\n", *mode, taskType, count, errs)
}
