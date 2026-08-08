package spaces

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)

	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		offset := parsePageSizeOffset(r)
		items, err := h.content.SearchContentsBySpace(r.Context(), tenantID, space.ID, q, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to search space content")
			return
		}
		if items == nil {
			items = []gen.SearchContentsBySpaceRow{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items})
		return
	}

	var cursor pgtype.Timestamptz
	if cs := r.URL.Query().Get("cursor"); cs != "" {
		if err := cursor.Scan(cs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid cursor timestamp")
			return
		}
	}

	items, err := h.content.ListContentsBySpace(r.Context(), tenantID, space.ID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list space content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ListPinned handles GET /api/v1/spaces/{slug}/pinned.
func (h *Handlers) ListPinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	pins, err := h.content.ListPinnedContent(r.Context(), space.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list pinned content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": pins})
}

// AddPinned handles POST /api/v1/spaces/{slug}/pinned — pins a content item
// to the space (moderator action, gated by spaces.moderate at the router).
func (h *Handlers) AddPinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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

	pin, err := h.content.PinContent(r.Context(), tenantID, space.ID, contentID, user.UserID, input.Position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to pin content")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": pin})
}

// RemovePinned handles DELETE /api/v1/spaces/{slug}/pinned/{content_id}.
func (h *Handlers) RemovePinned(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(chi.URLParam(r, "content_id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	if err := h.content.UnpinContent(r.Context(), space.ID, contentID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unpin content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"unpinned": true}})
}

// ReorderPinned handles POST /admin/spaces/:id/pinned/reorder.
func (h *Handlers) ReorderPinned(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var input struct {
		Order []string `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	contentIDs := make([]pgtype.UUID, 0, len(input.Order))
	for _, contentIDStr := range input.Order {
		var contentID pgtype.UUID
		if scanErr := contentID.Scan(contentIDStr); scanErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content ID: "+contentIDStr)
			return
		}
		contentIDs = append(contentIDs, contentID)
	}

	if err := h.Service.ReorderPinned(r.Context(), spaceID, contentIDs); err != nil {
		slog.Error("reordering pinned content", "error", err, "space_id", spaceID.String())
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reorder pinned content")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
}
