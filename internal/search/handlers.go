// Package search provides Meilisearch-backed search for JapaConnect.
//
// The handlers file serves search endpoints: multi-index search, autocomplete,
// space-scoped search, tag-scoped search, and admin search analytics.
// When no Meilisearch client is configured the handler degrades gracefully
// to empty results so the server always boots and tests run without a search engine.
package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
	vkcache "github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/meilisearch/meilisearch-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// defaultSearchLimit is the number of results returned per index when the
// client does not request a specific limit.
const defaultSearchLimit = 20

// maxSearchLimit caps the per-request limit to prevent abuse.
const maxSearchLimit = 100

// searchCacheTTL is the TTL for jc:cache:search:* keys. Short enough that
// new content appears quickly, long enough to absorb burst traffic on
// popular queries (BUG-010).
const searchCacheTTL = 2 * time.Minute

// indexTypeByUID maps a Meilisearch index UID to the result type label
// returned in the API response. Only indexes queried by the search endpoint
// appear here.
var indexTypeByUID = map[string]string{
	IndexPosts:             "content",
	IndexForumThreads:      "forum_thread",
	IndexSpaces:            "space",
	IndexUsers:             "user",
	IndexTags:              "tag",
	IndexKnowledgeArticles: "article",
	IndexLearningTracks:    "course",
	IndexDocPages:          "doc_page",
	IndexOrganizations:     "organization",
}

// searchAttributesToRetrieve limits each hit to the fields the response needs,
// keeping payloads small and the mapping logic simple.
var searchAttributesToRetrieve = []string{"id", "title", "excerpt", "url"}

// indexesWithRelevantCountries is the set of Meilisearch indexes that carry
// a `relevant_countries` filterable attribute (ADR-064 + ADR-064a). The
// geo-boost filter is applied to these indexes only.
var indexesWithRelevantCountries = map[string]bool{
	IndexPosts:          true,
	IndexLearningTracks: true,
}

// hasRelevantCountries returns true when the given index UID has a
// relevant_countries filterable attribute configured.
func hasRelevantCountries(indexUID string) bool {
	return indexesWithRelevantCountries[indexUID]
}

// searchSettingsReader is the subset of config.Settings the search handlers need.
type searchSettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// Handlers holds dependencies for search HTTP handlers.
type Handlers struct {
	client   meilisearch.ServiceManager
	queries  *gen.Queries
	settings searchSettingsReader
	cache    valkeycompat.Cmdable // nil disables caching (tests)
}

// NewHandlers creates Handlers backed by the given Meilisearch client.
// Pass nil to disable search at runtime — the handler returns empty results
// instead of querying the engine, which keeps tests and dev boot simple.
func NewHandlers(client meilisearch.ServiceManager) *Handlers {
	return &Handlers{client: client}
}

// SetQueries injects the database queries for analytics logging.
func (h *Handlers) SetQueries(q *gen.Queries) {
	h.queries = q
}

// SetSettings injects the hot-reloadable settings reader (AUDIT-BE-009).
func (h *Handlers) SetSettings(s searchSettingsReader) {
	h.settings = s
}

// SetCache injects a Valkey client for read-through search result caching
// (jc:cache:search:* per valkey-keys-catalog.md). nil disables caching.
func (h *Handlers) SetCache(c valkeycompat.Cmdable) {
	h.cache = c
}

// searchCacheKey builds jc:cache:search:{tenant_id}:{query_hash}:{type}:{geo}:{limit}.
// The query is SHA-256 hashed to keep keys short and avoid storing raw user
// input in Valkey keys. geoCountry is included so visitors from different
// countries don't share a cached result set (ADR-064 geo-boost).
func searchCacheKey(tenantID pgtype.UUID, query, typeFilter, geoCountry string, limit int) string {
	h := sha256.Sum256([]byte(query))
	queryHash := hex.EncodeToString(h[:16]) // 32-char hex, enough for uniqueness
	return vkcache.MustKey("cache:search:%s:%s:%s:%s:%d", tenantID.String(), queryHash, typeFilter, geoCountry, limit)
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
	Total   int            `json:"total"`
	TookMs  int64          `json:"took_ms"`
}

// Search handles GET /api/v1/search?q=...&type=...&limit=20.
//
// It resolves the authenticated user and tenant from the request context,
// validates the query and limit, then runs a multi-index Meilisearch search
// scoped to the tenant. The optional type parameter filters which indexes
// are queried. Results are logged to search_analytics asynchronously.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	// OptionalAuth: guests can search published content (Phase 10.01 public
	// SSR search). A nil user is valid — user-scoped features (analytics
	// logging) are skipped. Ref: sections/13-search.md
	user := auth.FromContext(r.Context())

	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	if len([]rune(query)) < 2 {
		errpkg.WriteError(w, http.StatusBadRequest, "query_too_short", "Query must be at least 2 characters")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	typeFilter := typeFromParam(r.URL.Query().Get("type"))
	geoCountry := visitor.FromContext(r.Context()).ContentCountry

	// §54: ?geo=NG override takes precedence over the visitor's resolved
	// ContentCountry (used by the public search API for testing/locale).
	if geoParam := r.URL.Query().Get("geo"); geoParam != "" {
		geoCountry = strings.ToUpper(strings.TrimSpace(geoParam))
	}

	// §54: build the extra posts-index filter from content_type, space_id,
	// and tag_slug params. These only apply to the posts index.
	extraPostFilter := buildPostFilter(r.URL.Query().Get("content_type"), r.URL.Query().Get("space_id"), r.URL.Query().Get("tag_slug"))

	// §54 RBAC: the users index requires authentication (privacy). Guests
	// can search content, spaces, tags, organizations, etc. but NOT users.
	// When the user is nil (OptionalAuth fell through), strip the users
	// index from the query set so a guest never receives user hits.
	authenticated := user != nil && user.UserID != (pgtype.UUID{})
	if !authenticated && typeFilter == "user" {
		// Guest explicitly requested user-only search — return empty
		// instead of silently broadening to all indexes.
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": SearchResponse{Results: []SearchResult{}},
		})
		return
	}

	// Graceful degradation: no Meilisearch client configured or reachable.
	if h.client == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": SearchResponse{Results: []SearchResult{}},
		})

		return
	}

	// Read-through Valkey cache (jc:cache:search:*, BUG-010). The cache key
	// includes the tenant, query hash, type filter, and limit so identical
	// queries from the same tenant are served from cache. Uses distributed
	// lock stampede protection (BUG-008) to prevent thundering herd on
	// popular queries.
	cacheKey := searchCacheKey(t.ID, query, typeFilter, geoCountry, limit)

	searchStart := time.Now()
	results, err := vkcache.GetOrComputeCompat(r.Context(), h.cache, cacheKey, searchCacheTTL, func(ctx context.Context) ([]SearchResult, error) {
		// §54: guests cannot search the users index. When typeFilter is
		// empty (all indexes), exclude IndexUsers from the query set.
		effectiveFilter := typeFilter
		if !authenticated && effectiveFilter == "" {
			effectiveFilter = "all_no_users"
		}
		return h.runMultiSearchFiltered(ctx, query, limit, t.ID, effectiveFilter, geoCountry, extraPostFilter)
	})
	if err != nil {
		slog.Warn("search query failed", "query", query, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")

		return
	}

	tookMs := time.Since(searchStart).Milliseconds()

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": SearchResponse{
			Results: results,
			Total:   len(results),
			TookMs:  tookMs,
		},
	})

	// Log search analytics asynchronously (fire-and-forget). Guests have a
	// zero UUID — the analytics row is still written for aggregate metrics.
	var userID pgtype.UUID
	if user != nil && user.UserID != (pgtype.UUID{}) {
		userID = user.UserID
	}

	h.logSearchAsync(r.Context(), t.ID, query, len(results), userID, typeFilter)
}

// SearchSpaces handles GET /api/v1/spaces/search — the same multi-search
// pipeline as Search, with the type filter pinned to "space".
func (h *Handlers) SearchSpaces(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	if len([]rune(query)) < 2 {
		errpkg.WriteError(w, http.StatusBadRequest, "query_too_short", "Query must be at least 2 characters")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))

	if h.client == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": SearchResponse{Results: []SearchResult{}}})
		return
	}

	results, err := h.runMultiSearchFiltered(r.Context(), query, limit, t.ID, "space", "", "")
	if err != nil {
		slog.Warn("space search query failed", "query", query, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": SearchResponse{Results: results}})

	var userID pgtype.UUID
	if user.UserID != (pgtype.UUID{}) {
		userID = user.UserID
	}

	h.logSearchAsync(r.Context(), t.ID, query, len(results), userID, "space")
}

// runMultiSearch queries the posts, spaces, users, forum_threads, and tags
// indexes in a single Meilisearch multi-search call, scoped to the given
// tenant. It returns the flattened, type-tagged results.
func (h *Handlers) runMultiSearch(ctx context.Context, query string, limit int, tenantID pgtype.UUID) ([]SearchResult, error) {
	return h.runMultiSearchFiltered(ctx, query, limit, tenantID, "", "", "")
}

// buildPostFilter builds the extra Meilisearch filter expression for the
// posts index from the §54 ?content_type, ?space_id, and ?tag_slug params.
// Returns "" when none are set so no extra filter is applied.
func buildPostFilter(contentType, spaceID, tagSlug string) string {
	var parts []string
	if contentType != "" {
		parts = append(parts, fmt.Sprintf(`content_type = "%s"`, contentType))
	}
	if spaceID != "" {
		parts = append(parts, fmt.Sprintf(`space_id = "%s"`, spaceID))
	}
	if tagSlug != "" {
		parts = append(parts, fmt.Sprintf(`tag_slugs = "%s"`, tagSlug))
	}
	return strings.Join(parts, " AND ")
}

// runMultiSearchFiltered queries indexes filtered by type. When typeFilter
// is empty, all indexes are queried (original behavior). geoCountry, when
// non-empty, adds a soft boost for learning_tracks results whose
// relevant_countries array contains the visitor's country (ADR-064).
// extraPostFilter, when non-empty, is appended to the posts index filter
// (supports §54 ?content_type, ?space_id, ?tag_slug params).
func (h *Handlers) runMultiSearchFiltered(ctx context.Context, query string, limit int, tenantID pgtype.UUID, typeFilter, geoCountry, extraPostFilter string) ([]SearchResult, error) {
	tenantFilter := fmt.Sprintf(`tenant_id = "%s"`, tenantID.String())
	indexes := searchTypeIndexes(typeFilter)

	// A type filter with no backing index (e.g. "organization" before its
	// Meilisearch index is provisioned) yields no queries. Short-circuit to
	// empty results instead of issuing a zero-query multi-search, which
	// Meilisearch rejects with a 400.
	if len(indexes) == 0 {
		return []SearchResult{}, nil
	}

	queries := make([]*meilisearch.SearchRequest, len(indexes))
	for i, idx := range indexes {
		filter := tenantFilter
		// §54: apply content_type / space_id / tag_slug filters to the
		// posts index only. These attributes don't exist on other indexes.
		if idx == IndexPosts && extraPostFilter != "" {
			filter = fmt.Sprintf(`%s AND %s`, tenantFilter, extraPostFilter)
		}
		// ADR-064 + ADR-064a: for indexes that carry a relevant_countries
		// filterable attribute (posts, learning_tracks), boost results
		// relevant to the visitor's country. We use an OR filter so
		// globally-relevant content (empty relevant_countries) still appears.
		//
		// `relevant_countries` is an array of ISO codes. Meilisearch
		// array-membership filtering uses the `=` operator (or `IN [...]`).
		// Do NOT use `CONTAINS` — that is a substring operator on strings
		// requiring the experimental "contains filter" feature; using it
		// makes Meilisearch reject the entire multi-search with a 400,
		// breaking search for every user with a non-empty ContentCountry.
		if geoCountry != "" && hasRelevantCountries(idx) {
			filter = fmt.Sprintf(`%s AND (relevant_countries IS EMPTY OR relevant_countries = "%s")`,
				filter, geoCountry)
		}
		queries[i] = &meilisearch.SearchRequest{
			IndexUID:             idx,
			Query:                query,
			Limit:                int64(limit),
			Filter:               filter,
			AttributesToRetrieve: searchAttributesToRetrieve,
		}
	}

	req := &meilisearch.MultiSearchRequest{
		Queries: queries,
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

// maxAutocompleteLimit caps autocomplete results.
const maxAutocompleteLimit = 8

// AutocompleteSuggestion is a single autocomplete suggestion.
type AutocompleteSuggestion struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Excerpt     string `json:"excerpt"`
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	Level       string `json:"level,omitempty"`
	Slug        string `json:"slug,omitempty"`
}

// AutocompleteResponse is the API response for autocomplete.
type AutocompleteResponse struct {
	Suggestions []AutocompleteSuggestion `json:"suggestions"`
}

// Autocomplete handles GET /api/v1/search/autocomplete?q=...&limit=5.
//
// Returns lightweight suggestions across users, spaces, tags, and content.
// Maximum 8 results. NOT logged to search analytics per Section 54.
func (h *Handlers) Autocomplete(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	if len([]rune(query)) < 2 {
		errpkg.WriteError(w, http.StatusBadRequest, "query_too_short", "Query must be at least 2 characters")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	cap := maxAutocompleteLimit
	if h.settings != nil {
		cap = h.settings.GetIntOr(r.Context(), t.ID, "search.autocomplete_max", maxAutocompleteLimit)
	}
	if limit > cap {
		limit = cap
	}

	// §54: ?type=user|space|tag|content (default: all). Filter the
	// autocomplete indexes accordingly. An unrecognized value defaults
	// to all indexes.
	typeFilter := r.URL.Query().Get("type")

	if h.client == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": AutocompleteResponse{Suggestions: []AutocompleteSuggestion{}},
		})
		return
	}

	suggestions, err := h.runAutocomplete(r.Context(), query, limit, t.ID, typeFilter)
	if err != nil {
		slog.Warn("autocomplete query failed", "query", query, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")
		return
	}

	// Section 41 — filter out blocked users from @mention autocomplete
	// suggestions. A user should not see (and therefore @mention) someone
	// who has blocked them or whom they have blocked. Best-effort: a
	// database error degrades to unfiltered results.
	if user := auth.FromContext(r.Context()); user != nil && h.queries != nil {
		suggestions = h.filterBlockedUsers(r.Context(), t.ID, user.UserID, suggestions)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": AutocompleteResponse{Suggestions: suggestions},
	})
}

// filterBlockedUsers removes user-type suggestions where a block relationship
// exists between the current user and the suggested user (either direction).
// Non-user suggestions are passed through unchanged. Best-effort: on database
// error the suggestion is kept (fail-open for visibility).
func (h *Handlers) filterBlockedUsers(ctx context.Context, tenantID, currentUserID pgtype.UUID, suggestions []AutocompleteSuggestion) []AutocompleteSuggestion {
	filtered := suggestions[:0]
	for _, s := range suggestions {
		if s.Type != "user" {
			filtered = append(filtered, s)
			continue
		}
		var uid pgtype.UUID
		if err := uid.Scan(s.ID); err != nil {
			filtered = append(filtered, s)
			continue
		}
		blocked, err := h.queries.IsBlocked(ctx, gen.IsBlockedParams{
			BlockerID: currentUserID,
			BlockedID: uid,
			TenantID:  tenantID,
		})
		if err != nil {
			slog.Warn("autocomplete: IsBlocked check failed",
				"user_id", uid.String(), "error", err)
			filtered = append(filtered, s)
			continue
		}
		if !blocked {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// runAutocomplete queries the users, spaces, tags, and posts indexes with
// a small limit for typeahead suggestions. typeFilter (§54: user|space|tag|
// content) restricts which indexes are queried; empty means all four.
func (h *Handlers) runAutocomplete(ctx context.Context, query string, limit int, tenantID pgtype.UUID, typeFilter string) ([]AutocompleteSuggestion, error) {
	tenantFilter := fmt.Sprintf(`tenant_id = "%s"`, tenantID.String())
	limit64 := int64(limit)

	// Build the index list from the type filter. Default: all four
	// autocomplete indexes (users, spaces, tags, posts).
	var indexes []string
	switch typeFilter {
	case "user":
		indexes = []string{IndexUsers}
	case "space":
		indexes = []string{IndexSpaces}
	case "tag":
		indexes = []string{IndexTags}
	case "content":
		indexes = []string{IndexPosts}
	default:
		indexes = []string{IndexUsers, IndexSpaces, IndexTags, IndexPosts}
	}

	queries := make([]*meilisearch.SearchRequest, len(indexes))
	for i, idx := range indexes {
		queries[i] = &meilisearch.SearchRequest{
			IndexUID:             idx,
			Query:                query,
			Limit:                limit64,
			Filter:               tenantFilter,
			AttributesToRetrieve: []string{"id", "title", "excerpt", "url"},
		}
	}

	req := &meilisearch.MultiSearchRequest{
		Queries: queries,
	}

	resp, err := h.client.MultiSearchWithContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("autocomplete multi-search: %w", err)
	}

	return mapAutocompleteResults(resp), nil
}

// mapAutocompleteResults flattens multi-search response into suggestions.
func mapAutocompleteResults(resp *meilisearch.MultiSearchResponse) []AutocompleteSuggestion {
	suggestions := make([]AutocompleteSuggestion, 0, resp.EstimatedTotalHits)

	for _, sr := range resp.Results {
		resultType, ok := indexTypeByUID[sr.IndexUID]
		if !ok {
			continue
		}

		for _, hit := range sr.Hits {
			suggestions = append(suggestions, AutocompleteSuggestion{
				Type:    resultType,
				ID:      stringField(hit, "id"),
				Title:   stringField(hit, "title"),
				Excerpt: stringField(hit, "excerpt"),
				URL:     stringField(hit, "url"),
			})
		}
	}

	return suggestions
}

// SearchSpace handles GET /api/v1/spaces/:id/search?q=...&content_type=.
//
// Runs a content search scoped to a specific space via the space_id filter.
func (h *Handlers) SearchSpace(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	spaceID := r.PathValue("id")
	if spaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "Space ID is required")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))

	if h.client == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": SearchResponse{Results: []SearchResult{}},
		})
		return
	}

	filter := fmt.Sprintf(`tenant_id = "%s" AND space_id = "%s"`, t.ID.String(), spaceID)

	if ct := r.URL.Query().Get("content_type"); ct != "" {
		filter += fmt.Sprintf(` AND content_type = "%s"`, ct)
	}

	req := &meilisearch.SearchRequest{
		IndexUID:             IndexPosts,
		Query:                query,
		Limit:                int64(limit),
		Filter:               filter,
		AttributesToRetrieve: searchAttributesToRetrieve,
	}

	resp, err := h.client.Index(IndexPosts).SearchWithContext(r.Context(), query, req)
	if err != nil {
		slog.Warn("space search failed", "space_id", spaceID, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")
		return
	}

	results := make([]SearchResult, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		results = append(results, SearchResult{
			Type:    "content",
			ID:      stringField(hit, "id"),
			Title:   stringField(hit, "title"),
			Excerpt: stringField(hit, "excerpt"),
			URL:     stringField(hit, "url"),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": SearchResponse{Results: results},
	})
}

// SearchTag handles GET /api/v1/tags/:slug/search?q=.
//
// Runs a content search filtered by tag slug via the content_tags junction.
func (h *Handlers) SearchTag(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	tagSlug := r.PathValue("slug")
	if tagSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "Tag slug is required")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))

	if h.client == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": SearchResponse{Results: []SearchResult{}},
		})
		return
	}

	filter := fmt.Sprintf(`tenant_id = "%s" AND tag_slugs = "%s"`, t.ID.String(), tagSlug)

	req := &meilisearch.SearchRequest{
		IndexUID:             IndexPosts,
		Query:                query,
		Limit:                int64(limit),
		Filter:               filter,
		AttributesToRetrieve: searchAttributesToRetrieve,
	}

	resp, err := h.client.Index(IndexPosts).SearchWithContext(r.Context(), query, req)
	if err != nil {
		slog.Warn("tag search failed", "tag_slug", tagSlug, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")
		return
	}

	results := make([]SearchResult, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		results = append(results, SearchResult{
			Type:    "content",
			ID:      stringField(hit, "id"),
			Title:   stringField(hit, "title"),
			Excerpt: stringField(hit, "excerpt"),
			URL:     stringField(hit, "url"),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": SearchResponse{Results: results},
	})
}

// SearchAdminAnalytics handles GET /api/v1/admin/search/analytics.
//
// Returns search analytics: top queries, zero-result queries, and daily volume.
// Requires admin.analytics.search RBAC permission (enforced at router level).
func (h *Handlers) SearchAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	if h.queries == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "analytics_unavailable", "Analytics not configured")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d"
	}

	// The admin frontend sends explicit from/to date params for custom
	// date ranges. When both are present, they take precedence over the
	// period preset. When only one is present, the other bound is open.
	// The sqlc query uses nullable timestamptz bounds ($2 = since, $3 = until).
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var since, until time.Time
	var sinceValid, untilValid bool

	if fromStr != "" {
		if parsed, err := time.Parse("2006-01-02", fromStr); err == nil {
			since = parsed
			sinceValid = true
		}
	}
	if toStr != "" {
		if parsed, err := time.Parse("2006-01-02", toStr); err == nil {
			// "to" is inclusive — set to end of that day.
			until = parsed.Add(24*time.Hour - time.Second)
			untilValid = true
		}
	}

	// Fall back to period-based bounds when no explicit from/to dates.
	if !sinceValid && !untilValid {
		switch period {
		case "24h":
			since = time.Now().Add(-24 * time.Hour)
		case "7d":
			since = time.Now().AddDate(0, 0, -7)
		case "30d":
			since = time.Now().AddDate(0, 0, -30)
		case "90d":
			since = time.Now().AddDate(0, 0, -90)
		case "1y":
			since = time.Now().AddDate(-1, 0, 0)
		default:
			since = time.Now().AddDate(0, 0, -7)
		}
		sinceValid = true
	}

	limit := int32(20)

	topQueries, err := h.queries.GetSearchAnalyticsSummary(r.Context(), gen.GetSearchAnalyticsSummaryParams{
		TenantID: t.ID,
		Column2:  pgtype.Timestamptz{Time: since, Valid: sinceValid},
		Column3:  pgtype.Timestamptz{Time: until, Valid: untilValid},
		Limit:    limit,
		Offset:   0,
	})
	if err != nil {
		slog.Warn("failed to fetch search analytics", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "analytics_error", "Failed to fetch analytics")
		return
	}

	zeroResultQueries, err := h.queries.GetZeroResultQueries(r.Context(), gen.GetZeroResultQueriesParams{
		TenantID: t.ID,
		Column2:  pgtype.Timestamptz{Time: since, Valid: sinceValid},
		Column3:  pgtype.Timestamptz{Time: until, Valid: untilValid},
		Limit:    limit,
		Offset:   0,
	})
	if err != nil {
		slog.Warn("failed to fetch zero-result queries", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "analytics_error", "Failed to fetch analytics")
		return
	}

	dailyVolume, err := h.queries.GetSearchVolumeByDay(r.Context(), gen.GetSearchVolumeByDayParams{
		TenantID: t.ID,
		Column2:  pgtype.Timestamptz{Time: since, Valid: sinceValid},
		Column3:  pgtype.Timestamptz{Time: until, Valid: untilValid},
	})
	if err != nil {
		slog.Warn("failed to fetch search volume", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "analytics_error", "Failed to fetch analytics")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"period":              period,
			"top_queries":         topQueries,
			"zero_result_queries": zeroResultQueries,
			"daily_volume":        dailyVolume,
		},
	})
}

// logSearchAsync logs a search query to search_analytics asynchronously.
// Fire-and-forget — failures are logged but never block the response.
func (h *Handlers) logSearchAsync(ctx context.Context, tenantID pgtype.UUID, query string, resultsCount int, userID pgtype.UUID, contentType string) {
	if h.queries == nil {
		return
	}

	var contentTypePtr *string
	if contentType != "" {
		contentTypePtr = &contentType
	}

	go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := h.queries.LogSearchQuery(bgCtx, gen.LogSearchQueryParams{
			TenantID:     tenantID,
			Query:        query,
			ResultsCount: int32(resultsCount), // #nosec G115 -- search result count, bounded by page size
			UserID:       userID,
			ContentType:  contentTypePtr,
		}); err != nil {
			slog.Warn("failed to log search query", "error", err)
		}
	}()
}

// typeFromParam extracts and validates the type filter parameter.
//
// Recognized values must match the `?type=` keys the frontend filter bar
// emits (apps/main /search: all, content, user, space, forum_thread,
// organization) plus the backend-only surfaces (tag, course, doc).
// §54 allows comma-separated values (e.g. "content,user"); this function
// returns a comma-joined string of validated types, or "" for "all indexes".
// An unrecognized value falls back to "" which means "all indexes".
func typeFromParam(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	valid := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "content", "user", "space", "tag", "doc", "course", "forum_thread", "organization", "article":
			valid = append(valid, p)
		}
	}
	return strings.Join(valid, ",")
}

// searchTypeIndexes maps a type filter to the indexes to query.
//
// Recognized values match the ?type= keys the frontend filter bar emits
// (apps/main /search: all, content, user, space, forum_thread, organization)
// plus the backend-only surfaces (tag, course, doc). §54 allows comma-
// separated values (e.g. "content,user") — these are split and the index
// sets are merged. An unrecognized/empty value means "all indexes".
func searchTypeIndexes(typeFilter string) []string {
	if typeFilter == "" || strings.Contains(typeFilter, ",") {
		// Comma-separated: merge index sets for each type.
		if typeFilter == "" {
			return []string{IndexPosts, IndexForumThreads, IndexSpaces, IndexUsers, IndexTags, IndexKnowledgeArticles, IndexLearningTracks, IndexDocPages, IndexOrganizations}
		}
		seen := map[string]bool{}
		var out []string
		for _, p := range strings.Split(typeFilter, ",") {
			for _, idx := range singleTypeIndexes(strings.TrimSpace(p)) {
				if !seen[idx] {
					seen[idx] = true
					out = append(out, idx)
				}
			}
		}
		return out
	}
	return singleTypeIndexes(typeFilter)
}

// singleTypeIndexes maps a single type value to its indexes.
func singleTypeIndexes(typeFilter string) []string {
	switch typeFilter {
	case "content":
		return []string{IndexPosts, IndexForumThreads}
	case "user":
		return []string{IndexUsers}
	case "space":
		return []string{IndexSpaces}
	case "tag":
		return []string{IndexTags}
	case "course":
		return []string{IndexLearningTracks}
	case "doc":
		return []string{IndexDocPages}
	case "forum_thread":
		return []string{IndexForumThreads}
	case "organization":
		return []string{IndexOrganizations}
	case "article":
		return []string{IndexKnowledgeArticles}
	case "all_no_users":
		// §54: guests get all indexes EXCEPT the users index (privacy).
		return []string{IndexPosts, IndexForumThreads, IndexSpaces, IndexTags, IndexKnowledgeArticles, IndexLearningTracks, IndexDocPages, IndexOrganizations}
	default:
		return []string{IndexPosts, IndexForumThreads, IndexSpaces, IndexUsers, IndexTags, IndexKnowledgeArticles, IndexLearningTracks, IndexDocPages, IndexOrganizations}
	}
}
