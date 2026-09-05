package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/docs"
	"github.com/meilisearch/meilisearch-go"
)

// docPageHit is the document shape returned by the doc_pages Meilisearch index.
type docPageHit struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Audience string `json:"audience"`
	Excerpt  string `json:"excerpt"`
}

// DocPagesSearchAdapter implements docs.DocPagesSearcher using Meilisearch.
// It searches the doc_pages index scoped to the requesting tenant and audience
// set, returning lightweight search results for the docs nav and search UI.
type DocPagesSearchAdapter struct {
	client meilisearch.ServiceManager
}

// NewDocPagesSearchAdapter creates a DocPagesSearchAdapter from a Meilisearch client.
// Pass nil to create a no-op adapter (returns empty results) — useful for tests.
func NewDocPagesSearchAdapter(client meilisearch.ServiceManager) *DocPagesSearchAdapter {
	return &DocPagesSearchAdapter{client: client}
}

// SearchDocPages searches the doc_pages index for published pages matching the
// query, scoped to the tenant and audience list. Empty query returns all
// published pages in the audiences (limited by limit).
func (a *DocPagesSearchAdapter) SearchDocPages(ctx context.Context, tenantID pgtype.UUID, query string, audiences []string, limit int32) ([]docs.DocSearchResult, error) {
	if a.client == nil {
		return nil, errors.New("meilisearch client not configured")
	}

	tenantFilter := fmt.Sprintf("tenant_id = '%s'", quoteMeiliFilter(tenantID.String()))
	statusFilter := "review_status = 'published'"
	audienceFilter := buildAudienceFilter(audiences)

	filter := fmt.Sprintf("%s AND %s", tenantFilter, statusFilter)
	if audienceFilter != "" {
		filter = fmt.Sprintf("%s AND (%s)", filter, audienceFilter)
	}

	resp, err := a.client.Index(IndexDocPages).Search(query, &meilisearch.SearchRequest{
		Filter: filter,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("meilisearch doc page search failed: %w", err)
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

func buildAudienceFilter(audiences []string) string {
	if len(audiences) == 0 {
		return ""
	}

	parts := make([]string, 0, len(audiences))
	for _, a := range audiences {
		parts = append(parts, fmt.Sprintf("audience = '%s'", quoteMeiliFilter(a)))
	}

	return strings.Join(parts, " OR ")
}

func quoteMeiliFilter(s string) string {
	// Meilisearch filter strings use single quotes; escape embedded single quotes
	// by doubling them.
	return strings.ReplaceAll(s, "'", "''")
}
