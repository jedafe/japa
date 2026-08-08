// Command requeue-universities re-enqueues space:auto_populate tasks for
// all university spaces that have no wiki_content yet. Run once after
// deploying the fixed Wikidata enrichment code.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/hibiken/asynq"
)

func main() {
	dbURL := os.Getenv("JC_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("JC_DATABASE_URL not set")
	}
	redisAddr := os.Getenv("JC_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()

	rows, err := pool.Query(context.Background(), `
		SELECT id::text
		FROM spaces
		WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
		  AND space_type_id = (SELECT id FROM space_types WHERE slug='university' AND tenant_id='00000000-0000-0000-0000-000000000001')
		  AND deleted_at IS NULL
		  AND (wiki_content IS NULL OR wiki_content = '{}'::jsonb)
	`)
	if err != nil {
		log.Fatalf("querying universities: %v", err)
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
		payload, _ := json.Marshal(map[string]string{
			"space_id":  spaceID,
			"tenant_id": "00000000-0000-0000-0000-000000000001",
		})
		_, err := client.Enqueue(
			asynq.NewTask("space:auto_populate", payload),
			asynq.Queue("low"),
			asynq.MaxRetry(25),
			asynq.Timeout(60*1000*1000*1000), // 60s
		)
		if err != nil {
			log.Printf("enqueue error for %s: %v", spaceID, err)
			continue
		}
		count++
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows error: %v", err)
	}

	fmt.Printf("Enqueued %d space:auto_populate tasks\n", count)
}
