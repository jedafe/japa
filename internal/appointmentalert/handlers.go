// Package appointmentalert — handlers.go provides the HTTP handlers for
// the Embassy Appointment Alerter routes (§91, ADR-099).
//
// Routes:
//
//	GET   /api/v1/embassies                    — list monitored embassies ?dest= (public)
//	GET   /api/v1/me/appointment-alerts        — user's alerts (auth, appointment_alert.manage_own)
//	POST  /api/v1/me/appointment-alerts        — create alert (auth, appointment_alert.create)
//	PATCH /api/v1/me/appointment-alerts/:id    — pause/activate/update alert (auth, appointment_alert.manage_own)
//	DELETE /api/v1/me/appointment-alerts/:id   — delete alert (auth, appointment_alert.manage_own)
//	GET   /api/v1/admin/embassies              — all monitored embassies (admin, embassy_monitor.manage)
//	POST  /api/v1/admin/embassies              — add embassy to monitor (admin, embassy_monitor.manage)
//	PATCH /api/v1/admin/embassies/:id          — update embassy config (admin, embassy_monitor.manage)
//	POST  /api/v1/admin/embassies/:id/poll     — trigger immediate poll (admin, embassy_monitor.manage)
//
// Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md
//
//	japaconnect-engineering/adr/ADR-099-embassy-appointment-alerter.md
package appointmentalert

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the embassy appointment alerter endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates appointment alerter handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (embassy list — anonymous browse per §91)
// ---------------------------------------------------------------------------

// HandleListEmbassies handles GET /embassies — list monitored embassies
// with an optional destination filter. Query params: ?dest=&page=&limit=.
func (h *Handlers) HandleListEmbassies(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	page, limit := parsePageParams(r)

	embassies, total, err := h.service.ListEmbassies(r.Context(), destSpaceID, true, page, limit, "", "", "")
	if err != nil {
		slog.Error("appointmentalert: list embassies failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list embassies")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": embassies,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// ---------------------------------------------------------------------------
// User handlers (me/appointment-alerts — auth + ownership required)
// ---------------------------------------------------------------------------

// HandleListMyAlerts handles GET /me/appointment-alerts — list the
// authenticated user's alerts. Auth required (appointment_alert.manage_own
// permission checked at route registration). Query: ?status=&page=&limit=.
func (h *Handlers) HandleListMyAlerts(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	status := r.URL.Query().Get("status")
	page, limit := parsePageParams(r)

	alerts, total, err := h.service.ListUserAlerts(r.Context(), user.UserID, status, page, limit)
	if err != nil {
		slog.Error("appointmentalert: list my alerts failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list alerts")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": alerts,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleCreateMyAlert handles POST /me/appointment-alerts — create a new
// appointment alert for the authenticated user. Auth required
// (appointment_alert.create permission checked at route registration).
func (h *Handlers) HandleCreateMyAlert(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	var input CreateAlertInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.EmbassySlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_embassy_slug", "embassy_slug is required")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	if input.DateRangeStart == "" || input.DateRangeEnd == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_date_range", "date_range_start and date_range_end are required")
		return
	}

	alert, err := h.service.CreateAlert(r.Context(), user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrSMSPremiumOnly) {
			errpkg.WriteError(w, http.StatusForbidden, "sms_premium_only", "The SMS notification channel is only available to premium users")
			return
		}

		slog.Error("appointmentalert: create alert failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create alert")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": alert})
}

// HandleUpdateMyAlert handles PATCH /me/appointment-alerts/:id — pause,
// activate, or update the authenticated user's own alert. Auth required
// (appointment_alert.manage_own permission checked at route registration).
func (h *Handlers) HandleUpdateMyAlert(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	alertID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	var input UpdateAlertInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	alert, err := h.service.UpdateAlert(r.Context(), alertID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "alert_not_found", "Alert not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only manage your own alerts")
			return
		}

		if errors.Is(err, ErrSMSPremiumOnly) {
			errpkg.WriteError(w, http.StatusForbidden, "sms_premium_only", "The SMS notification channel is only available to premium users")
			return
		}

		slog.Error("appointmentalert: update alert failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update alert")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": alert})
}

// HandleDeleteMyAlert handles DELETE /me/appointment-alerts/:id — delete
// the authenticated user's own alert. Auth required
// (appointment_alert.manage_own permission checked at route registration).
func (h *Handlers) HandleDeleteMyAlert(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	alertID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	if err := h.service.DeleteAlert(r.Context(), alertID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "alert_not_found", "Alert not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only manage your own alerts")
			return
		}

		slog.Error("appointmentalert: delete alert failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete alert")

		return
	}

	// 204 No Content — no response body (§3.1).
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Admin handlers (embassy config + poll trigger — embassy_monitor.manage)
// ---------------------------------------------------------------------------

// HandleAdminListEmbassies handles GET /admin/embassies — list all
// monitored embassies (active and inactive) for the moderation/config
// view. Admin-only (embassy_monitor.manage permission checked at route
// registration). Query: ?dest=&active=&page=&limit=.
func (h *Handlers) HandleAdminListEmbassies(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "appointment_alert_unavailable", "Appointment alerter not configured")
		return
	}

	// Feature-flag gate: return 404 (not 403) when the feature is disabled,
	// consistent with the public endpoints (§91, feature-flag-naming.md).
	if !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	activeOnly := false

	if v := r.URL.Query().Get("active"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			activeOnly = b
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	// Optional search (q) and sort (sort/order) params for the admin
	// DataTable (§17, geo-aware tool pattern).
	search := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")

	sortOrder := r.URL.Query().Get("order")
	if sortOrder == "" {
		sortOrder = r.URL.Query().Get("dir") // DataTable sends ?dir=
	}

	embassies, total, err := h.service.ListEmbassies(r.Context(), destSpaceID, activeOnly, page, limit, search, sortBy, sortOrder)
	if err != nil {
		slog.Error("appointmentalert: admin list embassies failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list embassies")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": embassies,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminCreateEmbassy handles POST /admin/embassies — add a new
// embassy to monitor. Admin-only (embassy_monitor.manage permission
// checked at route registration).
func (h *Handlers) HandleAdminCreateEmbassy(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	var input CreateEmbassyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.BookingURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_booking_url", "booking_url is required")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	embassy, err := h.service.CreateEmbassy(r.Context(), input)
	if err != nil {
		slog.Error("appointmentalert: admin create embassy failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create embassy monitor")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": embassy})
}

// HandleAdminUpdateEmbassy handles PATCH /admin/embassies/:id — update a
// monitored embassy's config. Admin-only (embassy_monitor.manage
// permission checked at route registration).
func (h *Handlers) HandleAdminUpdateEmbassy(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	embassyID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	var input UpdateEmbassyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	embassy, err := h.service.UpdateEmbassy(r.Context(), embassyID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "embassy_not_found", "Embassy not found")
			return
		}

		slog.Error("appointmentalert: admin update embassy failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update embassy config")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": embassy})
}

// HandleAdminPollEmbassy handles POST /admin/embassies/:id/poll — trigger
// an immediate poll of a monitored embassy. This sets last_polled_at =
// now() and returns 202 Accepted; the actual scraping is performed by the
// Asynq worker, not this handler. Admin-only (embassy_monitor.manage
// permission checked at route registration).
func (h *Handlers) HandleAdminPollEmbassy(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	embassyID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "appointment_alert_disabled", "Appointment alerts are not available")
		return
	}

	if err := h.service.TriggerPoll(r.Context(), embassyID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "embassy_not_found", "Embassy not found")
			return
		}

		slog.Error("appointmentalert: admin poll embassy failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "poll_failed", "Failed to trigger poll")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status":     "poll_triggered",
			"embassy_id": embassyID.String(),
		},
	})
}
