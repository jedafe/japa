package tags

import (
	"encoding/json"
	"fmt"
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

// parseUUIDParam reads a UUID URL param, writing a 400 and returning the zero
// value on failure.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}
	return id, nil
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

// ListTags handles GET /api/v1/tags and GET /api/v1/admin/tags. [public + admin]
// The admin route uses the q parameter for search filtering.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")

	tags, err := h.service.AdminListTags(r.Context(), tenantID, q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tags")
		return
	}

	count, err := h.service.AdminCountTags(r.Context(), tenantID, q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count tags")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tags, "total": count})
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
// ListTagModerators handles GET /api/v1/admin/tags/:id/moderators.
func (h *Handlers) ListTagModerators(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	rows, err := h.service.ListTagModerators(r.Context(), tagID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tag moderators")
		return
	}

	moderators := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		moderators = append(moderators, map[string]any{
			"user_id":  row.UserID,
			"username": row.Username,
			"added_at": row.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": moderators})
}

// AddTagModerator handles POST /api/v1/admin/tags/:id/moderators (Section 49).
func (h *Handlers) AddTagModerator(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	var input struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var userID pgtype.UUID
	switch {
	case input.UserID != "":
		if err := userID.Scan(input.UserID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
			return
		}
	case input.Username != "":
		userID, err = h.service.GetUserIDByUsername(r.Context(), tenantID, input.Username)
		if err != nil {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "missing_fields", "user_id or username is required")
		return
	}

	row, err := h.service.AddTagModerator(r.Context(), tagID, userID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to add tag moderator")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"tag_id":   row.TagID,
		"user_id":  row.UserID,
		"added_at": row.CreatedAt,
	}})
}

// RemoveTagModerator handles DELETE /api/v1/admin/tags/:id/moderators/:userId (Section 49).
func (h *Handlers) RemoveTagModerator(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	tagID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "userId")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	if err := h.service.RemoveTagModerator(r.Context(), tagID, userID, tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove tag moderator")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"removed": true}})
}
