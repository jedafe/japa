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
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/seo"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// JSONLDResolver resolves admin-managed JSON-LD templates (Section 14 /
// Section 26). The forum uses it to emit QAPage structured data from the
// active template, falling back to the hardcoded generator when no template
// exists.
type JSONLDResolver interface {
	ResolveJSONLD(ctx context.Context, tenantID pgtype.UUID, contentType string, fields map[string]string) (json.RawMessage, error)
}

// Handlers holds dependencies for forum HTTP handlers.
type Handlers struct {
	Service         *Service
	jsonldResolver  JSONLDResolver
	mentionCheckers *mentions.DBCheckers // nil disables erased-mention redaction
}

// NewHandlers creates Handlers with the given Service and optional JSON-LD
// resolver. A nil resolver is acceptable — the forum falls back to its
// hardcoded QAPage generator.
func NewHandlers(service *Service, resolver JSONLDResolver) *Handlers {
	return &Handlers{Service: service, jsonldResolver: resolver}
}

// SetMentionCheckers wires the DB-backed erased-user checker used to
// redact mentions of deleted users in thread/reply responses (Section 15 /
// ADR-029). nil disables redaction (backward compat / tests).
func (h *Handlers) SetMentionCheckers(c *mentions.DBCheckers) { h.mentionCheckers = c }

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
	case errors.Is(err, ErrTooManyTags):
		writeError(w, http.StatusConflict, "too_many_tags", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
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
	tagSlug := r.URL.Query().Get("tag")

	switch {
	case spaceIDStr != "":
		var spaceID pgtype.UUID
		if err := spaceID.Scan(spaceIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}

		threads, err := h.Service.ListThreadsBySpaceWithDetails(r.Context(), tenantID, spaceID, cursor, limit)
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

		threads, err := h.Service.ListThreadsByAuthorWithDetails(r.Context(), tenantID, authorID, cursor, limit)
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

		threads, err := h.Service.ListThreadsByTagWithDetails(r.Context(), tenantID, tagID, cursor, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": threads})

	case tagSlug != "":
		// Resolve the slug to a tag ID, then filter threads by that tag.
		// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (?tag=slug)
		tag, err := h.Service.GetTagBySlug(r.Context(), tagSlug, tenantID)
		if err != nil {
			writeError(w, http.StatusNotFound, "tag_not_found", "Tag not found")
			return
		}

		threads, err := h.Service.ListThreadsByTagWithDetails(r.Context(), tenantID, tag.ID, cursor, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": threads})

	default:
		// List all public threads for the tenant regardless of auth — the
		// forum index shows everybody's threads. Callers wanting their own
		// threads pass author_id explicitly. Uses the WithDetails variant so
		// the frontend receives joined author + space data in one response.
		threads, err := h.Service.ListThreadsByTenantWithDetails(r.Context(), tenantID, cursor, limit)
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

	// Use the WithDetails variant so the frontend receives joined author +
	// space data in one response. Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
	thread, err := h.Service.GetThreadBySlugWithDetails(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-021: enqueue the view-count increment to a shared background
	// worker instead of spawning a per-request goroutine. Non-blocking.
	h.Service.enqueueViewIncrement(thread.ID, tenantID)

	// BL-012 / Section 08: emit QAPage JSON-LD for solved threads so search
	// engines can index the accepted answer. The accepted-answer body is the
	// top-voted reply (the forum has no explicit accepted_reply_id, so the
	// highest-voted reply is the best proxy). For unsolved threads we still
	// emit a QAPage with answerCount=0 (valid schema.org).
	response := map[string]any{"data": thread}
	authorName := ""
	if thread.AuthorUsername != nil {
		if name, ok := thread.AuthorUsername.(string); ok {
			authorName = name
		}
	}
	questionBody := ""
	if len(thread.Body) > 0 {
		questionBody = string(thread.Body)
	}

	// BUG-020: cache the JSON-LD for solved threads to avoid a synchronous
	// DB round-trip (GetTopReplyForThread) on every request. The cache has
	// a 5-minute TTL; on a cache miss we fetch the top reply, generate the
	// JSON-LD, and store it. For unsolved threads we generate inline (no
	// DB fetch needed).
	//
	// Section 14 / Section 26: prefer the admin-managed QAPage template
	// (interpolated with the thread's field values) over the hardcoded
	// generator. The generator remains the fallback when no active template
	// exists or the resolver is unavailable.
	var jsonLd any
	if thread.Solved {
		if cached, ok := h.Service.getJSONLDCache(thread.ID); ok {
			var parsed any
			if err := json.Unmarshal([]byte(cached), &parsed); err == nil {
				jsonLd = parsed
			} else {
				jsonLd = cached
			}
		} else {
			var answerBody string
			var answerAuthor string
			var answerCreatedAt string
			var answerUpvotes string
			if top, topErr := h.Service.GetTopReplyForThread(r.Context(), thread.ID, tenantID); topErr == nil {
				if len(top.Body) > 0 {
					answerBody = string(top.Body)
				}
				answerAuthor = ifaceString(top.AuthorDisplayName, top.AuthorUsername)
				answerCreatedAt = tsString(top.CreatedAt)
				answerUpvotes = strconv.Itoa(int(top.VoteScore))
			} else if !errors.Is(topErr, pgx.ErrNoRows) {
				slog.Warn("QAPage JSON-LD: failed to fetch top reply", "thread_id", thread.ID, "error", topErr)
			}
			jsonLd = h.resolveQAPageJSONLD(r, tenantID, thread, questionBody, authorName, answerBody, answerAuthor, answerCreatedAt, answerUpvotes)
			if b, err := json.Marshal(jsonLd); err == nil {
				h.Service.setJSONLDCache(thread.ID, string(b))
			}
		}
	} else {
		jsonLd = h.resolveQAPageJSONLD(r, tenantID, thread, questionBody, authorName, "", "", "", "")
	}

	response["json_ld"] = jsonLd

	// Add can_solve flag for the frontend so it can show the solve/unsolve
	// affordance to the OP or moderators (BUG-014).
	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md ("OP or moderator")
	canSolve := false
	canEdit := false
	isBookmarked := false
	bookmarkID := ""
	if user := auth.FromContext(r.Context()); user != nil {
		threadModel := gen.ForumThread{
			AuthorUserID: thread.AuthorUserID,
			SpaceID:      thread.SpaceID,
		}
		canSolve = h.Service.canSolveThread(r.Context(), threadModel, user.UserID, tenantID)
		// The author, a space moderator, or a holder of forum.thread.edit_any
		// may edit. Mirrors the check in UpdateThread so the frontend can show
		// the edit affordance only when the PATCH would succeed.
		// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Editing)
		canEdit = h.Service.canModerateThread(r.Context(), threadModel, user.UserID, tenantID, "forum.thread.edit_any")

		// Expose whether the current user has bookmarked this thread so the
		// frontend can render the correct bookmark toggle state, plus the
		// bookmark's ID so DELETE /forum/bookmarks/{id} can be called
		// without first listing all bookmarks.
		// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Bookmarks)
		if bm, err := h.Service.GetBookmarkByEntity(r.Context(), user.UserID, EntityTypeThread, thread.ID); err == nil {
			isBookmarked = true
			bookmarkID = bm.ID.String()
		}
	}
	response["can_solve"] = canSolve
	response["can_edit"] = canEdit
	response["is_bookmarked"] = isBookmarked
	response["bookmark_id"] = bookmarkID

	// GDPR / ADR-029: expose deleted_user_ids so the frontend can render
	// @[deleted user] for mentions of erased users in the thread body
	// (Section 15). Best-effort: a failure logs a warning but does not
	// fail the request.
	if h.mentionCheckers != nil && len(thread.Body) > 0 {
		erasedIDs, err := mentions.ComputeErasedMentionIDs(
			r.Context(), thread.Body, tenantID, h.mentionCheckers,
		)
		if err != nil {
			slog.Warn("forum: ComputeErasedMentionIDs failed",
				"thread_id", thread.ID.String(), "error", err)
		} else if len(erasedIDs) > 0 {
			response["deleted_user_ids"] = erasedIDs
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// resolveQAPageJSONLD resolves the QAPage structured data for a forum thread.
// It prefers the admin-managed JSON-LD template (Section 14 / Section 26),
// interpolating the thread and accepted-answer field values into the
// {{field_key}} placeholders. When no active template exists (or the resolver
// is unavailable), it falls back to the hardcoded seo.GenerateQAPageJSONLD
// generator so existing behaviour is preserved.
func (h *Handlers) resolveQAPageJSONLD(
	r *http.Request,
	tenantID pgtype.UUID,
	thread gen.GetForumThreadBySlugWithDetailsRow,
	questionBody, authorName, answerBody, answerAuthor, answerCreatedAt, answerUpvotes string,
) any {
	isSolved := answerBody != ""
	fields := map[string]string{
		"title":                          thread.Title,
		"body":                           questionBody,
		"answer_count":                   strconv.Itoa(int(thread.ReplyCount)),
		"accepted_answer_body":           answerBody,
		"accepted_answer_author":         answerAuthor,
		"accepted_answer_created_at":     answerCreatedAt,
		"accepted_answer_upvotes":        answerUpvotes,
		"author_name":                    authorName,
		"created_at":                     tsString(thread.CreatedAt),
	}

	if h.jsonldResolver != nil {
		if raw, err := h.jsonldResolver.ResolveJSONLD(r.Context(), tenantID, "QAPage", fields); err == nil && len(raw) > 0 {
			var parsed any
			if err := json.Unmarshal(raw, &parsed); err == nil {
				return parsed
			}
			return string(raw)
		} else if err != nil {
			slog.Warn("QAPage JSON-LD: template resolution failed, falling back to generator", "thread_id", thread.ID, "error", err)
		}
	}

	return seo.GenerateQAPageJSONLD(thread.Title, questionBody, answerBody, authorName, isSolved)
}

// ifaceString returns the first non-empty string representation of the given
// interface{} values (used for author display name / username fallbacks).
func ifaceString(values ...any) string {
	for _, v := range values {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// tsString renders a pgtype.Timestamptz as RFC3339 or empty.
func tsString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
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

	if err := h.Service.DeleteThread(r.Context(), id, tenantID, user.UserID); err != nil {
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

// UnsolveThread handles DELETE /api/v1/forum/threads/{id}/solve.
// Ref: api-routes-catalog L590.
func (h *Handlers) UnsolveThread(w http.ResponseWriter, r *http.Request) {
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

	if err := h.Service.UnsolveThread(r.Context(), id, tenantID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "unsolved"}})
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

	// Use the WithAuthor variant so the frontend receives joined author data
	// (username, display_name, avatar_url) in the same response.
	replies, err := h.Service.ListRepliesByThreadWithAuthor(r.Context(), threadID, tenantID)
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

	if err := h.Service.DeleteReply(r.Context(), id, tenantID, user.UserID); err != nil {
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

// VoteOnThread handles POST /api/v1/forum/{id}/vote.
// The entity id is resolved from the URL param; the entity_type defaults to
// "thread" but may be overridden to "reply" via the body for reply votes.
// Ref: api-routes-catalog L591.
func (h *Handlers) VoteOnThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	entityID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID")
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		Value      int16  `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.EntityType == "" {
		input.EntityType = EntityTypeThread
	}

	if input.EntityType != EntityTypeThread && input.EntityType != EntityTypeReply {
		writeError(w, http.StatusBadRequest, "invalid_entity_type", "entity_type must be 'thread' or 'reply'")
		return
	}

	if input.Value != VoteDown && input.Value != VoteUp {
		writeError(w, http.StatusBadRequest, "invalid_vote_value", "value must be -1 or 1")
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

// RemoveThreadVote handles DELETE /api/v1/forum/{id}/vote.
// Retracts the user's vote on the entity resolved from the URL param.
// Ref: api-routes-catalog L591.
func (h *Handlers) RemoveThreadVote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	entityID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID")
		return
	}

	entityType := r.URL.Query().Get("entity_type")
	if entityType == "" {
		entityType = EntityTypeThread
	}

	if entityType != EntityTypeThread && entityType != EntityTypeReply {
		writeError(w, http.StatusBadRequest, "invalid_entity_type", "entity_type must be 'thread' or 'reply'")
		return
	}

	if err := h.Service.RemoveVote(r.Context(), tenantID, user.UserID, entityType, entityID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
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
// Resolves the bookmark by its primary key from the URL param — no query
// params required. Ref: api-routes-catalog L610.
func (h *Handlers) RemoveBookmark(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid bookmark ID")
		return
	}

	if err := h.Service.RemoveBookmarkByID(r.Context(), id, user.UserID, tenantID); err != nil {
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

// CreatePollForThread handles POST /api/v1/forum/{id}/poll.
// The thread id is resolved from the URL param, not the body.
// Ref: api-routes-catalog L592.
func (h *Handlers) CreatePollForThread(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	var input struct {
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

	if input.Question == "" {
		writeError(w, http.StatusBadRequest, "missing_question", "question is required")
		return
	}

	if len(input.Options) < 2 {
		writeError(w, http.StatusBadRequest, "insufficient_options", "At least 2 options required")
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

// VotePollOption handles POST /api/v1/poll-options/{id}/vote.
// The URL param is the option_id (not the poll_id). The poll is resolved
// from the option. Ref: api-routes-catalog L612.
func (h *Handlers) VotePollOption(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	optionID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_option_id", "Invalid poll option ID")
		return
	}

	if err := h.Service.VotePollByOptionID(r.Context(), optionID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "voted"}})
}

// RetractPollVoteHandler handles DELETE /api/v1/poll-options/{id}/vote.
// The URL param is the option_id; the poll is resolved from the option.
// Ref: api-routes-catalog L613.
func (h *Handlers) RetractPollVoteHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	optionID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_option_id", "Invalid poll option ID")
		return
	}

	if err := h.Service.RetractPollVote(r.Context(), optionID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
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
		if err := coverImageID.Scan(*input.CoverImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cover_image_id", "Invalid cover image ID")
			return
		}
	}

	if input.ModeratorUserID != nil {
		if err := moderatorUserID.Scan(*input.ModeratorUserID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_moderator_user_id", "Invalid moderator user ID")
			return
		}
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

// AddThreadTagsBulk handles POST /api/v1/forum/{id}/tags.
// Accepts a JSON array of tag ids and assigns them all to the thread,
// enforcing the MaxTagsPerThread cap. Ref: api-routes-catalog L596.
func (h *Handlers) AddThreadTagsBulk(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	var input struct {
		TagIDs []string `json:"tag_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.TagIDs) == 0 {
		writeError(w, http.StatusBadRequest, "missing_tag_ids", "tag_ids array is required")
		return
	}

	tagIDs := make([]pgtype.UUID, 0, len(input.TagIDs))
	for _, idStr := range input.TagIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag_id in list")
			return
		}
		tagIDs = append(tagIDs, id)
	}

	if err := h.Service.AddThreadTagsBulk(r.Context(), user.TenantID, threadID, tagIDs); err != nil {
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

// ListThreadTags handles GET /api/v1/forum/{id}/tags.
// Returns the tags attached to a thread. Public — no auth required.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (tags are
// first-class on discussions).
func (h *Handlers) ListThreadTags(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	tags, err := h.Service.ListThreadTags(r.Context(), threadID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tags})
}
