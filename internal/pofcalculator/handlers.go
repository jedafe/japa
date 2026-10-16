// Package pofcalculator — handlers.go provides the HTTP handlers for the
// Proof of Funds Calculator routes (§108, ADR-119).
//
// Routes:
//
//	GET   /api/v1/pof/requirements                    — ?dest=&visa_type= (public)
//	GET   /api/v1/pof/visa-types                      — ?dest= distinct visa types (public)
//	GET   /api/v1/pof/calculate                       — ?dest=&visa_type=&members=&dependants= (public)
//	GET   /api/v1/admin/pof-requirements              — manage requirements (admin)
//	POST  /api/v1/admin/pof-requirements              — create requirement (admin)
//	POST  /api/v1/admin/pof-requirements/generate     — Content Agent draft placeholder (admin)
//	PATCH /api/v1/admin/pof-requirements/:id          — update (admin)
//	PATCH /api/v1/admin/pof-requirements/:id/approve  — HITL approve (admin)
//
// Ref: japaconnect-engineering/sections/108-pof-calculator.md
//
//	japaconnect-engineering/adr/ADR-119-pof-calculator.md
package pofcalculator

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the POF calculator endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates POF handlers bound to the service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
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

func scanDestParam(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID

	v := r.URL.Query().Get("dest")
	if v == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_dest", "dest query param is required")
		return id, false
	}

	if err := id.Scan(v); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_dest", "Invalid destination space ID")
		return id, false
	}

	return id, true
}

func parsePageParams(r *http.Request) (page, limit int32) {
	page, limit = 1, 20

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

func unavailable(w http.ResponseWriter) {
	errpkg.WriteError(w, http.StatusServiceUnavailable, "pof_calculator_unavailable", "POF calculator not configured")
}

// HandleListRequirements handles GET /pof/requirements. Public.
func (h *Handlers) HandleListRequirements(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_dest", "Invalid destination space ID")
			return
		}
	}

	reqs, err := h.service.ListRequirements(r.Context(), destSpaceID, r.URL.Query().Get("visa_type"))
	if err != nil {
		slog.Error("pofcalculator: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list requirements")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": reqs})
}

// HandleVisaTypes handles GET /pof/visa-types — distinct visa types for a
// destination (filter options). Public; requires ?dest=.
func (h *Handlers) HandleVisaTypes(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	destSpaceID, ok := scanDestParam(w, r)
	if !ok {
		return
	}

	types, err := h.service.VisaTypes(r.Context(), destSpaceID)
	if err != nil {
		slog.Error("pofcalculator: visa types failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list visa types")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// HandleCalculate handles GET /pof/calculate — resolve amounts for
// ?dest=&visa_type=&members=&dependants=. Public.
func (h *Handlers) HandleCalculate(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	destSpaceID, ok := scanDestParam(w, r)
	if !ok {
		return
	}

	visaType := r.URL.Query().Get("visa_type")
	if visaType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_visa_type", "visa_type query param is required")
		return
	}

	members, dependants := 1, 0

	if v := r.URL.Query().Get("members"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			members = n
		}
	}

	if v := r.URL.Query().Get("dependants"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			dependants = n
		}
	}

	results, err := h.service.Calculate(r.Context(), destSpaceID, visaType, members, dependants)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_params", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results})
}

// HandleAdminList handles GET /admin/pof-requirements (any status).
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		unavailable(w)
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_dest", "Invalid destination space ID")
			return
		}
	}

	// The admin EntityPage sends sort as "col:dir"; accept the split
	// order/dir params too.
	sortKey, sortDir, _ := strings.Cut(r.URL.Query().Get("sort"), ":")
	if v := r.URL.Query().Get("order"); v != "" {
		sortDir = v
	}

	if v := r.URL.Query().Get("dir"); v != "" {
		sortDir = v
	}

	page, limit := parsePageParams(r)

	reqs, total, err := h.service.AdminListRequirements(r.Context(), destSpaceID,
		r.URL.Query().Get("visa_type"), r.URL.Query().Get("status"), r.URL.Query().Get("q"),
		sortKey, sortDir, page, limit)
	if err != nil {
		slog.Error("pofcalculator: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list requirements")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       reqs,
		"pagination": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleAdminCreate handles POST /admin/pof-requirements.
func (h *Handlers) HandleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		unavailable(w)
		return
	}

	var input RequirementRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	req, err := h.service.CreateRequirement(r.Context(), input)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "create_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": req})
}

// HandleAdminGenerate handles POST /admin/pof-requirements/generate —
// enqueues tool:generate_draft; the Content Agent writes an ai_draft
// requirement for HITL review (ADR-088). Body carries generation hints;
// destination_space_id is required.
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		unavailable(w)
		return
	}

	var body map[string]string
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	if err := h.service.EnqueueGenerate(r.Context(), body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "generate_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"status": "queued"}})
}

// HandleAdminUpdate handles PATCH /admin/pof-requirements/:id.
func (h *Handlers) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input RequirementRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdateRequirement(r.Context(), id, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "requirement_not_found", "Requirement not found")
			return
		}

		if errors.Is(err, errInvalidInput) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}

		slog.Error("pofcalculator: update failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update requirement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id")}})
}

// HandleAdminApprove handles PATCH /admin/pof-requirements/:id/approve —
// HITL publish.
func (h *Handlers) HandleAdminApprove(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body ApproveRequest
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.service.ApproveRequirement(r.Context(), id, user.UserID, body.ReviewNotes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "requirement_not_found", "Requirement not found")
			return
		}

		slog.Error("pofcalculator: approve failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve requirement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": StatusPublished}})
}

// HandleAdminGet handles GET /admin/pof-requirements/:id — requirement
// detail (any status). Admin-only.
func (h *Handlers) HandleAdminGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, err := h.service.GetRequirement(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "requirement_not_found", "Requirement not found")
			return
		}

		slog.Error("pofcalculator: get failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to load requirement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": req})
}

// HandleAdminDelete handles DELETE /admin/pof-requirements/:id —
// removes a non-published requirement. Admin-only.
func (h *Handlers) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.DeleteRequirement(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "requirement_not_found", "Requirement not found or is published")
			return
		}

		slog.Error("pofcalculator: delete failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete requirement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id")}})
}

// HandleAdminReject handles PATCH /admin/pof-requirements/:id/reject —
// HITL reject.
func (h *Handlers) HandleAdminReject(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body ApproveRequest
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.service.RejectRequirement(r.Context(), id, user.UserID, body.ReviewNotes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "requirement_not_found", "Requirement not found")
			return
		}

		slog.Error("pofcalculator: reject failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject requirement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": StatusRejected}})
}
