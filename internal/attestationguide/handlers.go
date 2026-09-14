// Package attestationguide — handlers.go provides the HTTP handlers for
// the Document Attestation & Apostille Guide routes (§97, ADR-105).
//
// Routes:
//
//	GET   /api/v1/attestation                       — lookup ?dest=&doc_type=&issuing= (public)
//	GET   /api/v1/attestation/:id                   — pathway detail (public)
//	GET   /api/v1/attestation/:id/export/pdf        — export as PDF placeholder (public)
//	POST  /api/v1/attestation/:id/updates           — submit community update (auth, attestation_guide.update)
//	GET   /api/v1/admin/attestation                 — manage pathways (admin, attestation_guide.manage)
//	POST  /api/v1/admin/attestation                 — create pathway (admin, attestation_guide.manage)
//	POST  /api/v1/admin/attestation/generate        — Content Agent draft placeholder (admin, attestation_guide.manage)
//	PATCH /api/v1/admin/attestation/:id/approve     — HITL approve (admin, attestation_guide.manage)
//	PATCH /api/v1/admin/attestation/updates/:id     — approve/reject community update (admin, attestation_guide.manage)
//
// Ref: japaconnect-engineering/sections/97-attestation-guide.md
//
//	japaconnect-engineering/adr/ADR-105-attestation-guide.md
package attestationguide

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

// Handlers exposes the attestation guide endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates attestation guide handlers bound to the provided
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
	// Convention: page_size + page_offset (api-response-format.md).
	// Backward compat: also accept page + limit.
	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (lookup + detail + export — anonymous browse per §97)
// ---------------------------------------------------------------------------

// HandleList handles GET /attestation — lookup published attestation
// pathways with filters. Query params: ?dest=&doc_type=&issuing=&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "attestation_guide_disabled", "Attestation guide is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var issuingSpaceID pgtype.UUID
	if v := r.URL.Query().Get("issuing"); v != "" {
		if err := issuingSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid issuing space ID")
			return
		}
	}

	documentType := r.URL.Query().Get("doc_type")

	page, limit := parsePageParams(r)

	pathways, total, err := h.service.List(r.Context(), destSpaceID, issuingSpaceID, documentType, page, limit)
	if err != nil {
		slog.Error("attestationguide: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list attestation pathways")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": pathways,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGet handles GET /attestation/:id — single published attestation
// pathway detail.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "attestation_guide_disabled", "Attestation guide is not available")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	pathway, err := h.service.GetByID(r.Context(), id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "pathway_not_found", "Attestation pathway not found")
			return
		}

		slog.Error("attestationguide: get by id failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get attestation pathway")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pathway})
}

// HandleExportPDF handles GET /attestation/:id/export/pdf — returns the
// pathway data as a downloadable JSON file. The route is named /export/pdf
// per §97/ADR-105 (which specify PDF export via §pdf); until a §pdf
// rendering service is available, the handler returns the structured
// pathway JSON so the client can render it to PDF. The Content-Disposition
// filename uses .json to reflect the actual format. Public.
func (h *Handlers) HandleExportPDF(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "attestation_guide_disabled", "Attestation guide is not available")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	pathway, err := h.service.GetByID(r.Context(), id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "pathway_not_found", "Attestation pathway not found")
			return
		}

		slog.Error("attestationguide: export pdf get failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get attestation pathway")

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="attestation-pathway-`+pathway.ID+`.json"`)
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]any{
		"data":   pathway,
		"format": "attestation_pathway_v1",
	}); err != nil {
		slog.Error("attestationguide: encode export response", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Authenticated handlers (community update submission — auth required)
// ---------------------------------------------------------------------------

// HandleSubmitUpdate handles POST /attestation/:id/updates — submit a
// community update for a pathway step. Auth required
// (attestation_guide.update permission checked at route registration).
func (h *Handlers) HandleSubmitUpdate(w http.ResponseWriter, r *http.Request) {
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

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "attestation_guide_disabled", "Attestation guide is not available")
		return
	}

	var input CreateCommunityUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	update, err := h.service.SubmitCommunityUpdate(r.Context(), pathwayID, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "pathway_not_found", "Attestation pathway not found")
			return
		}

		slog.Error("attestationguide: submit update failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit community update")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": update})
}

// ---------------------------------------------------------------------------
// Admin handlers (manage pathways, generate draft, HITL approve,
// review community updates — attestation_guide.manage permission)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/attestation — moderation queue
// listing all attestation pathways (any status). Admin-only
// (attestation_guide.manage permission checked at route registration).
// Supports ?status=&doc_type=&dest=&active=&q=&sort=&dir=&page=&limit=.
// BUG-004: sort and q (search) params now honoured.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "attestation_guide_unavailable", "Attestation guide not configured")
		return
	}

	status := r.URL.Query().Get("status")
	documentType := r.URL.Query().Get("doc_type")
	search := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("dir")

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
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

	pathways, total, err := h.service.AdminList(r.Context(), destSpaceID, status, documentType, activePtr, search, sortBy, sortDir, page, limit)
	if err != nil {
		slog.Error("attestationguide: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list attestation pathways")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": pathways,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminListUpdates handles GET /admin/attestation/updates — lists
// pending (or filtered) community updates for the moderation queue.
// Admin-only (attestation_guide.manage permission checked at route
// registration). Supports ?status=&page=&limit= (BUG-005).
func (h *Handlers) HandleAdminListUpdates(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "attestation_guide_unavailable", "Attestation guide not configured")
		return
	}

	status := r.URL.Query().Get("status")

	page, limit := parsePageParams(r)
	if limit > 100 {
		limit = 100
	}

	updates, total, err := h.service.AdminListUpdates(r.Context(), status, page, limit)
	if err != nil {
		slog.Error("attestationguide: admin list updates failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_updates_failed", "Failed to list community updates")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": updates,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminCreate handles POST /admin/attestation — create a new
// attestation pathway. Admin-only (attestation_guide.manage permission
// checked at route registration).
func (h *Handlers) HandleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "attestation_guide_disabled", "Attestation guide is not available")
		return
	}

	var input CreatePathwayInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DocumentType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_document_type", "document_type is required")
		return
	}

	if input.IssuingSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_issuing_space_id", "issuing_space_id is required")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	pathway, err := h.service.Create(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("attestationguide: admin create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create attestation pathway")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": pathway})
}

// HandleAdminGenerate handles POST /admin/attestation/generate — creates
// an AI-drafted pathway in 'ai_draft' status. The pathway is created with
// empty pathway_steps and requires HITL approval before publication.
// Admin-only (attestation_guide.manage permission checked at route
// registration).
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "attestation_guide_disabled", "Attestation guide is not available")
		return
	}

	var input GeneratePathwayInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DocumentType == "" || input.IssuingSpaceID == "" || input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "document_type, issuing_space_id, and destination_space_id are required")
		return
	}

	pathway, err := h.service.GenerateDraft(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("attestationguide: admin generate failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to generate attestation pathway draft")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data":    pathway,
		"status":  "accepted",
		"message": "AI draft pathway created in 'ai_draft' status. Review and approve via PATCH /admin/attestation/:id/approve before publication.",
	})
}

// HandleAdminApprove handles PATCH /admin/attestation/:id/approve — HITL
// approval of an AI-drafted pathway. Sets hitl_reviewed_by,
// hitl_reviewed_at = now(), status = 'published'. Admin-only
// (attestation_guide.manage permission checked at route registration).
// Optional body: {"review_notes": "..."}.
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
		errpkg.WriteError(w, http.StatusServiceUnavailable, "attestation_guide_unavailable", "Attestation guide not configured")
		return
	}

	var body struct {
		ReviewNotes *string `json:"review_notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.ReviewNotes != nil && *body.ReviewNotes != "" {
		// notes captured below
	}

	notes := ""
	if body.ReviewNotes != nil {
		notes = *body.ReviewNotes
	}

	pathway, err := h.service.Approve(r.Context(), id, tenantID, user.UserID, notes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "pathway_not_found", "Attestation pathway not found")
			return
		}

		slog.Error("attestationguide: admin approve failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve attestation pathway")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pathway})
}

// HandleAdminReviewUpdate handles PATCH /admin/attestation/updates/:id —
// approve or reject a pending community update. Admin-only
// (attestation_guide.manage permission checked at route registration).
// Body: {"approve": true|false}.
func (h *Handlers) HandleAdminReviewUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	updateID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "attestation_guide_disabled", "Attestation guide is not available")
		return
	}

	var body struct {
		Approve *bool `json:"approve"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Approve == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_approve", "approve field is required")
		return
	}

	update, err := h.service.ReviewCommunityUpdate(r.Context(), updateID, tenantID, user.UserID, *body.Approve)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "update_not_found", "Community update not found")
			return
		}

		slog.Error("attestationguide: admin review update failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "review_failed", "Failed to review community update")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": update})
}
