// Package housingguide — handlers.go provides the HTTP handlers for the
// Housing & Accommodation Finder Guide routes (§96, ADR-104).
//
// Routes:
//
//	GET   /api/v1/housing-guide               — list ?dest=&city=&type=&page=&limit=
//	GET   /api/v1/housing-guide/:id           — single guide detail
//	GET   /api/v1/admin/housing-guides        — manage guides (housing_guide.manage)
//	POST  /api/v1/admin/housing-guides        — create guide (housing_guide.manage)
//	POST  /api/v1/admin/housing-guides/generate — Content Agent draft placeholder (housing_guide.manage)
//	PATCH /api/v1/admin/housing-guides/:id    — update guide (housing_guide.manage)
//	PATCH /api/v1/admin/housing-guides/:id/approve — HITL approve (housing_guide.manage)
//
// Ref: japaconnect-engineering/sections/96-housing-guide.md
//
//	japaconnect-engineering/adr/ADR-104-housing-guide.md
package housingguide

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
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the housing guide endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates housing guide handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
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
// Public handlers (list + single — anonymous browse per §96)
// ---------------------------------------------------------------------------

// HandleList handles GET /housing-guide — list published housing guides
// with filters. Query params: ?dest=&city=&type=&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "housing_guide_disabled", "Housing guide is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var citySpaceID pgtype.UUID
	if v := r.URL.Query().Get("city"); v != "" {
		if err := citySpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid city space ID")
			return
		}
	}

	accommodationType := r.URL.Query().Get("type")
	page, limit := parsePageParams(r)

	guides, total, err := h.service.List(r.Context(), tenantID, destSpaceID, citySpaceID, accommodationType, page, limit)
	if err != nil {
		slog.Error("housingguide: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list housing guides")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": guides,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGet handles GET /housing-guide/:id — single published housing
// guide detail.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "housing_guide_disabled", "Housing guide is not available")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	guide, err := h.service.GetByID(r.Context(), id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Housing guide not found")
			return
		}

		slog.Error("housingguide: get by id failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get housing guide")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guide})
}

// ---------------------------------------------------------------------------
// Admin handlers (manage + create + generate + update + approve —
// housing_guide.manage permission)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/housing-guides — management queue
// listing all housing guides (any status). Admin-only
// (housing_guide.manage permission checked at route registration).
// Supports ?status=&type=&dest=&city=&q=&active=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "housing_guide_unavailable", "Housing guide not configured")
		return
	}

	status := r.URL.Query().Get("status")
	accommodationType := r.URL.Query().Get("type")
	q := r.URL.Query().Get("q")

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var citySpaceID pgtype.UUID
	if v := r.URL.Query().Get("city"); v != "" {
		if err := citySpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid city space ID")
			return
		}
	}

	var activePtr *bool

	if v := r.URL.Query().Get("active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			activePtr = &b
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	guides, total, err := h.service.AdminList(r.Context(), tenantID, status, accommodationType, q, destSpaceID, citySpaceID, activePtr, page, limit)
	if err != nil {
		slog.Error("housingguide: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list housing guides")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": guides,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminCreate handles POST /admin/housing-guides — create a new
// housing guide. Admin-only (housing_guide.manage permission checked at
// route registration).
func (h *Handlers) HandleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "housing_guide_disabled", "Housing guide is not available")
		return
	}

	var input CreateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	guide, err := h.service.Create(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("housingguide: admin create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create housing guide")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": guide})
}

// HandleAdminGenerate handles POST /admin/housing-guides/generate —
// Content Agent draft placeholder. Admin-only (housing_guide.manage
// permission checked at route registration). Returns 202 Accepted with a
// placeholder response indicating the generation has been queued.
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "housing_guide_disabled", "Housing guide is not available")
		return
	}

	var input GenerateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	result, err := h.service.GenerateDraft(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("housingguide: admin generate failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to queue draft generation")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": result})
}

// HandleAdminUpdate handles PATCH /admin/housing-guides/:id — update an
// existing housing guide. Admin-only (housing_guide.manage permission
// checked at route registration). Only provided fields are applied.
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

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "housing_guide_disabled", "Housing guide is not available")
		return
	}

	var input UpdateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.Update(r.Context(), id, tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Housing guide not found")
			return
		}

		slog.Error("housingguide: admin update failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update housing guide")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guide})
}

// HandleAdminApprove handles PATCH /admin/housing-guides/:id/approve —
// HITL approve a housing guide. Admin-only (housing_guide.manage
// permission checked at route registration). Optional body:
// {"notes": "..."}.
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

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "housing_guide_disabled", "Housing guide is not available")
		return
	}

	var input ApproveGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err.Error() != "EOF" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.Approve(r.Context(), id, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Housing guide not found")
			return
		}

		slog.Error("housingguide: admin approve failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve housing guide")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guide})
}

// HandleAdminReject rejects a pending housing guide (admin HITL).
func (h *Handlers) HandleAdminReject(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusNotFound, "housing_guide_disabled", "Housing guide is not available")
		return
	}

	var input struct {
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err.Error() != "EOF" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.Reject(r.Context(), id, tenantID, user.UserID, input.Reason)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Housing guide not found or not pending review")
			return
		}

		slog.Error("housingguide: admin reject failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject housing guide")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guide})
}
