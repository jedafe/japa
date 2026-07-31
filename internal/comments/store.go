package comments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/notifications"
)

// ---------------------------------------------------------------------------
// Comments CRUD
// ---------------------------------------------------------------------------

// CreateCommentInput holds the parameters for creating a new comment.
type CreateCommentInput struct {
	ContentID pgtype.UUID
	ParentID  pgtype.UUID
	Body      string
}

// CreateComment creates a new comment or reply. The depth is computed from the
// parent comment's depth (0 for top-level).
func (s *Service) CreateComment(ctx context.Context, tenantID, userID pgtype.UUID, input CreateCommentInput) (gen.Comment, error) {
	var depth int32

	if input.ParentID.Valid {
		parent, err := s.read.GetComment(ctx, gen.GetCommentParams{
			ID:       input.ParentID,
			TenantID: tenantID,
		})
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
		TenantID:     tenantID,
		ContentID:    input.ContentID,
		AuthorUserID: userID,
		ParentID:     parentID,
		Body:         input.Body,
		Depth:        depth,
	})
	if err != nil {
		return gen.Comment{}, fmt.Errorf("creating comment: %w", err)
	}

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
		content, err := s.read.GetContentByID(ctx, gen.GetContentByIDParams{
			ID:       comment.ContentID,
			TenantID: tenantID,
		})
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
		parent, err := s.read.GetComment(ctx, gen.GetCommentParams{
			ID:       comment.ParentID,
			TenantID: tenantID,
		})
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
	mentionedUsers, err := mentions.ExtractMentions([]byte(comment.Body))
	if err != nil {
		// Body may not be valid TipTap JSON (e.g., plain text); skip mentions.
		return
	}

	if s.mentionCheckers != nil {
		// Fetch the content to get its spaceID for private-space filtering.
		content, err := s.read.GetContentByID(ctx, gen.GetContentByIDParams{
			ID:       comment.ContentID,
			TenantID: tenantID,
		})
		if err != nil {
			slog.Warn("comments: failed to fetch content for mention filtering",
				"content_id", comment.ContentID.String(), "error", err)
		} else {
			isPrivateSpace := false
			if content.SpaceID.Valid {
				space, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
					ID:       content.SpaceID,
					TenantID: tenantID,
				})
				if err == nil && space.Visibility != "public" {
					isPrivateSpace = true
				}
			}

			mentionedUsers, err = mentions.FilterMentions(
				ctx, mentionedUsers, tenantID, comment.AuthorUserID, content.SpaceID, isPrivateSpace,
				s.mentionCheckers, s.mentionCheckers, s.mentionCheckers,
			)
			if err != nil {
				slog.Warn("comments: FilterMentions failed, dispatching unfiltered",
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
func (s *Service) GetComment(ctx context.Context, tenantID, commentID pgtype.UUID) (gen.GetCommentRow, error) {
	comment, err := s.read.GetComment(ctx, gen.GetCommentParams{
		ID:       commentID,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetCommentRow{}, fmt.Errorf("getting comment: %w", err)
	}

	return comment, nil
}

// ListCommentsByContent returns top-level comments for the given content.
func (s *Service) ListCommentsByContent(ctx context.Context, tenantID, contentID pgtype.UUID, limit, offset int32) ([]gen.ListCommentsByContentRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	comments, err := s.read.ListCommentsByContent(ctx, gen.ListCommentsByContentParams{
		ContentID:  contentID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing comments: %w", err)
	}

	return comments, nil
}

// ListRepliesByParent returns replies for a given parent comment.
func (s *Service) ListRepliesByParent(ctx context.Context, tenantID, parentID pgtype.UUID, limit, offset int32) ([]gen.ListRepliesByParentRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	replies, err := s.read.ListRepliesByParent(ctx, gen.ListRepliesByParentParams{
		ParentID:   parentID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing replies: %w", err)
	}

	return replies, nil
}

// UpdateComment updates a comment's body. Only the author can edit (enforced
// by the sqlc UpdateComment WHERE clause). The edit is rejected if the
// `comments.edit_window_minutes` window has elapsed since creation (BL-009).
func (s *Service) UpdateComment(ctx context.Context, tenantID, commentID, userID pgtype.UUID, body string) (gen.Comment, error) {
	// Edit-window enforcement (BL-009 / Section 49). Fetch the comment first
	// to read created_at. GetComment joins users/user_profiles, so it returns
	// a GetCommentRow; we only need the CreatedAt + AuthorUserID fields.
	existing, err := s.read.GetComment(ctx, gen.GetCommentParams{
		ID:       commentID,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.Comment{}, fmt.Errorf("getting comment for edit-window check: %w", err)
	}

	// Only enforce the window for the author. The sqlc query already rejects
	// non-authors (no rows), so this guard is belt-and-suspenders.
	if existing.AuthorUserID.Valid && existing.AuthorUserID.Bytes == userID.Bytes &&
		existing.CreatedAt.Valid {
		windowMinutes := defaultEditWindowMinutes
		if s.settings != nil {
			windowMinutes = s.settings.GetIntOr(ctx, tenantID, "comments.edit_window_minutes", defaultEditWindowMinutes)
		}
		if windowMinutes > 0 {
			deadline := existing.CreatedAt.Time.Add(time.Duration(windowMinutes) * time.Minute)
			if time.Now().After(deadline) {
				return gen.Comment{}, ErrEditWindowExpired
			}
		}
	}

	comment, err := s.write.UpdateComment(ctx, gen.UpdateCommentParams{
		ID:           commentID,
		TenantID:     tenantID,
		AuthorUserID: userID,
		Body:         body,
	})
	if err != nil {
		return gen.Comment{}, fmt.Errorf("updating comment: %w", err)
	}

	return comment, nil
}

// SoftDeleteComment soft-deletes a comment by setting deleted_at.
func (s *Service) SoftDeleteComment(ctx context.Context, tenantID, commentID pgtype.UUID) error {
	if err := s.write.SoftDeleteComment(ctx, gen.SoftDeleteCommentParams{
		ID:       commentID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting comment: %w", err)
	}

	return nil
}

// PinComment pins a comment (only content author or moderator).
func (s *Service) PinComment(ctx context.Context, tenantID, commentID pgtype.UUID) error {
	if err := s.write.PinComment(ctx, gen.PinCommentParams{
		ID:       commentID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("pinning comment: %w", err)
	}

	return nil
}

// UnpinComment unpins a comment.
func (s *Service) UnpinComment(ctx context.Context, tenantID, commentID pgtype.UUID) error {
	if err := s.write.UnpinComment(ctx, gen.UnpinCommentParams{
		ID:       commentID,
		TenantID: tenantID,
	}); err != nil {
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
		TenantID:  tenantID,
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

	if err := s.write.RecalculateCommentVoteScore(ctx, commentID); err != nil {
		return fmt.Errorf("recalculating comment vote score: %w", err)
	}

	return nil
}

// RemoveCommentVote removes a user's vote and recalculates the score.
func (s *Service) RemoveCommentVote(ctx context.Context, commentID, userID pgtype.UUID) error {
	if err := s.write.DeleteCommentVote(ctx, gen.DeleteCommentVoteParams{
		CommentID: commentID,
		UserID:    userID,
	}); err != nil {
		return fmt.Errorf("removing comment vote: %w", err)
	}

	if err := s.write.RecalculateCommentVoteScore(ctx, commentID); err != nil {
		return fmt.Errorf("recalculating comment vote score: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Counts
// ---------------------------------------------------------------------------

// CountCommentsByContent returns the total comment count for content.
func (s *Service) CountCommentsByContent(ctx context.Context, tenantID, contentID pgtype.UUID) (int32, error) {
	count, err := s.read.CountCommentsByContent(ctx, gen.CountCommentsByContentParams{
		ContentID: contentID,
		TenantID:  tenantID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting comments: %w", err)
	}

	return count, nil
}
