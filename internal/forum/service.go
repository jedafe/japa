package forum

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
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
	// Poll, when provided, is created in the same transaction as the thread.
	Poll *PollInput
}

// forumSettingsReader is the subset of config.Settings the forum service reads
// for hot-reloadable limits. Local interface so forum does not import config.
type forumSettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// SetSettings injects the hot-reloadable settings reader.
func (s *Service) SetSettings(r forumSettingsReader) {
	s.settings = r
}

// CreateThread creates a new forum thread and optionally a poll in a single
// transaction. The poll is only created when input.Poll is non-nil and has at
// least two options, so the thread and its poll are always committed (or
// rolled back) together.
func (s *Service) CreateThread(ctx context.Context, authorUserID pgtype.UUID, input CreateThreadInput) (gen.ForumThread, error) {
	if input.Title == "" {
		return gen.ForumThread{}, errors.New("title is required")
	}

	editWindow := input.EditWindowMinutes
	if editWindow <= 0 {
		// Hot-reloadable default (forum.edit_window_minutes); falls back to the
		// compiled-in default when unset or settings not injected. See AUDIT-BE-009.
		def := DefaultEditWindowMinutes
		if s.settings != nil {
			def = s.settings.GetIntOr(ctx, "forum.edit_window_minutes", DefaultEditWindowMinutes)
		}

		editWindow = int32(def) //nolint:gosec // G115: bounded
	}

	if input.Body == nil {
		input.Body = []byte("[]")
	}

	if input.Poll != nil {
		if err := validatePollInput(input.Poll); err != nil {
			return gen.ForumThread{}, err
		}
	}

	thread, err := db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (gen.ForumThread, error) {
		t, err := q.CreateForumThread(ctx, gen.CreateForumThreadParams{

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

		if input.Poll != nil {
			poll, err := q.CreateForumPoll(ctx, gen.CreateForumPollParams{
				ThreadID:      t.ID,
				Question:      input.Poll.Question,
				IsMultiSelect: input.Poll.IsMultiSelect,
				CloseAt:       input.Poll.CloseAt,
			})
			if err != nil {
				return gen.ForumThread{}, fmt.Errorf("creating forum poll: %w", err)
			}

			for i, opt := range input.Poll.Options {
				pos := opt.Position
				if pos == 0 {
					pos = int32(i) //nolint:gosec // options count is bounded by validatePollInput
				}

				if _, err := q.AddForumPollOption(ctx, gen.AddForumPollOptionParams{
					PollID:   poll.ID,
					Label:    opt.Label,
					Position: pos,
				}); err != nil {
					return gen.ForumThread{}, fmt.Errorf("adding poll option: %w", err)
				}
			}
		}

		return forumThreadFromCreateRow(t), nil
	})
	if err != nil {
		return gen.ForumThread{}, err
	}

	// Fire-and-forget forum.thread.created webhook (Section 12). Rule 8.
	s.emitWebhook(ctx, "forum.thread.created", map[string]any{
		"thread_id": thread.ID, "author_id": authorUserID, "slug": thread.Slug,
	})

	return thread, nil
}

// validatePollInput checks the minimum fields required for an embedded poll.
func validatePollInput(p *PollInput) error {
	if p.Question == "" {
		return errors.New("poll question is required")
	}

	if len(p.Options) < 2 {
		return errors.New("poll requires at least 2 options")
	}

	for i, opt := range p.Options {
		if opt.Label == "" {
			return fmt.Errorf("poll option %d label is required", i+1)
		}
	}

	return nil
}

// GetThread retrieves a forum thread by ID.
func (s *Service) GetThread(ctx context.Context, id pgtype.UUID) (gen.ForumThread, error) {
	return s.store.getThreadByID(ctx, id)
}

// GetThreadBySlug retrieves a forum thread by slug.
func (s *Service) GetThreadBySlug(ctx context.Context, slug string) (gen.ForumThread, error) {
	return s.store.getThreadBySlug(ctx, slug)
}

// ListThreadsBySpace returns paginated threads for a space.
func (s *Service) ListThreadsBySpace(ctx context.Context, spaceID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsBySpace(ctx, spaceID, cursorCreatedAt, limit)
}

// ListThreadsByAuthor returns paginated threads for an author.
func (s *Service) ListThreadsByAuthor(ctx context.Context, authorUserID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByAuthor(ctx, authorUserID, cursorCreatedAt, limit)
}

// ListThreadsByPlatform returns all paginated threads for a platform (public listing).
func (s *Service) ListThreadsByPlatform(ctx context.Context, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByPlatform(ctx, cursorCreatedAt, limit)
}

// ListThreadsByTag returns paginated threads for a tag.
func (s *Service) ListThreadsByTag(ctx context.Context, tagID pgtype.UUID, cursorCreatedAt pgtype.Timestamptz, limit int32) ([]gen.ForumThread, error) {
	return s.store.listThreadsByTag(ctx, tagID, cursorCreatedAt, limit)
}

// UpdateThread updates a forum thread. The author can edit within
// edit_window_minutes; a moderator (or any holder of forum.thread.edit_any /
// super-admin wildcard) may edit at any time. Ref: Section 08 + §26 RBAC CRUD.
func (s *Service) UpdateThread(ctx context.Context, id, userID pgtype.UUID, title string, body []byte, isAnonymous bool) (gen.ForumThread, error) {
	thread, err := s.store.getThreadByID(ctx, id)
	if err != nil {
		return gen.ForumThread{}, fmt.Errorf("getting thread for update: %w", err)
	}

	if !s.canModerateThread(ctx, thread, userID, "forum.thread.edit_any") {
		return gen.ForumThread{}, errors.New("only the author or a moderator can edit this thread")
	}

	if !s.canModerateAny(ctx, thread.SpaceID, userID, "forum.thread.edit_any") && thread.AuthorUserID == userID {
		if expired := editWindowExpired(thread.EditWindowMinutes, thread.CreatedAt); expired {
			return gen.ForumThread{}, fmt.Errorf("%w: thread edit window has expired", ErrEditWindowExpired)
		}
	}

	if title == "" {
		title = thread.Title
	}

	if body == nil {
		body = thread.Body
	}

	updated, err := s.store.updateThread(ctx, gen.UpdateForumThreadParams{
		ID: id,

		Title:       title,
		Body:        body,
		IsAnonymous: isAnonymous,
	})
	if err != nil {
		return gen.ForumThread{}, err
	}

	s.emitWebhook(ctx, "forum.thread.updated", map[string]any{"thread_id": id, "actor_id": userID})

	return updated, nil
}

// DeleteThread soft-deletes a forum thread. The author or a moderator
// (forum.thread.delete_any / super-admin) may delete. Ref: Section 08 + §26.
func (s *Service) DeleteThread(ctx context.Context, id, userID pgtype.UUID) error {
	thread, err := s.store.getThreadByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting thread for delete: %w", err)
	}

	if !s.canModerateThread(ctx, thread, userID, "forum.thread.delete_any") {
		return errors.New("only the author or a moderator can delete this thread")
	}

	if err := s.store.softDeleteThread(ctx, id); err != nil {
		return err
	}

	s.emitWebhook(ctx, "forum.thread.deleted", map[string]any{"thread_id": id, "actor_id": userID})

	return nil
}

// SolveThread marks a thread as solved. Per Section 08, only the OP or a
// moderator (or a holder of forum.thread.solve / super-admin) may solve.
// When parentReplyID is not provided, the top-voted reply is used as the
// accepted answer (BL-012). Ref: Section 08 Solved flag.
func (s *Service) SolveThread(ctx context.Context, id, solverUserID, parentReplyID pgtype.UUID) error {
	thread, err := s.store.getThreadByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting thread for solve: %w", err)
	}

	if !s.canSolveThread(ctx, thread, solverUserID) {
		return errors.New("only the author or a moderator can solve this thread")
	}

	// If the caller did not pick an accepted reply, default to the top-voted
	// non-deleted reply so parent_reply_id is always populated.
	if !parentReplyID.Valid {
		if top, topErr := s.GetTopReplyForThread(ctx, id); topErr == nil {
			parentReplyID = top.ID
		} else if !errors.Is(topErr, pgx.ErrNoRows) {
			slog.Warn("forum: failed to fetch top reply when solving", "thread_id", id, "error", topErr)
		}
	}

	if err := s.store.solveThread(ctx, id, solverUserID, parentReplyID); err != nil {
		return err
	}

	s.emitWebhook(ctx, "forum.thread.solved", map[string]any{"thread_id": id, "solver_id": solverUserID, "parent_reply_id": parentReplyID})

	return nil
}

// UnsolveThread reverts the solved flag (DELETE /forum/{id}/solve).
// Ref: Section 08 Solved flag + api-routes-catalog L590.
func (s *Service) UnsolveThread(ctx context.Context, id, userID pgtype.UUID) error {
	thread, err := s.store.getThreadByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting thread for unsolve: %w", err)
	}

	if !s.canSolveThread(ctx, thread, userID) {
		return errors.New("only the author or a moderator can unsolve this thread")
	}

	return s.store.unsolveThread(ctx, id)
}

// canModerateThread reports whether the user may edit/delete the thread:
// the author, a space moderator of the thread's space, or a holder of the
// given _any node (super-admin satisfies this via the wildcard). Ref: §26.
func (s *Service) canModerateThread(ctx context.Context, thread gen.ForumThread, userID pgtype.UUID, anyNode string) bool {
	if thread.AuthorUserID == userID {
		return true
	}

	if s.store.isSpaceModerator(ctx, userID, thread.SpaceID) {
		return true
	}

	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, anyNode) {
		return true
	}

	return false
}

// canModerateAny reports whether the user has a moderator-scope grant for the
// given permission node, without considering ownership. Used for reply edit/delete
// so a thread author who is not a moderator does not get other users' replies.
func (s *Service) canModerateAny(ctx context.Context, spaceID, userID pgtype.UUID, anyNode string) bool {
	if s.store.isSpaceModerator(ctx, userID, spaceID) {
		return true
	}

	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, anyNode) {
		return true
	}

	return false
}

// canSolveThread reports whether the user may solve/unsolve the thread:
// the OP, a space moderator, or a holder of forum.thread.solve (super-admin
// via wildcard). Ref: Section 08 "OP or moderator".
func (s *Service) canSolveThread(ctx context.Context, thread gen.ForumThread, userID pgtype.UUID) bool {
	if thread.AuthorUserID == userID {
		return true
	}

	if s.store.isSpaceModerator(ctx, userID, thread.SpaceID) {
		return true
	}

	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, "forum.thread.solve") {
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
func (s *Service) CreateReply(ctx context.Context, threadID, authorUserID pgtype.UUID, input CreateReplyInput) (gen.ForumReply, error) {
	if input.Body == nil {
		input.Body = []byte("[]")
	}

	reply, err := s.store.createReply(ctx, gen.CreateForumReplyParams{

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
	thread, threadErr := s.store.getThreadByID(ctx, threadID)

	// §88 question_approval_required: if the parent thread is an AMA with
	// question_approval_required = true and the reply author is not the
	// host, mark the reply as pending host approval (BUG-003).
	amaPending := false

	if threadErr == nil && s.amaQuestions != nil && thread.AmaConfig != nil {
		if thread.AuthorUserID != authorUserID && amaRequiresApproval(thread.AmaConfig) {
			if markErr := s.amaQuestions.MarkReplyPending(ctx, reply.ID); markErr != nil {
				slog.Warn("ama: failed to mark reply pending", "reply_id", reply.ID, "error", markErr)
			} else {
				amaPending = true
				reply.AmaPending = true
			}
		}
	}

	// Increment the parent thread's reply count.
	if err := s.store.incrementReplyCount(ctx, threadID, 1); err != nil {
		// Log but don't fail the reply creation.
		slog.Warn("failed to increment reply count", "thread_id", threadID, "error", err)
	}

	s.emitWebhook(ctx, "forum.reply.created", map[string]any{
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
			subjectURL := fmt.Sprintf("/forum/%s", thread.Slug)
			_ = s.notifier.DispatchNotification(
				notifyCtx, thread.AuthorUserID,
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
			})
			_, _ = s.asynq.Enqueue(
				asynq.NewTask(
					worker.TypeContentFanOutToFeeds, payload,
					asynq.Queue(worker.QueueForTask(worker.TypeContentFanOutToFeeds)),
				),
				asynq.MaxRetry(3),
			)
			// §12 stale-index fix: re-index the parent thread (not the reply)
			// since there is no forum_reply index — the thread document's
			// excerpt/solved status may change when a reply is added.
			searchPayload, _ := json.Marshal(map[string]string{
				"entity_type": "forum_thread",
				"entity_id":   threadID.String(),

				"operation": "index",
			})
			_, _ = s.asynq.Enqueue(
				asynq.NewTask(
					worker.TypeSearchIndexSync, searchPayload,
					asynq.Queue(worker.QueueForTask(worker.TypeSearchIndexSync)),
				),
				asynq.MaxRetry(3),
			)
		}()
	}

	return reply, nil
}

// GetReply retrieves a reply by ID.
func (s *Service) GetReply(ctx context.Context, id pgtype.UUID) (gen.ForumReply, error) {
	return s.store.getReplyByID(ctx, id)
}

// ListRepliesByThread returns all replies for a thread.
func (s *Service) ListRepliesByThread(ctx context.Context, threadID pgtype.UUID) ([]gen.ForumReply, error) {
	return s.store.listRepliesByThread(ctx, threadID)
}

// UpdateReply updates a reply's body. The author can edit within the parent
// thread's edit_window_minutes; a moderator (forum.reply.edit_any /
// super-admin) may edit at any time. Ref: §26 RBAC CRUD.
func (s *Service) UpdateReply(ctx context.Context, id, userID pgtype.UUID, body []byte) (gen.ForumReply, error) {
	reply, err := s.store.getReplyByID(ctx, id)
	if err != nil {
		return gen.ForumReply{}, fmt.Errorf("getting reply for update: %w", err)
	}

	if reply.AuthorUserID != userID && !s.canModerateReply(ctx, reply, userID, "forum.reply.edit_any") {
		return gen.ForumReply{}, errors.New("only the author or a moderator can edit this reply")
	}

	if reply.AuthorUserID == userID && !s.canModerateReply(ctx, reply, userID, "forum.reply.edit_any") {
		thread, terr := s.store.getThreadByID(ctx, reply.ThreadID)
		if terr == nil {
			if expired := editWindowExpired(thread.EditWindowMinutes, reply.CreatedAt); expired {
				return gen.ForumReply{}, fmt.Errorf("%w: reply edit window has expired", ErrEditWindowExpired)
			}
		}
	}

	if body == nil {
		body = reply.Body
	}

	updated, err := s.store.updateReply(ctx, gen.UpdateForumReplyParams{
		ID: id,

		Body: body,
	})
	if err != nil {
		return gen.ForumReply{}, err
	}

	s.emitWebhook(ctx, "forum.reply.updated", map[string]any{"reply_id": id, "actor_id": userID})

	// The reply may be the accepted answer; clear the thread's QAPage JSON-LD
	// cache so the next GET uses the updated body.
	s.deleteJSONLDCache(ctx, reply.ThreadID)

	return updated, nil
}

// DeleteReply soft-deletes a reply and decrements the thread reply count.
// The author or a moderator (forum.reply.delete_any / super-admin) may
// delete. Ref: §26 RBAC CRUD.
func (s *Service) DeleteReply(ctx context.Context, id, userID pgtype.UUID) error {
	reply, err := s.store.getReplyByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting reply for delete: %w", err)
	}

	if reply.AuthorUserID != userID && !s.canModerateReply(ctx, reply, userID, "forum.reply.delete_any") {
		return errors.New("only the author or a moderator can delete this reply")
	}

	if err := s.store.softDeleteReply(ctx, id); err != nil {
		return err
	}

	// BUG-007: decrement the denormalized reply_count on the parent thread.
	if err := s.store.incrementReplyCount(ctx, reply.ThreadID, -1); err != nil {
		slog.Warn("failed to decrement thread reply count", "error", err)
	}

	s.emitWebhook(ctx, "forum.reply.deleted", map[string]any{"reply_id": id, "actor_id": userID})

	// The reply may have been the accepted answer; clear QAPage JSON-LD cache.
	s.deleteJSONLDCache(ctx, reply.ThreadID)

	return nil
}

// canModerateReply reports whether the user may edit/delete the reply via a
// moderator-override node. Resolves the reply's thread to check space
// moderation. Ref: §26.
func (s *Service) canModerateReply(ctx context.Context, reply gen.ForumReply, userID pgtype.UUID, anyNode string) bool {
	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, anyNode) {
		return true
	}
	// Resolve the thread to check space moderation.
	thread, err := s.store.getThreadByID(ctx, reply.ThreadID)
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
func (s *Service) Vote(ctx context.Context, userID pgtype.UUID, input VoteInput) (gen.ForumVote, error) {
	if input.Value != VoteDown && input.Value != VoteUp {
		return gen.ForumVote{}, errors.New("vote value must be -1 or 1")
	}

	if input.EntityType != EntityTypeThread && input.EntityType != EntityTypeReply {
		return gen.ForumVote{}, errors.New("entity type must be 'thread' or 'reply'")
	}

	vote, err := s.store.upsertVote(ctx, gen.UpsertForumVoteParams{

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

	s.emitWebhook(ctx, "forum.vote.cast", map[string]any{
		"entity_type": input.EntityType, "entity_id": input.EntityID,
		"user_id": userID, "value": input.Value,
	})

	return vote, nil
}

// RemoveVote retracts the user's vote on a thread or reply and recalculates
// the entity's score. Ref: api-routes-catalog L591 (DELETE /forum/{id}/vote).
func (s *Service) RemoveVote(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if entityType != EntityTypeThread && entityType != EntityTypeReply {
		return errors.New("entity type must be 'thread' or 'reply'")
	}

	if err := s.store.deleteVote(ctx, userID, entityType, entityID); err != nil {
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

// GetUserVotesForThread returns the current user's vote values for a thread
// and all of its replies. This is used to hydrate per-entity user_vote state
// in the detail page (BUG-006).
func (s *Service) GetUserVotesForThread(ctx context.Context, threadID, userID pgtype.UUID) ([]gen.ListUserVotesForThreadRow, error) {
	return s.store.getUserVotesForThread(ctx, threadID, userID)
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

// AddBookmark adds a bookmark for a thread or reply.
func (s *Service) AddBookmark(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, folder string) (gen.Bookmark, error) {
	normalized := normalizeForumEntityType(entityType)
	if normalized != EntityTypeForumThread && normalized != EntityTypeForumReply {
		return gen.Bookmark{}, errors.New("entity type must be 'thread' or 'reply'")
	}

	folderID := pgtype.UUID{}

	if folder == "" && s.folderResolver != nil {
		f, err := s.folderResolver.Resolve(ctx, userID, normalized, "")
		if err == nil && f.ID != "" {
			folder = f.Name

			var fid pgtype.UUID
			if scanErr := fid.Scan(f.ID); scanErr == nil {
				folderID = fid
			}
		} else if err != nil {
			slog.Warn("forum: failed to resolve default bookmark folder", "error", err)
		}
	}

	if folder == "" {
		folder = "discussions"
	}

	bm, err := s.store.addBookmark(ctx, gen.AddForumBookmarkParams{

		UserID:     userID,
		EntityType: normalized,
		EntityID:   entityID,
		Folder:     folder,
		FolderID:   folderID,
	})
	if err != nil {
		return gen.Bookmark{}, fmt.Errorf("adding bookmark: %w", err)
	}

	// Increment the entity's bookmark count (only forum_threads carry the count).
	if normalized == EntityTypeForumThread {
		if err := s.store.incrementEntityBookmarkCount(ctx, entityID, 1); err != nil {
			slog.Warn("failed to increment bookmark count", "error", err)
		}
	}

	return bm, nil
}

// RemoveBookmark removes a bookmark.
func (s *Service) RemoveBookmark(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	normalized := normalizeForumEntityType(entityType)
	if err := s.store.removeBookmark(ctx, userID, normalized, entityID); err != nil {
		return fmt.Errorf("removing bookmark: %w", err)
	}
	// Decrement the entity's bookmark count (only forum_threads carry the count).
	if normalized == EntityTypeForumThread {
		if err := s.store.incrementEntityBookmarkCount(ctx, entityID, -1); err != nil {
			slog.Warn("failed to decrement bookmark count", "error", err)
		}
	}

	return nil
}

// RemoveBookmarkByID removes a bookmark by its primary key
// (DELETE /forum/bookmarks/{id}). Resolves the entity first so the
// denormalized bookmark_count can be decremented. Ref: api-routes-catalog L610.
func (s *Service) RemoveBookmarkByID(ctx context.Context, id, userID pgtype.UUID) error {
	bm, err := s.store.getBookmarkByID(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("resolving bookmark: %w", err)
	}

	if err := s.store.deleteBookmarkByID(ctx, id, userID); err != nil {
		return fmt.Errorf("deleting bookmark: %w", err)
	}

	// Decrement the entity's bookmark count (only forum_threads carry the count).
	if bm.EntityType == EntityTypeForumThread {
		if err := s.store.incrementEntityBookmarkCount(ctx, bm.EntityID, -1); err != nil {
			slog.Warn("failed to decrement bookmark count", "error", err)
		}
	}

	return nil
}

// ListBookmarksByUser returns paginated bookmarks for a user.
func (s *Service) ListBookmarksByUser(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Bookmark, error) {
	return s.store.listBookmarksByUser(ctx, userID, limit, offset)
}

// IsBookmarked checks if an entity is bookmarked by a user.
func (s *Service) IsBookmarked(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	return s.store.isBookmarked(ctx, userID, normalizeForumEntityType(entityType), entityID)
}

// GetBookmarkByEntity fetches the bookmark row for a (user, entity) pair.
// Returns the bookmark so the frontend can call DELETE /forum/bookmarks/{id}.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Bookmarks).
func (s *Service) GetBookmarkByEntity(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.Bookmark, error) {
	return s.store.getBookmarkByEntity(ctx, userID, normalizeForumEntityType(entityType), entityID)
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

// IncrementThreadView increments the view count for a thread. When a Valkey
// client is attached, the increment is buffered in the cache and flushed to the
// database by a background worker. Without Valkey, the increment is queued to
// the in-memory channel.
func (s *Service) IncrementThreadView(ctx context.Context, id pgtype.UUID) {
	if s.valkey != nil {
		s.enqueueViewIncrement(ctx, id)
		return
	}
	select {
	case s.viewChan <- viewIncrement{threadID: id}:
	default:
	}
}

// ---------------------------------------------------------------------------
// Enriched methods — return joined author + space data for public endpoints.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
// ---------------------------------------------------------------------------

// GetThreadBySlugWithDetails returns a thread plus joined author + space data.
func (s *Service) GetThreadBySlugWithDetails(ctx context.Context, slug string) (gen.GetForumThreadBySlugWithDetailsRow, error) {
	return s.store.getThreadBySlugWithDetails(ctx, slug)
}

// GetTopReplyForThread returns the highest-voted non-deleted reply for a
// thread, used as the accepted-answer candidate for QAPage JSON-LD on solved
// threads (BL-012). Returns pgx.ErrNoRows when the thread has no replies.
func (s *Service) GetTopReplyForThread(ctx context.Context, threadID pgtype.UUID) (gen.GetTopReplyForThreadRow, error) {
	return s.store.read.GetTopReplyForThread(ctx,
		threadID,
	)
}

// GetReplyByIDWithAuthor returns a single reply with author details.
// Used for the accepted-answer in QAPage JSON-LD (BL-012).
func (s *Service) GetReplyByIDWithAuthor(ctx context.Context, replyID pgtype.UUID) (gen.GetForumReplyByIDWithAuthorRow, error) {
	return s.store.read.GetForumReplyByIDWithAuthor(ctx,
		replyID,
	)
}

// ListThreadsByPlatformWithDetails returns paginated threads with author + space.
// BUG-011: supports ?solved and ?sort=top|latest filters and keyset cursors.
func (s *Service) ListThreadsByPlatformWithDetails(ctx context.Context, solved *bool, sortBy string, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, cursorVoteScore *int32, limit int32) ([]gen.ListForumThreadsWithDetailsRow, error) {
	return s.store.listThreadsByPlatformWithDetails(ctx, solved, sortBy, cursorCreatedAt, cursorID, cursorVoteScore, limit)
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
func (s *Service) ListThreadsBySpaceWithDetails(ctx context.Context, spaceID pgtype.UUID, solved *bool, sortBy string, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, cursorVoteScore *int32, limit int32) ([]gen.ListForumThreadsBySpaceWithDetailsRow, error) {
	return s.store.listThreadsBySpaceWithDetails(ctx, spaceID, solved, sortBy, cursorCreatedAt, cursorID, cursorVoteScore, limit)
}

// ListThreadsByAuthorWithDetails returns paginated threads for an author with author + space.
func (s *Service) ListThreadsByAuthorWithDetails(ctx context.Context, authorUserID pgtype.UUID, solved *bool, sortBy string, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, cursorVoteScore *int32, limit int32) ([]gen.ListForumThreadsByAuthorWithDetailsRow, error) {
	return s.store.listThreadsByAuthorWithDetails(ctx, authorUserID, solved, sortBy, cursorCreatedAt, cursorID, cursorVoteScore, limit)
}

// ListThreadsByTagWithDetails returns paginated threads for a tag with author + space.
func (s *Service) ListThreadsByTagWithDetails(ctx context.Context, tagID pgtype.UUID, solved *bool, sortBy string, cursorCreatedAt pgtype.Timestamptz, cursorID pgtype.UUID, cursorVoteScore *int32, limit int32) ([]gen.ListForumThreadsByTagWithDetailsRow, error) {
	return s.store.listThreadsByTagWithDetails(ctx, tagID, solved, sortBy, cursorCreatedAt, cursorID, cursorVoteScore, limit)
}

// ListRepliesByThreadWithAuthor returns all replies for a thread with author data.
func (s *Service) ListRepliesByThreadWithAuthor(ctx context.Context, threadID pgtype.UUID) ([]gen.ListForumRepliesByThreadWithAuthorRow, error) {
	return s.store.listRepliesByThreadWithAuthor(ctx, threadID)
}
