package publicapi

// Phase 3 (spec 2026-09-24-agent-data-api-phase3): API-key-scoped bookmark
// read/write endpoints. The key owner's bookmarks are exposed at
// /public/me/bookmarks; writes require the admin-granted api.write.bookmarks
// scope. Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// PublicBookmarkItem is the wire shape returned by GET /public/me/bookmarks.
// Title/slug let agents and extensions resolve the saved item back to a
// canonical URL without a second round-trip.
type PublicBookmarkItem struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Title      string `json:"title,omitempty"`
	Slug       string `json:"slug,omitempty"`
	Folder     string `json:"folder,omitempty"`
	FolderID   string `json:"folder_id,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// BookmarkService is the narrow seam into the session-side bookmark domain.
// It is satisfied by an adapter in internal/server that composes the users
// and content services, so publicapi never imports them (dependency
// direction: server wires concrete services into publicapi interfaces).
// Implementations also enqueue the engagement-count side effects the
// session bookmark endpoints perform.
type BookmarkService interface {
	// ListBookmarks returns the owner's bookmarks, newest first.
	ListBookmarks(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]PublicBookmarkItem, error)
	// AddContentBookmark idempotently bookmarks a content item (existing
	// bookmarks update the folder). Returns errpkg.ErrNotFound when the
	// content is absent or not visible to the owner, or when folderID is
	// set and not owned by the caller.
	AddContentBookmark(ctx context.Context, userID, contentID, folderID pgtype.UUID) (PublicBookmarkItem, error)
	// RemoveContentBookmark deletes the owner's bookmark for a content
	// item. Returns errpkg.ErrNotFound when no such bookmark exists.
	RemoveContentBookmark(ctx context.Context, userID, contentID pgtype.UUID) error
}

// SetBookmarkService injects the bookmark adapter.
func (h *Handlers) SetBookmarkService(svc BookmarkService) {
	h.bookmarks = svc
}

// ListMyBookmarks handles GET /api/v1/public/me/bookmarks — the key owner's
// bookmarks. Requires api.read.me.
func (h *Handlers) ListMyBookmarks(w http.ResponseWriter, r *http.Request) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}
	if h.bookmarks == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	limit := parsePageSize(r, defaultPageSize, 100)
	offset := parseOffset(r)

	items, err := h.bookmarks.ListBookmarks(r.Context(), keyCtx.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if items == nil {
		items = []PublicBookmarkItem{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// PutMyBookmark handles PUT /api/v1/public/me/bookmarks/{content_id}.
// Requires api.write.bookmarks. Invisible or absent content returns 404 —
// never 403 — so the endpoint cannot be used to probe for the existence of
// restricted content.
func (h *Handlers) PutMyBookmark(w http.ResponseWriter, r *http.Request) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}
	if h.bookmarks == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	contentID, err := parseUUIDParam(r, "content_id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id format")
		return
	}

	var input struct {
		FolderID string `json:"folder_id"`
	}
	// The body is optional — tolerate an absent/empty body (io.EOF) so a
	// bare PUT still creates the bookmark.
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	var folderID pgtype.UUID
	if input.FolderID != "" {
		if err := folderID.Scan(input.FolderID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_folder_id", "Invalid folder_id format")
			return
		}
	}

	item, err := h.bookmarks.AddContentBookmark(r.Context(), keyCtx.UserID, contentID, folderID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": item})
}

// DeleteMyBookmark handles DELETE /api/v1/public/me/bookmarks/{content_id}.
// Requires api.write.bookmarks.
func (h *Handlers) DeleteMyBookmark(w http.ResponseWriter, r *http.Request) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}
	if h.bookmarks == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	contentID, err := parseUUIDParam(r, "content_id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id format")
		return
	}

	if err := h.bookmarks.RemoveContentBookmark(r.Context(), keyCtx.UserID, contentID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
