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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/mentions"
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

// PermissionChecker resolves RBAC permissions at runtime. Implemented by
// *rbac.RBAC. Used by DeleteComment to distinguish comments.delete_own
// (author) from comments.delete_any (moderator) at request time (BUG-003).
type PermissionChecker interface {
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// SpaceCommentChecker checks whether a user may comment or reply in a space,
// based on the space's commenting_perm and replying_perm axes (Section 05
// lines 145-154, BUG-002). Implemented by spaces.PostCheckerAdapter.
type SpaceCommentChecker interface {
	CanCommentInSpace(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error)
	CanReplyInSpace(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error)
}

// Handlers holds dependencies for comment HTTP handlers.
type Handlers struct {
	Service         *Service
	SpamPipeline    *spam.SpamPipeline
	asynqClient     TaskEnqueueClient      // nil disables engagement updates (test-only)
	notifier        NotificationDispatcher // nil disables notifications (test-only)
	perms           PermissionChecker      // nil disables delete_any moderator path
	mentionCheckers *mentions.DBCheckers   // nil disables erased-mention redaction
	spaceChecker    SpaceCommentChecker    // nil disables space perm checks (test-only)
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

// SetPermissionChecker wires the RBAC resolver used by DeleteComment to allow
// the comments.delete_any moderator path for non-authors (BUG-003).
func (h *Handlers) SetPermissionChecker(p PermissionChecker) { h.perms = p }

// SetMentionCheckers wires the DB-backed erased-user checker used to
// redact mentions of deleted users in comment responses (Section 15 /
// ADR-029). nil disables redaction (backward compat / tests).
func (h *Handlers) SetMentionCheckers(c *mentions.DBCheckers) { h.mentionCheckers = c }

// SetSpaceChecker wires the space permission checker used to enforce the
// commenting_perm and replying_perm axes (Section 05, BUG-002). nil disables
// the check (backward compat / tests).
func (h *Handlers) SetSpaceChecker(c SpaceCommentChecker) { h.spaceChecker = c }

// computeErasedMentionIDs returns the set of mentioned user IDs in a comment
// body whose accounts have been erased (Section 15 / ADR-029). Returns nil
// when checkers are not wired or no mentions are erased. Best-effort: errors
// are logged and an empty result is returned so the request still succeeds.
func (h *Handlers) computeErasedMentionIDs(ctx context.Context, tenantID pgtype.UUID, body []byte) []string {
	if h.mentionCheckers == nil {
		return nil
	}

	erased, err := mentions.ComputeErasedMentionIDs(ctx, body, tenantID, h.mentionCheckers)
	if err != nil {
		slog.Warn("comments: ComputeErasedMentionIDs failed", "error", err)
		return nil
	}

	return erased
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
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

// enqueueContentFanOut enqueues a content:fan_out_to_feeds task so a newly
// created discussion thread appears in the author's followers' feeds
// (Section 49 — Comment Thread Link Flow, BL-005). Fire-and-forget per Rule 8.
func (h *Handlers) enqueueContentFanOut(contentID, tenantID, authorID, spaceID pgtype.UUID, contentType, fanOutType string) {
	if h.asynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]interface{}{
		"content_id":   contentID.String(),
		"tenant_id":    tenantID.String(),
		"author_id":    authorID.String(),
		"space_id":     spaceID.String(),
		"content_type": contentType,
		"fan_out_type": fanOutType,
	})
	if err != nil {
		slog.Warn("comments: failed to marshal fan-out payload", "error", err)
		return
	}

	if _, err := h.asynqClient.Enqueue(
		asynq.NewTask(worker.TypeContentFanOutToFeeds, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeContentFanOutToFeeds)),
	); err != nil {
		slog.Warn("comments: failed to enqueue fan-out task",
			"content_id", contentID.String(), "error", err)
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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// viewerIDFromRequest returns the authenticated user's ID, or a zero UUID for
// anonymous requests. It is used to compute per-row user_vote in list queries.
func viewerIDFromRequest(r *http.Request) pgtype.UUID {
	if user := auth.FromContext(r.Context()); user != nil {
		return user.UserID
	}
	return pgtype.UUID{}
}

// ---------------------------------------------------------------------------
// Comment Handlers
// ---------------------------------------------------------------------------

// redactAnonymousComments zeroes out author-identifying fields (author_user_id,
// username, display_name, avatar_url) on anonymous comments so the API never
// leaks the identity of an anonymous commenter (Section 49 — is_anonymous,
// BUG-002). Works on all sqlc row types that carry the standard author fields.
func redactAnonymousComments[T any](
	rows []T,
	isAnon func(*T) bool,
	clear func(*T),
) {
	for i := range rows {
		if isAnon(&rows[i]) {
			clear(&rows[i])
		}
	}
}

// ListComments handles GET /api/v1/content/{id}/comments.
func (h *Handlers) ListComments(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32

	if os := r.URL.Query().Get("page_offset"); os != "" {
		n, err := strconv.Atoi(os)
		if err == nil && n >= 0 {
			// #nosec G109 // value bounded by MaxPageSize check
			// #nosec G115 // value bounded by caller
			offset = int32(n)
		}
	}

	// Section 49 — ?sort=top|new|oldest for top-level comments. "top" is the
	// default when the param is absent or unrecognized.
	sort := r.URL.Query().Get("sort")
	if sort != "new" && sort != "oldest" {
		sort = "top"
	}

	parentIDStr := r.URL.Query().Get("parent_id")
	viewerID := viewerIDFromRequest(r)
	if parentIDStr != "" {
		var parentID pgtype.UUID
		if err := parentID.Scan(parentIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent_id", "Invalid parent_id format")
			return
		}

		replies, err := h.Service.ListRepliesByParent(r.Context(), tenantID, parentID, viewerID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if replies == nil {
			replies = []gen.ListRepliesByParentRow{}
		}

		// BUG-002: redact author identity on anonymous replies.
		redactAnonymousComments(replies,
			func(r *gen.ListRepliesByParentRow) bool { return r.IsAnonymous },
			func(r *gen.ListRepliesByParentRow) {
				r.AuthorUserID = pgtype.UUID{}
				r.Username = ""
				r.DisplayName = ""
				r.AvatarUrl = ""
			})

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": replies})

		return
	}

	comments, err := h.Service.ListCommentsByContent(r.Context(), tenantID, contentID, viewerID, sort, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if comments == nil {
		comments = []gen.ListCommentsByContentRow{}
	}

	// BUG-002: redact author identity on anonymous comments.
	redactAnonymousComments(comments,
		func(c *gen.ListCommentsByContentRow) bool { return c.IsAnonymous },
		func(c *gen.ListCommentsByContentRow) {
			c.AuthorUserID = pgtype.UUID{}
			c.Username = ""
			c.DisplayName = ""
			c.AvatarUrl = ""
		})

	// GDPR / ADR-029: compute erased-mention IDs across all comments so the
	// frontend can render @[deleted user] for mentions of erased users.
	resp := map[string]any{"data": comments}
	erasedSet := make(map[string]struct{})

	for _, c := range comments {
		for _, id := range h.computeErasedMentionIDs(r.Context(), tenantID, c.Body) {
			erasedSet[id] = struct{}{}
		}
	}

	if len(erasedSet) > 0 {
		erasedIDs := make([]string, 0, len(erasedSet))
		for id := range erasedSet {
			erasedIDs = append(erasedIDs, id)
		}

		resp["deleted_user_ids"] = erasedIDs
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// GetComment handles GET /api/v1/comments/{id}.
func (h *Handlers) GetComment(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	commentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comment ID")
		return
	}

	comment, err := h.Service.GetComment(r.Context(), tenantID, commentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Section 49 — soft-deleted comments are not accessible via the API.
	// The list queries already filter deleted_at IS NULL; the single-get
	// endpoint must also reject deleted comments so the original body is
	// never leaked (BL-004).
	if comment.DeletedAt.Valid {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Comment not found")
		return
	}

	// BUG-002: redact author identity on anonymous comments.
	if comment.IsAnonymous {
		comment.AuthorUserID = pgtype.UUID{}
		comment.Username = ""
		comment.DisplayName = ""
		comment.AvatarUrl = ""
	}

	// GDPR / ADR-029: expose deleted_user_ids for frontend rendering.
	resp := map[string]any{"data": comment}
	if erasedIDs := h.computeErasedMentionIDs(r.Context(), tenantID, comment.Body); len(erasedIDs) > 0 {
		resp["deleted_user_ids"] = erasedIDs
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		ParentID    *string `json:"parent_id"`
		Body        []byte  `json:"body"` // TipTap JSON (base64-encoded in transport, decoded to raw JSON bytes by encoding/json)
		IsAnonymous bool    `json:"is_anonymous"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Body) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_body", "Comment body is required")
		return
	}

	createInput := CreateCommentInput{
		ContentID:   contentID,
		Body:        input.Body,
		IsAnonymous: input.IsAnonymous,
	}

	if input.ParentID != nil && *input.ParentID != "" {
		if err := createInput.ParentID.Scan(*input.ParentID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent_id", "Invalid parent_id format")
			return
		}
	}

	// Spam pipeline check — multi-layer content safety (BE-034).
	if h.SpamPipeline != nil {
		clientIP := clientip.FromRequest(r)

		spamResult, err := h.SpamPipeline.Check(r.Context(), tenantID, spam.SpamCheckRequest{
			Content:    string(input.Body),
			UserID:     user.UserID,
			IP:         clientIP,
			UserType:   "existing",
			EntityType: "comment",
		})
		if err != nil {
			slog.Warn("spam check failed", "error", err)
		} else if spamResult.Blocked {
			errpkg.WriteError(w, http.StatusForbidden, "content_blocked", "Content blocked by safety filters")
			return
		}
	}

	// Space permission check — enforce the space's commenting_perm (for
	// top-level comments) and replying_perm (for replies) axes (Section 05
	// lines 145-154, BUG-002). Skipped when no space checker is wired or
	// the content has no space.
	if h.spaceChecker != nil {
		spaceID, err := h.Service.GetContentSpaceID(r.Context(), tenantID, contentID)
		if err != nil {
			slog.Warn("comments: failed to resolve content space_id for perm check",
				"content_id", contentID.String(), "error", err)
		} else if spaceID.Valid {
			isReply := createInput.ParentID.Valid

			var allowed bool

			if isReply {
				allowed, err = h.spaceChecker.CanReplyInSpace(r.Context(), spaceID, user.UserID, tenantID)
			} else {
				allowed, err = h.spaceChecker.CanCommentInSpace(r.Context(), spaceID, user.UserID, tenantID)
			}

			if err != nil {
				slog.Warn("comments: space perm check failed",
					"space_id", spaceID.String(), "error", err)
			} else if !allowed {
				errpkg.WriteError(w, http.StatusForbidden, "space_permission_denied",
					"You do not have permission to comment in this space")
				return
			}
		}
	}

	comment, err := h.Service.CreateComment(r.Context(), tenantID, user.UserID, createInput)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Record the creation for the spam rate-limit layer (BUG-004). Without
	// this the jc:spam:rate:{tenant}:{user} sorted set is never populated and
	// the rate_limit scoring layer always returns 0.
	if h.SpamPipeline != nil {
		h.SpamPipeline.RecordContentCreation(r.Context(), tenantID, user.UserID)
	}

	// Fire-and-forget comment notifications (Section 49 — BL-001/BL-002).
	// comment:notify_author/parent/mention → notification:dispatch_batch.
	h.Service.DispatchCommentNotifications(r.Context(), tenantID, comment, h.notifier)

	// Enqueue an async engagement count update so content_scores.comment_count
	// (used by the feed ranking pipeline) stays in sync with the denormalized
	// contents.comment_count incremented in the service layer. Fire-and-forget
	// per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "comment", 1)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": comment})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Body []byte `json:"body"` // TipTap JSON
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Body) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_body", "Comment body is required")
		return
	}

	comment, err := h.Service.UpdateComment(r.Context(), tenantID, commentID, user.UserID, input.Body)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Dispatch @mention notifications for newly added mentions — fire-and-forget
	// (Section 49 — comment:notify_mention → notification:dispatch_batch).
	// Only mention notifications are dispatched on edit; content.comment and
	// comment.reply are "new comment" events and should not fire on edit
	// (BL-006). The notification batch system deduplicates by (recipient,
	// type, subject) so mentions that were already notified on create won't
	// produce duplicates.
	if h.notifier != nil {
		h.Service.DispatchMentionNotifications(r.Context(), tenantID, comment, h.notifier)
	}

	// GDPR / ADR-029: expose deleted_user_ids for frontend rendering.
	resp := map[string]any{"data": comment}
	if erasedIDs := h.computeErasedMentionIDs(r.Context(), tenantID, comment.Body); len(erasedIDs) > 0 {
		resp["deleted_user_ids"] = erasedIDs
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// DeleteComment handles DELETE /api/v1/comments/{id}.
//
// Section 49 — soft-delete own comment (comments.delete_own) or any comment
// as a moderator (comments.delete_any). The route is wired with RequireAuth
// only; the own-vs-any decision is made here at request time so a moderator
// holding only comments.delete_any is not blocked by a delete_own middleware
// gate, and a plain member cannot delete another author's comment (BUG-003).
func (h *Handlers) DeleteComment(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// Fetch the comment to determine authorship. GetComment returns a joined
	// row; we only need AuthorUserID.
	existing, err := h.Service.GetComment(r.Context(), tenantID, commentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	isAuthor := existing.AuthorUserID.Valid && existing.AuthorUserID.Bytes == user.UserID.Bytes

	if isAuthor {
		// Author path — comments.delete_own. The sqlc query enforces
		// author_user_id atomically, so even if the row changed hands between
		// the GetComment and the delete, a non-author cannot delete.
		if err := h.Service.SoftDeleteCommentByAuthor(r.Context(), tenantID, commentID, user.UserID); err != nil {
			handleServiceError(w, err)
			return
		}
		// Keep content_scores.comment_count in sync with contents.comment_count.
		if existing.Status == "published" && !existing.DeletedAt.Valid {
			h.enqueueEngagementUpdate("content", existing.ContentID, "comment", -1)
		}

		errpkg.WriteJSON(w, http.StatusNoContent, nil)

		return
	}

	// Moderator path — comments.delete_any. Requires the RBAC checker; when
	// not wired, deny (test-only default).
	if h.perms == nil || !h.perms.HasPermission(r.Context(), user.UserID, tenantID, "comments.delete_any") {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You can only delete your own comments")
		return
	}

	if err := h.Service.SoftDeleteComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Keep content_scores.comment_count in sync with contents.comment_count.
	if existing.Status == "published" && !existing.DeletedAt.Valid {
		h.enqueueEngagementUpdate("content", existing.ContentID, "comment", -1)
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// VoteComment handles POST /api/v1/comments/{id}/vote.
//
// Section 49 — all comments support upvote/downvote.
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Value int16 `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if err := h.Service.VoteComment(r.Context(), tenantID, commentID, user.UserID, input.Value); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"voted": true}})
}

// RemoveVote handles DELETE /api/v1/comments/{id}/vote.
func (h *Handlers) RemoveVote(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.RemoveCommentVote(r.Context(), tenantID, commentID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// PinComment handles POST /api/v1/comments/{id}/pin.
//
// Section 49 — pinning is allowed for moderators (comments.pin) OR the content
// author. The route is wired with RequireAuth only; the own-vs-moderator
// decision is made here at request time so a content author who doesn't hold
// comments.pin can still pin comments on their own content (BL-003).
func (h *Handlers) PinComment(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if !h.canPinComment(r, user.UserID, tenantID, commentID) {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to pin this comment")
		return
	}

	if err := h.Service.PinComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"pinned": true}})
}

// UnpinComment handles DELETE /api/v1/comments/{id}/pin.
//
// Same author-or-moderator gate as PinComment (BL-003).
func (h *Handlers) UnpinComment(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if !h.canPinComment(r, user.UserID, tenantID, commentID) {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to unpin this comment")
		return
	}

	if err := h.Service.UnpinComment(r.Context(), tenantID, commentID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// canPinComment returns true if the user holds comments.pin (moderator) or is
// the author of the parent content (Section 49, BL-003).
func (h *Handlers) canPinComment(r *http.Request, userID, tenantID, commentID pgtype.UUID) bool {
	// Moderator path — comments.delete_any RBAC node.
	if h.perms != nil && h.perms.HasPermission(r.Context(), userID, tenantID, "comments.pin") {
		return true
	}
	// Content-author path — the author of the parent content may pin.
	contentAuthorID, err := h.Service.GetCommentContentAuthorID(r.Context(), tenantID, commentID)
	if err != nil {
		slog.Warn("comments: failed to resolve content author for pin check",
			"comment_id", commentID.String(), "error", err)
		return false
	}

	return contentAuthorID.Valid && contentAuthorID.Bytes == userID.Bytes
}

// RemoveComment handles POST /api/v1/admin/comments/{id}/remove.
//
// Section 49 — moderator removal-by-status. Sets status='removed' +
// removed_by + removed_reason, leaving the row visible for audit (unlike a
// soft-delete). The route is gated by comments.delete_any (BUG-010).
func (h *Handlers) RemoveComment(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comment ID")
		return
	}

	var input struct {
		Reason string `json:"reason"`
	}
	// Body is optional — a moderator may remove without a reason.
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
			return
		}
	}

	// Fetch the comment to get the content_id and current status for the
	// engagement count update (Section 32).
	existing, err := h.Service.GetComment(r.Context(), tenantID, commentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if err := h.Service.RemoveComment(r.Context(), tenantID, commentID, user.UserID, input.Reason); err != nil {
		handleServiceError(w, err)
		return
	}

	// Keep content_scores.comment_count in sync with contents.comment_count.
	if existing.Status == "published" && !existing.DeletedAt.Valid {
		h.enqueueEngagementUpdate("content", existing.ContentID, "comment", -1)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"removed": true}})
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errpkg.ErrMaxDepth):
		errpkg.WriteError(w, http.StatusBadRequest, "depth_exceeded", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
	}
}
