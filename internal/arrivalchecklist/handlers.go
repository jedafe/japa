// Package arrivalchecklist — handlers.go provides the HTTP handlers for
// the Arrival Checklist Generator routes (§92, ADR-100).
//
// Routes:
//
//	GET   /api/v1/arrival-checklist/selector             — available dest + visa_type combos (public)
//	GET   /api/v1/arrival-checklist/:guide_id            — checklist items for a guide (public)
//	GET   /api/v1/admin/arrival-checklists               — manage guides (admin, arrival_checklist.manage)
//	POST  /api/v1/admin/arrival-checklists               — create guide (admin, arrival_checklist.manage)
//	POST  /api/v1/admin/arrival-checklists/generate      — Content Agent draft placeholder (admin, arrival_checklist.manage)
//	PATCH /api/v1/admin/arrival-checklists/:id/approve   — HITL approve (admin, arrival_checklist.manage)
//	PATCH /api/v1/admin/arrival-checklists/:id           — update guide (admin, arrival_checklist.manage)
//
// Ref: japaconnect-engineering/sections/92-arrival-checklist.md
//      japaconnect-engineering/adr/ADR-100-arrival-checklist.md
package arrivalchecklist

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
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the arrival checklist endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates arrival checklist handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("arrivalchecklist: encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
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
// Public handlers (selector + guide items — anonymous browse per §92)
// ---------------------------------------------------------------------------

// HandleSelector handles GET /arrival-checklist/selector — returns the
// available destination + visa category combinations. Anonymous-accessible.
func (h *Handlers) HandleSelector(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "arrival_checklist_disabled", "Arrival checklist is not available")
		return
	}

	selectors, err := h.service.ListSelectors(r.Context(), tenantID)
	if err != nil {
		slog.Error("arrivalchecklist: selector failed", "error", err)
		writeError(w, http.StatusInternalServerError, "selector_failed", "Failed to list selectors")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": selectors})
}

// HandleGetGuide handles GET /arrival-checklist/:guide_id — returns the
// checklist items for a guide. Anonymous-accessible.
func (h *Handlers) HandleGetGuide(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "arrival_checklist_disabled", "Arrival checklist is not available")
		return
	}

	guideID, ok := scanUUIDParam(w, r, "guide_id")
	if !ok {
		return
	}

	items, err := h.service.GetGuideItems(r.Context(), guideID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}
		slog.Error("arrivalchecklist: get guide items failed", "guide_id", guideID, "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get checklist items")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ---------------------------------------------------------------------------
// Admin handlers (manage guides — arrival_checklist.manage permission)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/arrival-checklists — manage guides.
// Admin-only (arrival_checklist.manage permission checked at route
// registration). Supports ?dest=&visa_category=&applicant_type=&status=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa_category")
	applicantType := r.URL.Query().Get("applicant_type")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	guides, total, err := h.service.AdminList(r.Context(), tenantID, destSpaceID, visaCategory, applicantType, status, q, page, limit)
	if err != nil {
		slog.Error("arrivalchecklist: admin list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list guides")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": guides,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminCreate handles POST /admin/arrival-checklists — create a
// guide. Admin-only (arrival_checklist.manage permission checked at route
// registration).
func (h *Handlers) HandleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var input CreateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	guide, err := h.service.CreateGuide(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("arrivalchecklist: admin create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create guide")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": guide})
}

// HandleAdminGenerate handles POST /admin/arrival-checklists/generate —
// Content Agent draft placeholder. Admin-only (arrival_checklist.manage
// permission checked at route registration). Returns 202 Accepted; actual
// AI generation is dispatched as a separate Asynq task.
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var input GenerateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	resp, err := h.service.GenerateGuide(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("arrivalchecklist: admin generate failed", "error", err)
		writeError(w, http.StatusInternalServerError, "generate_failed", "Failed to accept generation request")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"data": resp})
}

// HandleAdminApprove handles PATCH /admin/arrival-checklists/:id/approve
// — HITL approve a guide. Admin-only (arrival_checklist.manage permission
// checked at route registration). Optional body: {"notes": "..."}
func (h *Handlers) HandleAdminApprove(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var input ApproveGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.ApproveGuide(r.Context(), id, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}
		slog.Error("arrivalchecklist: admin approve failed", "error", err)
		writeError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve guide")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": guide})
}

// HandleAdminUpdate handles PATCH /admin/arrival-checklists/:id — update
// a guide. Admin-only (arrival_checklist.manage permission checked at
// route registration).
func (h *Handlers) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var input UpdateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.UpdateGuide(r.Context(), id, tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}
		slog.Error("arrivalchecklist: admin update failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update guide")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": guide})
}
