// Package scamalert — handlers.go provides the HTTP handlers for the Scam
// Alert Board routes (§81, ADR-089).
//
// Routes:
//
//	GET    /api/v1/scam-alerts                    — list confirmed reports (public)
//	GET    /api/v1/scam-alerts/:id                — single confirmed report (public)
//	POST   /api/v1/scam-alerts                    — submit report (auth or anonymous)
//	PATCH  /api/v1/scam-alerts/:id                — edit own report (auth, scam_alert.edit_own/edit_any)
//	POST   /api/v1/scam-alerts/:id/subscribe      — subscribe to space (auth)
//	DELETE /api/v1/scam-alerts/:id/subscribe      — unsubscribe from space (auth)
//	GET    /api/v1/admin/scam-alerts              — moderation queue (auth, scam_alert.review)
//	PATCH  /api/v1/admin/scam-alerts/:id          — confirm/reject report (auth, scam_alert.review)
//	POST   /api/v1/admin/scam-alerts/:id/pin      — pin report to space (auth, scam_alert.pin)
//	DELETE /api/v1/admin/scam-alerts/:id/pin      — unpin report (auth, scam_alert.pin)
//
// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
//
//	japaconnect-engineering/adr/ADR-089-scam-alert-board.md
package scamalert

import (
	"context"
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
	"github.com/jedafe/japa/internal/tenant"
)

// updater is the subset of Service methods used by the public edit handler.
type updater interface {
	Enabled(ctx context.Context, tenantID pgtype.UUID) bool
	Update(ctx context.Context, id, tenantID, callerID pgtype.UUID, input UpdateReportInput) (ScamReportDTO, error)
}

// Handlers exposes the scam-alert board endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	updater  updater
}

// NewHandlers creates scam-alert handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings, updater: service}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
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
			page = int32(n)
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (list + single — anonymous browse per §81)
// ---------------------------------------------------------------------------

// HandleList handles GET /scam-alerts — list confirmed scam reports.
// Query params: ?dest=&scam_type=&page=&limit= (also accepts ?destination_space_id= as legacy alias)
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scam_alert_disabled", "Scam alert board is not available")
		return
	}

	var destSpaceID pgtype.UUID
	// Accept ?dest= (geo-aware standard) or ?destination_space_id= (legacy alias).
	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}
	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	scamType := r.URL.Query().Get("scam_type")
	page, limit := parsePageParams(r)

	reports, total, err := h.service.List(r.Context(), tenantID, destSpaceID, scamType, page, limit)
	if err != nil {
		slog.Error("scamalert: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list scam reports")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": reports,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleSchema handles GET /scam-alerts/schema — returns the canonical
// scam_type and platform option lists (Rule 16).
func (h *Handlers) HandleSchema(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scam_alert_disabled", "Scam alert board is not available")
		return
	}

	schema := h.service.Schema()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": schema})
}

// HandleGet handles GET /scam-alerts/:id — single confirmed scam report.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scam_alert_disabled", "Scam alert board is not available")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	report, err := h.service.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "report_not_found", "Scam report not found")
			return
		}
		slog.Error("scamalert: get failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get scam report")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": report})
}

// ---------------------------------------------------------------------------
// Submit handler (auth or anonymous per §81)
// ---------------------------------------------------------------------------

// HandleSubmit handles POST /scam-alerts — submit a new scam report. Auth
// optional: authenticated users have their reporter_id stamped; anonymous
// reports are accepted when scam_alert.anonymous_reports is enabled.
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scam_alert_disabled", "Scam alert board is not available")
		return
	}

	var input SubmitReportInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.ScamType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_scam_type", "Scam type is required")
		return
	}
	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}
	if input.Description == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_description", "Description is required")
		return
	}

	// reporterID is zero-valued (invalid) for anonymous submissions.
	var reporterID pgtype.UUID
	user := auth.FromContext(r.Context())
	if user != nil {
		reporterID = user.UserID
	} else {
		// Anonymous submission — only allowed when the setting is enabled.
		if !h.service.AnonymousReportsAllowed(r.Context(), tenantID) {
			errpkg.WriteError(w, http.StatusUnauthorized, "anonymous_disabled", "Authentication required to submit a scam report")
			return
		}
	}

	report, err := h.service.Submit(r.Context(), tenantID, reporterID, input)
	if err != nil {
		if errors.Is(err, ErrInvalidScamType) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_scam_type", "Scam type must be one of: fraudulent_agent, fake_job, housing_fraud, fake_scholarship, whatsapp_telegram, other")
			return
		}
		if errors.Is(err, ErrInvalidPlatform) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_platform", "Platform must be one of: whatsapp, instagram, linkedin, email, website, other")
			return
		}
		slog.Error("scamalert: submit failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit scam report")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": report})
}

// HandleUpdate handles PATCH /scam-alerts/:id — edit an existing scam report.
// Requires authentication and either scam_alert.edit_own (author) or
// scam_alert.edit_any (moderator/admin). Anonymous reports cannot be edited
// because ownership cannot be resolved.
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.updater == nil || !h.updater.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scam_alert_disabled", "Scam alert board is not available")
		return
	}

	var input UpdateReportInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.ScamType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_scam_type", "Scam type is required")
		return
	}
	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}
	if input.Description == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_description", "Description is required")
		return
	}

	report, err := h.updater.Update(r.Context(), id, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrInvalidScamType) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_scam_type", "Scam type must be one of: fraudulent_agent, fake_job, housing_fraud, fake_scholarship, whatsapp_telegram, other")
			return
		}
		if errors.Is(err, ErrInvalidPlatform) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_platform", "Platform must be one of: whatsapp, instagram, linkedin, email, website, other")
			return
		}
		if errors.Is(err, ErrEditForbidden) {
			errpkg.WriteError(w, http.StatusForbidden, "edit_forbidden", "You do not have permission to edit this report")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "report_not_found", "Scam report not found")
			return
		}
		slog.Error("scamalert: update failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update scam report")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": report})
}

// ---------------------------------------------------------------------------
// Subscription handlers (auth required)
// ---------------------------------------------------------------------------

// HandleSubscribe handles POST /scam-alerts/:id/subscribe — subscribe the
// authenticated user to scam alerts for the report's destination space.
// Auth required (no separate RBAC node per §81 — subscribe is auth-only).
func (h *Handlers) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scam_alert_disabled", "Scam alert board is not available")
		return
	}

	// Fetch the report to resolve its destination_space_id.
	report, err := h.service.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "report_not_found", "Scam report not found")
			return
		}
		slog.Error("scamalert: subscribe get failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get scam report")
		return
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(report.DestinationSpaceID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_destination", "Report has no valid destination space")
		return
	}

	if err := h.service.Subscribe(r.Context(), user.UserID, tenantID, destSpaceID); err != nil {
		slog.Error("scamalert: subscribe failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "subscribe_failed", "Failed to subscribe")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"subscribed": true}})
}

// HandleUnsubscribe handles DELETE /scam-alerts/:id/subscribe — unsubscribe
// the authenticated user from scam alerts for the report's destination space.
// Auth required (no separate RBAC node per §81 — subscribe is auth-only).
func (h *Handlers) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "scam_alert_disabled", "Scam alert board is not available")
		return
	}

	// Fetch the report to resolve its destination_space_id.
	report, err := h.service.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "report_not_found", "Scam report not found")
			return
		}
		slog.Error("scamalert: unsubscribe get failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get scam report")
		return
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(report.DestinationSpaceID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_destination", "Report has no valid destination space")
		return
	}

	if err := h.service.Unsubscribe(r.Context(), user.UserID, tenantID, destSpaceID); err != nil {
		slog.Error("scamalert: unsubscribe failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "unsubscribe_failed", "Failed to unsubscribe")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"subscribed": false}})
}

// ---------------------------------------------------------------------------
// Admin handlers (review, pin — scam_alert.review / scam_alert.pin)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/scam-alerts — moderation queue listing
// all reports (any status). Admin-only (scam_alert.review permission checked
// at route registration). Supports ?status=&dest=&scam_type=&page=&limit= (also accepts ?destination_space_id= as legacy alias).
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "scam_alert_unavailable", "Scam alert service not configured")
		return
	}

	status := r.URL.Query().Get("status")
	scamType := r.URL.Query().Get("scam_type")

	var destSpaceID pgtype.UUID
	// Accept ?dest= (geo-aware standard) or ?destination_space_id= (legacy alias).
	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}
	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	reports, total, err := h.service.AdminList(r.Context(), tenantID, status, destSpaceID, scamType, page, limit)
	if err != nil {
		slog.Error("scamalert: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list scam reports")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": reports,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleReview handles PATCH /admin/scam-alerts/:id — confirm or reject a
// scam report. Admin-only (scam_alert.review permission checked at route
// registration).
func (h *Handlers) HandleReview(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "scam_alert_unavailable", "Scam alert service not configured")
		return
	}

	var input ReviewInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Status != "confirmed" && input.Status != "debunked" && input.Status != "duplicate" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be 'confirmed', 'debunked', or 'duplicate'")
		return
	}

	if err := h.service.Review(r.Context(), id, tenantID, input.Status, input.ModeratorNote, user.UserID); err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be 'confirmed', 'debunked', or 'duplicate'")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "report_not_found", "Scam report not found")
			return
		}
		slog.Error("scamalert: review failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "review_failed", "Failed to review scam report")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": input.Status}})
}

// HandlePin handles POST /admin/scam-alerts/:id/pin — pin a confirmed report
// to a space. Admin-only (scam_alert.pin permission checked at route
// registration).
func (h *Handlers) HandlePin(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "scam_alert_unavailable", "Scam alert service not configured")
		return
	}

	var body struct {
		SpaceID string `json:"space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.SpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_space_id", "space_id is required")
		return
	}

	var spaceID pgtype.UUID
	if err := spaceID.Scan(body.SpaceID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space_id")
		return
	}

	if err := h.service.Pin(r.Context(), id, tenantID, spaceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "report_not_found", "Confirmed scam report not found")
			return
		}
		slog.Error("scamalert: pin failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "pin_failed", "Failed to pin scam report")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"pinned": true}})
}

// HandleUnpin handles DELETE /admin/scam-alerts/:id/pin — remove the pin
// from a report. Admin-only (scam_alert.pin permission checked at route
// registration).
func (h *Handlers) HandleUnpin(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "scam_alert_unavailable", "Scam alert service not configured")
		return
	}

	if err := h.service.Unpin(r.Context(), id, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "report_not_found", "Scam report not found")
			return
		}
		slog.Error("scamalert: unpin failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "unpin_failed", "Failed to unpin scam report")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"pinned": false}})
}
