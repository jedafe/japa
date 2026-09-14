package blog

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// AdminCreatePost handles POST /api/v1/admin/blog — creates a blog post
// with admin-level permissions (bypasses content.create permission check;
// the route itself is gated on admin.moderation).
func (h *Handlers) AdminCreatePost(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok = h.requireTenant(w, r)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Status == "" {
		input.Status = "draft"
	}

	if input.Visibility == "" {
		input.Visibility = "public"
	}

	post, err := h.content.CreateContent(r.Context(), content.CreateContentInput{
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create blog post")

		return
	}

	// BUG-006: dispatch @mention notifications — fire-and-forget (BL-002,
	// Section 15). The public CreatePost does this; the admin path must
	// too, otherwise @mentions in admin-created posts are silently dropped.
	h.dispatchMentionNotifications(r.Context(), user.UserID, post.ID, post.SpaceID, input.Body)

	// BUG-009: enqueue rss:regenerate when an admin creates a post directly
	// as published. Ref: §24.
	if post.Status == content.StatusPublished {
		h.enqueueRSSRegenerate()
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": post})
}

// AdminUpdatePost handles PATCH /api/v1/admin/blog/{id} — updates a blog
// post with admin-level permissions. Admins bypass the edit-window check
// regardless of authorship.
func (h *Handlers) AdminUpdatePost(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
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

	// BUG-009: capture previous status to detect draft→published transition
	// for rss:regenerate enqueue. Ref: §24.
	prevStatus := ""
	if existing, gErr := h.content.GetContent(r.Context(), id); gErr == nil {
		prevStatus = existing.Status
	}

	post, err := h.content.UpdateContent(r.Context(), id, user.UserID, updateInput, true)
	if err != nil {
		slog.Error("failed to update blog post (admin)", "error", err, "id", id)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update blog post")

		return
	}

	// BUG-009: enqueue rss:regenerate on draft→published transition. Ref: §24.
	if prevStatus != content.StatusPublished && post.Status == content.StatusPublished {
		h.enqueueRSSRegenerate()
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": post})
}
