package comments

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/httpx"
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32
	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n > 0 {
			offset = int32(n)
		}
	}

	items, total, err := h.Service.ListByAuthor(r.Context(), tenantID, authorID, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to list replies")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"total": total, "page_size": limit, "page_offset": offset},
	})
}
