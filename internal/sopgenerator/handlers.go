// Package sopgenerator — handlers.go provides the HTTP handlers for the
// AI SOP Generator routes (§110, ADR-121).
//
// Routes:
//
//	GET    /api/v1/sop/templates                    — ?dest=&purpose= (public)
//	GET    /api/v1/sop/samples                      — ?dest=&level=&field= (public)
//	GET    /api/v1/sop/samples/:id                  — sample detail (public)
//	GET    /api/v1/sop/quota                        — remaining generations (auth)
//	POST   /api/v1/sop/generate                     — enqueue sop:generate → {draft_id} (auth, 202)
//	GET    /api/v1/sop/drafts                       — user's drafts (auth)
//	GET    /api/v1/sop/drafts/:id                   — draft detail; poll result (auth owner)
//	PATCH  /api/v1/sop/drafts/:id                   — edit draft (auth owner)
//	DELETE /api/v1/sop/drafts/:id                   — soft-delete draft (auth owner)
//	GET    /api/v1/admin/sop/templates              — manage templates (admin)
//	POST   /api/v1/admin/sop/templates/generate     — Content Agent draft placeholder (admin)
//	PATCH  /api/v1/admin/sop/samples/:id/approve    — publish sample (admin, HITL)
//
// Ref: japaconnect-engineering/sections/110-sop-generator.md
//
//	japaconnect-engineering/adr/ADR-121-sop-generator.md
package sopgenerator

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

// Handlers exposes the SOP generator endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates SOP handlers bound to the service.
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
	errpkg.WriteError(w, http.StatusServiceUnavailable, "sop_generator_unavailable", "SOP generator not configured")
}

func serviceErr(w http.ResponseWriter, r *http.Request, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", notFoundMsg)
	case errors.Is(err, ErrNotOwner):
		errpkg.WriteError(w, http.StatusForbidden, "not_owner", "Draft belongs to a different user")
	case errors.Is(err, ErrQuotaExceeded):
		errpkg.WriteError(w, http.StatusTooManyRequests, "quota_exceeded", "Monthly generation quota reached")
	default:
		slog.Error("sopgenerator: request failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "request_failed", "Request failed")
	}
}

// HandleListTemplates handles GET /sop/templates. Public.
func (h *Handlers) HandleListTemplates(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	var dest pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := dest.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_dest", "Invalid destination space ID")
			return
		}
	}

	templates, err := h.service.ListTemplates(r.Context(), dest, r.URL.Query().Get("purpose"))
	if err != nil {
		slog.Error("sopgenerator: list templates failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list templates")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": templates})
}

// HandleListSamples handles GET /sop/samples. Public.
func (h *Handlers) HandleListSamples(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	var dest pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := dest.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_dest", "Invalid destination space ID")
			return
		}
	}

	page, limit := parsePageParams(r)
	samples, total, err := h.service.ListSamples(r.Context(), dest,
		r.URL.Query().Get("level"), r.URL.Query().Get("field"), page, limit)
	if err != nil {
		slog.Error("sopgenerator: list samples failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list samples")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       samples,
		"pagination": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleGetSample handles GET /sop/samples/:id. Public.
func (h *Handlers) HandleGetSample(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	sample, err := h.service.GetSample(r.Context(), id)
	if err != nil {
		serviceErr(w, r, err, "Sample not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sample})
}

// HandleQuota handles GET /sop/quota — remaining monthly generations.
// Auth required.
func (h *Handlers) HandleQuota(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	q, err := h.service.Quota(r.Context(), user.UserID)
	if err != nil {
		slog.Error("sopgenerator: quota failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "quota_failed", "Failed to load quota")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": q})
}

// HandleGenerate handles POST /sop/generate — quota check, queued draft,
// sop:generate enqueue. Returns 202 with {draft_id}. Auth required.
func (h *Handlers) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	var input GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	draftID, err := h.service.Generate(r.Context(), user.UserID, input)
	if err != nil {
		serviceErr(w, r, err, "")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"draft_id": draftID, "status": DraftQueued}})
}

// HandleListDrafts handles GET /sop/drafts. Auth required.
func (h *Handlers) HandleListDrafts(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	page, limit := parsePageParams(r)
	drafts, total, err := h.service.ListDrafts(r.Context(), user.UserID, page, limit)
	if err != nil {
		slog.Error("sopgenerator: list drafts failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list drafts")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       drafts,
		"pagination": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleGetDraft handles GET /sop/drafts/:id — owner-only; the client
// polls this for the async generation result.
func (h *Handlers) HandleGetDraft(w http.ResponseWriter, r *http.Request) {
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

	draft, err := h.service.GetDraft(r.Context(), id, user.UserID)
	if err != nil {
		serviceErr(w, r, err, "Draft not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": draft})
}

// HandleUpdateDraft handles PATCH /sop/drafts/:id — owner-only edit.
func (h *Handlers) HandleUpdateDraft(w http.ResponseWriter, r *http.Request) {
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

	var input UpdateDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdateDraft(r.Context(), id, user.UserID, input); err != nil {
		serviceErr(w, r, err, "Draft not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id")}})
}

// HandleDeleteDraft handles DELETE /sop/drafts/:id — owner-only soft
// delete (Rule 11).
func (h *Handlers) HandleDeleteDraft(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.DeleteDraft(r.Context(), id, user.UserID); err != nil {
		serviceErr(w, r, err, "Draft not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleAdminListTemplates handles GET /admin/sop/templates (any status).
func (h *Handlers) HandleAdminListTemplates(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	page, limit := parsePageParams(r)
	templates, total, err := h.service.AdminListTemplates(r.Context(), page, limit)
	if err != nil {
		slog.Error("sopgenerator: admin list templates failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list templates")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       templates,
		"pagination": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleAdminGenerateTemplate handles POST /admin/sop/templates/generate —
// Content Agent draft placeholder. Returns 202.
func (h *Handlers) HandleAdminGenerateTemplate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"status": "queued"}})
}

// HandleAdminApproveSample handles PATCH /admin/sop/samples/:id/approve —
// HITL publish.
func (h *Handlers) HandleAdminApproveSample(w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		ReviewNotes string `json:"review_notes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.service.ApproveSample(r.Context(), id, user.UserID, body.ReviewNotes); err != nil {
		serviceErr(w, r, err, "Sample not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": StatusPublished}})
}
