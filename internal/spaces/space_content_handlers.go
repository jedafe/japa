package spaces

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// ListSpaceContent handles GET /api/v1/spaces/{slug}/content — content
// belonging to this space, newest first. Supports ?cursor= and ?page_size=.
// When ?q= is supplied, results are filtered by title (case-insensitive
// substring) and offset-paginated via ?page_offset= (space search tab).
func (h *Handlers) ListSpaceContent(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	// BUG-002: the content filter gates on content visibility, not the
	// space's — without this, guests could read public posts inside
	// members-only/private spaces.
	if space.Visibility != VisibilityPublic && !h.canViewRestrictedSpace(r, space.ID) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return
	}

	limit := parsePageSize(r)
	filter := visibility.VisibilityFilterFromRequest(r)

	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		offset := parsePageSizeOffset(r)

		items, err := h.content.SearchContentsBySpace(r.Context(), space.ID, q, limit, offset, filter)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to search space content")
			return
		}

		if items == nil {
			items = []gen.SearchContentsBySpaceRow{}
		}

		// Resolve template literals in titles/excerpts (best-effort).
		if err := h.content.ResolveContentFields(r.Context(), &items, literals.NewRequestContext(r, "")); err != nil {
			slog.Warn("spaces: failed to resolve content literals", "error", err)
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})

		return
	}

	var cursor pgtype.Timestamptz

	if cs := r.URL.Query().Get("cursor"); cs != "" {
		// Accept RFC3339 (the format this endpoint itself returns in
		// created_at fields) first, then fall back to Postgres text format
		// ("2006-01-02 15:04:05.999999-07") for compatibility.
		if t, err := time.Parse(time.RFC3339Nano, cs); err == nil {
			cursor = pgtype.Timestamptz{Time: t, Valid: true}
		} else if err := cursor.Scan(cs); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cursor", "Invalid cursor timestamp")
			return
		}
	}

	items, err := h.content.ListContentsBySpace(r.Context(), space.ID, cursor, limit, filter)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list space content")
		return
	}

	if items == nil {
		items = []gen.ListContentsBySpaceRow{}
	}

	// Resolve template literals in titles/excerpts (best-effort).
	if err := h.content.ResolveContentFields(r.Context(), &items, literals.NewRequestContext(r, "")); err != nil {
		slog.Warn("spaces: failed to resolve content literals", "error", err)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ListPinned handles GET /api/v1/spaces/{slug}/pinned. Returns the pinned
// content rows (in pin order) so the hub's Pinned tab can render FeedCards
// directly — pin metadata alone is not renderable. Visibility-gated like
// the content list so guests never see members/private pins.
func (h *Handlers) ListPinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	// BUG-002: same space-visibility gate as ListSpaceContent.
	if space.Visibility != VisibilityPublic && !h.canViewRestrictedSpace(r, space.ID) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return
	}

	filter := visibility.VisibilityFilterFromRequest(r)

	pins, err := h.content.ListPinnedContentBySpace(r.Context(), space.ID, filter)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list pinned content")
		return
	}

	if pins == nil {
		pins = []gen.ListPinnedContentBySpaceRow{}
	}

	// Resolve template literals in pinned-card titles/excerpts (best-effort).
	if err := h.content.ResolveContentFields(r.Context(), &pins, literals.NewRequestContext(r, "")); err != nil {
		slog.Warn("spaces: failed to resolve pinned content literals", "error", err)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pins})
}

// AddPinned handles POST /api/v1/spaces/{slug}/pinned — pins a content item
// to the space (moderator action, gated by spaces.moderate at the router).
func (h *Handlers) AddPinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var input struct {
		ContentID string `json:"content_id"`
		Position  int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content_id", "content_id is required")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	pin, err := h.content.PinContent(r.Context(), space.ID, contentID, user.UserID, input.Position)
	if err != nil {
		// Domain failures carry a prefixed code — map them to 4xx instead of
		// a misleading 500.
		switch {
		case strings.HasPrefix(err.Error(), "content_not_in_space"):
			errpkg.WriteError(w, http.StatusBadRequest, "content_not_in_space", "This content is not assigned to this space")
		case strings.HasPrefix(err.Error(), "max_pinned_posts_reached"):
			errpkg.WriteError(w, http.StatusConflict, "max_pinned_posts_reached", "This space has reached its pinned post limit")
		default:
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to pin content")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": pin})
}

// RemovePinned handles DELETE /api/v1/spaces/{slug}/pinned/{content_id}.
func (h *Handlers) RemovePinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(chi.URLParam(r, "content_id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	if err := h.content.UnpinContent(r.Context(), space.ID, contentID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unpin content")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"unpinned": true}})
}

// ReorderPinned handles PATCH /api/v1/spaces/{id}/pinned/reorder.
func (h *Handlers) ReorderPinned(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var input struct {
		OrderedContentIDs []string `json:"ordered_content_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	contentIDs := make([]pgtype.UUID, 0, len(input.OrderedContentIDs))

	for _, contentIDStr := range input.OrderedContentIDs {
		var contentID pgtype.UUID
		if scanErr := contentID.Scan(contentIDStr); scanErr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content ID: "+contentIDStr)
			return
		}

		contentIDs = append(contentIDs, contentID)
	}

	if err := h.Service.ReorderPinned(r.Context(), spaceID, contentIDs); err != nil {
		slog.Error("reordering pinned content", "error", err, "space_id", spaceID.String())
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder pinned content")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
}
