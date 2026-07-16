// Package forum — HTTP handlers for the Forum Q&A / Solved system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package forum

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for forum HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
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
	case errors.Is(err, errpkg.ErrDuplicate):
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

// parseCursorCreatedAt extracts the cursor_created_at query parameter.
func parseCursorCreatedAt(r *http.Request) pgtype.Timestamptz {
	s := r.URL.Query().Get("cursor_created_at")
	if s == "" {
		return pgtype.Timestamptz{}
	}

	var ts pgtype.Timestamptz
	if err := ts.Scan(s); err != nil {
		return pgtype.Timestamptz{}
	}

	return ts
}

// ---------------------------------------------------------------------------
// Thread Handlers
// ---------------------------------------------------------------------------

// CreateThread handles POST /api/v1/forum/threads.
func (h *Handlers) CreateThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Title             string  `json:"title"`
		Slug              string  `json:"slug"`
		Body              []byte  `json:"body"`
		SpaceID           *string `json:"space_id"`
		ContentID         *string `json:"content_id"`
		ParentReplyID     *string `json:"parent_reply_id"`
		IsAnonymous       bool    `json:"is_anonymous"`
		EditWindowMinutes *int32  `json:"edit_window_minutes"`
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

	var spaceID, contentID, parentReplyID pgtype.UUID
	if input.SpaceID != nil {
		if err := spaceID.Scan(*input.SpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
	}

	if input.ContentID != nil {
		if err := contentID.Scan(*input.ContentID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id format")
			return
		}
	}

	if input.ParentReplyID != nil {
		if err := parentReplyID.Scan(*input.ParentReplyID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_parent_reply_id", "Invalid parent_reply_id format")
			return
		}
	}

	// Pass 0 when the client does not specify an edit window so the service
	// applies the hot-reloadable default (forum.edit_window_minutes).
	editWindow := int32(0)
	if input.EditWindowMinutes != nil {
		editWindow = *input.EditWindowMinutes
	}

	thread, err := h.Service.CreateThread(r.Context(), tenantID, user.UserID, CreateThreadInput{
		ContentID:         contentID,
		SpaceID:           spaceID,
		ParentReplyID:     parentReplyID,
		Title:             input.Title,
		Slug:              input.Slug,
		Body:              input.Body,
		IsAnonymous:       input.IsAnonymous,
		EditWindowMinutes: editWindow,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": thread})
}

// ListThreads handles GET /api/v1/forum/threads.
// Supports ?space_id=, ?author_id=, ?tag_id=, ?page_size=, ?cursor_created_at=.
// Public: when no user is present, lists all public threads for the tenant.
func (h *Handlers) ListThreads(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	cursor := parseCursorCreatedAt(r)

	spaceIDStr := r.URL.Query().Get("space_id")
	authorIDStr := r.URL.Query().Get("author_id")
	tagIDStr := r.URL.Query().Get("tag_id")

	switch {
	case spaceIDStr != "":
		var spaceID pgtype.UUID
		if err := spaceID.Scan(spaceIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}

		threads, err := h.Service.ListThreadsBySpace(r.Context(), tenantID, spaceID, cursor, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": threads})

	case authorIDStr != "":
		var authorID pgtype.UUID
		if err := authorID.Scan(authorIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_author_id", "Invalid author_id")
			return
		}

		threads, err := h.Service.ListThreadsByAuthor(r.Context(), tenantID, authorID, cursor, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": threads})

	case tagIDStr != "":
		var tagID pgtype.UUID
		if err := tagID.Scan(tagIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag_id")
			return
		}

		threads, err := h.Service.ListThreadsByTag(r.Context(), tenantID, tagID, cursor, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": threads})

	default:
		// List all public threads for the tenant regardless of auth — the
		// forum index shows everyone's threads. Callers wanting their own
		// threads pass author_id explicitly.
		threads, err := h.Service.ListThreadsByTenant(r.Context(), tenantID, cursor, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": threads})
	}
}

// GetThread handles GET /api/v1/forum/threads/{slug}.
func (h *Handlers) GetThread(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	thread, err := h.Service.GetThreadBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Increment view count fire-and-forget — never blocks the response.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.Service.IncrementThreadView(bgCtx, thread.ID, tenantID)
	}()

	writeJSON(w, http.StatusOK, map[string]any{"data": thread})
}

// UpdateThread handles PATCH /api/v1/forum/threads/{id}.
func (h *Handlers) UpdateThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid thread ID")
		return
	}

	var input struct {
		Title       string `json:"title"`
		Body        []byte `json:"body"`
		IsAnonymous bool   `json:"is_anonymous"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	thread, err := h.Service.UpdateThread(r.Context(), id, tenantID, user.UserID, input.Title, input.Body, input.IsAnonymous)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": thread})
}

// DeleteThread handles DELETE /api/v1/forum/threads/{id}.
func (h *Handlers) DeleteThread(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid thread ID")
		return
	}

	if err := h.Service.DeleteThread(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// SolveThread handles POST /api/v1/forum/threads/{id}/solve.
func (h *Handlers) SolveThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid thread ID")
		return
	}

	if err := h.Service.SolveThread(r.Context(), id, tenantID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "solved"}})
}

// ---------------------------------------------------------------------------
// Reply Handlers
// ---------------------------------------------------------------------------

// CreateReply handles POST /api/v1/forum/threads/{id}/replies.
func (h *Handlers) CreateReply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	var input struct {
		Body          []byte  `json:"body"`
		ParentReplyID *string `json:"parent_reply_id"`
		IsAnonymous   bool    `json:"is_anonymous"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var parentReplyID pgtype.UUID
	if input.ParentReplyID != nil {
		if err := parentReplyID.Scan(*input.ParentReplyID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_parent_reply_id", "Invalid parent_reply_id format")
			return
		}
	}

	reply, err := h.Service.CreateReply(r.Context(), tenantID, threadID, user.UserID, CreateReplyInput{
		Body:          input.Body,
		ParentReplyID: parentReplyID,
		IsAnonymous:   input.IsAnonymous,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": reply})
}

// ListReplies handles GET /api/v1/forum/threads/{id}/replies.
func (h *Handlers) ListReplies(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	replies, err := h.Service.ListRepliesByThread(r.Context(), threadID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": replies})
}

// UpdateReply handles PATCH /api/v1/forum/replies/{id}.
func (h *Handlers) UpdateReply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid reply ID")
		return
	}

	var input struct {
		Body []byte `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	reply, err := h.Service.UpdateReply(r.Context(), id, tenantID, user.UserID, input.Body)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": reply})
}

// DeleteReply handles DELETE /api/v1/forum/replies/{id}.
func (h *Handlers) DeleteReply(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid reply ID")
		return
	}

	if err := h.Service.DeleteReply(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Vote Handler
// ---------------------------------------------------------------------------

// Vote handles POST /api/v1/forum/vote.
func (h *Handlers) Vote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Value      int16  `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.EntityType == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_type", "entity_type is required")
		return
	}

	if input.EntityID == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_id", "entity_id is required")
		return
	}

	if input.Value != VoteDown && input.Value != VoteUp {
		writeError(w, http.StatusBadRequest, "invalid_vote_value", "value must be -1 or 1")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(input.EntityID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
		return
	}

	vote, err := h.Service.Vote(r.Context(), tenantID, user.UserID, VoteInput{
		EntityType: input.EntityType,
		EntityID:   entityID,
		Value:      input.Value,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": vote})
}

// ---------------------------------------------------------------------------
// Bookmark Handlers
// ---------------------------------------------------------------------------

// AddBookmark handles POST /api/v1/forum/bookmarks.
func (h *Handlers) AddBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Folder     string `json:"folder"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.EntityType == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_type", "entity_type is required")
		return
	}

	if input.EntityID == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_id", "entity_id is required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(input.EntityID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
		return
	}

	bm, err := h.Service.AddBookmark(r.Context(), tenantID, user.UserID, input.EntityType, entityID, input.Folder)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": bm})
}

// RemoveBookmark handles DELETE /api/v1/forum/bookmarks/{id}.
func (h *Handlers) RemoveBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	if _, err := parseUUIDParam(r, "id"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid bookmark ID")
		return
	}

	// We need entity_type and entity_id from the bookmark to remove it.
	// The client should provide these via query params or we look them up.
	entityType := r.URL.Query().Get("entity_type")
	entityIDStr := r.URL.Query().Get("entity_id")

	if entityType == "" || entityIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "entity_type and entity_id query params required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
		return
	}

	if err := h.Service.RemoveBookmark(r.Context(), user.UserID, entityType, entityID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ListBookmarks handles GET /api/v1/forum/bookmarks.
func (h *Handlers) ListBookmarks(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	bookmarks, err := h.Service.ListBookmarksByUser(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": bookmarks})
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
// Poll Handlers
// ---------------------------------------------------------------------------

// CreatePoll handles POST /api/v1/forum/polls.
func (h *Handlers) CreatePoll(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		ThreadID      string  `json:"thread_id"`
		Question      string  `json:"question"`
		IsMultiSelect bool    `json:"is_multi_select"`
		CloseAt       *string `json:"close_at"`
		Options       []struct {
			Label    string `json:"label"`
			Position int32  `json:"position"`
		} `json:"options"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ThreadID == "" {
		writeError(w, http.StatusBadRequest, "missing_thread_id", "thread_id is required")
		return
	}

	if input.Question == "" {
		writeError(w, http.StatusBadRequest, "missing_question", "question is required")
		return
	}

	if len(input.Options) < 2 {
		writeError(w, http.StatusBadRequest, "insufficient_options", "At least 2 options required")
		return
	}

	var threadID pgtype.UUID
	if err := threadID.Scan(input.ThreadID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread_id")
		return
	}

	var closeAt pgtype.Timestamptz
	if input.CloseAt != nil {
		if err := closeAt.Scan(*input.CloseAt); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_close_at", "Invalid close_at date format")
			return
		}
	}

	opts := make([]PollOptionInput, len(input.Options))
	for i, o := range input.Options {
		opts[i] = PollOptionInput{Label: o.Label, Position: o.Position}
	}

	poll, err := h.Service.CreatePoll(r.Context(), CreatePollInput{
		ThreadID:      threadID,
		Question:      input.Question,
		IsMultiSelect: input.IsMultiSelect,
		CloseAt:       closeAt,
		Options:       opts,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": poll})
}

// VotePoll handles POST /api/v1/forum/polls/{id}/vote.
func (h *Handlers) VotePoll(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	pollID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_poll_id", "Invalid poll ID")
		return
	}

	var input struct {
		OptionID string `json:"option_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.OptionID == "" {
		writeError(w, http.StatusBadRequest, "missing_option_id", "option_id is required")
		return
	}

	var optionID pgtype.UUID
	if err := optionID.Scan(input.OptionID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_option_id", "Invalid option_id")
		return
	}

	if err := h.Service.VotePoll(r.Context(), pollID, optionID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "voted"}})
}

// GetPoll handles GET /api/v1/forum/polls/{id}.
func (h *Handlers) GetPoll(w http.ResponseWriter, r *http.Request) {
	pollID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_poll_id", "Invalid poll ID")
		return
	}

	poll, err := h.Service.GetPollByThreadID(r.Context(), pollID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	results, err := h.Service.GetPollResults(r.Context(), pollID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"poll":    poll,
			"results": results,
		},
	})
}

// ---------------------------------------------------------------------------
// Tag Handlers
// ---------------------------------------------------------------------------

// CreateTag handles POST /api/v1/forum/tags.
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name            string  `json:"name"`
		Slug            string  `json:"slug"`
		Description     string  `json:"description"`
		Rules           string  `json:"rules"`
		CoverImageID    *string `json:"cover_image_id"`
		ModeratorUserID *string `json:"moderator_user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	var coverImageID, moderatorUserID pgtype.UUID
	if input.CoverImageID != nil {
		if err := coverImageID.Scan(*input.CoverImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cover_image_id", "Invalid cover_image_id format")
			return
		}
	}

	if input.ModeratorUserID != nil {
		if err := moderatorUserID.Scan(*input.ModeratorUserID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_moderator_user_id", "Invalid moderator_user_id format")
			return
		}
	}

	tag, err := h.Service.CreateTag(r.Context(), tenantID, CreateTagInput{
		Name:            input.Name,
		Slug:            input.Slug,
		Description:     input.Description,
		Rules:           input.Rules,
		CoverImageID:    coverImageID,
		ModeratorUserID: moderatorUserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": tag})
}

// ListTags handles GET /api/v1/forum/tags.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	tags, err := h.Service.ListTags(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tags})
}

// GetTagBySlug handles GET /api/v1/forum/tags/{slug}.
func (h *Handlers) GetTagBySlug(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	tag, err := h.Service.GetTagBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tag})
}

// UpdateTag handles PATCH /api/v1/forum/tags/{id}.
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	var input struct {
		Name            string  `json:"name"`
		Slug            string  `json:"slug"`
		Description     string  `json:"description"`
		Rules           string  `json:"rules"`
		CoverImageID    *string `json:"cover_image_id"`
		ModeratorUserID *string `json:"moderator_user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var coverImageID, moderatorUserID pgtype.UUID
	if input.CoverImageID != nil {
		_ = coverImageID.Scan(*input.CoverImageID)
	}

	if input.ModeratorUserID != nil {
		_ = moderatorUserID.Scan(*input.ModeratorUserID)
	}

	tag, err := h.Service.UpdateTag(r.Context(), id, tenantID, UpdateTagInput{
		Name:            input.Name,
		Slug:            input.Slug,
		Description:     input.Description,
		Rules:           input.Rules,
		CoverImageID:    coverImageID,
		ModeratorUserID: moderatorUserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tag})
}

// DeleteTag handles DELETE /api/v1/forum/tags/{id}.
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	if err := h.Service.DeleteTag(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// AddThreadTag handles POST /api/v1/forum/threads/{id}/tags/{tagId}.
func (h *Handlers) AddThreadTag(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	tagID, err := parseUUIDParam(r, "tagId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag ID")
		return
	}

	if err := h.Service.AddThreadTag(r.Context(), user.TenantID, threadID, tagID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "added"}})
}

// RemoveThreadTag handles DELETE /api/v1/forum/threads/{id}/tags/{tagId}.
func (h *Handlers) RemoveThreadTag(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	tagID, err := parseUUIDParam(r, "tagId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag ID")
		return
	}

	if err := h.Service.RemoveThreadTag(r.Context(), user.TenantID, threadID, tagID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}
