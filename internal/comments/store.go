package comments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
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

// CreateComment creates a new comment or reply. The depth is computed from the
// parent comment's depth (0 for top-level).
func (s *Service) CreateComment(ctx context.Context, tenantID, userID pgtype.UUID, input CreateCommentInput) (gen.Comment, error) {
	var depth int32

	if input.ParentID.Valid {
		parent, err := s.read.GetComment(ctx,
			input.ParentID,
		)
		if err != nil {
			return gen.Comment{}, fmt.Errorf("getting parent comment for depth: %w", err)
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

	comment, err := s.write.CreateComment(ctx, gen.CreateCommentParams{

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
	if err := s.write.IncrementContentCommentCount(ctx,
		input.ContentID,
	); err != nil {
		slog.Warn("comments: failed to increment content comment_count",
			"content_id", input.ContentID.String(), "error", err)
	}

	// Keep the parent comment's denormalized reply_count in sync (Section 49,
	// BUG-008). Fire-and-forget: a failure only logs.
	if input.ParentID.Valid {
		if err := s.write.IncrementCommentReplyCount(ctx,
			input.ParentID,
		); err != nil {
			slog.Warn("comments: failed to increment parent reply_count",
				"parent_id", input.ParentID.String(), "error", err)
		}
	}

	// §64: record the comment.created activity so badge/challenge triggers
	// (e.g. first_comment) can fire. Anonymous comments still carry
	// author_user_id — anonymity is a display concern, not attribution.
	s.emitActivity(ctx, tenantID, userID, wallet.ActivityKeyCommentCreated, "comment:"+comment.ID.String(), map[string]interface{}{
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
func (s *Service) DispatchCommentNotifications(ctx context.Context, tenantID pgtype.UUID, comment gen.Comment, dispatcher NotificationDispatcher) {
	if dispatcher == nil {
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
				ctx, tenantID, content.AuthorID,
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
				ctx, tenantID, parent.AuthorUserID,
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
				ctx, mentionedUsers, tenantID, comment.AuthorUserID, content.SpaceID, isPrivateSpace,
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
			ctx, tenantID, m.ID,
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
func (s *Service) DispatchMentionNotifications(ctx context.Context, tenantID pgtype.UUID, comment gen.Comment, dispatcher NotificationDispatcher) {
	if dispatcher == nil {
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
				ctx, mentionedUsers, tenantID, comment.AuthorUserID, content.SpaceID, isPrivateSpace,
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
			ctx, tenantID, m.ID,
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
func (s *Service) GetContentSpaceID(ctx context.Context, tenantID, contentID pgtype.UUID) (pgtype.UUID, error) {
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
func (s *Service) GetCommentContentAuthorID(ctx context.Context, tenantID, commentID pgtype.UUID) (pgtype.UUID, error) {
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
func (s *Service) GetComment(ctx context.Context, tenantID, commentID pgtype.UUID) (gen.GetCommentRow, error) {
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
func (s *Service) ListCommentsByContent(ctx context.Context, tenantID, contentID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]gen.ListCommentsByContentRow, error) {
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

// ListRepliesByParent returns replies for a given parent comment.
func (s *Service) ListRepliesByParent(ctx context.Context, tenantID, parentID, viewerID pgtype.UUID, limit, offset int32) ([]gen.ListRepliesByParentRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	replies, err := s.read.ListRepliesByParent(ctx, gen.ListRepliesByParentParams{
		ViewerID: viewerID,
		ParentID: parentID,

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing replies: %w", err)
	}

	return replies, nil
}

// UpdateComment updates a comment's body. Only the author can edit (enforced
// by the sqlc WHERE clause). Section 49 Edit Window: edits are ALWAYS allowed
// — within the `comments.edit_window_minutes` grace period the edit is
// "clean" (is_edited not set); after the window the edit still succeeds but
// is_edited = true and edited_at are stamped (BL-001).
func (s *Service) UpdateComment(ctx context.Context, tenantID, commentID, userID pgtype.UUID, body []byte) (gen.Comment, error) {
	// Edit-window threshold (BL-001 / Section 49). Fetch the comment first
	// to read created_at. GetComment joins users/user_profiles, so it returns
	// a GetCommentRow; we only need the CreatedAt field.
	existing, err := s.read.GetComment(ctx,
		commentID,
	)
	if err != nil {
		return gen.Comment{}, fmt.Errorf("getting comment for edit-window check: %w", err)
	}

	// Determine whether this edit falls within the grace period. The sqlc
	// queries already reject non-authors (no rows), so the author check is
	// belt-and-suspenders.
	withinWindow := true

	if existing.CreatedAt.Valid {
		windowMinutes := defaultEditWindowMinutes
		if s.settings != nil {
			windowMinutes = s.settings.GetIntOr(ctx, pgtype.UUID{}, "comments.edit_window_minutes", defaultEditWindowMinutes)
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

// SoftDeleteComment soft-deletes a comment by setting deleted_at. This is the
// moderator path (comments.delete_any) — no author check. Authors must use
// SoftDeleteCommentByAuthor instead (Section 49, BUG-003).
func (s *Service) SoftDeleteComment(ctx context.Context, tenantID, commentID pgtype.UUID) error {
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

	if err := s.write.SoftDeleteComment(ctx,
		commentID,
	); err != nil {
		return fmt.Errorf("deleting comment: %w", err)
	}

	s.decrementCommentCounters(ctx, comment)

	return nil
}

// SoftDeleteCommentByAuthor soft-deletes a comment only when author_user_id
// matches. The author check is enforced atomically in the sqlc WHERE clause
// so a comments.delete_own holder cannot delete another author's comment
// (BUG-003). Returns errpkg.ErrNotFound when the comment does not exist or the
// user is not the author.
func (s *Service) SoftDeleteCommentByAuthor(ctx context.Context, tenantID, commentID, userID pgtype.UUID) error {
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

	if err := s.write.SoftDeleteCommentByAuthor(ctx, gen.SoftDeleteCommentByAuthorParams{
		ID: commentID,

		AuthorUserID: userID,
	}); err != nil {
		return fmt.Errorf("deleting own comment: %w", err)
	}

	s.decrementCommentCounters(ctx, comment)

	return nil
}

// RemoveComment marks a comment as removed by a moderator (Section 49 —
// status='removed' + removed_by + removed_reason). Unlike a soft-delete, the
// row remains visible for audit and is excluded from public queries by the
// status filter. The reason is optional (empty string → NULL). Used by the
// admin moderation surface (BUG-010).
func (s *Service) RemoveComment(ctx context.Context, tenantID, commentID, moderatorID pgtype.UUID, reason string) error {
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

	if err := s.write.RemoveComment(ctx, gen.RemoveCommentParams{
		ID: commentID,

		RemovedBy:     moderatorID,
		RemovedReason: reasonPtr,
	}); err != nil {
		return fmt.Errorf("removing comment: %w", err)
	}

	s.decrementCommentCounters(ctx, comment)

	return nil
}

// decrementCommentCounters keeps the denormalized content.comment_count and
// the parent comment's reply_count in sync when a published comment is
// soft-deleted or removed. Failures are logged, not returned, so the request
// still succeeds (Rule 8).
func (s *Service) decrementCommentCounters(ctx context.Context, comment gen.GetCommentRow) {
	if err := s.write.DecrementContentCommentCount(ctx,
		comment.ContentID,
	); err != nil {
		slog.Warn("comments: failed to decrement content comment_count",
			"content_id", comment.ContentID.String(), "error", err)
	}

	if comment.ParentID.Valid {
		if err := s.write.DecrementCommentReplyCount(ctx,
			comment.ParentID,
		); err != nil {
			slog.Warn("comments: failed to decrement parent reply_count",
				"parent_id", comment.ParentID.String(), "error", err)
		}
	}
}

// PinComment pins a comment (only content author or moderator).
func (s *Service) PinComment(ctx context.Context, tenantID, commentID pgtype.UUID) error {
	if err := s.write.PinComment(ctx,
		commentID,
	); err != nil {
		return fmt.Errorf("pinning comment: %w", err)
	}

	return nil
}

// UnpinComment unpins a comment.
func (s *Service) UnpinComment(ctx context.Context, tenantID, commentID pgtype.UUID) error {
	if err := s.write.UnpinComment(ctx,
		commentID,
	); err != nil {
		return fmt.Errorf("unpinning comment: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Comment Votes
// ---------------------------------------------------------------------------

// VoteComment upserts a vote on a comment and recalculates the vote score.
func (s *Service) VoteComment(ctx context.Context, tenantID, commentID, userID pgtype.UUID, value int16) error {
	if value != 1 && value != -1 {
		return errors.New("vote value must be 1 or -1")
	}

	if _, err := s.write.UpsertCommentVote(ctx, gen.UpsertCommentVoteParams{

		CommentID: commentID,
		UserID:    userID,
		Value:     value,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("duplicate vote: %w", errpkg.ErrDuplicate)
		}

		return fmt.Errorf("voting on comment: %w", err)
	}

	if err := s.write.RecalculateCommentVoteScore(ctx,
		commentID,
	); err != nil {
		return fmt.Errorf("recalculating comment vote score: %w", err)
	}

	return nil
}

// RemoveCommentVote removes a user's vote and recalculates the score.
// Tenant-scoped per Guide §4.2/§9 (SEC-001).
func (s *Service) RemoveCommentVote(ctx context.Context, tenantID, commentID, userID pgtype.UUID) error {
	if err := s.write.DeleteCommentVote(ctx, gen.DeleteCommentVoteParams{

		CommentID: commentID,
		UserID:    userID,
	}); err != nil {
		return fmt.Errorf("removing comment vote: %w", err)
	}

	if err := s.write.RecalculateCommentVoteScore(ctx,
		commentID,
	); err != nil {
		return fmt.Errorf("recalculating comment vote score: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Counts
// ---------------------------------------------------------------------------

// CountCommentsByContent returns the total comment count for content.
func (s *Service) CountCommentsByContent(ctx context.Context, tenantID, contentID pgtype.UUID) (int32, error) {
	count, err := s.read.CountCommentsByContent(ctx,
		contentID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting comments: %w", err)
	}

	return count, nil
}
