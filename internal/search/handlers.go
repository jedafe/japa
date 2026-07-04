// Package search provides Meilisearch-backed search for JapaConnect.
//
// The handlers file serves GET /api/v1/search. It runs a multi-index
// Meilisearch query across content (posts), spaces, and users, scoped to the
// requesting tenant, and returns grouped results. When no Meilisearch client
// is configured the handler degrades gracefully to empty results so the
// server always boots and tests run without a search engine.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/meilisearch/meilisearch-go"
)

// defaultSearchLimit is the number of results returned per index when the
// client does not request a specific limit.
const defaultSearchLimit = 20

// maxSearchLimit caps the per-request limit to prevent abuse.
const maxSearchLimit = 100

// indexTypeByUID maps a Meilisearch index UID to the result type label
// returned in the API response. Only indexes queried by the search endpoint
// appear here.
var indexTypeByUID = map[string]string{
	IndexPosts:        "content",
	IndexForumThreads: "forum_thread",
	IndexSpaces:       "space",
	IndexUsers:        "user",
	IndexTags:         "tag",
}

// searchAttributesToRetrieve limits each hit to the fields the response needs,
// keeping payloads small and the mapping logic simple.
var searchAttributesToRetrieve = []string{"id", "title", "excerpt", "url"}

// Handlers holds dependencies for search HTTP handlers.
type Handlers struct {
	client meilisearch.ServiceManager
}

// NewHandlers creates Handlers backed by the given Meilisearch client.
// Pass nil to disable search at runtime — the handler returns empty results
// instead of querying the engine, which keeps tests and dev boot simple.
func NewHandlers(client meilisearch.ServiceManager) *Handlers {
	return &Handlers{client: client}
}

// SearchResult represents a single search result.
type SearchResult struct { //nolint:revive // stuttering type name preserved for API compatibility
	Type    string `json:"type"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
	URL     string `json:"url"`
}

// SearchResponse is the API response for search.
type SearchResponse struct { //nolint:revive // stuttering type name preserved for API compatibility
	Results []SearchResult `json:"results"`
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// Search handles GET /api/v1/search?q=...&limit=20.
//
// It resolves the authenticated user and tenant from the request context,
// validates the query and limit, then runs a multi-index Meilisearch search
// across content, spaces, and users scoped to the tenant. Results are grouped
// and returned in a single flat list tagged by type.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))

	// Graceful degradation: no Meilisearch client configured or reachable.
	if h.client == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": SearchResponse{Results: []SearchResult{}},
		})

		return
	}

	results, err := h.runMultiSearch(r.Context(), query, limit, t.ID)
	if err != nil {
		slog.Warn("search query failed", "query", query, "error", err)
		writeError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": SearchResponse{Results: results},
	})
}

// runMultiSearch queries the posts, spaces, users, forum_threads, and tags
// indexes in a single Meilisearch multi-search call, scoped to the given
// tenant. It returns the flattened, type-tagged results.
func (h *Handlers) runMultiSearch(ctx context.Context, query string, limit int, tenantID pgtype.UUID) ([]SearchResult, error) {
	tenantFilter := fmt.Sprintf(`tenant_id = "%s"`, tenantID.String())

	req := &meilisearch.MultiSearchRequest{
		Queries: []*meilisearch.SearchRequest{
			{
				IndexUID:             IndexPosts,
				Query:                query,
				Limit:                int64(limit),
				Filter:               tenantFilter,
				AttributesToRetrieve: searchAttributesToRetrieve,
			},
			{
				IndexUID:             IndexSpaces,
				Query:                query,
				Limit:                int64(limit),
				Filter:               tenantFilter,
				AttributesToRetrieve: searchAttributesToRetrieve,
			},
			{
				IndexUID:             IndexUsers,
				Query:                query,
				Limit:                int64(limit),
				Filter:               tenantFilter,
				AttributesToRetrieve: searchAttributesToRetrieve,
			},
			{
				IndexUID:             IndexForumThreads,
				Query:                query,
				Limit:                int64(limit),
				Filter:               tenantFilter,
				AttributesToRetrieve: searchAttributesToRetrieve,
			},
			{
				IndexUID:             IndexTags,
				Query:                query,
				Limit:                int64(limit),
				Filter:               tenantFilter,
				AttributesToRetrieve: searchAttributesToRetrieve,
			},
		},
	}

	resp, err := h.client.MultiSearchWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("multi-search: %w", err)
	}

	return mapSearchResults(resp), nil
}

// mapSearchResults flattens a Meilisearch multi-search response into a single
// type-tagged result list. Results from unknown indexes are skipped so a
// misconfigured engine cannot corrupt the response shape.
func mapSearchResults(resp *meilisearch.MultiSearchResponse) []SearchResult {
	results := make([]SearchResult, 0, resp.EstimatedTotalHits)

	for _, sr := range resp.Results {
		resultType, ok := indexTypeByUID[sr.IndexUID]
		if !ok {
			continue
		}

		for _, hit := range sr.Hits {
			results = append(results, SearchResult{
				Type:    resultType,
				ID:      stringField(hit, "id"),
				Title:   stringField(hit, "title"),
				Excerpt: stringField(hit, "excerpt"),
				URL:     stringField(hit, "url"),
			})
		}
	}

	return results
}

// stringField extracts a string value from a Meilisearch hit by key. Returns
// an empty string when the field is absent or not a string, so a missing
// excerpt never breaks the response.
func stringField(hit meilisearch.Hit, key string) string {
	raw, ok := hit[key]
	if !ok || len(raw) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}

	return s
}

// parseLimit resolves the limit query parameter, falling back to the default
// when it is missing, non-numeric, zero, negative, or above the maximum.
func parseLimit(raw string) int {
	if raw == "" {
		return defaultSearchLimit
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 || parsed > maxSearchLimit {
		return defaultSearchLimit
	}

	return parsed
}
