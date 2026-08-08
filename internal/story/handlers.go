// Package story — handlers.go provides the HTTP handlers for the Community
// Story Wall routes (§79, ADR-087).
//
// Routes:
//
//	GET    /api/v1/stories                     — list ?origin=&dest=&visa=&profession=&sort=top|latest
//	GET    /api/v1/stories/:slug               — single story
//	POST   /api/v1/stories                     — submit story (auth, story.create)
//	PATCH  /api/v1/stories/:id                 — edit own story (auth, story.edit_own)
//	DELETE /api/v1/stories/:id                 — delete own story (auth, story.delete_own)
//	POST   /api/v1/admin/stories/:id/verify    — mark verified (auth, story.verify)
//	GET    /api/v1/admin/stories               — moderation queue (auth, story.verify)
//
// Ref: japaconnect-engineering/sections/79-community-story-wall.md
//      japaconnect-engineering/adr/ADR-087-community-story-wall.md
package story

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

// Handlers exposes the story wall endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates story handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as checklist/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("story: encode response", "error", err)
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
// Public handlers (list + single — anonymous browse per §79)
// ---------------------------------------------------------------------------

// HandleList handles GET /stories — list published stories with filters.
// Query params: ?origin=&dest=&visa=&profession=&sort=top|latest&page=&limit=
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	var originSpaceID pgtype.UUID
	if v := r.URL.Query().Get("origin"); v != "" {
		if err := originSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
			return
		}
	}
	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa")
	profession := r.URL.Query().Get("profession")
	sort := r.URL.Query().Get("sort")
	page, limit := parsePageParams(r)

	stories, total, err := h.service.List(r.Context(), tenantID, originSpaceID, destSpaceID, visaCategory, profession, sort, page, limit)
	if err != nil {
		slog.Error("story: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list stories")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": stories,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGetBySlug handles GET /stories/:slug — single published story.
func (h *Handlers) HandleGetBySlug(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Story slug is required")
		return
	}

	story, err := h.service.GetBySlug(r.Context(), slug, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}
		slog.Error("story: get by slug failed", "slug", slug, "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get story")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": story})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (submit, edit, delete — auth required)
// ---------------------------------------------------------------------------

// HandleSubmit handles POST /stories — submit a new story. Auth required
// (story.create permission checked at route registration).
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	var input SubmitStoryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}

	story, err := h.service.Submit(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		slog.Error("story: submit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit story")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": story})
}

// HandleEdit handles PATCH /stories/:id — edit own story. Auth required
// (story.edit_own permission checked at route registration).
func (h *Handlers) HandleEdit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	var input EditStoryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	story, err := h.service.EditOwn(r.Context(), contentID, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only edit your own stories")
			return
		}
		slog.Error("story: edit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "edit_failed", "Failed to edit story")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": story})
}

// HandleDelete handles DELETE /stories/:id — delete own story. Auth required
// (story.delete_own permission checked at route registration). Soft-delete
// per Rule 11.
func (h *Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	if err := h.service.DeleteOwn(r.Context(), contentID, tenantID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only delete your own stories")
			return
		}
		slog.Error("story: delete failed", "error", err)
		writeError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete story")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Admin handlers (verify + moderation queue — story.verify permission)
// ---------------------------------------------------------------------------

// HandleVerify handles POST /admin/stories/:id/verify — mark story as
// verified. Admin-only (story.verify permission checked at route registration).
func (h *Handlers) HandleVerify(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "story_unavailable", "Story service not configured")
		return
	}

	if err := h.service.Verify(r.Context(), contentID, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}
		slog.Error("story: verify failed", "error", err)
		writeError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify story")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"verified": true}})
}

// HandleAdminList handles GET /admin/stories — moderation queue listing all
// stories (any status). Admin-only (story.verify permission checked at route
// registration). Supports ?status=&verified=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "story_unavailable", "Story service not configured")
		return
	}

	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	var verifiedPtr *bool
	if v := r.URL.Query().Get("verified"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			verifiedPtr = &b
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	stories, total, err := h.service.AdminList(r.Context(), tenantID, status, q, verifiedPtr, page, limit)
	if err != nil {
		slog.Error("story: admin list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list stories")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": stories,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}
