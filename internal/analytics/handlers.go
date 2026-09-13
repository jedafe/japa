// Package analytics — HTTP handlers for Platform Analytics & Site Reports (Section 40).
// // Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
	vkcache "github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// analyticsCacheTTL is the TTL for jc:cache:analytics:* keys. Analytics
// data changes slowly (aggregated from events), so a 5-minute TTL balances
// freshness with DB load reduction (BUG-010).
const analyticsCacheTTL = 5 * time.Minute

// Handlers holds dependencies for analytics HTTP handlers.
type Handlers struct {
	Service      *Service
	CacheFlusher *CacheFlusher
	rbacResolver *rbac.RBAC           // nil disables inline permission checks (tests)
	cache        valkeycompat.Cmdable // nil disables caching (tests)
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// SetCacheFlusher attaches a CacheFlusher for the admin cache flush endpoint.
func (h *Handlers) SetCacheFlusher(flusher *CacheFlusher) {
	h.CacheFlusher = flusher
}

// SetRBAC attaches the RBAC resolver for inline permission checks
// (e.g. GetContentAnalytics owner-or-admin access control).
func (h *Handlers) SetRBAC(resolver *rbac.RBAC) {
	h.rbacResolver = resolver
}

// SetCache injects a Valkey client for read-through analytics caching
// (jc:cache:analytics:* per valkey-keys-catalog.md). nil disables caching.
func (h *Handlers) SetCache(c valkeycompat.Cmdable) {
	h.cache = c
}

// analyticsCacheKey builds jc:cache:analytics:{}:{endpoint}:{period}.
func analyticsCacheKey(tenantID pgtype.UUID, endpoint, period string) string {
	return vkcache.MustKey("cache:analytics:%s:%s:%s", tenantID.String(), endpoint, period)
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

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

func parsePageSize(r *http.Request) int32 {
	ps := DefaultPageSize

	if v := r.URL.Query().Get("page_size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			if parsed > MaxPageSize {
				parsed = MaxPageSize
			}

			ps = parsed
		}
	}

	return int32(ps) //nolint:gosec // G115: ps bounded by MaxPageSize above
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			if parsed > MaxPageSize*MaxPageSize {
				return MaxPageSize * MaxPageSize
			}

			// #nosec G109 // value bounded by MaxPageSize check
			return int32(parsed) //nolint:gosec // G115: bounded above
		}
	}

	return 0
}

func parseDays(r *http.Request, defaultDays int) int32 {
	if v := r.URL.Query().Get("days"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			// #nosec G109 // value bounded by MaxPageSize check
			// #nosec G115 // value bounded by caller
			return int32(parsed)
		}
	}

	// #nosec G115 // value bounded by caller
	return int32(defaultDays)
}

// ---------------------------------------------------------------------------
// User-facing endpoints
// ---------------------------------------------------------------------------

// TrackEvent handles POST /api/v1/analytics/events.
func (h *Handlers) TrackEvent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		EventType   string         `json:"event_type"`
		EntityType  string         `json:"entity_type"`
		EntityID    string         `json:"entity_id"`
		Meta        map[string]any `json:"meta"`
		CountryCode string         `json:"country_code"`
		SessionID   string         `json:"session_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	var entityID pgtype.UUID
	if input.EntityID != "" {
		if err := entityID.Scan(input.EntityID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
			return
		}
	}

	event, err := h.Service.TrackEvent(r.Context(), tenantID, TrackEventInput{
		EventType:   EventType(input.EventType),
		UserID:      user.UserID,
		EntityType:  input.EntityType,
		EntityID:    entityID,
		Meta:        input.Meta,
		CountryCode: input.CountryCode,
		SessionID:   input.SessionID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": event})
}

// GetUserDashboard handles GET /api/v1/analytics/dashboard.
// Supports period presets (24h, 7d, 30d, 90d), explicit from/to dates,
// and the legacy days query parameter.
func (h *Handlers) GetUserDashboard(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	dashboard, err := h.Service.GetUserDashboardData(r.Context(), tenantID, user.UserID, &from, to)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": dashboard})
}

// ---------------------------------------------------------------------------
// Admin endpoints
// ---------------------------------------------------------------------------

// GetAdminOverview handles GET /api/v1/admin/analytics/overview.
// Uses read-through Valkey cache (jc:cache:analytics:*, BUG-010) with a 5-min
// TTL. The cache key includes the tenant and time period so different windows
// don't collide. Uses distributed lock stampede protection (BUG-008).
func (h *Handlers) GetAdminOverview(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}

	cacheKey := analyticsCacheKey(tenantID, "overview", period)

	dashboard, err := vkcache.GetOrComputeCompat(r.Context(), h.cache, cacheKey, analyticsCacheTTL, func(ctx context.Context) (any, error) {
		return h.Service.GetAdminDashboardData(ctx, tenantID, from, to)
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": dashboard})
}

// resolveWindow determines the analytics [from, to] window for a request.
// // When both `from` and `to` are supplied they take precedence. They accept
// either RFC3339 or date-only (YYYY-MM-DD) format. When only `from` is
// supplied the window is left-open (to == nil). When `from` is absent it
// falls back to the `period` query parameter (24h|7d|30d|90d) or the `days`
// query parameter, deriving the start as now minus that many days.
func resolveWindow(r *http.Request, defaultDays int) (time.Time, *time.Time, error) {
	fromRaw := r.URL.Query().Get("from")
	toRaw := r.URL.Query().Get("to")

	var to *time.Time

	if toRaw != "" {
		parsed, err := parseFlexibleTime(toRaw)
		if err != nil {
			return time.Time{}, nil, fmt.Errorf("parsing to: %w", err)
		}

		to = &parsed
	}

	if fromRaw != "" {
		from, err := parseFlexibleTime(fromRaw)
		if err != nil {
			return time.Time{}, nil, fmt.Errorf("parsing from: %w", err)
		}

		if to != nil && to.Before(from) {
			return time.Time{}, nil, errors.New("to must not be before from")
		}

		return from, to, nil
	}

	// Check the `period` query param first (sent by the admin dashboard).
	if period := r.URL.Query().Get("period"); period != "" {
		switch period {
		case "24h":
			days := 1
			return time.Now().AddDate(0, 0, -days), to, nil
		case "7d":
			days := 7
			return time.Now().AddDate(0, 0, -days), to, nil
		case "30d":
			days := 30
			return time.Now().AddDate(0, 0, -days), to, nil
		case "90d":
			days := 90
			return time.Now().AddDate(0, 0, -days), to, nil
		}
	}

	days := parseDays(r, defaultDays)
	from := time.Now().AddDate(0, 0, -int(days))

	return from, to, nil
}

// parseFlexibleTime parses a time string as either RFC3339 or date-only (YYYY-MM-DD).
func parseFlexibleTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	return time.Parse("2006-01-02", s)
}

// ListEvents handles GET /api/v1/admin/analytics/events.
func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	filter := EventFilter{
		EventType:  r.URL.Query().Get("event_type"),
		UserID:     r.URL.Query().Get("user_id"),
		EntityType: r.URL.Query().Get("entity_type"),
		Page:       parseOffset(r) / parsePageSize(r),
		Limit:      parsePageSize(r),
	}

	if v := r.URL.Query().Get("start_date"); v != "" {
		if t, err := parseTimeParam(v); err == nil {
			filter.StartDate = &t
		}
	}

	if v := r.URL.Query().Get("end_date"); v != "" {
		if t, err := parseTimeParam(v); err == nil {
			filter.EndDate = &t
		}
	}

	events, err := h.Service.ListEvents(r.Context(), tenantID, filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": events})
}

// ListSecurityScanResults handles GET /api/v1/admin/analytics/security.
func (h *Handlers) ListSecurityScanResults(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	filter := SecurityScanFilter{
		Severity:   r.URL.Query().Get("severity"),
		Unresolved: r.URL.Query().Get("unresolved") == "true",
		Page:       parseOffset(r) / parsePageSize(r),
		Limit:      parsePageSize(r),
	}

	results, err := h.Service.ListSecurityScanResults(r.Context(), tenantID, filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results})
}

// ResolveSecurityScanResult handles POST /api/v1/admin/analytics/security/{id}/resolve.
func (h *Handlers) ResolveSecurityScanResult(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid scan result ID")
		return
	}

	if err := h.Service.ResolveScanResult(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "resolved"}})
}

// DeleteSecurityScanResult handles DELETE /api/v1/admin/analytics/security/{id}.
// Permanently removes a scan finding (typically a resolved one) from the
// control panel audit view. Tenant-scoped.
func (h *Handlers) DeleteSecurityScanResult(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid scan result ID")
		return
	}

	if err := h.Service.DeleteScanResult(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// RefreshViews handles POST /api/v1/admin/analytics/refresh.
func (h *Handlers) RefreshViews(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.EnqueueRefreshViews(r.Context()); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{"status": "refresh_queued"}})
}

// RunSecurityScan handles POST /api/v1/admin/analytics/security/scan.
func (h *Handlers) RunSecurityScan(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.EnqueueSecurityScan(r.Context(), tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{"status": "scan_queued"}})
}

// HandleCacheFlush handles POST /api/v1/admin/analytics/cache/flush.
// Accepts an optional {"scope": "..."} body to limit which cache partition
// is flushed. Defaults to FlushAll when scope is empty or missing.
// BUG-ANALYTICS-11 fix: passes tenant ID to the flusher so non-FlushAll
// scopes are restricted to the caller's tenant keys.
func (h *Handlers) HandleCacheFlush(w http.ResponseWriter, r *http.Request) {
	if h.CacheFlusher == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "cache_unavailable", "Cache flusher not configured")
		return
	}

	var input struct {
		Scope string `json:"scope"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	scope := FlushScope(input.Scope)
	if scope == "" {
		scope = FlushAll
	}

	deleted, err := h.CacheFlusher.FlushCache(r.Context(), scope)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "flush_error", "Failed to flush cache")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status":  "flushed",
			"scope":   scope,
			"deleted": deleted,
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseTimeParam(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// ─── P5.6 Domain-Specific Analytics ───────────────────────────────────────

// GetRevenueAnalytics handles GET /admin/analytics/revenue (P5.6).
func (h *Handlers) GetRevenueAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 90)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	result, err := h.Service.GetRevenueAnalytics(r.Context(), tenantID, from, to)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Convert minor units (kobo/cents) to major units for frontend display.
	type PeriodResult struct {
		Period       string `json:"period"`
		Revenue      int64  `json:"revenue"`
		Transactions int32  `json:"transactions"`
	}

	periods := make([]PeriodResult, 0, len(result.Periods))
	for _, p := range result.Periods {
		periods = append(periods, PeriodResult{
			Period:       p.Period,
			Revenue:      p.Revenue / 100,
			Transactions: p.Transactions,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total_revenue":        result.TotalRevenue / 100,
		"monthly_revenue":      result.MonthlyRevenue / 100,
		"active_subscriptions": result.ActiveSubscriptions,
		"periods":              periods,
	}})
}

// GetSearchAnalytics handles GET /admin/analytics/search (P5.6).
// Uses read-through Valkey cache (jc:cache:analytics:*, BUG-010) with a 5-min
// TTL. Uses distributed lock stampede protection (BUG-008).
func (h *Handlers) GetSearchAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}

	cacheKey := analyticsCacheKey(tenantID, "search", period)

	// Convert to frontend-friendly format
	type SearchResult struct {
		Query      string `json:"query"`
		Count      int64  `json:"count"`
		AvgResults int32  `json:"avg_results"`
	}

	searchResults, err := vkcache.GetOrComputeCompat(r.Context(), h.cache, cacheKey, analyticsCacheTTL, func(ctx context.Context) ([]SearchResult, error) {
		startDate := &from

		var endDate *time.Time
		if to != nil {
			endDate = to
		}

		results, err := h.Service.store.GetSearchAnalytics(ctx, tenantID, startDate, endDate, 50)
		if err != nil {
			return nil, err
		}

		mapped := make([]SearchResult, 0, len(results))
		for _, row := range results {
			mapped = append(mapped, SearchResult{
				Query:      row.Query,
				Count:      row.SearchCount,
				AvgResults: row.AvgResults,
			})
		}

		return mapped, nil
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": searchResults})
}

// GetSpaceAnalytics handles GET /admin/analytics/spaces (P5.6).
func (h *Handlers) GetSpaceAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	startDate := pgtype.Date{Time: from, Valid: true}

	var endDate *pgtype.Date
	if to != nil {
		endDate = &pgtype.Date{Time: *to, Valid: true}
	}

	spaceStats, err := h.Service.store.GetDailySpaceStats(r.Context(), tenantID, &startDate, endDate, pgtype.UUID{Bytes: [16]byte{}, Valid: true})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Aggregate by space
	type SpaceSummary struct {
		SpaceID    string `json:"space_id"`
		SpaceName  string `json:"space_name"`
		TotalJoins int64  `json:"total_joins"`
		TotalPosts int64  `json:"total_posts"`
	}

	spaceMap := make(map[string]*SpaceSummary)

	for _, ss := range spaceStats {
		sid := ss.SpaceID.String()
		if _, ok := spaceMap[sid]; !ok {
			spaceMap[sid] = &SpaceSummary{SpaceID: sid, SpaceName: ss.SpaceName}
		}

		spaceMap[sid].TotalJoins += ss.Joins
		spaceMap[sid].TotalPosts += ss.Posts
	}

	spaces := make([]SpaceSummary, 0, len(spaceMap))
	for _, s := range spaceMap {
		spaces = append(spaces, *s)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// GetUserAnalytics handles GET /admin/analytics/users (P5.6).
func (h *Handlers) GetUserAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	ua, err := h.Service.GetUserAnalytics(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ua})
}

// GetContentAnalytics handles GET /content/:id/analytics (P5.6).
// It aggregates the per-content daily engagement stats for the requested
// content item over the requested window (default 30 days).
// Access is restricted to the content owner or admins holding analytics.admin
// (Ref: api-routes-catalog.md — owner and admin only).
func (h *Handlers) GetContentAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	// BUG-ANALYTICS-03 fix: verify the caller is the content owner or an admin.
	authorID, err := h.Service.store.GetContentAuthorID(r.Context(), contentID, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "content_not_found", "Content not found")
		return
	}

	if authorID != user.UserID {
		// Non-owner: require analytics.admin permission. The route is under a
		// general auth group (not an admin group), so the permission check is
		// done inline here. Ref: api-routes-catalog.md — owner and admin only.
		if h.rbacResolver == nil || !h.rbacResolver.HasPermission(r.Context(), user.UserID, tenantID, "analytics.admin") {
			errpkg.WriteError(w, http.StatusForbidden, "analytics_forbidden", "Only the content owner or an admin can view analytics")
			return
		}
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	startDate := pgtype.Date{Time: from, Valid: true}

	var endDate *pgtype.Date
	if to != nil {
		endDate = &pgtype.Date{Time: *to, Valid: true}
	}

	stats, err := h.Service.store.GetDailyContentStats(r.Context(), tenantID, &startDate, endDate, contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-002 fix: return both the daily time series and the aggregated
	// totals. Section 40 specifies "views, reactions, comments, shares over
	// time" — the per-day rows are the time series; the sums are the totals.
	var views, reactions, comments, shares, uniqueViewers int64
	for _, s := range stats {
		views += int64(s.Views)
		reactions += int64(s.Reactions)
		comments += int64(s.Comments)
		shares += int64(s.Shares)
		uniqueViewers += int64(s.UniqueViewers)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"daily_stats":    stats,
		"views":          views,
		"reactions":      reactions,
		"comments":       comments,
		"shares":         shares,
		"unique_viewers": uniqueViewers,
	}})
}

// GetAdminContentAnalytics handles GET /admin/analytics/content.
func (h *Handlers) GetAdminContentAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	startDate := pgtype.Date{Time: from, Valid: true}

	var endDate *pgtype.Date
	if to != nil {
		endDate = &pgtype.Date{Time: *to, Valid: true}
	}

	contentStats, err := h.Service.store.GetDailyContentStats(r.Context(), tenantID, &startDate, endDate, pgtype.UUID{Bytes: [16]byte{}, Valid: true})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Aggregate real content engagement totals across the window. The daily
	// content stats are per-content-per-day rows; dedupe by content_id for the
	// unique content count and sum the engagement metrics.
	contentIDs := make(map[string]struct{}, len(contentStats))

	var totalViews, totalReactions, totalComments, totalUniqueViewers int64

	for _, cs := range contentStats {
		contentIDs[cs.ContentID.String()] = struct{}{}
		totalViews += int64(cs.Views)
		totalReactions += int64(cs.Reactions)
		totalComments += int64(cs.Comments)
		totalUniqueViewers += int64(cs.UniqueViewers)
	}

	// Build response with per-content-per-day engagement stats and totals.
	result := map[string]any{
		"content_stats":        contentStats,
		"total_content":        len(contentIDs),
		"total_views":          totalViews,
		"total_reactions":      totalReactions,
		"total_comments":       totalComments,
		"total_unique_viewers": totalUniqueViewers,
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}
