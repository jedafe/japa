// Package search provides HTTP handlers for the search endpoint.
// Currently returns stub results; Meilisearch integration is deferred.
package search

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for search HTTP handlers.
type Handlers struct{}

// NewHandlers creates Handlers.
func NewHandlers() *Handlers {
	return &Handlers{}
}

// SearchResult represents a single search result.
type SearchResult struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
	URL     string `json:"url"`
}

// SearchResponse is the API response for search.
type SearchResponse struct {
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
// Returns stub results for now; Meilisearch integration is deferred.
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

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	_ = t // tenant-scoped search would go here
	_ = limit // would be used to limit results

	// Stub: return empty results until Meilisearch is integrated.
	writeJSON(w, http.StatusOK, map[string]any{
		"data": SearchResponse{
			Results: []SearchResult{},
		},
	})
}
