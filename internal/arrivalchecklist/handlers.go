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
//
//	japaconnect-engineering/adr/ADR-100-arrival-checklist.md
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
	errpkg "github.com/jedafe/japa/internal/errors"
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
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			page = int32(n)
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 && n <= 100 {
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

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "arrival_checklist_disabled", "Arrival checklist is not available")
		return
	}

	selectors, err := h.service.ListSelectors(r.Context())
	if err != nil {
		slog.Error("arrivalchecklist: selector failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "selector_failed", "Failed to list selectors")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": selectors})
}

// HandleGetGuide handles GET /arrival-checklist/:guide_id — returns the
// checklist items for a guide. Anonymous-accessible.
func (h *Handlers) HandleGetGuide(w http.ResponseWriter, r *http.Request) {

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "arrival_checklist_disabled", "Arrival checklist is not available")
		return
	}

	guideID, ok := scanUUIDParam(w, r, "guide_id")
	if !ok {
		return
	}

	items, err := h.service.GetGuideItems(r.Context(), guideID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}

		slog.Error("arrivalchecklist: get guide items failed", "guide_id", guideID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get checklist items")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ---------------------------------------------------------------------------
// Authenticated user handlers (progress — /me/arrival-checklists)
// ---------------------------------------------------------------------------

// HandleListMyChecklists handles GET /me/arrival-checklists — the
// authenticated user's saved arrival checklists with progress summaries.
func (h *Handlers) HandleListMyChecklists(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "arrival_checklist_disabled", "Arrival checklist is not available")
		return
	}

	checklists, err := h.service.ListUserChecklists(r.Context(), user.UserID)
	if err != nil {
		slog.Error("arrivalchecklist: list my checklists failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list checklists")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": checklists})
}

// HandleGetMyChecklist handles GET /me/arrival-checklists/:id — returns
// the items for a single saved checklist with the user's progress state
// merged in. Auth required.
func (h *Handlers) HandleGetMyChecklist(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "arrival_checklist_disabled", "Arrival checklist is not available")
		return
	}

	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	items, err := h.service.GetUserChecklistItems(r.Context(), user.UserID, guideID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}

		slog.Error("arrivalchecklist: get my checklist failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get checklist items")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// HandleStartChecklist handles POST /me/arrival-checklists — start a
// checklist (save initial progress). Auth required.
func (h *Handlers) HandleStartChecklist(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "arrival_checklist_disabled", "Arrival checklist is not available")
		return
	}

	var input StartChecklistRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.GuideID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_guide_id", "guide_id is required")
		return
	}

	guide, err := h.service.StartChecklist(r.Context(), user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}

		slog.Error("arrivalchecklist: start checklist failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "start_failed", "Failed to start checklist")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": guide})
}

// HandleTickItem handles PATCH /me/arrival-checklists/:id/items/:item_id —
// tick/untick an item + add note. Auth required.
func (h *Handlers) HandleTickItem(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	itemID := chi.URLParam(r, "item_id")
	if itemID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_item_id", "item_id is required")
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "arrival_checklist_disabled", "Arrival checklist is not available")
		return
	}

	var input TickItemRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err.Error() != "EOF" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.TickItem(r.Context(), user.UserID, guideID, itemID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}

		slog.Error("arrivalchecklist: tick item failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "tick_failed", "Failed to update item")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa_category")
	applicantType := r.URL.Query().Get("applicant_type")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	guides, total, err := h.service.AdminList(r.Context(), destSpaceID, visaCategory, applicantType, status, q, sort, order, page, limit)
	if err != nil {
		slog.Error("arrivalchecklist: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list guides")

		return
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": guides,
		"meta": map[string]any{
			"page":        page,
			"per_page":    limit,
			"total":       total,
			"total_pages": totalPages,
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var input CreateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	guide, err := h.service.CreateGuide(r.Context(), input)
	if err != nil {
		slog.Error("arrivalchecklist: admin create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create guide")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": guide})
}

// HandleAdminGenerate handles POST /admin/arrival-checklists/generate —
// Content Agent draft placeholder. Admin-only (arrival_checklist.manage
// permission checked at route registration). Returns 202 Accepted; actual
// AI generation is dispatched as a separate Asynq task.
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var input GenerateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	resp, err := h.service.GenerateGuide(r.Context(), input)
	if err != nil {
		slog.Error("arrivalchecklist: admin generate failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to accept generation request")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": resp})
}

// HandleAdminApprove handles PATCH /admin/arrival-checklists/:id/approve
// — HITL approve a guide. Admin-only (arrival_checklist.manage permission
// checked at route registration). Optional body: {"notes": "..."}.
func (h *Handlers) HandleAdminApprove(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var input ApproveGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err.Error() != "EOF" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.ApproveGuide(r.Context(), id, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}

		slog.Error("arrivalchecklist: admin approve failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve guide")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guide})
}

// HandleAdminUpdate handles PATCH /admin/arrival-checklists/:id — update
// a guide. Admin-only (arrival_checklist.manage permission checked at
// route registration).
func (h *Handlers) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "arrival_checklist_unavailable", "Arrival checklist not configured")
		return
	}

	var input UpdateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.UpdateGuide(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, ErrPublishViaUpdate) {
			errpkg.WriteError(w, http.StatusBadRequest, "publish_via_update_blocked",
				"Use the approve endpoint to publish a guide (HITL review required)")
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Arrival checklist guide not found")
			return
		}

		slog.Error("arrivalchecklist: admin update failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update guide")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guide})
}
