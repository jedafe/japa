// Package comments — HTTP handlers for the unified comment system (Section 49).
package comments

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

const defaultPageSize int32 = 20

// NotificationDispatcher dispatches notifications via the batch system
// (Section 35 — consolidated from comment:notify_author/parent/mention
// into notification:dispatch_batch per Section 11). The return type is
// intentionally ignored — comment creation is fire-and-forget for
// notifications (Rule 8).
type NotificationDispatcher interface {
	DispatchNotification(
		ctx context.Context,
		tenantID, recipientID pgtype.UUID,
		nType, subjectTitle string,
		actorID pgtype.UUID,
		subjectType string,
		subjectID pgtype.UUID,
		subjectURL, actorName, actorAvatar string,
	) error
}

// TaskEnqueueClient is the subset of the Asynq client needed to enqueue
// background tasks. Implemented by *asynq.Client.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handlers holds dependencies for comment HTTP handlers.
type Handlers struct {
	Service      *Service
	SpamPipeline *spam.SpamPipeline
	asynqClient  TaskEnqueueClient      // nil disables engagement updates (test-only)
	notifier     NotificationDispatcher // nil disables notifications (test-only)
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// SetSpamPipeline injects the spam detection pipeline for comment safety checks.
func (h *Handlers) SetSpamPipeline(p *spam.SpamPipeline) {
	h.SpamPipeline = p
}

// SetAsynqClient wires the Asynq client used to enqueue engagement count
// updates after comment creation (content_scores.comment_count sync).
func (h *Handlers) SetAsynqClient(c TaskEnqueueClient) { h.asynqClient = c }

// SetNotifier wires the notification dispatcher used after comment creation
// (Section 49 — comment:notify_author/parent/mention → notification:dispatch_batch).
func (h *Handlers) SetNotifier(n NotificationDispatcher) { h.notifier = n }

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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// enqueueEngagementUpdate enqueues an async content engagement count update
// task (worker.TypeEngagementCountUpdate). Fire-and-forget per Rule 8 —
// errors are logged but never returned to the caller. Used to keep
// content_scores.comment_count in sync with contents.comment_count.
func (h *Handlers) enqueueEngagementUpdate(entityType string, entityID pgtype.UUID, engagementType string, delta int) {
	if h.asynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]interface{}{
		"entity_type":     entityType,
		"entity_id":       entityID.String(),
		"engagement_type": engagementType,
		"delta":           delta,
	})
	if err != nil {
		slog.Warn("comments: failed to marshal engagement update payload", "error", err)
		return
	}

	if _, err := h.asynqClient.Enqueue(
		asynq.NewTask(worker.TypeEngagementCountUpdate, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeEngagementCountUpdate)),
	); err != nil {
		slog.Warn("comments: failed to enqueue engagement update task",
			"entity_id", entityID.String(), "error", err)
	}
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > 100 {
		return 100
	}

	return int32(n) //nolint:gosec // G109: bounded by max check above
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// ---------------------------------------------------------------------------
// Comment Handlers
// ---------------------------------------------------------------------------

// ListComments handles GET /api/v1/content/{id}/comments.
func (h *Handlers) ListComments(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32

	if os := r.URL.Query().Get("page_offset"); os != "" {
		n, err := strconv.Atoi(os)
		if err == nil && n >= 0 {
			offset = int32(n) //nolint:gosec // G109: validated
		}
	}

	parentIDStr := r.URL.Query().Get("parent_id")
	if parentIDStr != "" {
		var parentID pgtype.UUID
		if err := parentID.Scan(parentIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_parent_id", "Invalid parent_id format")
			return
		}

		replies, err := h.Service.ListRepliesByParent(r.Context(), tenantID, parentID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": replies})

		return
	}

	comments, err := h.Service.ListCommentsByContent(r.Context(), tenantID, contentID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": comments})
}

// GetComment handles GET /api/v1/comments/{id}.
func (h *Handlers) GetComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comment ID")
		return
	}

	comment, err := h.Service.GetComment(r.Context(), tenantID, commentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": comment})
}

// CreateComment handles POST /api/v1/content/{id}/comments.
func (h *Handlers) CreateComment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
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

	var input struct {
		ParentID *string `json:"parent_id"`
		Body     string  `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Body == "" {
		writeError(w, http.StatusBadRequest, "missing_body", "Comment body is required")
		return
	}

	createInput := CreateCommentInput{
		ContentID: contentID,
		Body:      input.Body,
	}

	if input.ParentID != nil && *input.ParentID != "" {
		if err := createInput.ParentID.Scan(*input.ParentID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_parent_id", "Invalid parent_id format")
			return
		}
	}

	// Spam pipeline check — multi-layer content safety (BE-034).
	if h.SpamPipeline != nil {
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		spamResult, err := h.SpamPipeline.Check(r.Context(), tenantID, spam.SpamCheckRequest{
			Content:    input.Body,
			UserID:     user.UserID,
			IP:         clientIP,
			UserType:   "existing",
			EntityType: "comment",
		})
		if err != nil {
			slog.Warn("spam check failed", "error", err)
		} else if spamResult.Blocked {
			writeError(w, http.StatusForbidden, "content_blocked", "Content blocked by safety filters")
			return
		}
	}

	comment, err := h.Service.CreateComment(r.Context(), tenantID, user.UserID, createInput)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Fire-and-forget comment notifications (Section 49 — BL-001/BL-002).
	// comment:notify_author/parent/mention → notification:dispatch_batch.
	h.Service.DispatchCommentNotifications(r.Context(), tenantID, comment, h.notifier)

	// Enqueue an async engagement count update so content_scores.comment_count
	// (used by the feed ranking pipeline) stays in sync with the denormalized
	// contents.comment_count incremented in the service layer. Fire-and-forget
	// per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "comment", 1)

	writeJSON(w, http.StatusCreated, map[string]any{"data": comment})
}

// UpdateComment handles PATCH /api/v1/comments/{id}.
func (h *Handlers) UpdateComment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Body string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Body == "" {
		writeError(w, http.StatusBadRequest, "missing_body", "Comment body is required")
		return
	}

	comment, err := h.Service.UpdateComment(r.Context(), tenantID, commentID, user.UserID, input.Body)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": comment})
}

// DeleteComment handles DELETE /api/v1/comments/{id}.
func (h *Handlers) DeleteComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.SoftDeleteComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// VoteComment handles POST /api/v1/comments/{id}/vote.
func (h *Handlers) VoteComment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Value int16 `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if err := h.Service.VoteComment(r.Context(), tenantID, commentID, user.UserID, input.Value); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"voted": true}})
}

// RemoveVote handles DELETE /api/v1/comments/{id}/vote.
func (h *Handlers) RemoveVote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	commentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.RemoveCommentVote(r.Context(), commentID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// PinComment handles POST /api/v1/comments/{id}/pin.
func (h *Handlers) PinComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.PinComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"pinned": true}})
}

// UnpinComment handles DELETE /api/v1/comments/{id}/pin.
func (h *Handlers) UnpinComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.UnpinComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrDuplicate):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrMaxDepth):
		writeError(w, http.StatusBadRequest, "depth_exceeded", err.Error())
	case errors.Is(err, ErrEditWindowExpired):
		writeError(w, http.StatusForbidden, "edit_window_expired", "The edit window for this comment has expired")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
