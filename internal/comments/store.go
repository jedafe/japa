package comments

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/wallet"
)

// ---------------------------------------------------------------------------
// Comments CRUD
// ---------------------------------------------------------------------------

// CreateCommentInput holds the parameters for creating a new comment.
type CreateCommentInput struct {
	ContentID   pgtype.UUID
	ParentID    pgtype.UUID
	Body        []byte // TipTap JSON document (jsonb column)
	IsAnonymous bool
}

// CanViewComments reports whether the viewer may read or create comments on
// the given content (C-001/C-002). It mirrors the detail-page visibility
// predicate (IsContentVisibleToUser) plus the members-only-needs-auth nuance:
// the session list route serves anonymous guests, for whom 'members' content
// comments must stay hidden. Invisible or absent content must surface as 404
// (never 403) so callers cannot probe restricted content.
func (s *Service) CanViewComments(ctx context.Context, contentID, viewerID pgtype.UUID, authed bool) (bool, error) {
	visible, err := s.read.CanViewComments(ctx, gen.CanViewCommentsParams{
		ContentID: contentID,
		UserID:    viewerID,
		Authed:    authed,
	})
	if err != nil {
		return false, fmt.Errorf("checking comment visibility: %w", err)
	}

	return visible, nil
}

// CreateComment creates a new comment or reply. The depth is computed from the
// parent comment's depth (0 for top-level). The comment insert and the
// denormalized counters (contents.comment_count, parent reply_count) commit
// in one transaction so a partial failure cannot drift the counts (C-010).
func (s *Service) CreateComment(ctx context.Context, userID pgtype.UUID, input CreateCommentInput) (gen.Comment, error) {
	// C-002: reject comments on content the author cannot see (drafts,
	// deleted, members-only for the wrong audience, private) and on
	// nonexistent content — 404, never 403.
	visible, err := s.CanViewComments(ctx, input.ContentID, userID, true)
	if err != nil {
		return gen.Comment{}, err
	}

	if !visible {
		return gen.Comment{}, fmt.Errorf("%w: content", errpkg.ErrNotFound)
	}

	var depth int32

	if input.ParentID.Valid {
		parent, err := s.read.GetComment(ctx,
			input.ParentID,
		)
		if err != nil {
			return gen.Comment{}, fmt.Errorf("getting parent comment for depth: %w", err)
		}

		// C-005: the reply target must be a live comment on the SAME content
		// item — otherwise the reply attaches to a foreign thread and the
		// denormalized counters split across two contents.
		if parent.ContentID != input.ContentID ||
			parent.DeletedAt.Valid ||
			parent.Status != "published" {
			return gen.Comment{}, fmt.Errorf("%w: parent comment", errpkg.ErrNotFound)
		}

		depth = parent.Depth + 1
		if depth > 3 {
			return gen.Comment{}, fmt.Errorf("comment depth %d exceeds maximum 3: %w", depth, errpkg.ErrMaxDepth)
		}
	}

	var parentID pgtype.UUID
	if input.ParentID.Valid {
		parentID = input.ParentID
	}

	comment, err := db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (gen.Comment, error) {
		c, err := q.CreateComment(ctx, gen.CreateCommentParams{
			ContentID:    input.ContentID,
			AuthorUserID: userID,
			ParentID:     parentID,
			Body:         input.Body,
			Depth:        depth,
			IsAnonymous:  input.IsAnonymous,
		})
		if err != nil {
			return gen.Comment{}, fmt.Errorf("creating comment: %w", err)
		}

		// Increment the denormalized comment_count on the content row so feed
		// cards (which read contents.comment_count) show accurate counts
		// immediately, without waiting for the async scoring pipeline.
		// Ref: Section 32, Guide §21.
		if err := q.IncrementContentCommentCount(ctx,
			input.ContentID,
		); err != nil {
			return gen.Comment{}, fmt.Errorf("incrementing content comment_count: %w", err)
		}

		// Keep the parent comment's denormalized reply_count in sync
		// (Section 49, BUG-008).
		if input.ParentID.Valid {
			if err := q.IncrementCommentReplyCount(ctx,
				input.ParentID,
			); err != nil {
				return gen.Comment{}, fmt.Errorf("incrementing parent reply_count: %w", err)
			}
		}

		return c, nil
	})
	if err != nil {
		return gen.Comment{}, err
	}

	// §64: record the comment.created activity so badge/challenge triggers
	// (e.g. first_comment) can fire. Anonymous comments still carry
	// author_user_id — anonymity is a display concern, not attribution.
	s.emitActivity(ctx, userID, wallet.ActivityKeyCommentCreated, "comment:"+comment.ID.String(), map[string]interface{}{
		"content_id": input.ContentID.String(),
	})

	return comment, nil
}

// DispatchCommentNotifications fires notifications for a newly created comment
// (Section 49 — comment:notify_author/parent/mention, consolidated into
// notification:dispatch_batch per Section 11). Fire-and-forget per Rule 8:
// errors are logged but never returned to the caller.
//
// Notifications dispatched:
//  1. content.comment — to the content author (if commenter ≠ author, top-level)
//  2. comment.reply — to the parent comment author (if reply and replier ≠ parent author)
//  3. mention.comment — to each @mentioned user (extracted from TipTap JSON body)
func (s *Service) DispatchCommentNotifications(ctx context.Context, comment gen.Comment, dispatcher NotificationDispatcher) {
	if dispatcher == nil {
		return
	}

	// C-003: anonymous comments never generate actor-attributed notifications.
	// The notification pipeline renders the real actor_id to recipients, which
	// would de-anonymize the commenter to the content/parent/mentioned authors.
	if comment.IsAnonymous {
		return
	}

	// 1. Notify content author (for top-level comments only).
	if !comment.ParentID.Valid {
		content, err := s.read.GetContentByID(ctx,
			comment.ContentID,
		)
		if err != nil {
			slog.Warn("comments: failed to fetch content for author notification", "content_id", comment.ContentID.String(), "error", err)
		} else if content.AuthorID.Valid && content.AuthorID != comment.AuthorUserID {
			if err := dispatcher.DispatchNotification(
				ctx, content.AuthorID,
				notifications.TypeContentComment, "",
				comment.AuthorUserID,
				"comment", comment.ID,
				"", "", "",
			); err != nil {
				slog.Warn("comments: failed to dispatch content.comment notification", "error", err)
			}
		}
	}

	// 2. Notify parent comment author (for replies).
	if comment.ParentID.Valid {
		parent, err := s.read.GetComment(ctx,
			comment.ParentID,
		)
		if err != nil {
			slog.Warn("comments: failed to fetch parent for reply notification", "parent_id", comment.ParentID.String(), "error", err)
		} else if parent.AuthorUserID.Valid && parent.AuthorUserID != comment.AuthorUserID {
			if err := dispatcher.DispatchNotification(
				ctx, parent.AuthorUserID,
				notifications.TypeCommentReply, "",
				comment.AuthorUserID,
				"comment", comment.ID,
				"", "", "",
			); err != nil {
				slog.Warn("comments: failed to dispatch comment.reply notification", "error", err)
			}
		}
	}

	// 3. Notify @mentioned users (extracted from TipTap JSON body).
	// Apply privacy filtering when DB checkers are wired (BUG-004):
	// skip erased users (GDPR), blocked users (privacy), and non-members
	// of private spaces (visibility leak). Section 15 mention edge cases.
	mentionedUsers, err := mentions.ExtractMentions(comment.Body)
	if err != nil {
		// Body may not be valid TipTap JSON (e.g., plain text); skip mentions.
		return
	}

	if s.mentionCheckers != nil {
		// Fetch the content to get its spaceID for private-space filtering.
		content, err := s.read.GetContentByID(ctx,
			comment.ContentID,
		)
		if err != nil {
			slog.Warn("comments: failed to fetch content for mention filtering",
				"content_id", comment.ContentID.String(), "error", err)
		} else {
			isPrivateSpace := false

			if content.SpaceID.Valid {
				space, err := s.read.GetSpaceByID(ctx,
					content.SpaceID,
				)
				if err == nil && space.Visibility != "public" {
					isPrivateSpace = true
				}
			}

			mentionedUsers, err = mentions.FilterMentions(
				ctx, mentionedUsers, comment.AuthorUserID, content.SpaceID, isPrivateSpace,
				s.mentionCheckers, s.mentionCheckers, s.mentionCheckers,
			)
			if err != nil {
				// FilterMentions logs individual checker errors internally and
				// never returns a fatal error. This branch is retained for
				// forward-compatibility but is not expected to fire.
				slog.Warn("comments: FilterMentions returned unexpected error",
					"comment_id", comment.ID.String(), "error", err)
			}
		}
	}

	for _, m := range mentionedUsers {
		if m.EntityType != "user" || !m.ID.Valid || m.ID == comment.AuthorUserID {
			continue
		}

		if err := dispatcher.DispatchNotification(
			ctx, m.ID,
			"mention.comment", "",
			comment.AuthorUserID,
			"comment", comment.ID,
			"", "", "",
		); err != nil {
			slog.Warn("comments: failed to dispatch mention.comment notification", "user_id", m.ID.String(), "error", err)
		}
	}
}

// DispatchMentionNotifications fires only @mention notifications for a comment
// edit (BL-006). Unlike DispatchCommentNotifications, this skips the
// content.comment and comment.reply notifications which are "new comment"
// events and should not fire on edit. The notification batch system
// deduplicates by (recipient, type, subject), so newly added mentions on edit
// are dispatched while pre-existing mentions are suppressed.
func (s *Service) DispatchMentionNotifications(ctx context.Context, comment gen.Comment, dispatcher NotificationDispatcher) {
	if dispatcher == nil {
		return
	}

	// C-003: anonymous comments never generate actor-attributed notifications.
	if comment.IsAnonymous {
		return
	}

	mentionedUsers, err := mentions.ExtractMentions(comment.Body)
	if err != nil {
		return
	}

	if s.mentionCheckers != nil {
		content, err := s.read.GetContentByID(ctx,
			comment.ContentID,
		)
		if err != nil {
			slog.Warn("comments: failed to fetch content for mention filtering",
				"content_id", comment.ContentID.String(), "error", err)
		} else {
			isPrivateSpace := false

			if content.SpaceID.Valid {
				space, err := s.read.GetSpaceByID(ctx,
					content.SpaceID,
				)
				if err == nil && space.Visibility != "public" {
					isPrivateSpace = true
				}
			}

			mentionedUsers, err = mentions.FilterMentions(
				ctx, mentionedUsers, comment.AuthorUserID, content.SpaceID, isPrivateSpace,
				s.mentionCheckers, s.mentionCheckers, s.mentionCheckers,
			)
			if err != nil {
				slog.Warn("comments: FilterMentions returned unexpected error",
					"comment_id", comment.ID.String(), "error", err)
			}
		}
	}

	for _, m := range mentionedUsers {
		if m.EntityType != "user" || !m.ID.Valid || m.ID == comment.AuthorUserID {
			continue
		}

		if err := dispatcher.DispatchNotification(
			ctx, m.ID,
			"mention.comment", "",
			comment.AuthorUserID,
			"comment", comment.ID,
			"", "", "",
		); err != nil {
			slog.Warn("comments: failed to dispatch mention.comment notification on edit", "user_id", m.ID.String(), "error", err)
		}
	}
}

// GetContentSpaceID returns the space_id of the content that a comment is
// posted on. Returns pgtype.UUID{} with Valid=false if the content has no
// space. Used by the handler to enforce the space's commenting_perm and
// replying_perm axes (Section 05 lines 145-154, BUG-002).
func (s *Service) GetContentSpaceID(ctx context.Context, contentID pgtype.UUID) (pgtype.UUID, error) {
	content, err := s.read.GetContentByID(ctx,
		contentID,
	)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting content for space_id: %w", err)
	}

	return content.SpaceID, nil
}

// GetCommentContentAuthorID returns the author_user_id of the parent content
// for the given comment. Used by PinComment to allow the content author to
// pin comments on their own content (Section 49, BL-003).
func (s *Service) GetCommentContentAuthorID(ctx context.Context, commentID pgtype.UUID) (pgtype.UUID, error) {
	comment, err := s.read.GetComment(ctx,
		commentID,
	)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting comment for content author check: %w", err)
	}

	content, err := s.read.GetContentByID(ctx,
		comment.ContentID,
	)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting content for content author check: %w", err)
	}

	return content.AuthorID, nil
}

// GetComment is an exported method.
func (s *Service) GetComment(ctx context.Context, commentID pgtype.UUID) (gen.GetCommentRow, error) {
	comment, err := s.read.GetComment(ctx,
		commentID,
	)
	if err != nil {
		return gen.GetCommentRow{}, fmt.Errorf("getting comment: %w", err)
	}

	return comment, nil
}

// ListCommentsByContent returns top-level comments for the given content,
// honoring the Section 49 ?sort=top|new|oldest query parameter. "top" (the
// default) orders by pin then vote score then recency; "new" by recency;
// "oldest" by ascending creation time. Pinned comments always surface first.
func (s *Service) ListCommentsByContent(ctx context.Context, contentID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]gen.ListCommentsByContentRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	switch sort {
	case "new":
		rows, err := s.read.ListCommentsByContentNew(ctx, gen.ListCommentsByContentNewParams{
			ViewerID:  viewerID,
			ContentID: contentID,

			PageLimit:  limit,
			PageOffset: offset,
		})
		if err != nil {
			return nil, fmt.Errorf("listing comments (new): %w", err)
		}

		return s.coerceNewRows(rows), nil

	case "oldest":
		rows, err := s.read.ListCommentsByContentOldest(ctx, gen.ListCommentsByContentOldestParams{
			ViewerID:  viewerID,
			ContentID: contentID,

			PageLimit:  limit,
			PageOffset: offset,
		})
		if err != nil {
			return nil, fmt.Errorf("listing comments (oldest): %w", err)
		}

		return s.coerceOldestRows(rows), nil

	default: // "top" and any unrecognized value
		comments, err := s.read.ListCommentsByContent(ctx, gen.ListCommentsByContentParams{
			ViewerID:  viewerID,
			ContentID: contentID,

			PageLimit:  limit,
			PageOffset: offset,
		})
		if err != nil {
			return nil, fmt.Errorf("listing comments: %w", err)
		}

		return comments, nil
	}
}

// coerceNewRows / coerceOldestRows adapt the per-sort row types to the
// canonical ListCommentsByContentRow so callers (handler + frontend) see one
// shape regardless of sort. The field sets are identical by construction.
func (s *Service) coerceNewRows(rows []gen.ListCommentsByContentNewRow) []gen.ListCommentsByContentRow {
	out := make([]gen.ListCommentsByContentRow, len(rows))
	for i := range rows {
		out[i] = gen.ListCommentsByContentRow{
			ID: rows[i].ID,

			ContentID:     rows[i].ContentID,
			AuthorUserID:  rows[i].AuthorUserID,
			ParentID:      rows[i].ParentID,
			Body:          rows[i].Body,
			Depth:         rows[i].Depth,
			VoteScore:     rows[i].VoteScore,
			UpvoteCount:   rows[i].UpvoteCount,
			DownvoteCount: rows[i].DownvoteCount,
			IsPinned:      rows[i].IsPinned,
			IsEdited:      rows[i].IsEdited,
			EditedAt:      rows[i].EditedAt,
			CreatedAt:     rows[i].CreatedAt,
			UpdatedAt:     rows[i].UpdatedAt,
			DeletedAt:     rows[i].DeletedAt,
			ReplyCount:    rows[i].ReplyCount,
			UserVote:      rows[i].UserVote,
			IsAnonymous:   rows[i].IsAnonymous,
			Username:      rows[i].Username,
			DisplayName:   rows[i].DisplayName,
			AvatarUrl:     rows[i].AvatarUrl,
		}
	}

	return out
}

func (s *Service) coerceOldestRows(rows []gen.ListCommentsByContentOldestRow) []gen.ListCommentsByContentRow {
	out := make([]gen.ListCommentsByContentRow, len(rows))
	for i := range rows {
		out[i] = gen.ListCommentsByContentRow{
			ID: rows[i].ID,

			ContentID:     rows[i].ContentID,
			AuthorUserID:  rows[i].AuthorUserID,
			ParentID:      rows[i].ParentID,
			Body:          rows[i].Body,
			Depth:         rows[i].Depth,
			VoteScore:     rows[i].VoteScore,
			UpvoteCount:   rows[i].UpvoteCount,
			DownvoteCount: rows[i].DownvoteCount,
			IsPinned:      rows[i].IsPinned,
			IsEdited:      rows[i].IsEdited,
			EditedAt:      rows[i].EditedAt,
			CreatedAt:     rows[i].CreatedAt,
			UpdatedAt:     rows[i].UpdatedAt,
			DeletedAt:     rows[i].DeletedAt,
			ReplyCount:    rows[i].ReplyCount,
			UserVote:      rows[i].UserVote,
			IsAnonymous:   rows[i].IsAnonymous,
			Username:      rows[i].Username,
			DisplayName:   rows[i].DisplayName,
			AvatarUrl:     rows[i].AvatarUrl,
		}
	}

	return out
}

// ListRepliesByParent returns replies for a given parent comment, scoped to
// the parent content (C-005 hardening — a parent_id from another content item
// returns an empty page rather than a foreign thread). Also returns the total
// for pagination meta.
func (s *Service) ListRepliesByParent(ctx context.Context, contentID, parentID, viewerID pgtype.UUID, limit, offset int32) ([]gen.ListRepliesByParentRow, int32, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	replies, err := s.read.ListRepliesByParent(ctx, gen.ListRepliesByParentParams{
		ViewerID:  viewerID,
		ParentID:  parentID,
		ContentID: contentID,

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing replies: %w", err)
	}

	total, err := s.read.CountRepliesByParent(ctx, gen.CountRepliesByParentParams{
		ParentID:  parentID,
		ContentID: contentID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("counting replies: %w", err)
	}

	return replies, total, nil
}

// UpdateComment updates a comment's body. Only the author can edit (enforced
// by the sqlc WHERE clause). Section 49 Edit Window: edits are ALWAYS allowed
// — within the `comments.edit_window_minutes` grace period the edit is
// "clean" (is_edited not set); after the window the edit still succeeds but
// is_edited = true and edited_at are stamped (BL-001).
func (s *Service) UpdateComment(ctx context.Context, commentID, userID pgtype.UUID, body []byte) (gen.Comment, error) {
	// Edit-window threshold (BL-001 / Section 49). Fetch the comment first
	// to read created_at. GetComment joins users/user_profiles, so it returns
	// a GetCommentRow; we only need the CreatedAt field.
	existing, err := s.read.GetComment(ctx,
		commentID,
	)
	if err != nil {
		return gen.Comment{}, fmt.Errorf("getting comment for edit-window check: %w", err)
	}

	// C-018: the comment is only editable while its parent content is
	// visible to the editor — consistent with vote/pin guards.
	visible, err := s.CanViewComments(ctx, existing.ContentID, userID, true)
	if err != nil {
		return gen.Comment{}, err
	}

	if !visible {
		return gen.Comment{}, fmt.Errorf("%w: comment", errpkg.ErrNotFound)
	}

	// Determine whether this edit falls within the grace period. The sqlc
	// queries already reject non-authors (no rows), so the author check is
	// belt-and-suspenders.
	withinWindow := true

	if existing.CreatedAt.Valid {
		windowMinutes := defaultEditWindowMinutes
		if s.settings != nil {
			windowMinutes = s.settings.GetIntOr(ctx, "comments.edit_window_minutes", defaultEditWindowMinutes)
		}

		if windowMinutes > 0 {
			deadline := existing.CreatedAt.Time.Add(time.Duration(windowMinutes) * time.Minute)
			withinWindow = time.Now().Before(deadline)
		}
	}

	// Within the window: clean edit (no is_edited mark).
	// After the window: edit still allowed, but is_edited + edited_at stamped.
	if withinWindow {
		comment, err := s.write.UpdateCommentUnmarked(ctx, gen.UpdateCommentUnmarkedParams{
			ID: commentID,

			AuthorUserID: userID,
			Body:         body,
		})
		if err != nil {
			return gen.Comment{}, fmt.Errorf("updating comment: %w", err)
		}

		return comment, nil
	}

	comment, err := s.write.UpdateComment(ctx, gen.UpdateCommentParams{
		ID: commentID,

		AuthorUserID: userID,
		Body:         body,
	})
	if err != nil {
		return gen.Comment{}, fmt.Errorf("updating comment: %w", err)
	}

	return comment, nil
}

// UpdateCommentModerate is the comments.edit_any path — any published
// comment regardless of author, no edit window. The comment must still be on
// content the moderator can see (C-018).
func (s *Service) UpdateCommentModerate(ctx context.Context, commentID, userID pgtype.UUID, body []byte) (gen.Comment, error) {
	if err := s.checkCommentVisible(ctx, commentID, userID); err != nil {
		return gen.Comment{}, err
	}

	comment, err := s.write.UpdateCommentByID(ctx, gen.UpdateCommentByIDParams{
		ID:   commentID,
		Body: body,
	})
	if err != nil {
		return gen.Comment{}, fmt.Errorf("moderator updating comment: %w", err)
	}

	return comment, nil
}

// SoftDeleteComment soft-deletes a comment by setting deleted_at. This is the
// moderator path (comments.delete_any) — no author check. Authors must use
// SoftDeleteCommentByAuthor instead (Section 49, BUG-003).
func (s *Service) SoftDeleteComment(ctx context.Context, commentID pgtype.UUID) error {
	comment, err := s.read.GetComment(ctx,
		commentID,
	)
	if err != nil {
		return fmt.Errorf("getting comment for delete: %w", err)
	}

	// Only decrement counters for currently-published, non-deleted comments.
	// Removed/flagged comments were already excluded from public counts.
	if comment.DeletedAt.Valid || comment.Status != "published" {
		return nil
	}

	// C-010: the soft-delete and the counter decrements commit atomically.
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if err := q.SoftDeleteComment(ctx,
			commentID,
		); err != nil {
			return fmt.Errorf("deleting comment: %w", err)
		}

		return decrementCommentCountersTx(ctx, q, comment)
	}); err != nil {
		return err
	}

	return nil
}

// SoftDeleteCommentByAuthor soft-deletes a comment only when author_user_id
// matches. The author check is enforced atomically in the sqlc WHERE clause
// so a comments.delete_own holder cannot delete another author's comment
// (BUG-003). Returns errpkg.ErrNotFound when the comment does not exist or the
// user is not the author.
func (s *Service) SoftDeleteCommentByAuthor(ctx context.Context, commentID, userID pgtype.UUID) error {
	comment, err := s.read.GetComment(ctx,
		commentID,
	)
	if err != nil {
		return fmt.Errorf("getting comment for delete: %w", err)
	}

	// Only decrement counters for currently-published, non-deleted comments.
	if comment.DeletedAt.Valid || comment.Status != "published" {
		return nil
	}

	// C-010: the soft-delete and the counter decrements commit atomically.
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if err := q.SoftDeleteCommentByAuthor(ctx, gen.SoftDeleteCommentByAuthorParams{
			ID: commentID,

			AuthorUserID: userID,
		}); err != nil {
			return fmt.Errorf("deleting own comment: %w", err)
		}

		return decrementCommentCountersTx(ctx, q, comment)
	}); err != nil {
		return err
	}

	return nil
}

// RemoveComment marks a comment as removed by a moderator (Section 49 —
// status='removed' + removed_by + removed_reason). Unlike a soft-delete, the
// row remains visible for audit and is excluded from public queries by the
// status filter. The reason is optional (empty string → NULL). Used by the
// admin moderation surface (BUG-010).
func (s *Service) RemoveComment(ctx context.Context, commentID, moderatorID pgtype.UUID, reason string) error {
	comment, err := s.read.GetComment(ctx,
		commentID,
	)
	if err != nil {
		return fmt.Errorf("getting comment for remove: %w", err)
	}

	if comment.DeletedAt.Valid || comment.Status != "published" {
		return nil
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	// C-010: the removal and the counter decrements commit atomically.
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if err := q.RemoveComment(ctx, gen.RemoveCommentParams{
			ID: commentID,

			RemovedBy:     moderatorID,
			RemovedReason: reasonPtr,
		}); err != nil {
			return fmt.Errorf("removing comment: %w", err)
		}

		return decrementCommentCountersTx(ctx, q, comment)
	}); err != nil {
		return err
	}

	return nil
}

// decrementCommentCountersTx keeps the denormalized content.comment_count and
// the parent comment's reply_count in sync when a published comment is
// soft-deleted or removed. Runs inside the caller's transaction so the
// delete/remove and the counters commit or roll back together (C-010).
func decrementCommentCountersTx(ctx context.Context, q *gen.Queries, comment gen.GetCommentRow) error {
	if err := q.DecrementContentCommentCount(ctx,
		comment.ContentID,
	); err != nil {
		return fmt.Errorf("decrementing content comment_count: %w", err)
	}

	if comment.ParentID.Valid {
		if err := q.DecrementCommentReplyCount(ctx,
			comment.ParentID,
		); err != nil {
			return fmt.Errorf("decrementing parent reply_count: %w", err)
		}
	}

	return nil
}

// PinComment pins a comment (only content author or moderator). The pin must
// only be possible on comments the pinner can actually see (C-018).
func (s *Service) PinComment(ctx context.Context, commentID, userID pgtype.UUID) error {
	if err := s.checkCommentVisible(ctx, commentID, userID); err != nil {
		return err
	}

	if err := s.write.PinComment(ctx,
		commentID,
	); err != nil {
		return fmt.Errorf("pinning comment: %w", err)
	}

	return nil
}

// UnpinComment unpins a comment.
func (s *Service) UnpinComment(ctx context.Context, commentID, userID pgtype.UUID) error {
	if err := s.checkCommentVisible(ctx, commentID, userID); err != nil {
		return err
	}

	if err := s.write.UnpinComment(ctx,
		commentID,
	); err != nil {
		return fmt.Errorf("unpinning comment: %w", err)
	}

	return nil
}

// checkCommentVisible resolves a comment's parent content and verifies the
// caller can see it — shared by pin/unpin/update-moderate guards.
func (s *Service) checkCommentVisible(ctx context.Context, commentID, userID pgtype.UUID) error {
	comment, err := s.read.GetComment(ctx, commentID)
	if err != nil {
		return fmt.Errorf("getting comment: %w", err)
	}

	visible, err := s.CanViewComments(ctx, comment.ContentID, userID, true)
	if err != nil {
		return err
	}

	if !visible {
		return fmt.Errorf("%w: comment", errpkg.ErrNotFound)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Comment Votes
// ---------------------------------------------------------------------------

// VoteComment upserts a vote on a comment and recalculates the vote score.
// The target must be a live published comment — votes on deleted, removed, or
// nonexistent comments are rejected (C-007).
func (s *Service) VoteComment(ctx context.Context, commentID, userID pgtype.UUID, value int16) error {
	if value != 1 && value != -1 {
		return fmt.Errorf("vote value must be 1 or -1: %w", errpkg.ErrInvalid)
	}

	target, err := s.read.GetComment(ctx,
		commentID,
	)
	if err != nil {
		return fmt.Errorf("getting comment for vote: %w", err)
	}

	if target.DeletedAt.Valid || target.Status != "published" {
		return fmt.Errorf("%w: comment", errpkg.ErrNotFound)
	}

	// C-014: votes only count on comments the voter can actually see — the
	// comment is only as visible as its parent content.
	visible, err := s.CanViewComments(ctx, target.ContentID, userID, true)
	if err != nil {
		return err
	}

	if !visible {
		return fmt.Errorf("%w: comment", errpkg.ErrNotFound)
	}

	// The vote upsert and the score recalculation commit atomically (C-010).
	return db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if _, err := q.UpsertCommentVote(ctx, gen.UpsertCommentVoteParams{
			CommentID: commentID,
			UserID:    userID,
			Value:     value,
		}); err != nil {
			return fmt.Errorf("voting on comment: %w", err)
		}

		if err := q.RecalculateCommentVoteScore(ctx,
			commentID,
		); err != nil {
			return fmt.Errorf("recalculating comment vote score: %w", err)
		}

		return nil
	})
}

// RemoveCommentVote removes a user's vote and recalculates the score.
// Platform-scoped per Guide §4.2/§9 (SEC-001).
func (s *Service) RemoveCommentVote(ctx context.Context, commentID, userID pgtype.UUID) error {
	return db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if err := q.DeleteCommentVote(ctx, gen.DeleteCommentVoteParams{
			CommentID: commentID,
			UserID:    userID,
		}); err != nil {
			return fmt.Errorf("removing comment vote: %w", err)
		}

		if err := q.RecalculateCommentVoteScore(ctx,
			commentID,
		); err != nil {
			return fmt.Errorf("recalculating comment vote score: %w", err)
		}

		return nil
	})
}

// ---------------------------------------------------------------------------
// Counts
// ---------------------------------------------------------------------------

// CountTopLevelCommentsByContent returns the total number of top-level
// comments for content (tombstones included, matching the list predicate) —
// used for the list endpoint's pagination meta (C-011).
func (s *Service) CountTopLevelCommentsByContent(ctx context.Context, contentID pgtype.UUID) (int32, error) {
	count, err := s.read.CountTopLevelCommentsByContent(ctx,
		contentID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting top-level comments: %w", err)
	}

	return count, nil
}

// CountCommentsByContent returns the total comment count for content.
func (s *Service) CountCommentsByContent(ctx context.Context, contentID pgtype.UUID) (int32, error) {
	count, err := s.read.CountCommentsByContent(ctx,
		contentID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting comments: %w", err)
	}

	return count, nil
}
