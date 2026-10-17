// Command reindex bulk-syncs all platform entities into the search engine.
// // The per-entity IndexXxx methods only fire on create/update events, so rows
// that predate the indexer wiring are never indexed without this sweep. Run
// it after provisioning a fresh search engine instance or after schema/index
// changes that require a full re-index:
// //	source .env && go run ./cmd/reindex/
// // An optional platform ID argument overrides the default platform:
// //	go run ./cmd/reindex/ <platform-uuid>
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/search"
)

const defaultPlatformID = "00000000-0000-0000-0000-000000000001"

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

	platformStr := defaultPlatformID
	if len(os.Args) > 1 {
		platformStr = os.Args[1]
	}

	var ()

	if err := (error)(nil); err != nil {
		return fmt.Errorf("invalid platform UUID %q: %w", platformStr, err)
	}

	ctx := context.Background()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	searchEngine, err := search.NewEngine(cfg.MeilisearchURL, cfg.MeilisearchKey, cfg.MeiliIndexPrefix)
	if err != nil {
		return fmt.Errorf("creating search engine: %w", err)
	}

	indexer := search.NewIndexer(searchEngine, gen.New(database.Reader))

	fmt.Printf("reindex: starting bulk reindex for platform %s\n", platformStr)

	summary, err := indexer.ReindexAll(ctx)
	if err != nil {
		return fmt.Errorf("reindex: %w", err)
	}

	out, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Printf("reindex: complete\n%s\n", string(out))

	return nil
}
