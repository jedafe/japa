// Package blog — HTTP handlers for the Blog Extension system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package blog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// contentTypeBlogPost is the content_type used for blog posts — blog posts
// are regular content rows (Section 24 is an extension over Section 14's
// content system), not a separate table.
const contentTypeBlogPost = "blog_post"

// statusInReview is the content status used while a post awaits editorial review.
const statusInReview = "in_review"

// ContentService is the subset of content.Service the blog post handlers need.
type ContentService interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, input content.CreateContentInput) (gen.CreateContentRow, error)
	GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error)
	GetContentBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetContentBySlugRow, error)
	ListContentsByType(ctx context.Context, tenantID pgtype.UUID, contentType string, cursor pgtype.Timestamptz, limit int32) ([]gen.ListContentsByTypeRow, error)
	UpdateContentStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (gen.UpdateContentStatusRow, error)
}

// Handlers holds dependencies for blog HTTP handlers.
type Handlers struct {
	Service *Service
	content ContentService
	siteURL string
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service, contentService ContentService, siteURL string) *Handlers {
	return &Handlers{Service: service, content: contentService, siteURL: siteURL}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
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

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
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

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Series Handlers
// ---------------------------------------------------------------------------

// CreateSeries handles POST /api/v1/blog/series.
func (h *Handlers) CreateSeries(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Title        string  `json:"title"`
		Slug         string  `json:"slug"`
		Description  string  `json:"description"`
		CoverImageID *string `json:"cover_image_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	var coverImageID pgtype.UUID
	if input.CoverImageID != nil {
		if err := coverImageID.Scan(*input.CoverImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cover_image_id", "Invalid cover_image_id format")
			return
		}
	}

	series, err := h.Service.CreateSeries(r.Context(), tenantID, user.UserID, CreateSeriesInput{
		Title:        input.Title,
		Slug:         input.Slug,
		Description:  input.Description,
		CoverImageID: coverImageID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": series})
}

// ListSeries handles GET /api/v1/blog/series.
// Supports ?author_id=, ?page_size=, ?page_offset=.
func (h *Handlers) ListSeries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	authorIDStr := r.URL.Query().Get("author_id")
	if authorIDStr != "" {
		var authorID pgtype.UUID
		if err := authorID.Scan(authorIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_author_id", "Invalid author_id")
			return
		}

		series, err := h.Service.ListSeriesByAuthor(r.Context(), tenantID, authorID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": series})

		return
	}

	series, err := h.Service.ListSeriesByTenant(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": series})
}

// GetSeries handles GET /api/v1/blog/series/{slug}.
func (h *Handlers) GetSeries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	series, err := h.Service.GetSeriesBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": series})
}

// UpdateSeries handles PATCH /api/v1/blog/series/{id}.
func (h *Handlers) UpdateSeries(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid series ID")
		return
	}

	var input struct {
		Title        string  `json:"title"`
		Description  string  `json:"description"`
		CoverImageID *string `json:"cover_image_id"`
		Status       string  `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	var coverImageID pgtype.UUID
	if input.CoverImageID != nil {
		if err := coverImageID.Scan(*input.CoverImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cover_image_id", "Invalid cover_image_id format")
			return
		}
	}

	series, err := h.Service.UpdateSeries(r.Context(), id, tenantID, input.Title, input.Description, coverImageID, input.Status)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": series})
}

// DeleteSeries handles DELETE /api/v1/blog/series/{id}.
func (h *Handlers) DeleteSeries(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid series ID")
		return
	}

	if err := h.Service.DeleteSeries(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Series Posts Handlers
// ---------------------------------------------------------------------------

// AddPostToSeries handles POST /api/v1/blog/series/{id}/posts.
func (h *Handlers) AddPostToSeries(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	seriesID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_series_id", "Invalid series ID")
		return
	}

	var input struct {
		ContentID string `json:"content_id"`
		Position  int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentID == "" {
		writeError(w, http.StatusBadRequest, "missing_content_id", "content_id is required")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	if err := h.Service.AddPostToSeries(r.Context(), seriesID, contentID, input.Position); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "added"}})
}

// RemovePostFromSeries handles DELETE /api/v1/blog/series/{id}/posts/{postId}.
func (h *Handlers) RemovePostFromSeries(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	seriesID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_series_id", "Invalid series ID")
		return
	}

	contentID, err := parseUUIDParam(r, "postId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid post ID")
		return
	}

	if err := h.Service.RemovePostFromSeries(r.Context(), seriesID, contentID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ReorderSeriesPosts handles PATCH /api/v1/blog/series/{id}/posts/reorder.
func (h *Handlers) ReorderSeriesPosts(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	seriesID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_series_id", "Invalid series ID")
		return
	}

	var input struct {
		Posts []struct {
			ContentID string `json:"content_id"`
			Position  int32  `json:"position"`
		} `json:"posts"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Posts) == 0 {
		writeError(w, http.StatusBadRequest, "missing_posts", "At least one post required")
		return
	}

	posts := make([]struct {
		ContentID pgtype.UUID
		Position  int32
	}, len(input.Posts))
	for i, p := range input.Posts {
		if err := posts[i].ContentID.Scan(p.ContentID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id in posts array")
			return
		}

		posts[i].Position = p.Position
	}

	if err := h.Service.BulkReorderSeriesPosts(r.Context(), seriesID, posts); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ---------------------------------------------------------------------------
// Review Handlers
// ---------------------------------------------------------------------------

// CreateReview handles POST /api/v1/blog/reviews.
func (h *Handlers) CreateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentID string `json:"content_id"`
		Status    string `json:"status"`
		Feedback  string `json:"feedback"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentID == "" {
		writeError(w, http.StatusBadRequest, "missing_content_id", "content_id is required")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	review, err := h.Service.CreateReview(r.Context(), tenantID, user.UserID, CreateReviewInput{
		ContentID: contentID,
		Status:    input.Status,
		Feedback:  input.Feedback,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": review})
}

// ListReviews handles GET /api/v1/blog/reviews.
// Supports ?content_id=, ?pending=true, ?page_size=, ?page_offset=.
func (h *Handlers) ListReviews(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	contentIDStr := r.URL.Query().Get("content_id")
	if contentIDStr != "" {
		var contentID pgtype.UUID
		if err := contentID.Scan(contentIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
			return
		}

		reviews, err := h.Service.ListReviewsByContent(r.Context(), contentID, tenantID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": reviews})

		return
	}

	if r.URL.Query().Get("pending") == "true" {
		reviews, err := h.Service.ListPendingReviews(r.Context(), tenantID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": reviews})

		return
	}

	// Default: list all pending reviews.
	reviews, err := h.Service.ListPendingReviews(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": reviews})
}

// UpdateReview handles PATCH /api/v1/blog/reviews/{id}.
func (h *Handlers) UpdateReview(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	var input struct {
		Status   string `json:"status"`
		Feedback string `json:"feedback"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Status == "" {
		writeError(w, http.StatusBadRequest, "missing_status", "Status is required")
		return
	}

	review, err := h.Service.UpdateReviewStatus(r.Context(), id, tenantID, input.Status, input.Feedback)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": review})
}

// ---------------------------------------------------------------------------
// Featured Handlers
// ---------------------------------------------------------------------------

// FeaturePost handles POST /api/v1/blog/featured.
func (h *Handlers) FeaturePost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentID     string `json:"content_id"`
		FeaturedOrder int32  `json:"featured_order"`
		FeaturedUntil string `json:"featured_until"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentID == "" {
		writeError(w, http.StatusBadRequest, "missing_content_id", "content_id is required")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	var featuredUntil pgtype.Timestamptz
	if input.FeaturedUntil != "" {
		if err := featuredUntil.Scan(input.FeaturedUntil); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_featured_until", "Invalid featured_until date format")
			return
		}
	}

	featured, err := h.Service.FeaturePost(r.Context(), tenantID, FeaturePostInput{
		ContentID:     contentID,
		FeaturedOrder: input.FeaturedOrder,
		FeaturedUntil: featuredUntil,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": featured})
}

// UnfeaturePost handles DELETE /api/v1/blog/featured/{contentId}.
func (h *Handlers) UnfeaturePost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "contentId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content ID")
		return
	}

	if err := h.Service.UnfeaturePost(r.Context(), tenantID, contentID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ListFeatured handles GET /api/v1/blog/featured.
func (h *Handlers) ListFeatured(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	featured, err := h.Service.ListFeaturedPosts(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": featured})
}

// ReorderFeatured handles PATCH /api/v1/blog/featured/reorder.
func (h *Handlers) ReorderFeatured(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Items []struct {
			ID    string `json:"id"`
			Order int32  `json:"order"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Items) == 0 {
		writeError(w, http.StatusBadRequest, "missing_items", "At least one item required")
		return
	}

	for _, item := range input.Items {
		var id pgtype.UUID
		if err := id.Scan(item.ID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid featured ID in items array")
			return
		}

		if err := h.Service.UpdateFeaturedPostOrder(r.Context(), id, tenantID, item.Order); err != nil {
			handleServiceError(w, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ---------------------------------------------------------------------------
// RSS Feed Handler
// ---------------------------------------------------------------------------

// GetRSSFeed handles GET /api/v1/blog/feed.xml (public, no auth required).
func (h *Handlers) GetRSSFeed(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	posts, err := h.Service.ListPublishedPostsForRSS(r.Context(), tenantID, 50)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if err := generateRSSXML(w, posts, h.siteURL); err != nil {
		// Headers already sent, just log the error.
		return
	}
}

// ListPosts handles GET /api/v1/blog — published blog posts, newest first.
// Supports ?cursor= (RFC3339 timestamp) and ?page_size=. Blog posts are
// content rows with content_type=blog_post; this is a thin wrapper over
// content.Service.ListContentsByType with the type pinned.
func (h *Handlers) ListPosts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)

	var cursor pgtype.Timestamptz
	if cs := r.URL.Query().Get("cursor"); cs != "" {
		if err := cursor.Scan(cs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid cursor timestamp")
			return
		}
	}

	posts, err := h.content.ListContentsByType(r.Context(), tenantID, contentTypeBlogPost, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list blog posts")
		return
	}

	// This route is public (no auth middleware) — ListContentsByType has no
	// status filter, so drafts/archived posts must be filtered out here rather
	// than exposed to anonymous readers.
	published := make([]gen.ListContentsByTypeRow, 0, len(posts))

	for _, p := range posts {
		if p.Status == content.StatusPublished {
			published = append(published, p)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": published})
}

// GetPost handles GET /api/v1/blog/{slug} — a single published blog post.
// Public (no auth middleware); only content_type=blog_post and
// status=published are visible here — anything else 404s rather than
// disclosing the existence/content of drafts or unrelated content types.
func (h *Handlers) GetPost(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	post, err := h.content.GetContentBySlug(r.Context(), slug, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}

	if post.ContentType != contentTypeBlogPost || post.Status != content.StatusPublished {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": post})
}

// CreatePost handles POST /api/v1/blog — creates a blog post (content_type
// forced to blog_post regardless of any type field in the request body).
func (h *Handlers) CreatePost(w http.ResponseWriter, r *http.Request) {
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
		AuthorID:    user.UserID,
		ContentType: contentTypeBlogPost,
		Title:       input.Title,
		Slug:        input.Slug,
		Body:        content.SanitizeText(input.Body),
		BodyHtml:    content.SanitizeHTML(input.BodyHtml),
		Status:      input.Status,
		Visibility:  input.Visibility,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create blog post")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": post})
}

// SubmitForReview handles POST /api/v1/blog/{id}/submit-for-review —
// transitions the post's content status to in_review.
func (h *Handlers) SubmitForReview(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	existing, err := h.content.GetContent(r.Context(), contentID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}

	// A blog-specific action must not be usable to change the status of
	// unrelated content (forum threads, pages, etc.) reached by guessing IDs.
	if existing.ContentType != contentTypeBlogPost {
		writeError(w, http.StatusBadRequest, "wrong_content_type", "Not a blog post")
		return
	}

	post, err := h.content.UpdateContentStatus(r.Context(), contentID, tenantID, statusInReview)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to submit post for review")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": post})
}

// FeaturePostByID handles POST /api/v1/blog/{id}/feature — the catalog's
// path-based featuring, alongside the existing body-based POST /blog/featured.
func (h *Handlers) FeaturePostByID(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	existing, err := h.content.GetContent(r.Context(), contentID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}

	if existing.ContentType != contentTypeBlogPost {
		writeError(w, http.StatusBadRequest, "wrong_content_type", "Not a blog post")
		return
	}

	var input struct {
		FeaturedOrder int32  `json:"featured_order"`
		FeaturedUntil string `json:"featured_until"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var featuredUntil pgtype.Timestamptz
	if input.FeaturedUntil != "" {
		if err := featuredUntil.Scan(input.FeaturedUntil); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_featured_until", "Invalid featured_until date format")
			return
		}
	}

	featured, err := h.Service.FeaturePost(r.Context(), tenantID, FeaturePostInput{
		ContentID:     contentID,
		FeaturedOrder: input.FeaturedOrder,
		FeaturedUntil: featuredUntil,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": featured})
}
