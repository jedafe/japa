package tags

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for tag HTTP handlers. Composes the existing
// content and follows services directly (no import cycle: neither imports
// this package) rather than through an adapter interface.
type Handlers struct {
	service *Service
	content *content.Service
	follows *follows.Service
}

// NewHandlers creates Handlers with the given services.
func NewHandlers(service *Service, contentService *content.Service, followsService *follows.Service) *Handlers {
	return &Handlers{service: service, content: contentService, follows: followsService}
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// resolveTag looks up a tag by slug, writing a 404 and returning ok=false if
// it doesn't exist. Shared by every handler below GetTag.
func (h *Handlers) resolveTag(w http.ResponseWriter, r *http.Request, tenantID pgtype.UUID) (gen.Tag, bool) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Tag slug is required")
		return gen.Tag{}, false
	}

	tag, err := h.service.GetTagBySlug(r.Context(), tenantID, slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Tag not found")
		return gen.Tag{}, false
	}

	return tag, true
}

// ListTags handles GET /api/v1/tags. [public]
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tags, err := h.service.ListTags(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tags")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tags})
}

// GetTag handles GET /api/v1/tags/{slug}. [public]
func (h *Handlers) GetTag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r, tenantID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tag})
}

// GetTagContent handles GET /api/v1/tags/{slug}/content — published content
// tagged with this tag, newest first. Supports ?cursor= (RFC3339 timestamp)
// and ?page_size= for pagination. [public]
func (h *Handlers) GetTagContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r, tenantID)
	if !ok {
		return
	}

	var cursor pgtype.Timestamptz
	if c := r.URL.Query().Get("cursor"); c != "" {
		t, err := time.Parse(time.RFC3339, c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor must be an RFC3339 timestamp")
			return
		}

		cursor = pgtype.Timestamptz{Time: t, Valid: true}
	}

	limit := parsePageSize(r)

	items, err := h.content.ListContentByTag(r.Context(), tenantID, tag.ID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tagged content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ToggleTagFollow handles POST/DELETE /api/v1/tags/{slug}/follow.
func (h *Handlers) ToggleTagFollow(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r, tenantID)
	if !ok {
		return
	}

	following, err := h.follows.ToggleFollow(r.Context(), tenantID, user.UserID, follows.EntityTypeTag, tag.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to toggle tag follow")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"following": following}})
}

// GetTagFollowers handles GET /api/v1/tags/{slug}/followers.
func (h *Handlers) GetTagFollowers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tag, ok := h.resolveTag(w, r, tenantID)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	followers, err := h.follows.GetFollowers(r.Context(), tenantID, follows.EntityTypeTag, tag.ID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get tag followers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": followers})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v <= 0 || v > 100 {
		return defaultPageSize
	}

	return int32(v)
}

// parsePageOffset extracts the page_offset query parameter.
func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < 0 {
		return 0
	}

	return int32(v)
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// CreateTag handles POST /api/v1/admin/tags.
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	slug := input.Slug
	if slug == "" {
		slug = content.Slugify(input.Name)
	}

	if input.Description != nil {
		sanitized := content.SanitizeText(*input.Description)
		input.Description = &sanitized
	}

	tag, err := h.service.CreateTag(r.Context(), tenantID, CreateTagInput{
		Name:        content.SanitizeText(input.Name),
		Slug:        slug,
		Description: input.Description,
		Color:       input.Color,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create tag")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": tag})
}

// UpdateTag handles PATCH /api/v1/admin/tags/{id}.
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.Description != nil {
		sanitized := content.SanitizeText(*input.Description)
		input.Description = &sanitized
	}

	tag, err := h.service.UpdateTag(r.Context(), id, tenantID, UpdateTagInput{
		Name:        content.SanitizeText(input.Name),
		Description: input.Description,
		Color:       input.Color,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update tag")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tag})
}

// DeleteTag handles DELETE /api/v1/admin/tags/{id}.
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.service.DeleteTag(r.Context(), id, tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete tag")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// AddTagModerator handles POST /admin/tags/:id/moderators (P5.6 — Task 7).
// NOTE: No tag_moderators table exists yet — returns accepted but does not persist.
func (h *Handlers) AddTagModerator(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}
	var input struct {
		UserID string `json:"user_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{
		"tag_id": id.String(), "user_id": input.UserID, "status": "pending_migration",
	}})
}

// RemoveTagModerator handles DELETE /admin/tags/:id/moderators/:userId (P5.6 — Task 7).
// NOTE: No tag_moderators table exists yet — returns accepted but does not persist.
func (h *Handlers) RemoveTagModerator(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNoContent, nil)
}
