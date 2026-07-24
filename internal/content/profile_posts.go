package content

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ListPublishedByAuthor returns a user's published+public content, newest first,
// paginated. Powers the public profile Posts tab (GET /users/{id}/content).
func (s *Service) ListPublishedByAuthor(ctx context.Context, tenantID, authorID pgtype.UUID, limit, offset int32) ([]gen.ListPublishedContentByAuthorRow, error) {
	rows, err := s.read.ListPublishedContentByAuthor(ctx, gen.ListPublishedContentByAuthorParams{
		TenantID:   tenantID,
		AuthorID:   authorID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing published content by author: %w", err)
	}

	return rows, nil
}

// ListByAuthor handles GET /users/{id}/content — the public profile Posts tab.
// Author-scoped: fixes the prior wiring to ListContent, which ignored {id} and
// returned all tenant content. Public-safe: only published+public content;
// non-active users are already 404'd by the requireActiveProfileUser gate.
func (h *Handlers) ListByAuthor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var authorID pgtype.UUID
	if err := authorID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32
	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n > 0 {
			offset = int32(n)
		}
	}

	items, err := h.Service.ListPublishedByAuthor(r.Context(), tenantID, authorID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_error", "Failed to list content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toPostCards(items)})
}

// PostCard is the lean public shape for the profile Posts tab. It omits the
// internal columns the raw gen row carries — tenant_id, the raw TipTap `body`
// (body_html is the rendered public form), field_values, content_hash,
// geo_space_id, canonical_url, status/visibility (always published+public
// here), is_locked, reshare_count, publish_at, updated_at.
type PostCard struct {
	ID              pgtype.UUID        `json:"id"`
	AuthorID        pgtype.UUID        `json:"author_id"`
	SpaceID         pgtype.UUID        `json:"space_id"`
	ContentType     string             `json:"content_type"`
	Title           string             `json:"title"`
	Slug            string             `json:"slug"`
	BodyHtml        string             `json:"body_html"`
	HeaderImageID   pgtype.UUID        `json:"header_image_id"`
	FeaturedImageID pgtype.UUID        `json:"featured_image_id"`
	ReadTimeMinutes int32              `json:"read_time_minutes"`
	ViewCount       int32              `json:"view_count"`
	CommentCount    int32              `json:"comment_count"`
	ShareCount      int32              `json:"share_count"`
	IsPinned        bool               `json:"is_pinned"`
	PublishedAt     pgtype.Timestamptz `json:"published_at"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
}

func toPostCards(items []gen.ListPublishedContentByAuthorRow) []PostCard {
	cards := make([]PostCard, len(items))
	for i := range items {
		c := &items[i]
		cards[i] = PostCard{
			ID:              c.ID,
			AuthorID:        c.AuthorID,
			SpaceID:         c.SpaceID,
			ContentType:     c.ContentType,
			Title:           c.Title,
			Slug:            c.Slug,
			BodyHtml:        c.BodyHtml,
			HeaderImageID:   c.HeaderImageID,
			FeaturedImageID: c.FeaturedImageID,
			ReadTimeMinutes: c.ReadTimeMinutes,
			ViewCount:       c.ViewCount,
			CommentCount:    c.CommentCount,
			ShareCount:      c.ShareCount,
			IsPinned:        c.IsPinned,
			PublishedAt:     c.PublishedAt,
			CreatedAt:       c.CreatedAt,
		}
	}
	return cards
}
