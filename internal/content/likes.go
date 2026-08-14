package content

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

// ListContentLikedByUser returns published+public content the given user has
// reacted to (any reaction key), newest reaction first, paginated. Powers the
// public profile Likes tab — draft/private content is excluded by the query.
func (s *Service) ListContentLikedByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.ListContentLikedByUserRow, error) {
	rows, err := s.read.ListContentLikedByUser(ctx, gen.ListContentLikedByUserParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing content liked by user: %w", err)
	}

	return rows, nil
}

// ListLikedByUser handles GET /users/{id}/likes — the public profile Likes tab.
// Public-safe: only published+public content; non-active users are already
// 404'd by the requireActiveProfileUser gate on the route.
func (h *Handlers) ListLikedByUser(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32
	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n > 0 {
			offset = int32(n)
		}
	}

	rows, err := h.Service.ListContentLikedByUser(r.Context(), tenantID, userID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to list liked content")
		return
	}
	if rows == nil {
		rows = []gen.ListContentLikedByUserRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}
