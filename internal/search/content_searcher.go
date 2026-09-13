// Package search — content_searcher.go implements the content.ContentSearcher
// interface using the posts index. When wired into the content service,
// list-page ?q= queries use the search engine (typo tolerance, ranking)
// instead of PostgreSQL ILIKE. Ref: docs/PERFORMANCE_AUDIT.md §4.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ContentSearchAdapter implements content.ContentSearcher using the search
// engine. It searches the posts index (contents table) scoped to the
// requesting tenant and returns matching content IDs for subsequent
// PostgreSQL enrichment.
type ContentSearchAdapter struct {
	engine Engine
}

// NewContentSearchAdapter creates a ContentSearchAdapter from a search engine.
// Pass nil to create a no-op adapter (returns empty results) — useful for tests.
func NewContentSearchAdapter(engine Engine) *ContentSearchAdapter {
	return &ContentSearchAdapter{engine: engine}
}

// SearchContentIDs searches the posts index for content matching the query,
// scoped to the given tenant, and returns the matching content IDs.
func (a *ContentSearchAdapter) SearchContentIDs(ctx context.Context, tenantID pgtype.UUID, query string, limit int) ([]pgtype.UUID, error) {
	if a.engine == nil {
		return nil, errors.New("search engine not configured")
	}

	if query == "" {
		return []pgtype.UUID{}, nil
	}

	if limit <= 0 {
		limit = 20
	}

	resp, err := a.engine.Search(ctx, IndexPosts, Query{
		IndexUID: IndexPosts,
		Text:     query,
		Limit:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("content search: %w", err)
	}

	ids := make([]pgtype.UUID, 0, len(resp.Hits))

	for _, hit := range resp.Hits {
		rawID, ok := hit["id"]
		if !ok {
			continue
		}

		var idStr string
		if err := json.Unmarshal(rawID, &idStr); err != nil {
			continue
		}

		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}

		ids = append(ids, pgtype.UUID{Bytes: id, Valid: true})
	}

	return ids, nil
}
