// Package story — handlers.go provides the HTTP handlers for the Community
// Story Wall routes (§79, ADR-087).
// // Routes:
// //	GET /api/v1/stories — list ?origin=&dest=&visa=&profession=&sort=top|latest
// GET /api/v1/stories/:slug — single story
// POST /api/v1/stories — submit story (auth, story.create)
// PATCH /api/v1/stories/:id — edit own story (auth, story.edit_own)
// DELETE /api/v1/stories/:id — delete own story (auth, story.delete_own)
// POST /api/v1/admin/stories/:id/verify — mark verified (auth, story.verify)
// GET /api/v1/admin/stories — moderation queue (auth, story.verify)
// // Ref: japaconnect-engineering/sections/79-community-story-wall.md
// //	japaconnect-engineering/adr/ADR-087-community-story-wall.md
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
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
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

// hasPermission checks if the authenticated user has a specific permission
// via the RBAC resolver stored in the request context by the middleware.
// Returns false if the resolver is not in the context.
func hasPermission(r *http.Request, user *auth.AuthenticatedUser, permKey string) bool {
	resolver, ok := r.Context().Value(rbac.ResolverKey).(*rbac.RBAC)
	if !ok || resolver == nil {
		return false
	}

	return resolver.HasPermission(r.Context(), user.UserID, user.TenantID, permKey)
}

// storyVisibility derives the §14 visibility rights for story list/detail
// queries: authenticated users see members-visible stories; moderators
// (super-admin, story.verify, content.edit_any, admin.moderation) also see
// private ones. Authors always see their own rows — enforced in SQL.
func storyVisibility(r *http.Request) (requesterID pgtype.UUID, canSeeMembers, canSeePrivate bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		return pgtype.UUID{}, false, false
	}

	requesterID = user.UserID
	canSeeMembers = true

	resolver, ok := r.Context().Value(rbac.ResolverKey).(*rbac.RBAC)
	if !ok || resolver == nil {
		return requesterID, canSeeMembers, false
	}

	if resolver.IsSuperAdmin(r.Context(), user.UserID, user.TenantID) ||
		resolver.HasPermission(r.Context(), user.UserID, user.TenantID, "story.verify") ||
		resolver.HasPermission(r.Context(), user.UserID, user.TenantID, "content.edit_any") ||
		resolver.HasPermission(r.Context(), user.UserID, user.TenantID, "admin.moderation") {
		canSeePrivate = true
	}

	return requesterID, canSeeMembers, canSeePrivate
}

// parsePageParams reads page_size and page_offset query params per
// conventions/api-response-format.md offset pagination convention.
// Defaults: page_size=20, page_offset=0. Max page_size=100.
func parsePageParams(r *http.Request) (pageSize, pageOffset int32) {
	pageSize = 20
	pageOffset = 0

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			pageSize = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			pageOffset = int32(n) // #nosec
		}
	}

	return pageSize, pageOffset
}

// ---------------------------------------------------------------------------
// Public handlers (list + single — anonymous browse per §79)
// ---------------------------------------------------------------------------

// HandleList handles GET /stories — list published stories with filters.
// Query params: ?origin=&dest=&visa=&profession=&sort=top|latest&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	var originSpaceID pgtype.UUID
	if v := r.URL.Query().Get("origin"); v != "" {
		if err := originSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
			return
		}
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	visaCategory := r.URL.Query().Get("visa")
	profession := r.URL.Query().Get("profession")
	sort := r.URL.Query().Get("sort")
	pageSize, pageOffset := parsePageParams(r)

	requesterID, canSeeMembers, canSeePrivate := storyVisibility(r)

	stories, total, err := h.service.List(r.Context(), tenantID, originSpaceID, destSpaceID, visaCategory, profession, sort, pageSize, pageOffset, requesterID, canSeeMembers, canSeePrivate)
	if err != nil {
		slog.Error("story: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list stories")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": stories,
		"pagination": map[string]any{
			"page_size":   pageSize,
			"page_offset": pageOffset,
			"total":       total,
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
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Story slug is required")
		return
	}

	story, err := h.service.GetBySlug(r.Context(), slug, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}

		slog.Error("story: get by slug failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get story")

		return
	}

	// §79 status gating: non-published stories (in_review, drafts) are visible
	// only to their author and to moderators — authors need a stable URL to
	// preview their submitted story, and an edited published story re-enters
	// the moderation queue while remaining reachable for its author.
	// §14 visibility gating: members-visible stories require authentication;
	// private stories are author- or moderator-only. Guests and unauthorized
	// viewers get a 404 rather than a 403 so existence is not revealed.
	user := auth.FromContext(r.Context())
	_, _, canSeePrivate := storyVisibility(r)
	isAuthor := user != nil && story.AuthorID != "" && user.UserID.String() == story.AuthorID

	if story.Status != "published" {
		if !isAuthor && !canSeePrivate {
			errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}
	} else if story.Visibility == "members" && user == nil {
		errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
		return
	} else if story.Visibility == "private" && !isAuthor && !canSeePrivate {
		errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": story})
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
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	var input SubmitStoryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}

	story, err := h.service.Submit(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}

		slog.Error("story: submit failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit story")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": story})
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
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	var input EditStoryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	story, err := h.service.EditOwn(r.Context(), contentID, tenantID, user.UserID, input, hasPermission(r, user, "story.edit_any"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only edit your own stories")
			return
		}

		if errors.Is(err, ErrValidation) {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}

		slog.Error("story: edit failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "edit_failed", "Failed to edit story")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": story})
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
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	if err := h.service.DeleteOwn(r.Context(), contentID, tenantID, user.UserID, hasPermission(r, user, "story.delete_any")); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only delete your own stories")
			return
		}

		slog.Error("story: delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete story")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Admin handlers (verify + moderation queue — story.verify permission)
// ---------------------------------------------------------------------------

// HandleVerify handles POST /admin/stories/:id/verify — mark story as
// verified. Admin-only (story.verify permission checked at route registration).
// Sets hitl_reviewed_by and hitl_reviewed_at per ADR-088.
func (h *Handlers) HandleVerify(w http.ResponseWriter, r *http.Request) {
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "story_unavailable", "Story service not configured")
		return
	}

	if !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	if err := h.service.Verify(r.Context(), contentID, tenantID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}

		slog.Error("story: verify failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify story")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"verified": true}})
}

// HandleUnverify handles POST /admin/stories/:id/unverify — mark story as
// unverified. Admin-only (story.verify permission checked at route registration).
// Clears the verified flag and the HITL review audit trail.
func (h *Handlers) HandleUnverify(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "story_unavailable", "Story service not configured")
		return
	}

	if !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	if err := h.service.Unverify(r.Context(), contentID, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}

		slog.Error("story: unverify failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "unverify_failed", "Failed to unverify story")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"verified": false}})
}

// HandleAdminList handles GET /admin/stories — moderation queue listing all
// stories (any status). Admin-only (story.verify permission checked at route
// registration). Supports ?status=&verified=&q=&sort=latest|oldest|title&page_size=&page_offset=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "story_unavailable", "Story service not configured")
		return
	}

	if !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var verifiedPtr *bool

	if v := r.URL.Query().Get("verified"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			verifiedPtr = &b
		}
	}

	pageSize, pageOffset := parsePageParams(r)
	if pageSize > 200 {
		pageSize = 200
	}

	stories, total, err := h.service.AdminList(r.Context(), tenantID, status, q, verifiedPtr, sort, destSpaceID, pageSize, pageOffset)
	if err != nil {
		slog.Error("story: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list stories")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": stories,
		"pagination": map[string]any{
			"page_size":   pageSize,
			"page_offset": pageOffset,
			"total":       total,
		},
	})
}

// HandleAdminDelete handles DELETE /admin/stories/:id — admin deletion of any
// story (story.delete_any permission checked at route registration). Soft-deletes
// per Rule 11.
func (h *Handlers) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "story_unavailable", "Story service not configured")
		return
	}

	if !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "story_disabled", "Story wall is not available")
		return
	}

	// bypassOwnership=true: admin with story.delete_any can delete any story.
	if err := h.service.DeleteOwn(r.Context(), contentID, tenantID, user.UserID, true); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "story_not_found", "Story not found")
			return
		}

		slog.Error("story: admin delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete story")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}
