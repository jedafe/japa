// Package trending — HTTP handlers for Trending & Discovery (Phase 28).
//
// Every handler is a method on Handlers so dependencies (Store) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}.
package trending

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
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

// Handlers holds dependencies for trending HTTP handlers.
type Handlers struct {
	Store *Store
	cache valkey.Client // nil disables caching (tests)
}

// NewHandlers creates Handlers with the given Store.
func NewHandlers(store *Store) *Handlers {
	return &Handlers{Store: store}
}

// SetCache injects a Valkey cache client for jc:trending:tags caching (BUG-015).
func (h *Handlers) SetCache(c valkey.Client) {
	h.cache = c
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Warn("json encode error", "error", err)
		}
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
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
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)
	contentType := parseContentTypePtr(r) // BUG-007

	// Try Valkey cache (jc:trending:content:{tenant}:{geo}:{window}, Section 53, TTL 30min).
	// Include content_type in the cache key when filtering.
	tenantStr := tenantID.String()
	cacheKey := fmt.Sprintf("jc:trending:content:%s:%s:%s", tenantStr, geoCode, window)
	if contentType != nil {
		cacheKey += ":" + *contentType
	}
	if h.cache != nil {
		if cached, err := h.cache.Do(r.Context(), h.cache.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
			var data []gen.GetTrendingContentRow
			if json.Unmarshal([]byte(cached), &data) == nil {
				writeJSON(w, http.StatusOK, map[string]any{"data": data})
				return
			}
		}
	}

	// Fetch a larger pool when filtering by type, then filter in Go.
	fetchLimit := limit
	if contentType != nil {
		fetchLimit = maxLimit // fetch max, filter, then truncate
	}

	data, err := h.Store.GetTrendingContent(r.Context(), tenantID, window, geoCode, fetchLimit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Apply content_type filter in Go (BUG-007).
	if contentType != nil {
		filtered := make([]gen.GetTrendingContentRow, 0, len(data))
		for _, item := range data {
			if item.ContentType == *contentType {
				filtered = append(filtered, item)
			}
		}
		data = filtered
		if int32(len(data)) > limit {
			data = data[:limit]
		}
	}

	// Cache the result (fire-and-forget; cache miss is acceptable).
	if h.cache != nil {
		if encoded, err := json.Marshal(data); err == nil {
			_ = h.cache.Do(r.Context(), h.cache.B().Set().Key(cacheKey).Value(string(encoded)).Ex(trendingCacheTTL).Build()).Error()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingTags handles GET /api/v1/trending/tags.
// BUG-013: uses a short Valkey lock (SET NX EX 5s) on cache miss to prevent
// cache stampede — concurrent requests wait for the lock holder to populate
// the cache rather than all hitting the DB simultaneously.
func (h *Handlers) GetTrendingTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)

	// Try Valkey cache first (jc:trending:tags:{tenant}:{geo}, Section 53, TTL 30min).
	tenantStr := tenantID.String()
	cacheKey := fmt.Sprintf("jc:trending:tags:%s:%s", tenantStr, geoCode)
	if h.cache != nil {
		if cached, err := h.cache.Do(r.Context(), h.cache.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
			var data []gen.GetTrendingTagsRow
			if json.Unmarshal([]byte(cached), &data) == nil {
				writeJSON(w, http.StatusOK, map[string]any{"data": data})
				return
			}
		}
	}

	// BUG-013: cache stampede protection — acquire a short lock; if another
	// request holds it, briefly wait and re-check the cache.
	lockKey := cacheKey + ":lock"
	if h.cache != nil {
		acquired, _ := h.cache.Do(r.Context(), h.cache.B().Set().Key(lockKey).Value("1").Nx().Ex(5*time.Second).Build()).ToString()
		if acquired != "OK" {
			// Another request is populating the cache; wait briefly and re-check.
			time.Sleep(100 * time.Millisecond)
			if cached, err := h.cache.Do(r.Context(), h.cache.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
				var data []gen.GetTrendingTagsRow
				if json.Unmarshal([]byte(cached), &data) == nil {
					writeJSON(w, http.StatusOK, map[string]any{"data": data})
					return
				}
			}
			// Lock wait expired; proceed to DB (rare).
		}
		defer func() {
			if h.cache != nil {
				_ = h.cache.Do(r.Context(), h.cache.B().Del().Key(lockKey).Build()).Error()
			}
		}()
	}

	data, err := h.Store.GetTrendingTags(r.Context(), tenantID, window, geoCode, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Cache the result (fire-and-forget; cache miss is acceptable).
	if h.cache != nil {
		if encoded, err := json.Marshal(data); err == nil {
			_ = h.cache.Do(r.Context(), h.cache.B().Set().Key(cacheKey).Value(string(encoded)).Ex(trendingCacheTTL).Build()).Error()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingSpaces handles GET /api/v1/trending/spaces.
// BUG-008: supports ?level=country|state|city|custom filter (Section 53).
func (h *Handlers) GetTrendingSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)
	level := parseLevelPtr(r) // BUG-008

	// Try Valkey cache (jc:trending:spaces:{tenant}:{geo}, Section 53, TTL 30min).
	// Include level in the cache key when filtering.
	tenantStr := tenantID.String()
	cacheKey := fmt.Sprintf("jc:trending:spaces:%s:%s", tenantStr, geoCode)
	if level != nil {
		cacheKey += ":" + *level
	}
	if h.cache != nil {
		if cached, err := h.cache.Do(r.Context(), h.cache.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
			var data []gen.GetTrendingSpacesRow
			if json.Unmarshal([]byte(cached), &data) == nil {
				writeJSON(w, http.StatusOK, map[string]any{"data": data})
				return
			}
		}
	}

	// Fetch a larger pool when filtering by level, then filter in Go.
	fetchLimit := limit
	if level != nil {
		fetchLimit = maxLimit
	}

	data, err := h.Store.GetTrendingSpaces(r.Context(), tenantID, window, geoCode, fetchLimit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Apply level filter in Go (BUG-008). The row doesn't include level,
	// so we fetch the levels for the returned space IDs and filter.
	if level != nil {
		ids := make([]pgtype.UUID, 0, len(data))
		for _, item := range data {
			ids = append(ids, item.ID_2)
		}
		levels, err := h.Store.GetSpaceLevels(r.Context(), tenantID, ids)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		filtered := make([]gen.GetTrendingSpacesRow, 0, len(data))
		for _, item := range data {
			if l, ok := levels[item.ID_2.Bytes]; ok && l == *level {
				filtered = append(filtered, item)
			}
		}
		data = filtered
		if int32(len(data)) > limit {
			data = data[:limit]
		}
	}

	// Cache the result (fire-and-forget; cache miss is acceptable).
	if h.cache != nil {
		if encoded, err := json.Marshal(data); err == nil {
			_ = h.cache.Do(r.Context(), h.cache.B().Set().Key(cacheKey).Value(string(encoded)).Ex(trendingCacheTTL).Build()).Error()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetTrendingUsers handles GET /api/v1/trending/users.
func (h *Handlers) GetTrendingUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	window := parseWindow(r)
	geoCode := parseGeoCode(r)
	limit := parseLimit(r)

	data, err := h.Store.GetTrendingUsers(r.Context(), tenantID, window, geoCode, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// ---------------------------------------------------------------------------
// Auth-required suggestion endpoints
// ---------------------------------------------------------------------------

// GetSuggestedUsers handles GET /api/v1/suggestions/users.
// ADR-064 / roadmap task 38.04: defaults ?geo= to the VisitorContext's
// ContentCountry when omitted; explicit ?geo= (including "all") overrides.
func (h *Handlers) GetSuggestedUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)
	geoCode := parseGeoCodePtr(r)

	// Try Valkey cache (jc:suggestions:users:{user_id}, Section 53, TTL 1hr).
	userStr := user.UserID.String()
	cacheKey := fmt.Sprintf("jc:suggestions:users:%s", userStr)
	if h.cache != nil {
		if cached, err := h.cache.Do(r.Context(), h.cache.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
			var data []gen.GetSuggestedUsersRow
			if json.Unmarshal([]byte(cached), &data) == nil {
				writeJSON(w, http.StatusOK, map[string]any{"data": data})
				return
			}
		}
	}

	data, err := h.Store.GetSuggestedUsers(r.Context(), user.UserID, tenantID, limit, geoCode)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Cache the result (fire-and-forget; cache miss is acceptable).
	if h.cache != nil {
		if encoded, err := json.Marshal(data); err == nil {
			_ = h.cache.Do(r.Context(), h.cache.B().Set().Key(cacheKey).Value(string(encoded)).Ex(suggestionsCacheTTL).Build()).Error()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetSuggestedSpaces handles GET /api/v1/suggestions/spaces.
// ADR-064 / roadmap task 38.04: defaults ?geo= to the VisitorContext's
// ContentCountry when omitted; explicit ?geo= (including "all") overrides.
func (h *Handlers) GetSuggestedSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)
	geoCode := parseGeoCodePtr(r)

	// Try Valkey cache (jc:suggestions:spaces:{user_id}, Section 53, TTL 1hr).
	userStr := user.UserID.String()
	cacheKey := fmt.Sprintf("jc:suggestions:spaces:%s", userStr)
	if h.cache != nil {
		if cached, err := h.cache.Do(r.Context(), h.cache.B().Get().Key(cacheKey).Build()).ToString(); err == nil {
			var data []gen.GetSuggestedSpacesRow
			if json.Unmarshal([]byte(cached), &data) == nil {
				writeJSON(w, http.StatusOK, map[string]any{"data": data})
				return
			}
		}
	}

	data, err := h.Store.GetSuggestedSpaces(r.Context(), user.UserID, tenantID, limit, geoCode)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Cache the result (fire-and-forget; cache miss is acceptable).
	if h.cache != nil {
		if encoded, err := json.Marshal(data); err == nil {
			_ = h.cache.Do(r.Context(), h.cache.B().Set().Key(cacheKey).Value(string(encoded)).Ex(suggestionsCacheTTL).Build()).Error()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetSuggestedTags handles GET /api/v1/suggestions/tags.
func (h *Handlers) GetSuggestedTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r)

	data, err := h.Store.GetSuggestedTags(r.Context(), user.UserID, tenantID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetSuggestedContent handles GET /api/v1/suggestions/content.
// Section 53: "You might also like" — pgvector cosine similarity when
// ?seed_content_id is provided; falls back to top trending content otherwise.
// BUG-003: previously wired to GetTrendingContent which ignored the seed.
func (h *Handlers) GetSuggestedContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	_, ok = h.requireUser(w, r)
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

	data, err := h.Store.GetSuggestedContent(r.Context(), tenantID, window, geoCode, seedContentID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}
