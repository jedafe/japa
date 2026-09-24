// Package trending — HTTP handlers for Trending & Discovery (Phase 28).
//
// Every handler is a method on Handlers so dependencies (Store) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}.
package trending

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	vkcache "github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/valkey-io/valkey-go"
)

// Pagination defaults.
const (
	defaultLimit = 20
	maxLimit     = 50
)

// trendingCacheTTL is the TTL for jc:trending:* cache keys (Section 53).
const trendingCacheTTL = 30 * time.Minute

// suggestionsCacheTTL is the TTL for jc:suggestions:* cache keys (Section 53).
const suggestionsCacheTTL = 1 * time.Hour

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers holds dependencies for trending HTTP handlers.
type Handlers struct {
	Store *Store
	cache valkey.Client   // nil disables caching (tests)
	lit   literalResolver // nil disables literal resolution (§14)
}

// NewHandlers creates Handlers with the given Store.
func NewHandlers(store *Store) *Handlers {
	return &Handlers{Store: store}
}

// SetLiteralResolver injects the template-literal catalog so trending
// titles/names render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a response DTO or slice.
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

// SetCache injects a Valkey cache client for jc:trending:tags caching (BUG-015).
func (h *Handlers) SetCache(c valkey.Client) {
	h.cache = c
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

func parseLimit(r *http.Request) int32 {
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			if parsed > maxLimit {
				parsed = maxLimit
			}

			// #nosec G109 // value bounded by MaxPageSize check
			return int32(parsed) //nolint:gosec // G115: bounded by maxLimit
		}
	}

	return defaultLimit
}

func parseWindow(r *http.Request) string {
	if v := r.URL.Query().Get("window"); v != "" {
		switch v {
		case "24h", "7d", "30d":
			return v
		}
	}

	return "24h"
}

func parseGeoCode(r *http.Request) string {
	// ADR-064: when ?geo= is not explicitly provided, default to the
	// VisitorContext's ContentCountry (resolved by the visitor middleware
	// from user preference → GeoIP → system default).
	if g := r.URL.Query().Get("geo"); g != "" {
		return g
	}

	return visitor.FromContext(r.Context()).ContentCountry
}

// parseGeoCodePtr is like parseGeoCode but returns a *string suitable for
// sqlc narg binding (nil when empty → NULL → global ranking in the query).
func parseGeoCodePtr(r *http.Request) *string {
	g := parseGeoCode(r)
	if g == "" {
		return nil
	}

	return &g
}

// parseContentType extracts the ?type= filter for /trending/content
// (Section 53: ?type=discussion|blog_post). Returns empty string when
// absent or invalid (no filter applied).
func parseContentType(r *http.Request) string {
	v := r.URL.Query().Get("type")
	switch v {
	case "discussion", "blog_post", "article", "poll", "question":
		return v
	}

	return ""
}

// parseContentTypePtr is like parseContentType but returns a *string for
// sqlc narg binding (nil when empty → no filter).
func parseContentTypePtr(r *http.Request) *string {
	ct := parseContentType(r)
	if ct == "" {
		return nil
	}

	return &ct
}

// parseLevel extracts the ?level= filter for /trending/spaces
// (Section 53: ?level=country|state|city|custom). Returns empty string
// when absent or invalid (no filter applied).
func parseLevel(r *http.Request) string {
	v := r.URL.Query().Get("level")
	switch v {
	case "country", "state", "city", "custom":
		return v
	}

	return ""
}

// parseLevelPtr is like parseLevel but returns a *string for sqlc narg
// binding (nil when empty → no filter).
func parseLevelPtr(r *http.Request) *string {
	l := parseLevel(r)
	if l == "" {
		return nil
	}

	return &l
}

// ---------------------------------------------------------------------------
// Public trending endpoints
// ---------------------------------------------------------------------------

// GetTrendingContent handles GET /api/v1/trending/content.
// BUG-007: supports ?type=discussion|blog_post filter (Section 53).
func (h *Handlers) GetTrendingContent(w http.ResponseWriter, r *http.Request) {

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)
	contentType := parseContentTypePtr(r) // BUG-007

	// Try Valkey cache (jc:trending:content:{platform}:{geo}:{window}, Section 53, TTL 30min).
	// Include content_type in the cache key when filtering.
	// Uses distributed lock stampede protection (BUG-008).
	//
	// The cache always stores maxLimit rows; per-request ?limit= is applied
	// by truncating after the cache lookup so that different limit values
	// share one cache entry (no limit-suffix collision).
	platformStr := ""

	cacheKey := vkcache.MustKey("trending:content:%s:%s:%s", platformStr, geoCode, window)
	if contentType != nil {
		cacheKey += ":" + *contentType
	}

	data, err := vkcache.GetOrComputeNative(r.Context(), h.cache, cacheKey, trendingCacheTTL, func(ctx context.Context) ([]gen.GetTrendingContentRow, error) {
		// Always fetch the max pool so the cache serves any ?limit= value.
		rows, err := h.Store.GetTrendingContent(ctx, window, geoCode, maxLimit)
		if err != nil {
			return nil, err
		}

		// Geo fallback: a visitor whose country has no scoped snapshots gets
		// the global list rather than an empty widget.
		if len(rows) == 0 && geoCode != "" {
			rows, err = h.Store.GetTrendingContent(ctx, window, "", maxLimit)
			if err != nil {
				return nil, err
			}
		}

		// Apply content_type filter in Go (BUG-007). Truncation to the
		// requested limit happens after the cache lookup below.
		if contentType != nil {
			filtered := make([]gen.GetTrendingContentRow, 0, len(rows))

			for _, item := range rows {
				if item.ContentType == *contentType {
					filtered = append(filtered, item)
				}
			}

			rows = filtered
		}

		// Ensure non-nil so JSON encodes [] not null (BUG: trending/spaces
		// returned null when no snapshots existed).
		if rows == nil {
			rows = []gen.GetTrendingContentRow{}
		}

		return rows, nil
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Truncate the cached pool to the requested limit (per-request).
	//nolint:gosec // G115: bounded
	if int32(len(data)) > limit { // #nosec //nolint:gosec // G115: bounded
		data = data[:limit]
	}

	h.resolveLiterals(r, &data)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingTags handles GET /api/v1/trending/tags.
// Uses distributed lock stampede protection (BUG-008/BUG-013) via the
// shared vkcache.GetOrComputeNative helper.
func (h *Handlers) GetTrendingTags(w http.ResponseWriter, r *http.Request) {

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)

	// Valkey cache (jc:trending:tags:{platform}:{geo}:{window}, Section 53, TTL 30min).
	// window is included because the SQL filters by window; omitting it
	// caused different ?window= values to collide on the same cache entry
	// and return stale data.
	//
	// The cache always stores maxLimit rows; per-request ?limit= is applied
	// by truncating after the cache lookup.
	platformStr := ""
	cacheKey := vkcache.MustKey("trending:tags:%s:%s:%s", platformStr, geoCode, window)

	data, err := vkcache.GetOrComputeNative(r.Context(), h.cache, cacheKey, trendingCacheTTL, func(ctx context.Context) ([]gen.GetTrendingTagsRow, error) {
		rows, err := h.Store.GetTrendingTags(ctx, window, geoCode, maxLimit)
		if err != nil {
			return nil, err
		}

		// Geo fallback: a visitor whose country has no scoped snapshots gets
		// the global list rather than an empty widget.
		if len(rows) == 0 && geoCode != "" {
			rows, err = h.Store.GetTrendingTags(ctx, window, "", maxLimit)
			if err != nil {
				return nil, err
			}
		}

		if rows == nil {
			rows = []gen.GetTrendingTagsRow{}
		}

		return rows, nil
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Truncate the cached pool to the requested limit (per-request).
	//nolint:gosec // G115: bounded
	if int32(len(data)) > limit { // #nosec //nolint:gosec // G115: bounded
		data = data[:limit]
	}

	h.resolveLiterals(r, &data)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingSpaces handles GET /api/v1/trending/spaces.
// BUG-008: supports ?level=country|state|city|custom filter (Section 53).
func (h *Handlers) GetTrendingSpaces(w http.ResponseWriter, r *http.Request) {

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)
	level := parseLevelPtr(r) // BUG-008

	// Try Valkey cache (jc:trending:spaces:{platform}:{geo}:{window}, Section 53, TTL 30min).
	// window is included because the SQL filters by window; omitting it
	// caused different ?window= values to collide on the same cache entry
	// and return stale data. level is included when filtering.
	// Uses distributed lock stampede protection (BUG-008).
	//
	// The cache always stores maxLimit rows; per-request ?limit= is applied
	// by truncating after the cache lookup.
	platformStr := ""

	cacheKey := vkcache.MustKey("trending:spaces:%s:%s:%s", platformStr, geoCode, window)
	if level != nil {
		cacheKey += ":" + *level
	}

	data, err := vkcache.GetOrComputeNative(r.Context(), h.cache, cacheKey, trendingCacheTTL, func(ctx context.Context) ([]gen.GetTrendingSpacesRow, error) {
		// Always fetch the max pool so the cache serves any ?limit= value.
		rows, err := h.Store.GetTrendingSpaces(ctx, window, geoCode, maxLimit)
		if err != nil {
			return nil, err
		}

		// Geo fallback: a visitor whose country has no scoped snapshots gets
		// the global list rather than an empty widget.
		if len(rows) == 0 && geoCode != "" {
			rows, err = h.Store.GetTrendingSpaces(ctx, window, "", maxLimit)
			if err != nil {
				return nil, err
			}
		}

		// Apply level filter in Go (BUG-008). The row doesn't include level,
		// so we fetch the levels for the returned space IDs and filter.
		// Truncation to the requested limit happens after the cache lookup.
		if level != nil {
			ids := make([]pgtype.UUID, 0, len(rows))
			for _, item := range rows {
				ids = append(ids, item.ID_2)
			}

			levels, err := h.Store.GetSpaceLevels(ctx, ids)
			if err != nil {
				return nil, err
			}

			filtered := make([]gen.GetTrendingSpacesRow, 0, len(rows))

			for _, item := range rows {
				if l, ok := levels[item.ID_2.Bytes]; ok && l == *level {
					filtered = append(filtered, item)
				}
			}

			rows = filtered
		}

		// Ensure non-nil so JSON encodes [] not null (BUG: trending/spaces
		// returned null when no space snapshots existed).
		if rows == nil {
			rows = []gen.GetTrendingSpacesRow{}
		}

		return rows, nil
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Truncate the cached pool to the requested limit (per-request).
	//nolint:gosec // G115: bounded
	if int32(len(data)) > limit { // #nosec //nolint:gosec // G115: bounded
		data = data[:limit]
	}

	h.resolveLiterals(r, &data)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingUsers handles GET /api/v1/trending/users.
func (h *Handlers) GetTrendingUsers(w http.ResponseWriter, r *http.Request) {

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)

	data, err := h.Store.GetTrendingUsers(r.Context(), window, geoCode, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Geo fallback: a visitor whose country has no scoped snapshots gets the
	// global list rather than an empty widget.
	if len(data) == 0 && geoCode != "" {
		data, err = h.Store.GetTrendingUsers(r.Context(), window, "", limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}
	}

	if data == nil {
		data = []gen.GetTrendingUsersRow{}
	}

	h.resolveLiterals(r, &data)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// ---------------------------------------------------------------------------
// Auth-required suggestion endpoints
// ---------------------------------------------------------------------------

// GetSuggestedUsers handles GET /api/v1/suggestions/users.
// ADR-064 / roadmap task 38.04: defaults ?geo= to the VisitorContext's
// ContentCountry when omitted; explicit ?geo= (including "all") overrides.
func (h *Handlers) GetSuggestedUsers(w http.ResponseWriter, r *http.Request) {

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)
	geoCode := parseGeoCodePtr(r)

	// Valkey cache (jc:suggestions:users:{user_id}, Section 53, TTL 1hr).
	// Uses distributed lock stampede protection (BUG-008).
	userStr := user.UserID.String()
	cacheKey := vkcache.MustKey("suggestions:users:%s", userStr)

	data, err := vkcache.GetOrComputeNative(r.Context(), h.cache, cacheKey, suggestionsCacheTTL, func(ctx context.Context) ([]gen.GetSuggestedUsersRow, error) {
		rows, err := h.Store.GetSuggestedUsers(ctx, user.UserID, limit, geoCode)
		if err != nil {
			return nil, err
		}

		if rows == nil {
			rows = []gen.GetSuggestedUsersRow{}
		}

		return rows, nil
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &data)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetSuggestedSpaces handles GET /api/v1/suggestions/spaces.
// ADR-064 / roadmap task 38.04: defaults ?geo= to the VisitorContext's
// ContentCountry when omitted; explicit ?geo= (including "all") overrides.
func (h *Handlers) GetSuggestedSpaces(w http.ResponseWriter, r *http.Request) {

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)
	geoCode := parseGeoCodePtr(r)

	// Valkey cache (jc:suggestions:spaces:{user_id}, Section 53, TTL 1hr).
	// Uses distributed lock stampede protection (BUG-008).
	userStr := user.UserID.String()
	cacheKey := vkcache.MustKey("suggestions:spaces:%s", userStr)

	data, err := vkcache.GetOrComputeNative(r.Context(), h.cache, cacheKey, suggestionsCacheTTL, func(ctx context.Context) ([]gen.GetSuggestedSpacesRow, error) {
		rows, err := h.Store.GetSuggestedSpaces(ctx, user.UserID, limit, geoCode)
		if err != nil {
			return nil, err
		}

		if rows == nil {
			rows = []gen.GetSuggestedSpacesRow{}
		}

		return rows, nil
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &data)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetSuggestedTags handles GET /api/v1/suggestions/tags.
// ADR-064: accepts ?geo= to re-weight tags from the visitor's geo region.
func (h *Handlers) GetSuggestedTags(w http.ResponseWriter, r *http.Request) {

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)
	geoCode := parseGeoCodePtr(r)

	data, err := h.Store.GetSuggestedTags(r.Context(), user.UserID, limit, geoCode)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if data == nil {
		data = []gen.GetSuggestedTagsRow{}
	}

	h.resolveLiterals(r, &data)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetSuggestedContent handles GET /api/v1/suggestions/content.
// Section 53: "You might also like" — pgvector cosine similarity when
// ?seed_content_id is provided; falls back to top trending content otherwise.
// BUG-003: previously wired to GetTrendingContent which ignored the seed.
func (h *Handlers) GetSuggestedContent(w http.ResponseWriter, r *http.Request) {

	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCodePtr(r)
	limit := parseLimit(r)

	// Parse optional ?seed_content_id=uuid
	var seedContentID pgtype.UUID

	if v := r.URL.Query().Get("seed_content_id"); v != "" {
		if parsed, err := uuid.Parse(v); err == nil {
			seedContentID = pgtype.UUID{Bytes: parsed, Valid: true}
		}
	}

	data, err := h.Store.GetSuggestedContent(r.Context(), window, geoCode, seedContentID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if data == nil {
		data = []gen.GetSuggestedContentRow{}
	}

	h.resolveLiterals(r, &data)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}
