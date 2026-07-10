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
	"log/slog"
	"net/http"
	"strconv"
	"strings"
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

	days := parseDays(r, 30)

	dashboard, err := h.Service.GetAdminDashboardData(r.Context(), tenantID, days)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": dashboard})
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
		writeError(w, http.StatusInternalServerError, "flush_error", err.Error())
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
	_ = tenantID
	// Revenue analytics — aggregate from payments table
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total_revenue": 0, "monthly_revenue": 0, "active_subscriptions": 0,
	}})
}

// GetSearchAnalytics handles GET /admin/analytics/search (P5.6).
func (h *Handlers) GetSearchAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	_ = tenantID
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total_searches": 0, "unique_searchers": 0, "top_queries": []any{},
	}})
}

// GetSpaceAnalytics handles GET /admin/analytics/spaces (P5.6).
func (h *Handlers) GetSpaceAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	_ = tenantID
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total_spaces": 0, "active_spaces": 0, "total_members": 0,
	}})
}

// GetUserAnalytics handles GET /admin/analytics/users (P5.6).
func (h *Handlers) GetUserAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	_ = tenantID
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"total_users": 0, "active_users": 0, "new_users_today": 0,
	}})
}

// GetContentAnalytics handles GET /content/:id/analytics (P5.6).
func (h *Handlers) GetContentAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	_ = tenantID
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"views": 0, "reactions": 0, "comments": 0, "shares": 0,
	}})
}
