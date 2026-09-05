package comments

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ListByAuthor returns the user's comments on published+public content (with
// parent-content title/slug for context) plus the total count, paginated.
// Powers the public profile Replies tab. Public-safe: comments on draft/private
// content and soft-deleted comments are excluded by the query.
func (s *Service) ListByAuthor(ctx context.Context, tenantID, authorID pgtype.UUID, limit, offset int32) ([]gen.ListCommentsByAuthorRow, int32, error) {
	rows, err := s.read.ListCommentsByAuthor(ctx, gen.ListCommentsByAuthorParams{
		TenantID:     tenantID,
		AuthorUserID: authorID,
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing comments by author: %w", err)
	}

	total, err := s.read.CountCommentsByAuthor(ctx, gen.CountCommentsByAuthorParams{
		TenantID:     tenantID,
		AuthorUserID: authorID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("counting comments by author: %w", err)
	}

	return rows, total, nil
}

// ListByAuthor handles GET /users/{id}/replies — the public profile Replies tab.
// Public-safe: only replies on published+public content; non-active users are
// already 404'd by the requireActiveProfileUser gate on the route.
func (h *Handlers) ListByAuthor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var authorID pgtype.UUID
	if err := authorID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32

	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n > 0 {
			offset = int32(n) // #nosec
		}
	}

	items, total, err := h.Service.ListByAuthor(r.Context(), tenantID, authorID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to list replies")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": toReplyCards(items),
		"meta": map[string]any{"total": total, "page_size": limit, "page_offset": offset},
	})
}

// ReplyCard is the lean public shape for the profile Replies tab: the reply plus
// the parent content's title/slug for context. It omits the internal tenant_id
// the raw gen row carries, so a public endpoint never echoes it.
type ReplyCard struct {
	ID           pgtype.UUID        `json:"id"`
	ContentID    pgtype.UUID        `json:"content_id"`
	ParentID     pgtype.UUID        `json:"parent_id"`
	Body         []byte             `json:"body"` // TipTap JSON (base64 in JSON transport)
	VoteScore    int32              `json:"vote_score"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
	ContentTitle string             `json:"content_title"`
	ContentSlug  string             `json:"content_slug"`
}

func toReplyCards(items []gen.ListCommentsByAuthorRow) []ReplyCard {
	cards := make([]ReplyCard, len(items))

	for i := range items {
		c := &items[i]
		cards[i] = ReplyCard{
			ID:           c.ID,
			ContentID:    c.ContentID,
			ParentID:     c.ParentID,
			Body:         c.Body,
			VoteScore:    c.VoteScore,
			CreatedAt:    c.CreatedAt,
			ContentTitle: c.ContentTitle,
			ContentSlug:  c.ContentSlug,
		}
	}

	return cards
}
