// Command reindex bulk-syncs all tenant entities into the search engine.
// // The per-entity IndexXxx methods only fire on create/update events, so rows
// that predate the indexer wiring are never indexed without this sweep. Run
// it after provisioning a fresh search engine instance or after schema/index
// changes that require a full re-index:
// //	source .env && go run ./cmd/reindex/
// // An optional tenant ID argument overrides the default tenant:
// //	go run ./cmd/reindex/ <tenant-uuid>
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/search"
)

const defaultTenantID = "00000000-0000-0000-0000-000000000001"

func main() {
	if err := run(); err != nil {
		slog.Error("reindex failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	tenantStr := defaultTenantID
	if len(os.Args) > 1 {
		tenantStr = os.Args[1]
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan(tenantStr); err != nil {
		return fmt.Errorf("invalid tenant UUID %q: %w", tenantStr, err)
	}

	ctx := context.Background()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	searchEngine, err := search.NewEngine(cfg.MeilisearchURL, cfg.MeilisearchKey)
	if err != nil {
		return fmt.Errorf("creating search engine: %w", err)
	}

	indexer := search.NewIndexer(searchEngine, gen.New(database.Reader))

	fmt.Printf("reindex: starting bulk reindex for tenant %s\n", tenantStr)

	summary, err := indexer.ReindexAll(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("reindex: %w", err)
	}

	out, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Printf("reindex: complete\n%s\n", string(out))

	return nil
}
