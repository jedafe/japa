package blog

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jedafe/japa/internal/content"
)

// AdminCreatePost handles POST /api/v1/admin/blog — creates a blog post
// with admin-level permissions (bypasses content.create permission check;
// the route itself is gated on admin.moderation).
func (h *Handlers) AdminCreatePost(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Title      string `json:"title"`
		Slug       string `json:"slug"`
		Body       string `json:"body"`
		BodyHtml   string `json:"body_html"` //nolint:revive // matches sqlc-generated field name
		Status     string `json:"status"`
		Visibility string `json:"visibility"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Status == "" {
		input.Status = "draft"
	}

	if input.Visibility == "" {
		input.Visibility = "public"
	}

	post, err := h.content.CreateContent(r.Context(), tenantID, content.CreateContentInput{
		AuthorID:     user.UserID,
		ContentType:  contentTypeBlogPost,
		Title:        input.Title,
		Slug:         input.Slug,
		Body:         content.SanitizeText(input.Body),
		BodyHtml:     content.SanitizeHTML(input.BodyHtml),
		SeoIndexable: true,
		Status:       input.Status,
		Visibility:   input.Visibility,
	})
	if err != nil {
		slog.Error("failed to create blog post (admin)", "error", err, "title", input.Title, "slug", input.Slug)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create blog post")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": post})
}

// AdminUpdatePost handles PATCH /api/v1/admin/blog/{id} — updates a blog
// post with admin-level permissions. The content service bypasses the
// edit-window check when the editor is not the original author.
func (h *Handlers) AdminUpdatePost(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Title      *string `json:"title"`
		Slug       *string `json:"slug"`
		Body       *string `json:"body"`
		BodyHtml   *string `json:"body_html"` //nolint:revive // matches sqlc-generated field name
		Status     *string `json:"status"`
		Visibility *string `json:"visibility"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	updateInput := content.UpdateContentInput{}
	if input.Title != nil {
		updateInput.Title = *input.Title
	}
	if input.Slug != nil {
		updateInput.Slug = *input.Slug
	}
	if input.Body != nil {
		updateInput.Body = content.SanitizeText(*input.Body)
	}
	if input.BodyHtml != nil {
		updateInput.BodyHtml = content.SanitizeHTML(*input.BodyHtml)
	}
	if input.Status != nil {
		updateInput.Status = *input.Status
	}
	if input.Visibility != nil {
		updateInput.Visibility = *input.Visibility
	}

	post, err := h.content.UpdateContent(r.Context(), id, tenantID, user.UserID, updateInput)
	if err != nil {
		slog.Error("failed to update blog post (admin)", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update blog post")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": post})
}
