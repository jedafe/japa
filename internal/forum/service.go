package forum

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CreateThreadInput holds parameters for creating a new forum thread.
type CreateThreadInput struct {
	ContentID         pgtype.UUID
	SpaceID           pgtype.UUID
	ParentReplyID     pgtype.UUID
	Title             string
	Slug              string
	Body              []byte
	IsAnonymous       bool
	EditWindowMinutes int32
}

// forumSettingsReader is the subset of config.Settings the forum service reads
// for hot-reloadable limits. Local interface so forum does not import config.
type forumSettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// SetSettings injects the hot-reloadable settings reader.
func (s *Service) SetSettings(r forumSettingsReader) {
	s.settings = r
}

// CreateThread creates a new forum thread and increments the reply count
// on the parent thread if this is a reply-thread (parent_reply_id set).
func (s *Service) CreateThread(ctx context.Context, tenantID, authorUserID pgtype.UUID, input CreateThreadInput) (gen.ForumThread, error) {
	if input.Title == "" {
		return gen.ForumThread{}, errors.New("title is required")
	}

	editWindow := input.EditWindowMinutes
	if editWindow <= 0 {
		// Hot-reloadable default (forum.edit_window_minutes); falls back to the
		// compiled-in default when unset or settings not injected. See AUDIT-BE-009.
		def := DefaultEditWindowMinutes
		if s.settings != nil {
			def = s.settings.GetIntOr(ctx, tenantID, "forum.edit_window_minutes", DefaultEditWindowMinutes)
		}
		editWindow = int32(def)
	}

	if input.Body == nil {
		input.Body = []byte("[]")
	}

	thread, err := s.store.createThread(ctx, gen.CreateForumThreadParams{
		TenantID:          tenantID,
		ContentID:         input.ContentID,
		SpaceID:           input.SpaceID,
		AuthorUserID:      authorUserID,
		ParentReplyID:     input.ParentReplyID,
		Title:             input.Title,
		Slug:              input.Slug,
		Body:              input.Body,
		IsAnonymous:       input.IsAnonymous,
		EditWindowMinutes: editWindow,
	})
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("creating forum thread: %w", err)
	}

	// Fire-and-forget forum.thread.created webhook (Section 12). Rule 8.
	s.emitWebhook(ctx, tenantID, "forum.thread.created", map[string]any{
		"thread_id": thread.ID, "author_id": authorUserID, "slug": thread.Slug,
	})

	return thread, nil
}

// GetThread retrieves a forum thread by ID.
func (s *Service) GetThread(ctx context.Context, id, tenantID pgtype.UUID) (gen.ForumThread, error) {
	return s.store.getThreadByID(ctx, id, tenantID)
}

// GetThreadBySlug retrieves a forum thread by slug.
func (s *Service) GetThreadBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.ForumThread, error) {
	return s.store.getThreadBySlug(ctx, slug, tenantID)
}

// ListThreadsBySpace returns paginated threads for a space.
func (s *Service) ListThreadsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsBySpace(ctx, tenantID, spaceID, cursorCreatedAt, limit)
}

// ListThreadsByAuthor returns paginated threads for an author.
func (s *Service) ListThreadsByAuthor(ctx context.Context, tenantID, authorUserID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByAuthor(ctx, tenantID, authorUserID, cursorCreatedAt, limit)
}

// ListThreadsByTenant returns all paginated threads for a tenant (public listing).
func (s *Service) ListThreadsByTenant(ctx context.Context, tenantID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByTenant(ctx, tenantID, cursorCreatedAt, limit)
}

// ListThreadsByTag returns paginated threads for a tag.
func (s *Service) ListThreadsByTag(ctx context.Context, tenantID, tagID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByTag(ctx, tenantID, tagID, cursorCreatedAt, limit)
}

// UpdateThread updates a forum thread. The author can edit; a moderator
// (or any holder of forum.thread.edit_any / super-admin wildcard) may also
// edit. Ref: Section 08 + §26 RBAC CRUD.
func (s *Service) UpdateThread(ctx context.Context, id, tenantID, userID pgtype.UUID, title string, body []byte, isAnonymous bool) (gen.ForumThread, error) {
	thread, err := s.store.getThreadByID(ctx, id, tenantID)
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("getting thread for update: %w", err)
	}

	if !s.canModerateThread(ctx, thread, userID, tenantID, "forum.thread.edit_any") {
		return gen.ForumThread{}, errors.New("only the author or a moderator can edit this thread")
	}

	if title == "" {
		title = thread.Title
	}

	if body == nil {
		body = thread.Body
	}

	updated, err := s.store.updateThread(ctx, gen.UpdateForumThreadParams{
		ID:          id,
		TenantID:    tenantID,
		Title:       title,
		Body:        body,
		IsAnonymous: isAnonymous,
	})
	if err != nil {
		return gen.ForumThread{}, err
	}

	s.emitWebhook(ctx, tenantID, "forum.thread.updated", map[string]any{"thread_id": id, "actor_id": userID})
	return updated, nil
}

// DeleteThread soft-deletes a forum thread. The author or a moderator
// (forum.thread.delete_any / super-admin) may delete. Ref: Section 08 + §26.
func (s *Service) DeleteThread(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	thread, err := s.store.getThreadByID(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("getting thread for delete: %w", err)
	}

	if !s.canModerateThread(ctx, thread, userID, tenantID, "forum.thread.delete_any") {
		return errors.New("only the author or a moderator can delete this thread")
	}

	if err := s.store.softDeleteThread(ctx, id, tenantID); err != nil {
		return err
	}

	s.emitWebhook(ctx, tenantID, "forum.thread.deleted", map[string]any{"thread_id": id, "actor_id": userID})
	return nil
}

// SolveThread marks a thread as solved. Per Section 08, only the OP or a
// moderator (or a holder of forum.thread.solve / super-admin) may solve.
// The route RBAC gate (forum.thread.solve) admits members; this method
// enforces the OP/moderator scope. Ref: Section 08 Solved flag.
func (s *Service) SolveThread(ctx context.Context, id, tenantID, solverUserID pgtype.UUID) error {
	thread, err := s.store.getThreadByID(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("getting thread for solve: %w", err)
	}

	if !s.canSolveThread(ctx, thread, solverUserID, tenantID) {
		return errors.New("only the author or a moderator can solve this thread")
	}

	if err := s.store.solveThread(ctx, id, tenantID, solverUserID); err != nil {
		return err
	}

	s.emitWebhook(ctx, tenantID, "forum.thread.solved", map[string]any{"thread_id": id, "solver_id": solverUserID})
	return nil
}

// UnsolveThread reverts the solved flag (DELETE /forum/{id}/solve).
// Ref: Section 08 Solved flag + api-routes-catalog L590.
func (s *Service) UnsolveThread(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	thread, err := s.store.getThreadByID(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("getting thread for unsolve: %w", err)
	}

	if !s.canSolveThread(ctx, thread, userID, tenantID) {
		return errors.New("only the author or a moderator can unsolve this thread")
	}

	return s.store.unsolveThread(ctx, id, tenantID)
}

// canModerateThread reports whether the user may edit/delete the thread:
// the author, a space moderator of the thread's space, or a holder of the
// given _any node (super-admin satisfies this via the wildcard). Ref: §26.
func (s *Service) canModerateThread(ctx context.Context, thread gen.ForumThread, userID, tenantID pgtype.UUID, anyNode string) bool {
	if thread.AuthorUserID == userID {
		return true
	}
	if s.store.isSpaceModerator(ctx, userID, thread.SpaceID) {
		return true
	}
	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, tenantID, anyNode) {
		return true
	}
	return false
}

// canSolveThread reports whether the user may solve/unsolve the thread:
// the OP, a space moderator, or a holder of forum.thread.solve (super-admin
// via wildcard). Ref: Section 08 "OP or moderator".
func (s *Service) canSolveThread(ctx context.Context, thread gen.ForumThread, userID, tenantID pgtype.UUID) bool {
	if thread.AuthorUserID == userID {
		return true
	}
	if s.store.isSpaceModerator(ctx, userID, thread.SpaceID) {
		return true
	}
	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, tenantID, "forum.thread.solve") {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Replies
// ---------------------------------------------------------------------------

// CreateReplyInput holds parameters for creating a new forum reply.
type CreateReplyInput struct {
	ParentReplyID pgtype.UUID
	Body          []byte
	IsAnonymous   bool
}

// CreateReply creates a new reply and increments the parent thread's reply_count.
func (s *Service) CreateReply(ctx context.Context, tenantID, threadID, authorUserID pgtype.UUID, input CreateReplyInput) (gen.ForumReply, error) {
	if input.Body == nil {
		input.Body = []byte("[]")
	}

	reply, err := s.store.createReply(ctx, gen.CreateForumReplyParams{
		TenantID:      tenantID,
		ThreadID:      threadID,
		ParentReplyID: input.ParentReplyID,
		AuthorUserID:  authorUserID,
		Body:          input.Body,
		IsAnonymous:   input.IsAnonymous,
	})
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("creating forum reply: %w", err)
	}

	// Load the parent thread once for both the AMA question-approval check
	// and the reply notification (§88 / §35).
	thread, threadErr := s.store.getThreadByID(ctx, threadID, tenantID)

	// §88 question_approval_required: if the parent thread is an AMA with
	// question_approval_required = true and the reply author is not the
	// host, mark the reply as pending host approval (BUG-003).
	amaPending := false
	if threadErr == nil && s.amaQuestions != nil && thread.AmaConfig != nil {
		if thread.AuthorUserID != authorUserID && amaRequiresApproval(thread.AmaConfig) {
			if markErr := s.amaQuestions.MarkReplyPending(ctx, reply.ID, tenantID); markErr != nil {
				slog.Warn("ama: failed to mark reply pending", "reply_id", reply.ID, "error", markErr)
			} else {
				amaPending = true
				reply.AmaPending = true
			}
		}
	}

	// Increment the parent thread's reply count.
	if err := s.store.incrementReplyCount(ctx, threadID, tenantID, 1); err != nil {
		// Log but don't fail the reply creation.
		slog.Warn("failed to increment reply count", "thread_id", threadID, "error", err)
	}

	s.emitWebhook(ctx, tenantID, "forum.reply.created", map[string]any{
		"reply_id": reply.ID, "thread_id": threadID, "author_id": authorUserID,
	})

	// BUG-013: Notify the thread author about the reply (fire-and-forget, Rule 8).
	// Don't notify if the reply author is the thread author.
	if s.notifier != nil && threadErr == nil && thread.AuthorUserID != authorUserID {
		notifyType := "forum_reply"
		if amaPending {
			// §88: host notified when questions queue up.
			notifyType = "ama_question_pending"
		}
		go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
			notifyCtx := context.Background()
			subjectTitle := thread.Title
			subjectType := "forum_thread"
			subjectID := threadID
			actorName := ""
			actorAvatar := ""
			subjectURL := fmt.Sprintf("/forum/threads/%s", threadID)
			_ = s.notifier.DispatchNotification(
				notifyCtx, tenantID, thread.AuthorUserID,
				notifyType, subjectTitle,
				authorUserID,
				subjectType, subjectID,
				subjectURL, actorName, actorAvatar,
			)
		}()
	}

	// BUG-011/012: Enqueue fan-out tasks for activity feed and search indexing
	// (fire-and-forget, Rule 8).
	if s.asynq != nil {
		go func() {
			payload, _ := json.Marshal(map[string]any{
				"entity_type": "forum_reply",
				"entity_id":   reply.ID.String(),
				"thread_id":   threadID.String(),
				"author_id":   authorUserID.String(),
				"tenant_id":   tenantID.String(),
			})
			_, _ = s.asynq.Enqueue(
				asynq.NewTask("content:fan_out_to_feeds", payload),
				asynq.MaxRetry(3),
			)
			// §12 stale-index fix: re-index the parent thread (not the reply)
			// since there is no forum_reply index — the thread document's
			// excerpt/solved status may change when a reply is added.
			searchPayload, _ := json.Marshal(map[string]string{
				"entity_type": "forum_thread",
				"entity_id":   threadID.String(),
				"tenant_id":   tenantID.String(),
				"operation":   "index",
			})
			_, _ = s.asynq.Enqueue(
				asynq.NewTask("search:index_entity", searchPayload),
				asynq.MaxRetry(3),
			)
		}()
	}

	return reply, nil
}

// GetReply retrieves a reply by ID.
func (s *Service) GetReply(ctx context.Context, id, tenantID pgtype.UUID) (gen.ForumReply, error) {
	return s.store.getReplyByID(ctx, id, tenantID)
}

// ListRepliesByThread returns all replies for a thread.
func (s *Service) ListRepliesByThread(ctx context.Context, threadID, tenantID pgtype.UUID) ([]gen.ForumReply, error) {
	return s.store.listRepliesByThread(ctx, threadID, tenantID)
}

// UpdateReply updates a reply's body. The author can edit; a moderator
// (forum.reply.edit_any / super-admin) may also edit. Ref: §26 RBAC CRUD.
func (s *Service) UpdateReply(ctx context.Context, id, tenantID, userID pgtype.UUID, body []byte) (gen.ForumReply, error) {
	reply, err := s.store.getReplyByID(ctx, id, tenantID)
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("getting reply for update: %w", err)
	}

	if reply.AuthorUserID != userID && !s.canModerateReply(ctx, reply, userID, tenantID, "forum.reply.edit_any") {
		return gen.ForumReply{}, errors.New("only the author or a moderator can edit this reply")
	}

	if body == nil {
		body = reply.Body
	}

	updated, err := s.store.updateReply(ctx, gen.UpdateForumReplyParams{
		ID:       id,
		TenantID: tenantID,
		Body:     body,
	})
	if err != nil {
		return gen.ForumReply{}, err
	}

	s.emitWebhook(ctx, tenantID, "forum.reply.updated", map[string]any{"reply_id": id, "actor_id": userID})
	return updated, nil
}

// DeleteReply soft-deletes a reply and decrements the thread reply count.
// The author or a moderator (forum.reply.delete_any / super-admin) may
// delete. Ref: §26 RBAC CRUD.
func (s *Service) DeleteReply(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	reply, err := s.store.getReplyByID(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("getting reply for delete: %w", err)
	}

	if reply.AuthorUserID != userID && !s.canModerateReply(ctx, reply, userID, tenantID, "forum.reply.delete_any") {
		return errors.New("only the author or a moderator can delete this reply")
	}

	if err := s.store.softDeleteReply(ctx, id, tenantID); err != nil {
		return err
	}

	// BUG-007: decrement the denormalized reply_count on the parent thread.
	if err := s.store.incrementReplyCount(ctx, reply.ThreadID, tenantID, -1); err != nil {
		slog.Warn("failed to decrement thread reply count", "error", err)
	}

	s.emitWebhook(ctx, tenantID, "forum.reply.deleted", map[string]any{"reply_id": id, "actor_id": userID})
	return nil
}

// canModerateReply reports whether the user may edit/delete the reply via a
// moderator-override node. Resolves the reply's thread to check space
// moderation. Ref: §26.
func (s *Service) canModerateReply(ctx context.Context, reply gen.ForumReply, userID, tenantID pgtype.UUID, anyNode string) bool {
	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, tenantID, anyNode) {
		return true
	}
	// Resolve the thread to check space moderation.
	thread, err := s.store.getThreadByID(ctx, reply.ThreadID, tenantID)
	if err != nil {
		return false
	}
	return s.store.isSpaceModerator(ctx, userID, thread.SpaceID)
}

// ---------------------------------------------------------------------------
// Votes
// ---------------------------------------------------------------------------

// VoteInput holds parameters for a vote operation.
type VoteInput struct {
	EntityType string
	EntityID   pgtype.UUID
	Value      int16
}

// Vote upserts a vote on a thread or reply and recalculates the entity's score.
func (s *Service) Vote(ctx context.Context, tenantID, userID pgtype.UUID, input VoteInput) (gen.ForumVote, error) {
	if input.Value != VoteDown && input.Value != VoteUp {
		return gen.ForumVote{}, errors.New("vote value must be -1 or 1")
	}

	if input.EntityType != EntityTypeThread && input.EntityType != EntityTypeReply {
		return gen.ForumVote{}, errors.New("entity type must be 'thread' or 'reply'")
	}

	vote, err := s.store.upsertVote(ctx, gen.UpsertForumVoteParams{
		TenantID:   tenantID,
		UserID:     userID,
		EntityType: input.EntityType,
		EntityID:   input.EntityID,
		Value:      input.Value,
	})
	if err != nil {
		return gen.ForumVote{}, fmt.Errorf("upserting vote: %w", err)
	}

	// Recalculate the entity's vote score.
	s.recalcVoteScore(ctx, input.EntityType, input.EntityID)

	s.emitWebhook(ctx, tenantID, "forum.vote.cast", map[string]any{
		"entity_type": input.EntityType, "entity_id": input.EntityID,
		"user_id": userID, "value": input.Value,
	})

	return vote, nil
}

// RemoveVote retracts the user's vote on a thread or reply and recalculates
// the entity's score. Ref: api-routes-catalog L591 (DELETE /forum/{id}/vote).
func (s *Service) RemoveVote(ctx context.Context, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if entityType != EntityTypeThread && entityType != EntityTypeReply {
		return errors.New("entity type must be 'thread' or 'reply'")
	}

	if err := s.store.deleteVote(ctx, tenantID, userID, entityType, entityID); err != nil {
		return fmt.Errorf("removing vote: %w", err)
	}

	s.recalcVoteScore(ctx, entityType, entityID)
	return nil
}

// recalcVoteScore recalculates the denormalized vote_score for an entity.
func (s *Service) recalcVoteScore(ctx context.Context, entityType string, entityID pgtype.UUID) {
	switch entityType {
	case EntityTypeThread:
		if err := s.store.updateThreadVoteScore(ctx, entityID); err != nil {
			slog.Warn("failed to update thread vote score", "error", err)
		}
	case EntityTypeReply:
		if err := s.store.updateReplyVoteScore(ctx, entityID); err != nil {
			slog.Warn("failed to update reply vote score", "error", err)
		}
	}
}

// GetVoteScore returns the aggregate vote score for an entity.
func (s *Service) GetVoteScore(ctx context.Context, entityType string, entityID pgtype.UUID) (int32, error) {
	return s.store.getVoteScore(ctx, entityType, entityID)
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

// AddBookmark adds a bookmark for a thread or reply.
func (s *Service) AddBookmark(ctx context.Context, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID, folder string) (gen.ForumBookmark, error) {
	if entityType != EntityTypeThread && entityType != EntityTypeReply {
		return gen.ForumBookmark{}, errors.New("entity type must be 'thread' or 'reply'")
	}

	if folder == "" {
		folder = "default"
	}

	bm, err := s.store.addBookmark(ctx, gen.AddForumBookmarkParams{
		TenantID:   tenantID,
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Folder:     folder,
	})
	if err != nil {
		return gen.ForumBookmark{}, fmt.Errorf("adding bookmark: %w", err)
	}

	// Increment the entity's bookmark count.
	if err := s.store.incrementEntityBookmarkCount(ctx, entityID, 1); err != nil {
		slog.Warn("failed to increment bookmark count", "error", err)
	}

	return bm, nil
}

// RemoveBookmark removes a bookmark.
func (s *Service) RemoveBookmark(ctx context.Context, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.store.removeBookmark(ctx, tenantID, userID, entityType, entityID); err != nil {
		return fmt.Errorf("removing bookmark: %w", err)
	}
	// Decrement the entity's bookmark count.
	if err := s.store.incrementEntityBookmarkCount(ctx, entityID, -1); err != nil {
		slog.Warn("failed to decrement bookmark count", "error", err)
	}

	return nil
}

// RemoveBookmarkByID removes a bookmark by its primary key
// (DELETE /forum/bookmarks/{id}). Resolves the entity first so the
// denormalized bookmark_count can be decremented. Ref: api-routes-catalog L610.
func (s *Service) RemoveBookmarkByID(ctx context.Context, id, userID, tenantID pgtype.UUID) error {
	bm, err := s.store.getBookmarkByID(ctx, id, userID, tenantID)
	if err != nil {
		return fmt.Errorf("resolving bookmark: %w", err)
	}

	if err := s.store.deleteBookmarkByID(ctx, id, userID, tenantID); err != nil {
		return fmt.Errorf("deleting bookmark: %w", err)
	}

	if err := s.store.incrementEntityBookmarkCount(ctx, bm.EntityID, -1); err != nil {
		slog.Warn("failed to decrement bookmark count", "error", err)
	}

	return nil
}

// ListBookmarksByUser returns paginated bookmarks for a user.
func (s *Service) ListBookmarksByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.ForumBookmark, error) {
	return s.store.listBookmarksByUser(ctx, tenantID, userID, limit, offset)
}

// IsBookmarked checks if an entity is bookmarked by a user.
func (s *Service) IsBookmarked(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	return s.store.isBookmarked(ctx, userID, entityType, entityID)
}

// GetBookmarkByEntity fetches the bookmark row for a (user, entity) pair.
// Returns the bookmark so the frontend can call DELETE /forum/bookmarks/{id}.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Bookmarks)
func (s *Service) GetBookmarkByEntity(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.ForumBookmark, error) {
	return s.store.getBookmarkByEntity(ctx, userID, entityType, entityID)
}

// ListThreadTags returns the tags attached to a thread.
// Ref: Section 08 (tags are first-class on discussions) — GET /forum/{id}/tags.
func (s *Service) ListThreadTags(ctx context.Context, threadID pgtype.UUID) ([]gen.ListForumThreadTagsRow, error) {
	return s.store.listThreadTags(ctx, threadID)
}

// ---------------------------------------------------------------------------
// Reply Views
// ---------------------------------------------------------------------------

// RecordReplyView records that a user viewed a reply (idempotent).
func (s *Service) RecordReplyView(ctx context.Context, replyID, userID pgtype.UUID) error {
	return s.store.insertReplyView(ctx, replyID, userID)
}

// GetReplyViewCount returns the view count for a reply.
func (s *Service) GetReplyViewCount(ctx context.Context, replyID pgtype.UUID) (int32, error) {
	return s.store.getReplyViewCount(ctx, replyID)
}

// IncrementThreadView atomically increments the view count for a thread.
// Called fire-and-forget from the GetThread handler — errors are logged
// but never block the response.
func (s *Service) IncrementThreadView(ctx context.Context, id, tenantID pgtype.UUID) {
	if err := s.store.incrementThreadViewCount(ctx, id, tenantID); err != nil {
		slog.Warn("failed to increment thread view count", "thread_id", id, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Enriched methods — return joined author + space data for public endpoints.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
// ---------------------------------------------------------------------------

// GetThreadBySlugWithDetails returns a thread plus joined author + space data.
func (s *Service) GetThreadBySlugWithDetails(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetForumThreadBySlugWithDetailsRow, error) {
	return s.store.getThreadBySlugWithDetails(ctx, slug, tenantID)
}

// GetTopReplyForThread returns the highest-voted non-deleted reply for a
// thread, used as the accepted-answer candidate for QAPage JSON-LD on solved
// threads (BL-012). Returns pgx.ErrNoRows when the thread has no replies.
func (s *Service) GetTopReplyForThread(ctx context.Context, threadID, tenantID pgtype.UUID) (gen.GetTopReplyForThreadRow, error) {
	return s.store.read.GetTopReplyForThread(ctx, gen.GetTopReplyForThreadParams{
		ThreadID: threadID,
		TenantID: tenantID,
	})
}

// ListThreadsByTenantWithDetails returns paginated threads with author + space.
func (s *Service) ListThreadsByTenantWithDetails(ctx context.Context, tenantID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ListForumThreadsByTenantWithDetailsRow, error) {
	return s.store.listThreadsByTenantWithDetails(ctx, tenantID, cursorCreatedAt, limit)
}

// amaRequiresApproval parses an ama_config JSONB blob and returns true if
// question_approval_required is set to true (§88). Returns false on any
// parse error — fail-open to avoid blocking replies on malformed config.
func amaRequiresApproval(amaConfig []byte) bool {
	var cfg struct {
		QuestionApprovalRequired *bool `json:"question_approval_required"`
	}
	if err := json.Unmarshal(amaConfig, &cfg); err != nil {
		return false
	}
	return cfg.QuestionApprovalRequired != nil && *cfg.QuestionApprovalRequired
}

// ListThreadsBySpaceWithDetails returns paginated threads for a space with author + space.
func (s *Service) ListThreadsBySpaceWithDetails(ctx context.Context, tenantID, spaceID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ListForumThreadsBySpaceWithDetailsRow, error) {
	return s.store.listThreadsBySpaceWithDetails(ctx, tenantID, spaceID, cursorCreatedAt, limit)
}

// ListThreadsByAuthorWithDetails returns paginated threads for an author with author + space.
func (s *Service) ListThreadsByAuthorWithDetails(ctx context.Context, tenantID, authorUserID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ListForumThreadsByAuthorWithDetailsRow, error) {
	return s.store.listThreadsByAuthorWithDetails(ctx, tenantID, authorUserID, cursorCreatedAt, limit)
}

// ListThreadsByTagWithDetails returns paginated threads for a tag with author + space.
func (s *Service) ListThreadsByTagWithDetails(ctx context.Context, tenantID, tagID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ListForumThreadsByTagWithDetailsRow, error) {
	return s.store.listThreadsByTagWithDetails(ctx, tenantID, tagID, cursorCreatedAt, limit)
}

// ListRepliesByThreadWithAuthor returns all replies for a thread with author data.
func (s *Service) ListRepliesByThreadWithAuthor(ctx context.Context, threadID, tenantID pgtype.UUID) ([]gen.ListForumRepliesByThreadWithAuthorRow, error) {
	return s.store.listRepliesByThreadWithAuthor(ctx, threadID, tenantID)
}
