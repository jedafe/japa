// Package search provides Meilisearch-backed search for JapaConnect.
//
// content_searcher.go implements the content.ContentSearcher interface
// using Meilisearch's posts index. When wired into the content service,
// list-page ?q= queries use Meilisearch (typo tolerance, ranking) instead
// of PostgreSQL ILIKE. Ref: docs/PERFORMANCE_AUDIT.md §4.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meilisearch/meilisearch-go"
)

// ContentSearchAdapter implements content.ContentSearcher using Meilisearch.
// It searches the posts index (contents table) scoped to the requesting tenant
// and returns matching content IDs for subsequent PostgreSQL enrichment.
type ContentSearchAdapter struct {
	client meilisearch.ServiceManager
}

// NewContentSearchAdapter creates a ContentSearchAdapter from a Meilisearch client.
// Pass nil to create a no-op adapter (returns empty results) — useful for tests.
func NewContentSearchAdapter(client meilisearch.ServiceManager) *ContentSearchAdapter {
	return &ContentSearchAdapter{client: client}
}

// SearchContentIDs searches the posts index for content matching the query,
// scoped to the given tenant, and returns the matching content IDs.
func (a *ContentSearchAdapter) SearchContentIDs(ctx context.Context, tenantID pgtype.UUID, query string, limit int) ([]pgtype.UUID, error) {
	if a.client == nil {
		return nil, errors.New("meilisearch client not configured")
	}

	if query == "" {
		return []pgtype.UUID{}, nil
	}

	if limit <= 0 {
		limit = 20
	}

	tenantIDStr := tenantID.String()

	resp, err := a.client.Index(IndexPosts).Search(query, &meilisearch.SearchRequest{
		Filter: "tenant_id = " + tenantIDStr,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("meilisearch search: %w", err)
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
