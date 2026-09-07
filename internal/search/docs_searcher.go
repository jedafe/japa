package search

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/docs"
)

// docPageHit is the document shape returned by the doc_pages search index.
type docPageHit struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Audience string `json:"audience"`
	Excerpt  string `json:"excerpt"`
}

// DocPagesSearchAdapter implements docs.DocPagesSearcher using the search
// engine. It searches the doc_pages index scoped to the requesting tenant
// and audience set, returning lightweight search results for the docs nav
// and search UI.
type DocPagesSearchAdapter struct {
	engine Engine
}

// NewDocPagesSearchAdapter creates a DocPagesSearchAdapter from a search
// engine. Pass nil to create a no-op adapter (returns empty results) —
// useful for tests.
func NewDocPagesSearchAdapter(engine Engine) *DocPagesSearchAdapter {
	return &DocPagesSearchAdapter{engine: engine}
}

// SearchDocPages searches the doc_pages index for published pages matching the
// query, scoped to the tenant and audience list. Empty query returns all
// published pages in the audiences (limited by limit).
func (a *DocPagesSearchAdapter) SearchDocPages(ctx context.Context, tenantID pgtype.UUID, query string, audiences []string, limit int32) ([]docs.DocSearchResult, error) {
	if a.engine == nil {
		return nil, errors.New("search engine not configured")
	}

	filters := []FilterExpr{
		Eq{Field: "tenant_id", Value: tenantID.String()},
		Eq{Field: "review_status", Value: "published"},
	}

	if len(audiences) > 0 {
		filters = append(filters, In{Field: "audience", Values: audiences})
	}

	resp, err := a.engine.Search(ctx, IndexDocPages, Query{
		IndexUID: IndexDocPages,
		Text:     query,
		Filter:   andAll(filters...),
		Limit:    int(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("doc page search failed: %w", err)
	}

	results := make([]docs.DocSearchResult, 0, len(resp.Hits))

	for _, h := range resp.Hits {
		var hit docPageHit
		if err := h.DecodeInto(&hit); err != nil {
			continue
		}

		if hit.Title == "" && hit.Slug == "" {
			continue
		}

		results = append(results, docs.DocSearchResult{
			ID:       hit.ID,
			Title:    hit.Title,
			Slug:     hit.Slug,
			Audience: hit.Audience,
			Excerpt:  hit.Excerpt,
		})
	}

	return results, nil
}
