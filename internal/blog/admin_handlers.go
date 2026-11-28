package blog

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// AdminCreatePost handles POST /api/v1/admin/blog — creates a blog post
// with admin-level permissions (bypasses content.create permission check;
// the route itself is gated on admin.moderation).
func (h *Handlers) AdminCreatePost(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Title      string  `json:"title"`
		Slug       string  `json:"slug"`
		Body       string  `json:"body"`
		BodyHtml   string  `json:"body_html"` //nolint:revive // matches sqlc-generated field name
		Status     string  `json:"status"`
		Visibility string  `json:"visibility"`
		SpaceID    *string `json:"space_id"`
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

	createInput := content.CreateContentInput{
		AuthorID:     user.UserID,
		ContentType:  contentTypeBlogPost,
		Title:        input.Title,
		Slug:         input.Slug,
		Body:         content.SanitizeText(input.Body),
		BodyHtml:     content.SanitizeHTML(input.BodyHtml),
		SeoIndexable: true,
		Status:       input.Status,
		Visibility:   input.Visibility,
	}

	// Publishing requires a primary space (§14) — accept one explicitly so an
	// admin can create a live post instead of only drafts.
	if input.SpaceID != nil && *input.SpaceID != "" {
		if err := createInput.SpaceID.Scan(*input.SpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
	}

	post, err := h.content.CreateContent(r.Context(), createInput)
	if err != nil {
		slog.Error("failed to create blog post (admin)", "error", err, "title", input.Title, "slug", input.Slug)
		handleContentError(w, err)

		return
	}

	// BUG-006: dispatch @mention notifications — fire-and-forget (BL-002,
	// Section 15). The public CreatePost does this; the admin path must
	// too, otherwise @mentions in admin-created posts are silently dropped.
	h.dispatchMentionNotifications(r.Context(), user.UserID, post.ID, post.SpaceID, input.Body)

	// Lifecycle parity with the public create path — search index, moderation
	// scan, link previews.
	h.contentWriteEffects(post.BodyHtml, post.ID)

	// BUG-009: run the full publish pipeline when an admin creates a post
	// directly as published — content_scores row, affiliate scan, feed
	// fan-out, and rss:regenerate (§24/§32).
	if post.Status == content.StatusPublished {
		h.publishEffects(r.Context(), post.ID, post.AuthorID, post.SpaceID)
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

	// Scope check: /admin/blog/{id} must only act on blog posts — the shared
	// contents table means a guessed non-blog ID would otherwise be mutated
	// through the blog surface (BUG-BL-005).
	existing, gErr := h.content.GetContent(r.Context(), id)
	if gErr != nil {
		handleServiceError(w, gErr)
		return
	}

	if existing.ContentType != contentTypeBlogPost {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Blog post not found")
		return
	}

	post, err := h.content.UpdateContent(r.Context(), id, user.UserID, updateInput, true)
	if err != nil {
		slog.Error("failed to update blog post (admin)", "error", err, "id", id)
		handleContentError(w, err)

		return
	}

	// Lifecycle parity with the public update path (BUG-BL-006): leaving
	// published purges RAG chunks; other writes re-embed. Search index,
	// moderation scan, and link previews fire on every write.
	prevStatus := existing.Status
	if prevStatus == content.StatusPublished && post.Status != content.StatusPublished {
		worker.PurgeRAGEntity(r.Context(), "content", id)
	} else {
		h.enqueueContentEmbedding(id)
	}
	h.contentWriteEffects(post.BodyHtml, id)

	// §32: a draft→published transition fans the post out to feeds.
	if prevStatus != content.StatusPublished && post.Status == content.StatusPublished {
		h.enqueueAffiliateLinkScan(id)
		h.enqueueFeedFanOut(r.Context(), id, post.AuthorID, post.SpaceID)
	}

	// BUG-009: enqueue rss:regenerate on draft→published transition. Ref: §24.
	// The reverse (published→non-published) also regenerates so the post drops
	// out of cached feeds without waiting for the hourly cron.
	if prevStatus != post.Status &&
		(prevStatus == content.StatusPublished || post.Status == content.StatusPublished) {
		h.enqueueRSSRegenerate()
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": post})
}
