// Package credentialchecker — handlers.go provides the HTTP handlers for
// the Professional Credential Recognition Checker routes (§94, ADR-102).
//
// Routes:
//
//	GET   /api/v1/credentials/professions        — list professions (public)
//	GET   /api/v1/credentials                    — lookup ?dest=&profession=&issuing= (public)
//	GET   /api/v1/credentials/:id                — pathway detail + community notes (public)
//	POST  /api/v1/credentials/:id/notes          — submit community note (auth, credential_checker.note)
//	GET   /api/v1/admin/credentials              — manage pathways (admin, credential_checker.manage)
//	POST  /api/v1/admin/credentials              — create pathway (admin, credential_checker.manage)
//	POST  /api/v1/admin/credentials/generate     — Content Agent draft placeholder (admin, credential_checker.manage)
//	PATCH /api/v1/admin/credentials/:id          — update pathway (admin, credential_checker.manage)
//	PATCH /api/v1/admin/credentials/:id/approve  — HITL approve (admin, credential_checker.manage)
//
// Ref: japaconnect-engineering/sections/94-professional-credential-checker.md
//      japaconnect-engineering/adr/ADR-102-credential-checker.md
package credentialchecker

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

// Handlers exposes the credential checker endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates credential checker handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("credentialchecker: encode response", "error", err)
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

// HandleListProfessions handles GET /credentials/professions — list all
// credential professions. Public.
func (h *Handlers) HandleListProfessions(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireTenant(w, r); !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	professions, err := h.service.ListProfessions(r.Context())
	if err != nil {
		slog.Error("credentialchecker: list professions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list professions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": professions})
}

// HandleListPathways handles GET /credentials — lookup credential pathways
// with filters. Query params: ?dest=&profession=&issuing=&page=&limit=
// Public.
func (h *Handlers) HandleListPathways(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var issuingSpaceID pgtype.UUID
	if v := r.URL.Query().Get("issuing"); v != "" {
		if err := issuingSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid issuing space ID")
			return
		}
	}

	professionID := parseProfessionID(r.URL.Query().Get("profession"))
	page, limit := parsePageParams(r)

	pathways, total, err := h.service.ListPathways(r.Context(), tenantID, destSpaceID, issuingSpaceID, professionID, page, limit)
	if err != nil {
		slog.Error("credentialchecker: list pathways failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list pathways")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": pathways,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGetPathway handles GET /credentials/:id — pathway detail plus
// published community notes. Public.
func (h *Handlers) HandleGetPathway(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	detail, err := h.service.GetPathway(r.Context(), id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "pathway_not_found", "Pathway not found")
			return
		}
		slog.Error("credentialchecker: get pathway failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get pathway")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": detail})
}

// HandleCreateNote handles POST /credentials/:id/notes — submit a
// community note on a pathway. Auth required (credential_checker.note
// permission checked at route registration).
func (h *Handlers) HandleCreateNote(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	pathwayID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	var input CreateNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Note == "" {
		writeError(w, http.StatusBadRequest, "missing_note", "note is required")
		return
	}

	note, err := h.service.CreateNote(r.Context(), pathwayID, tenantID, user.UserID, input.Note)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "pathway_not_found", "Pathway not found")
			return
		}
		slog.Error("credentialchecker: create note failed", "error", err, "user_id", user.UserID)
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to submit note")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": note})
}

// HandleAdminList handles GET /admin/credentials — manage pathways
// (any status). Admin-only (credential_checker.manage permission checked
// at route registration). Supports ?dest=&profession=&issuing=&status=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var issuingSpaceID pgtype.UUID
	if v := r.URL.Query().Get("issuing"); v != "" {
		if err := issuingSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid issuing space ID")
			return
		}
	}

	professionID := parseProfessionID(r.URL.Query().Get("profession"))
	status := r.URL.Query().Get("status")

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	pathways, total, err := h.service.AdminListPathways(r.Context(), tenantID, destSpaceID, issuingSpaceID, professionID, status, page, limit)
	if err != nil {
		slog.Error("credentialchecker: admin list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list pathways")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": pathways,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminCreate handles POST /admin/credentials — create a new
// credential pathway. Admin-only (credential_checker.manage permission
// checked at route registration).
func (h *Handlers) HandleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	var input CreatePathwayRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.RegulatoryBody == "" {
		writeError(w, http.StatusBadRequest, "missing_regulatory_body", "regulatory_body is required")
		return
	}
	if input.PathwayDescription == "" {
		writeError(w, http.StatusBadRequest, "missing_pathway_description", "pathway_description is required")
		return
	}
	if input.PathwayType == "" {
		writeError(w, http.StatusBadRequest, "missing_pathway_type", "pathway_type is required")
		return
	}

	pathway, err := h.service.CreatePathway(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("credentialchecker: admin create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create pathway")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": pathway})
}

// HandleAdminGenerate handles POST /admin/credentials/generate — Content
// Agent draft placeholder. Returns 202 Accepted. Admin-only
// (credential_checker.manage permission checked at route registration).
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	var input GeneratePathwayRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if !input.ProfessionID.Valid {
		writeError(w, http.StatusBadRequest, "missing_profession_id", "profession_id is required")
		return
	}
	if input.IssuingSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_issuing_space_id", "issuing_space_id is required")
		return
	}
	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	resp, err := h.service.GeneratePathwayDraft(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("credentialchecker: admin generate failed", "error", err)
		writeError(w, http.StatusInternalServerError, "generate_failed", "Failed to queue draft generation")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"data": resp})
}

// HandleAdminUpdate handles PATCH /admin/credentials/:id — update a
// credential pathway. Admin-only (credential_checker.manage permission
// checked at route registration). Only provided (non-nil) fields are
// updated.
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
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	var input UpdatePathwayRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	pathway, err := h.service.UpdatePathway(r.Context(), id, tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "pathway_not_found", "Pathway not found")
			return
		}
		slog.Error("credentialchecker: admin update failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update pathway")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": pathway})
}

// HandleAdminApprove handles PATCH /admin/credentials/:id/approve — HITL
// approve a credential pathway. Admin-only (credential_checker.manage
// permission checked at route registration). Sets hitl_reviewed_by,
// hitl_reviewed_at = now(), status = 'published'. Optional body:
// {"review_notes": "..."}
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
		writeError(w, http.StatusServiceUnavailable, "credential_checker_unavailable", "Credential checker not configured")
		return
	}

	var body ApprovePathwayRequest
	// Body is optional; decode best-effort.
	_ = json.NewDecoder(r.Body).Decode(&body)

	pathway, err := h.service.ApprovePathway(r.Context(), id, tenantID, user.UserID, body.ReviewNotes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "pathway_not_found", "Pathway not found")
			return
		}
		slog.Error("credentialchecker: admin approve failed", "error", err)
		writeError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve pathway")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": pathway})
}
