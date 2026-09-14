// Package achievement — handlers.go provides the HTTP handlers for the
// Community Achievement Feed routes (§89, ADR-097).
//
// Routes:
//
//	GET    /api/v1/achievements                  — list ?dest=&milestone=&sort=top|latest
//	GET    /api/v1/achievements/:slug            — single achievement
//	POST   /api/v1/achievements                  — submit achievement (auth, achievement.create)
//	PATCH  /api/v1/achievements/:id              — edit own achievement (auth, achievement.edit_own)
//	DELETE /api/v1/achievements/:id              — delete own achievement (auth, achievement.delete_own)
//	GET    /api/v1/spaces/:slug/achievements     — Space widget (public)
//	GET    /api/v1/admin/achievements            — moderation queue (auth, achievement.moderate)
//
// Ref: japaconnect-engineering/sections/89-community-achievement-feed.md
//
//	japaconnect-engineering/adr/ADR-097-community-achievement-feed.md
package achievement

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

// Handlers exposes the achievement feed endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates achievement handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
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
// Public handlers (list + single — anonymous browse per §89)
// ---------------------------------------------------------------------------

// HandleList handles GET /achievements — list published achievements with
// filters. Query params: ?dest=&milestone=&sort=top|latest&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "achievement_disabled", "Achievement feed is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	milestoneType := r.URL.Query().Get("milestone")

	sort := r.URL.Query().Get("sort")
	if sort != "" && sort != "latest" && sort != "top" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_sort", "Sort must be 'latest' or 'top'")
		return
	}

	page, limit := parsePageParams(r)

	achievements, total, err := h.service.List(r.Context(), destSpaceID, milestoneType, sort, page, limit)
	if err != nil {
		slog.Error("achievement: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list achievements")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": achievements,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGet handles GET /achievements/:slug — single achievement. Only
// published achievements are visible to anonymous viewers; an unpublished
// (admin-moderated) achievement is visible only to its own author.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "achievement_disabled", "Achievement feed is not available")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Achievement slug is required")
		return
	}

	// Pass the authenticated viewer (if any) so authors can view their own
	// unpublished achievements; anonymous viewers see published only.
	var viewerID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		viewerID = u.UserID
	}

	achievement, err := h.service.GetBySlugWithReactions(r.Context(), slug, tenantID, viewerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "achievement_not_found", "Achievement not found")
			return
		}

		slog.Error("achievement: get by slug failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get achievement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": achievement})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (submit, edit, delete — auth required)
// ---------------------------------------------------------------------------

// HandleSubmit handles POST /achievements — submit a new achievement. Auth
// required (achievement.create permission checked at route registration).
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "achievement_disabled", "Achievement feed is not available")
		return
	}

	var input SubmitInput
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

	if input.MilestoneType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_milestone_type", "Milestone type is required")
		return
	}

	achievement, err := h.service.Submit(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrMissingMilestone) {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_milestone_type", "Milestone type is required")
			return
		}

		if errors.Is(err, ErrInvalidMilestone) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_milestone_type", "Milestone type must be one of: visa_approved, job_offer, university_accepted, arrived, one_year, citizenship, other")
			return
		}

		if errors.Is(err, ErrInvalidDestination) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_destination_space", "Destination must be a country-level space")
			return
		}

		slog.Error("achievement: submit failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit achievement")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": achievement})
}

// HandleEdit handles PATCH /achievements/:id — edit own achievement. Auth
// required (achievement.edit_own permission checked at route registration).
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

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "achievement_disabled", "Achievement feed is not available")
		return
	}

	var input EditInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	achievement, err := h.service.EditOwn(r.Context(), contentID, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "achievement_not_found", "Achievement not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only edit your own achievements")
			return
		}

		if errors.Is(err, ErrInvalidMilestone) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_milestone_type", "Milestone type must be one of: visa_approved, job_offer, university_accepted, arrived, one_year, citizenship, other")
			return
		}

		if errors.Is(err, ErrInvalidDestination) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_destination_space", "Destination must be a country-level space")
			return
		}

		slog.Error("achievement: edit failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "edit_failed", "Failed to edit achievement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": achievement})
}

// HandleDelete handles DELETE /achievements/:id — delete own achievement. Auth
// required (achievement.delete_own permission checked at route registration).
// Soft-delete per Rule 11.
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

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "achievement_disabled", "Achievement feed is not available")
		return
	}

	if err := h.service.DeleteOwn(r.Context(), contentID, tenantID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "achievement_not_found", "Achievement not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only delete your own achievements")
			return
		}

		slog.Error("achievement: delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete achievement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Space widget handler (public)
// ---------------------------------------------------------------------------

// HandleListBySpace handles GET /spaces/:slug/achievements — published
// achievements filed in the given Space, for the Space widget.
func (h *Handlers) HandleListBySpace(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "achievement_disabled", "Achievement feed is not available")
		return
	}

	spaceSlug := chi.URLParam(r, "slug")
	if spaceSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Space slug is required")
		return
	}

	limit := int32(10)

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = int32(n) // #nosec
		}
	}

	achievements, err := h.service.ListBySpace(r.Context(), spaceSlug, limit)
	if err != nil {
		slog.Error("achievement: list by space failed", "space_slug", spaceSlug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_by_space_failed", "Failed to list achievements for space")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": achievements})
}

// ---------------------------------------------------------------------------
// Admin handler (moderation queue — achievement.moderate permission)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/achievements — moderation queue listing
// all achievements (any status). Admin-only (achievement.moderate permission
// checked at route registration). Supports ?status=&milestone=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "achievement_unavailable", "Achievement service not configured")
		return
	}

	status := r.URL.Query().Get("status")
	milestoneType := r.URL.Query().Get("milestone")
	q := r.URL.Query().Get("q")

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	achievements, total, err := h.service.AdminList(r.Context(), status, milestoneType, q, page, limit)
	if err != nil {
		slog.Error("achievement: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list achievements")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": achievements,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminSetStatus handles PATCH /admin/achievements/:id/status —
// publish or unpublish an achievement. Admin-only (achievement.moderate
// permission checked at route registration).
func (h *Handlers) HandleAdminSetStatus(w http.ResponseWriter, r *http.Request) {
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

	var input struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "Status is required")
		return
	}

	achievement, err := h.service.AdminSetStatus(r.Context(), contentID, tenantID, input.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "achievement_not_found", "Achievement not found")
			return
		}

		slog.Error("achievement: admin set status failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_set_status_failed", "Failed to update achievement status")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": achievement})
}

// HandleAdminDelete handles DELETE /admin/achievements/:id — soft-delete
// an achievement from the moderation queue. Admin-only (achievement.moderate
// permission checked at route registration). Soft-delete per Rule 11.
func (h *Handlers) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.AdminDelete(r.Context(), contentID, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "achievement_not_found", "Achievement not found")
			return
		}

		slog.Error("achievement: admin delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_delete_failed", "Failed to delete achievement")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}
