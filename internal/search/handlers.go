// Package search provides pluggable-engine search for JapaConnect.
// // The handlers file serves search endpoints: multi-index search, autocomplete,
// space-scoped search, tag-scoped search, and admin search analytics.
// When no search engine is configured the handler degrades gracefully
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
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	vkcache "github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
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

// indexTypeByUID maps a search index UID to the result type label
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
// keeping payloads small and the mapping logic simple. Includes type-specific
// fields per §54 (avatar_url, username, display_name, name, level,
// member_count, content_count, content_type, slug).
var searchAttributesToRetrieve = []string{
	"id", "title", "excerpt", "url",
	"content_type", "slug", "avatar_url",
	"username", "display_name", "name", "level",
	"member_count", "content_count",
}

// indexesWithRelevantCountries is the set of search indexes that carry
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
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14 Global Template Literals). Implemented by *literals.Catalog;
// nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers holds dependencies for search HTTP handlers.
type Handlers struct {
	engine   Engine
	queries  *gen.Queries
	settings searchSettingsReader
	cache    valkeycompat.Cmdable // nil disables caching (tests)
	lit      literalResolver      // nil disables literal resolution (§14)
}

// NewHandlers creates Handlers backed by the given search engine.
// Pass nil to disable search at runtime — the handler returns empty results
// instead of querying the engine, which keeps tests and dev boot simple.
func NewHandlers(engine Engine) *Handlers {
	return &Handlers{engine: engine}
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

// SetLiteralResolver injects the template-literal catalog so search and
// autocomplete result titles/excerpts render {{...}} placeholders (§14).
// Resolution happens post-cache so each request gets its own render context.
func (h *Handlers) SetLiteralResolver(l literalResolver) {
	h.lit = l
}

// resolveLiterals resolves {{...}} placeholders on a search DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

// searchCacheKey builds jc:cache:search:{}:{query_hash}:{type}:{geo}:{limit}:{page}:{sort}.
// The query is SHA-256 hashed to keep keys short and avoid storing raw user
// input in Valkey keys. geoCountry is included so visitors from different
// countries don't share a cached result set (ADR-064 geo-boost). page and sort
// are included so different pages/sort orders don't share a cached result set.
func searchCacheKey(query, typeFilter, geoCountry string, limit, offset int, sortExpr string) string {
	h := sha256.Sum256([]byte(query))
	queryHash := hex.EncodeToString(h[:16]) // 32-char hex, enough for uniqueness

	return vkcache.MustKey("cache:search:%s:%s:%s:%s:%d:%d:%s", "", queryHash, typeFilter, geoCountry, limit, offset, sortExpr)
}

// SearchResultItem holds the type-specific fields for a search result. Per
// §54, each result type exposes its own field set. Fields are optional — only
// the fields relevant to the type are populated.
// //nolint:revive // exported: stutter naming is intentional
type SearchResultItem struct {
	ID           string `json:"id"`
	Title        string `json:"title,omitempty"`
	Excerpt      string `json:"excerpt,omitempty"`
	URL          string `json:"url,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	Slug         string `json:"slug,omitempty"`
	Username     string `json:"username,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	Name         string `json:"name,omitempty"`
	Level        string `json:"level,omitempty"`
	MemberCount  *int32 `json:"member_count,omitempty"`
	ContentCount *int32 `json:"content_count,omitempty"`
}

// SearchResult represents a single search result. Per §54 the response shape
// is { type, item } where item holds the type-specific fields.
type SearchResult struct { //nolint:revive // stuttering type name preserved for API compatibility
	Type string           `json:"type"`
	Item SearchResultItem `json:"item"`
}

// ResultGroup holds a grouped bucket of results for one type (ADR-017).
// When grouping is active, the response contains top 2–3 results per category
// instead of a flat cross-index merge.
type ResultGroup struct {
	Type    string         `json:"type"`
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
}

// SearchResponse is the API response for search. Per §54 + ADR-017 the
// response contains grouped results (top 2–3 per category) when no type
// filter is active, or a flat results list when a specific type is requested.
type SearchResponse struct { //nolint:revive // stuttering type name preserved for API compatibility
	Results []SearchResult `json:"results"`
	Groups  []ResultGroup  `json:"groups,omitempty"`
	Total   int            `json:"total"`
	TookMs  int64          `json:"took_ms"`
}

// Search handles GET /api/v1/search?q=...&type=...&limit=20.
// // It resolves the authenticated user and platform from the request context,
// validates the query and limit, then runs a multi-index engine search
// scoped to the platform. The optional type parameter filters which indexes
// are queried. Results are logged to search_analytics asynchronously.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	// OptionalAuth: guests can search published content (Phase 10.01 public
	// SSR search). A nil user is valid — user-scoped features (analytics
	// logging) are skipped. Ref: sections/13-search.md
	user := auth.FromContext(r.Context())

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
	offset := parsePage(r.URL.Query().Get("page"), limit)
	sortExprs := parseSort(r.URL.Query().Get("sort"))
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

	// Graceful degradation: no search engine configured or reachable.
	if h.engine == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": SearchResponse{Results: []SearchResult{}},
		})

		return
	}

	// Read-through Valkey cache (jc:cache:search:*, BUG-010). The cache key
	// includes the platform, query hash, type filter, and limit so identical
	// queries from the same platform are served from cache. Uses distributed
	// lock stampede protection (BUG-008) to prevent thundering herd on
	// popular queries.
	cacheKey := searchCacheKey(query, typeFilter, geoCountry, limit, offset, r.URL.Query().Get("sort"))

	searchStart := time.Now()

	results, err := vkcache.GetOrComputeCompat(r.Context(), h.cache, cacheKey, searchCacheTTL, func(ctx context.Context) ([]SearchResult, error) {
		// §54: guests cannot search the users index. When typeFilter is
		// empty (all indexes), exclude IndexUsers from the query set.
		effectiveFilter := typeFilter
		if !authenticated && effectiveFilter == "" {
			effectiveFilter = "all_no_users"
		}

		return h.runMultiSearchFiltered(ctx, query, limit, offset, sortExprs, effectiveFilter, geoCountry, extraPostFilter)
	})
	if err != nil {
		slog.Warn("search query failed", "query", query, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")

		return
	}

	tookMs := time.Since(searchStart).Milliseconds()

	// ADR-017: when no type filter is active, group results by type (top 2-3
	// per category) instead of returning a flat cross-index merge. When a
	// specific type is requested, return a flat results list.
	resp := SearchResponse{
		Results: results,
		Total:   len(results),
		TookMs:  tookMs,
	}
	if typeFilter == "" {
		resp.Groups = groupResults(results)
		// When grouping, results is still populated for clients that prefer
		// the flat view, but groups is the canonical field per ADR-017.
	}

	// §14: resolve post-cache so each request renders literals with its own
	// geo/time context — the cached payload must stay unresolved.
	h.resolveLiterals(r, &resp)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})

	// Log search analytics asynchronously (fire-and-forget). Guests have a
	// zero UUID — the analytics row is still written for aggregate metrics.
	var userID pgtype.UUID
	if user != nil && user.UserID != (pgtype.UUID{}) {
		userID = user.UserID
	}

	h.logSearchAsync(r.Context(), query, len(results), userID, typeFilter)
}

// SearchSpaces handles GET /api/v1/spaces/search — the same multi-search
// pipeline as Search, with the type filter pinned to "space".
// // §54: space search is public (guests can search). Uses OptionalAuth so a
// nil user is valid — analytics logging is skipped for guests.
func (h *Handlers) SearchSpaces(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())

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
	offset := parsePage(r.URL.Query().Get("page"), limit)
	sortExprs := parseSort(r.URL.Query().Get("sort"))

	if h.engine == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": SearchResponse{Results: []SearchResult{}}})
		return
	}

	results, err := h.runMultiSearchFiltered(r.Context(), query, limit, offset, sortExprs, "space", "", nil)
	if err != nil {
		slog.Warn("space search query failed", "query", query, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")

		return
	}

	h.resolveLiterals(r, &results)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": SearchResponse{Results: results}})

	// Analytics logging — skip for guests (nil or zero UUID).
	if user != nil && user.UserID != (pgtype.UUID{}) {
		h.logSearchAsync(r.Context(), query, len(results), user.UserID, "space")
	}
}

// runMultiSearch queries the posts, spaces, users, forum_threads, and tags
// indexes in a single engine multi-search call, scoped to the given
// platform. It returns the flattened, type-tagged results.
func (h *Handlers) runMultiSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	return h.runMultiSearchFiltered(ctx, query, limit, 0, nil, "", "", nil)
}

// buildPostFilter builds the extra posts-index filter for the §54
// ?content_type, ?space_id, and ?tag_slug params. Returns nil when none are
// set so no extra filter is applied. tag_slugs is an array attribute — Eq is
// membership on array fields.
func buildPostFilter(contentType, spaceID, tagSlug string) FilterExpr {
	var parts []FilterExpr
	if contentType != "" {
		parts = append(parts, Eq{Field: "content_type", Value: contentType})
	}

	if spaceID != "" {
		parts = append(parts, Eq{Field: "space_id", Value: spaceID})
	}

	if tagSlug != "" {
		parts = append(parts, Eq{Field: "tag_slugs", Value: tagSlug})
	}

	return andAll(parts...)
}

// runMultiSearchFiltered queries indexes filtered by type. When typeFilter
// is empty, all indexes are queried (original behavior). geoCountry, when
// non-empty, adds a soft boost for learning_tracks results whose
// relevant_countries array contains the visitor's country (ADR-064).
// extraPostFilter, when non-nil, is appended to the posts index filter
// (supports §54 ?content_type, ?space_id, ?tag_slug params). offset is the
// zero-based pagination offset (§54 ?page=). sortExprs, when non-nil, is
// passed to the engine as the sort expression (§54 ?sort=).
func (h *Handlers) runMultiSearchFiltered(ctx context.Context, query string, limit, offset int, sortExprs []string, typeFilter, geoCountry string, extraPostFilter FilterExpr) ([]SearchResult, error) {
	indexes := searchTypeIndexes(typeFilter)

	// A type filter with no backing index (e.g. "organization" before its
	// index is provisioned) yields no queries. Short-circuit to empty
	// results instead of issuing a zero-query multi-search, which engines
	// may reject.
	if len(indexes) == 0 {
		return []SearchResult{}, nil
	}

	queries := make([]Query, len(indexes))

	for i, idx := range indexes {
		filters := []FilterExpr{}

		// §54: apply content_type / space_id / tag_slug filters to the
		// posts index only. These attributes don't exist on other indexes.
		if idx == IndexPosts && extraPostFilter != nil {
			filters = append(filters, extraPostFilter)
		}
		// ADR-064 + ADR-064a: for indexes that carry a relevant_countries
		// filterable attribute (posts, learning_tracks), boost results
		// relevant to the visitor's country. The OR keeps globally-relevant
		// content (empty relevant_countries) eligible.
		// // `relevant_countries` is an array of ISO codes — Eq is membership
		// on array fields. Do NOT use substring/contains operators here:
		// engines may reject the whole multi-search, breaking search for
		// every user with a non-empty ContentCountry.
		if geoCountry != "" && hasRelevantCountries(idx) {
			filters = append(filters, orAny(
				IsEmpty{Field: "relevant_countries"},
				Eq{Field: "relevant_countries", Value: geoCountry},
			))
		}
		// §54 ?sort=: "top" uses engagement_score which only exists on the
		// posts index. For other indexes, fall back to created_at:desc.
		sort := sortExprs
		if sort != nil && idx != IndexPosts {
			sort = []string{"created_at:desc"}
		}

		queries[i] = Query{
			IndexUID:             idx,
			Text:                 query,
			Limit:                limit,
			Offset:               offset,
			Filter:               andAll(filters...),
			Sort:                 sort,
			AttributesToRetrieve: searchAttributesToRetrieve,
		}
	}

	results, err := h.engine.MultiSearch(ctx, queries)
	if err != nil {
		return nil, fmt.Errorf("multi-search: %w", err)
	}

	return mapSearchResults(results), nil
}

// mapSearchResults flattens an engine multi-search response into a single
// type-tagged result list. Results from unknown indexes are skipped so a
// misconfigured engine cannot corrupt the response shape. Per §54, each result
// includes a nested item with type-specific fields.
func mapSearchResults(indexResults []IndexResult) []SearchResult {
	results := []SearchResult{}

	for _, sr := range indexResults {
		resultType, ok := indexTypeByUID[sr.IndexUID]
		if !ok {
			continue
		}

		for _, hit := range sr.Hits {
			results = append(results, SearchResult{
				Type: resultType,
				Item: mapHitToItem(resultType, hit),
			})
		}
	}

	return results
}

// mapHitToItem builds a SearchResultItem from a search hit, populating
// only the fields relevant to the result type (§54 autocomplete/search
// type-specific fields).
func mapHitToItem(resultType string, hit Hit) SearchResultItem {
	item := SearchResultItem{
		ID:      stringField(hit, "id"),
		Title:   stringField(hit, "title"),
		Excerpt: stringField(hit, "excerpt"),
		URL:     stringField(hit, "url"),
	}

	switch resultType {
	case "content":
		item.ContentType = stringField(hit, "content_type")
		item.Slug = stringField(hit, "slug")
	case "user":
		// User documents store username in "title" and display_name in
		// "excerpt" for backward compatibility with the uniform mapper, but
		// §54 wants explicit username/display_name/avatar_url fields.
		item.Username = stringField(hit, "title")
		item.DisplayName = stringField(hit, "excerpt")
		item.AvatarURL = stringField(hit, "avatar_url")
		// Clear title/excerpt since we've mapped them to type-specific fields.
		item.Title = ""
		item.Excerpt = ""
	case "space":
		// Space documents store name in "title" for backward compatibility.
		// §54 wants explicit name/level/member_count fields.
		item.Name = stringField(hit, "title")
		item.Level = stringField(hit, "level")

		if mc := int32Field(hit, "member_count"); mc != nil {
			item.MemberCount = mc
		}

		item.Title = ""
	case "tag":
		// Tag documents store name in "title" for backward compatibility.
		// §54 wants explicit name/slug/content_count fields.
		item.Name = stringField(hit, "title")
		item.Slug = stringField(hit, "slug")

		if cc := int32Field(hit, "content_count"); cc != nil {
			item.ContentCount = cc
		}

		item.Title = ""
	case "organization":
		item.Name = stringField(hit, "title")
		item.Slug = stringField(hit, "slug")

		if mc := int32Field(hit, "member_count"); mc != nil {
			item.MemberCount = mc
		}

		item.Title = ""
	case "forum_thread":
		item.Slug = stringField(hit, "slug")
	case "learning_track":
		item.Slug = stringField(hit, "slug")
	}

	return item
}

// int32Field extracts an int32 value from a search hit by key. Returns
// nil when the field is absent or not a number.
func int32Field(hit Hit, key string) *int32 {
	raw, ok := hit[key]
	if !ok || len(raw) == 0 {
		return nil
	}

	var n int32
	if err := json.Unmarshal(raw, &n); err != nil {
		// Engines may return numbers as float64 in some cases.
		var f float64
		if err2 := json.Unmarshal(raw, &f); err2 == nil {
			n = int32(f)
			return &n
		}

		return nil
	}

	return &n
}

// groupResults groups flat search results by type, keeping at most
// maxPerGroup results per category (ADR-017: "top 2-3 per category"). The
// group order follows the canonical type order for consistent responses.
// Results within each group preserve the engine's relevance order.
const maxPerGroup = 3

// canonicalTypeOrder is the display order for grouped results.
var canonicalTypeOrder = []string{
	"content", "forum_thread", "space", "user", "tag",
	"organization", "course", "article", "doc_page",
}

func groupResults(flat []SearchResult) []ResultGroup {
	buckets := make(map[string][]SearchResult)
	for _, r := range flat {
		buckets[r.Type] = append(buckets[r.Type], r)
	}

	groups := make([]ResultGroup, 0, len(buckets))
	// Emit groups in canonical order for stable responses.
	for _, t := range canonicalTypeOrder {
		rs, ok := buckets[t]
		if !ok || len(rs) == 0 {
			continue
		}

		total := len(rs)
		if total > maxPerGroup {
			rs = rs[:maxPerGroup]
		}

		groups = append(groups, ResultGroup{
			Type:    t,
			Results: rs,
			Total:   total,
		})
	}

	return groups
}

// stringField extracts a string value from a search hit by key. Returns
// an empty string when the field is absent or not a string, so a missing
// excerpt never breaks the response.
func stringField(hit Hit, key string) string {
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

// parsePage resolves the ?page= query parameter (§54: ?page=1&limit=20,
// offset pagination). Returns the zero-based offset for the engine. page
// starts at 1; invalid values default to page 1 (offset 0).
func parsePage(raw string, limit int) int {
	if raw == "" {
		return 0
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 1 {
		return 0
	}

	return (parsed - 1) * limit
}

// parseSort resolves the ?sort= query parameter (§54: relevant|latest|top).
// Returns an engine sort expression slice in the shared "field:dir" form.
// "relevant" (default) returns nil so the engine uses its native relevance
// ranking. "latest" sorts by created_at descending. "top" sorts by
// engagement_score descending (posts index only — other indexes fall back
// to created_at:desc).
func parseSort(raw string) []string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "latest":
		return []string{"created_at:desc"}
	case "top":
		return []string{"engagement_score:desc", "created_at:desc"}
	default:
		// "relevant" or unrecognized — use the engine's native relevance.
		return nil
	}
}

// maxAutocompleteLimit caps autocomplete results.
const maxAutocompleteLimit = 8

// AutocompleteSuggestion is a single autocomplete suggestion. Per §54,
// each suggestion type exposes type-specific fields.
type AutocompleteSuggestion struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	Title        string `json:"title,omitempty"`
	Excerpt      string `json:"excerpt,omitempty"`
	URL          string `json:"url,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	Level        string `json:"level,omitempty"`
	Slug         string `json:"slug,omitempty"`
	Username     string `json:"username,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	Name         string `json:"name,omitempty"`
	MemberCount  *int32 `json:"member_count,omitempty"`
	ContentCount *int32 `json:"content_count,omitempty"`
}

// AutocompleteResponse is the API response for autocomplete.
type AutocompleteResponse struct {
	Suggestions []AutocompleteSuggestion `json:"suggestions"`
}

// Autocomplete handles GET /api/v1/search/autocomplete?q=...&limit=5.
// // Returns lightweight suggestions across users, spaces, tags, and content.
// Maximum 8 results. NOT logged to search analytics per Section 54.
func (h *Handlers) Autocomplete(w http.ResponseWriter, r *http.Request) {

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
		cap = h.settings.GetIntOr(r.Context(), "search.autocomplete_max", maxAutocompleteLimit)
	}

	if limit > cap {
		limit = cap
	}

	// §54: ?type=user|space|tag|content (default: all). Filter the
	// autocomplete indexes accordingly. An unrecognized value defaults
	// to all indexes.
	typeFilter := r.URL.Query().Get("type")

	if h.engine == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": AutocompleteResponse{Suggestions: []AutocompleteSuggestion{}},
		})

		return
	}

	suggestions, err := h.runAutocomplete(r.Context(), query, limit, typeFilter)
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
		suggestions = h.filterBlockedUsers(r.Context(), user.UserID, suggestions)
	}

	h.resolveLiterals(r, &suggestions)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": AutocompleteResponse{Suggestions: suggestions},
	})
}

// filterBlockedUsers removes user-type suggestions where a block relationship
// exists between the current user and the suggested user (either direction).
// Non-user suggestions are passed through unchanged. Best-effort: on database
// error the suggestion is kept (fail-open for visibility).
func (h *Handlers) filterBlockedUsers(ctx context.Context, currentUserID pgtype.UUID, suggestions []AutocompleteSuggestion) []AutocompleteSuggestion {
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
		})
		if err != nil {
			slog.Warn("autocomplete: IsBlocked check failed", "user_id", uid.String(), "error", err)

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
func (h *Handlers) runAutocomplete(ctx context.Context, query string, limit int, typeFilter string) ([]AutocompleteSuggestion, error) {
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
	case "doc", "doc_page":
		indexes = []string{IndexDocPages}
	default:
		// DOC-016: include doc_pages in omnibox autocomplete.
		indexes = []string{IndexUsers, IndexSpaces, IndexTags, IndexPosts, IndexDocPages}
	}

	queries := make([]Query, len(indexes))
	for i, idx := range indexes {
		queries[i] = Query{
			IndexUID:             idx,
			Text:                 query,
			Limit:                limit,
			Filter:               nil,
			AttributesToRetrieve: searchAttributesToRetrieve,
		}
	}

	results, err := h.engine.MultiSearch(ctx, queries)
	if err != nil {
		return nil, fmt.Errorf("autocomplete multi-search: %w", err)
	}

	return mapAutocompleteResults(results), nil
}

// mapAutocompleteResults flattens multi-search response into suggestions.
func mapAutocompleteResults(indexResults []IndexResult) []AutocompleteSuggestion {
	suggestions := []AutocompleteSuggestion{}

	for _, sr := range indexResults {
		resultType, ok := indexTypeByUID[sr.IndexUID]
		if !ok {
			continue
		}

		for _, hit := range sr.Hits {
			s := AutocompleteSuggestion{
				Type: resultType,
				ID:   stringField(hit, "id"),
				URL:  stringField(hit, "url"),
			}

			// §54: populate type-specific fields per suggestion type.
			switch resultType {
			case "content":
				s.Title = stringField(hit, "title")
				s.ContentType = stringField(hit, "content_type")
			case "user":
				s.Username = stringField(hit, "title")
				s.DisplayName = stringField(hit, "excerpt")
				s.AvatarURL = stringField(hit, "avatar_url")
			case "space":
				s.Name = stringField(hit, "title")
				s.Level = stringField(hit, "level")

				if mc := int32Field(hit, "member_count"); mc != nil {
					s.MemberCount = mc
				}
			case "tag":
				s.Name = stringField(hit, "title")
				s.Slug = stringField(hit, "slug")

				if cc := int32Field(hit, "content_count"); cc != nil {
					s.ContentCount = cc
				}
			case "organization":
				s.Name = stringField(hit, "title")
				s.Slug = stringField(hit, "slug")
			default:
				// forum_thread, course, article, doc_page — use the uniform
				// title/excerpt shape.
				s.Title = stringField(hit, "title")
				s.Excerpt = stringField(hit, "excerpt")
			}

			suggestions = append(suggestions, s)
		}
	}

	return suggestions
}

// SearchSpace handles GET /api/v1/spaces/:id/search?q=...&content_type=.
// // Runs a content search scoped to a specific space via the space_id filter.
func (h *Handlers) SearchSpace(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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

	if h.engine == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": SearchResponse{Results: []SearchResult{}},
		})

		return
	}

	filters := []FilterExpr{

		Eq{Field: "space_id", Value: spaceID},
		// The posts index is upserted on every content create/update —
		// drafts and scheduled rows are indexed too, so status must be
		// filtered explicitly (§14).
		Eq{Field: "status", Value: "published"},
	}

	if ct := r.URL.Query().Get("content_type"); ct != "" {
		filters = append(filters, Eq{Field: "content_type", Value: ct})
	}

	resp, err := h.engine.Search(r.Context(), IndexPosts, Query{
		IndexUID:             IndexPosts,
		Text:                 query,
		Limit:                limit,
		Filter:               andAll(filters...),
		AttributesToRetrieve: searchAttributesToRetrieve,
	})
	if err != nil {
		slog.Warn("space search failed", "space_id", spaceID, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")

		return
	}

	results := make([]SearchResult, 0, len(resp.Hits))
	for _, hit := range h.filterContentHits(r, resp.Hits) {
		results = append(results, SearchResult{
			Type: "content",
			Item: mapHitToItem("content", hit),
		})
	}

	h.resolveLiterals(r, &results)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": SearchResponse{Results: results},
	})
}

// filterContentHits drops posts-index hits the requester may not see. The
// index carries no `visibility` attribute, so hits are post-filtered through
// the visibility-gated ListContentsByIDs query (§14) — members-only and
// private content never leaks to viewers without access. When the queries
// dependency is not wired (tests) hits pass through unfiltered; on a
// post-filter error the result set is emptied (fail closed).
func (h *Handlers) filterContentHits(r *http.Request, hits []Hit) []Hit {
	if h.queries == nil || len(hits) == 0 {
		return hits
	}

	requesterID, canSeeUnpublished, canSeeMembers, canSeePrivate := visibility.VisibilityFilterFromRequest(r).Params()

	ids := make([]pgtype.UUID, 0, len(hits))
	for _, hit := range hits {
		var id pgtype.UUID
		if err := id.Scan(stringField(hit, "id")); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return hits
	}

	rows, err := h.queries.ListContentsByIDs(r.Context(), gen.ListContentsByIDsParams{
		Ids:               ids,
		CanSeeUnpublished: canSeeUnpublished,
		CanSeePrivate:     canSeePrivate,
		RequesterID:       requesterID,
		CanSeeMembers:     canSeeMembers,
		PageLimit:         int32(len(ids)),
	})
	if err != nil {
		slog.Warn("content hit visibility filter failed", "error", err)
		return []Hit{}
	}

	allowed := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		allowed[row.ID.String()] = struct{}{}
	}

	filtered := hits[:0]
	for _, hit := range hits {
		if _, ok := allowed[stringField(hit, "id")]; ok {
			filtered = append(filtered, hit)
		}
	}
	return filtered
}

// SearchTag handles GET /api/v1/tags/:slug/search?q=.
// // Runs a content search filtered by tag slug via the content_tags junction.
func (h *Handlers) SearchTag(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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

	if h.engine == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": SearchResponse{Results: []SearchResult{}},
		})

		return
	}

	resp, err := h.engine.Search(r.Context(), IndexPosts, Query{
		IndexUID: IndexPosts,
		Text:     query,
		Limit:    limit,
		Filter: andAll(

			Eq{Field: "tag_slugs", Value: tagSlug},
			// Drafts and scheduled rows are indexed alongside published
			// content — status must be filtered explicitly (§14).
			Eq{Field: "status", Value: "published"},
		),
		AttributesToRetrieve: searchAttributesToRetrieve,
	})
	if err != nil {
		slog.Warn("tag search failed", "tag_slug", tagSlug, "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Search is temporarily unavailable")

		return
	}

	results := make([]SearchResult, 0, len(resp.Hits))
	for _, hit := range h.filterContentHits(r, resp.Hits) {
		results = append(results, SearchResult{
			Type: "content",
			Item: mapHitToItem("content", hit),
		})
	}

	h.resolveLiterals(r, &results)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": SearchResponse{Results: results},
	})
}

// SearchAdminAnalytics handles GET /api/v1/admin/search/analytics.
// // Returns search analytics: top queries, zero-result queries, and daily volume.
// Requires admin.analytics.search RBAC permission (enforced at router level).
func (h *Handlers) SearchAdminAnalytics(w http.ResponseWriter, r *http.Request) {

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

	topQueries, err := h.queries.GetSearchAnalyticsSummary(r.Context(), gen.GetSearchAnalyticsSummaryParams{Column1: pgtype.Timestamptz{Time: since, Valid: sinceValid}, Column2: pgtype.Timestamptz{Time: until, Valid: untilValid}, Limit: limit,
		Offset: 0,
	})
	if err != nil {
		slog.Warn("failed to fetch search analytics", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "analytics_error", "Failed to fetch analytics")

		return
	}

	zeroResultQueries, err := h.queries.GetZeroResultQueries(r.Context(), gen.GetZeroResultQueriesParams{Column1: pgtype.Timestamptz{Time: since, Valid: sinceValid}, Column2: pgtype.Timestamptz{Time: until, Valid: untilValid}, Limit: limit,
		Offset: 0,
	})
	if err != nil {
		slog.Warn("failed to fetch zero-result queries", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "analytics_error", "Failed to fetch analytics")

		return
	}

	dailyVolume, err := h.queries.GetSearchVolumeByDay(r.Context(), gen.GetSearchVolumeByDayParams{Column1: pgtype.Timestamptz{Time: since, Valid: sinceValid}, Column2: pgtype.Timestamptz{Time: until, Valid: untilValid}})
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
func (h *Handlers) logSearchAsync(ctx context.Context, query string, resultsCount int, userID pgtype.UUID, contentType string) {
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
// // Recognized values must match the `?type=` keys the frontend filter bar
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
		case "content", "user", "space", "tag", "doc", "doc_page", "course", "forum_thread", "organization", "article":
			valid = append(valid, p)
		}
	}

	return strings.Join(valid, ",")
}

// searchTypeIndexes maps a type filter to the indexes to query.
// // Recognized values match the ?type= keys the frontend filter bar emits
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
	case "doc", "doc_page":
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
