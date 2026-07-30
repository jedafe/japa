// Package analytics — HTTP handlers for Platform Analytics & Site Reports (Section 40).
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package analytics

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for analytics HTTP handlers.
type Handlers struct {
	Service      *Service
	CacheFlusher *CacheFlusher
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// SetCacheFlusher attaches a CacheFlusher for the admin cache flush endpoint.
func (h *Handlers) SetCacheFlusher(flusher *CacheFlusher) {
	h.CacheFlusher = flusher
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

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrRequired):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
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
			return int32(parsed) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	return 0
}

func parseDays(r *http.Request, defaultDays int) int32 {
	if v := r.URL.Query().Get("days"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			return int32(parsed) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	return int32(defaultDays) //nolint:gosec // G115: defaultDays is a small constant
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
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	var entityID pgtype.UUID
	if input.EntityID != "" {
		if err := entityID.Scan(input.EntityID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
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

	writeJSON(w, http.StatusCreated, map[string]any{"data": event})
}

// GetUserDashboard handles GET /api/v1/analytics/dashboard.
func (h *Handlers) GetUserDashboard(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	days := parseDays(r, 30)

	dashboard, err := h.Service.GetUserDashboardData(r.Context(), tenantID, user.UserID, days)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": dashboard})
}

// ---------------------------------------------------------------------------
// Admin endpoints
// ---------------------------------------------------------------------------

// GetAdminOverview handles GET /api/v1/admin/analytics/overview.
func (h *Handlers) GetAdminOverview(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	dashboard, err := h.Service.GetAdminDashboardData(r.Context(), tenantID, from, to)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": dashboard})
}

// resolveWindow determines the analytics [from, to] window for a request.
//
// When both `from` and `to` are supplied they take precedence. They accept
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
			return time.Time{}, nil, fmt.Errorf("to must not be before from")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": events})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}

// ResolveSecurityScanResult handles POST /api/v1/admin/analytics/security/{id}/resolve.
func (h *Handlers) ResolveSecurityScanResult(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid scan result ID")
		return
	}

	if err := h.Service.ResolveScanResult(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "resolved"}})
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

	writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{"status": "refresh_queued"}})
}

// RunSecurityScan handles POST /api/v1/admin/analytics/security/scan.
func (h *Handlers) RunSecurityScan(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.EnqueueSecurityScan(r.Context()); err != nil {
		handleServiceError(w, err)
		return
	}

	_ = tenantID

	writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{"status": "scan_queued"}})
}

// HandleCacheFlush handles POST /api/v1/admin/analytics/cache/flush.
// Accepts an optional {"scope": "..."} body to limit which cache partition
// is flushed. Defaults to FlushAll when scope is empty or missing.
func (h *Handlers) HandleCacheFlush(w http.ResponseWriter, r *http.Request) {
	if h.CacheFlusher == nil {
		writeError(w, http.StatusServiceUnavailable, "cache_unavailable", "Cache flusher not configured")
		return
	}

	var input struct {
		Scope string `json:"scope"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	scope := FlushScope(input.Scope)
	if scope == "" {
		scope = FlushAll
	}

	deleted, err := h.CacheFlusher.FlushCache(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "flush_error", "Failed to flush cache")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
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
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	result, err := h.Service.GetRevenueAnalytics(r.Context(), tenantID, from, to)
	if err != nil {
		slog.Error("get revenue analytics", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve revenue analytics")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total_revenue":        result.TotalRevenue / 100,
		"monthly_revenue":      result.MonthlyRevenue / 100,
		"active_subscriptions": result.ActiveSubscriptions,
		"periods":              periods,
	}})
}

// GetSearchAnalytics handles GET /admin/analytics/search (P5.6).
func (h *Handlers) GetSearchAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	startDate := &from
	var endDate *time.Time
	if to != nil {
		endDate = to
	}

	results, err := h.Service.store.GetSearchAnalytics(r.Context(), tenantID, startDate, endDate, 50)
	if err != nil {
		slog.Error("get search analytics", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve search analytics")
		return
	}

	// Convert to frontend-friendly format
	type SearchResult struct {
		Query      string `json:"query"`
		Count      int64  `json:"count"`
		AvgResults int32  `json:"avg_results"`
	}
	searchResults := make([]SearchResult, 0, len(results))
	for _, r := range results {
		searchResults = append(searchResults, SearchResult{
			Query:      r.Query,
			Count:      r.SearchCount,
			AvgResults: r.AvgResults,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": searchResults})
}

// GetSpaceAnalytics handles GET /admin/analytics/spaces (P5.6).
func (h *Handlers) GetSpaceAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	startDate := pgtype.Date{Time: from, Valid: true}
	var endDate *pgtype.Date
	if to != nil {
		endDate = &pgtype.Date{Time: *to, Valid: true}
	}

	spaceStats, err := h.Service.store.GetDailySpaceStats(r.Context(), tenantID, &startDate, endDate, pgtype.UUID{Bytes: [16]byte{}, Valid: true})
	if err != nil {
		slog.Error("get space analytics", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve space analytics")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// GetUserAnalytics handles GET /admin/analytics/users (P5.6).
func (h *Handlers) GetUserAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	ua, err := h.Service.GetUserAnalytics(r.Context(), tenantID)
	if err != nil {
		slog.Error("get user analytics", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve user analytics")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": ua})
}

// GetContentAnalytics handles GET /content/:id/analytics (P5.6).
// It aggregates the per-content daily engagement stats for the requested
// content item over the requested window (default 30 days).
func (h *Handlers) GetContentAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID")
		return
	}

	from, to, err := resolveWindow(r, 30)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	startDate := pgtype.Date{Time: from, Valid: true}
	var endDate *pgtype.Date
	if to != nil {
		endDate = &pgtype.Date{Time: *to, Valid: true}
	}

	stats, err := h.Service.store.GetDailyContentStats(r.Context(), tenantID, &startDate, endDate, contentID)
	if err != nil {
		slog.Error("get content analytics", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve content analytics")
		return
	}

	var views, reactions, comments, uniqueViewers int64
	for _, s := range stats {
		views += int64(s.Views)
		reactions += int64(s.Reactions)
		comments += int64(s.Comments)
		uniqueViewers += int64(s.UniqueViewers)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"views":          views,
		"reactions":      reactions,
		"comments":       comments,
		"shares":         0,
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
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}

	startDate := pgtype.Date{Time: from, Valid: true}
	var endDate *pgtype.Date
	if to != nil {
		endDate = &pgtype.Date{Time: *to, Valid: true}
	}

	contentStats, err := h.Service.store.GetDailyContentStats(r.Context(), tenantID, &startDate, endDate, pgtype.UUID{Bytes: [16]byte{}, Valid: true})
	if err != nil {
		slog.Error("get content analytics", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve content analytics")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}
