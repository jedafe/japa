package comments

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
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
			return gen.Comment{}, errors.New("maximum comment depth exceeded (max 3)")
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

// GetComment returns a single comment by ID.
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

// UpdateComment updates a comment's body. Only the author can edit.
func (s *Service) UpdateComment(ctx context.Context, tenantID, commentID, userID pgtype.UUID, body string) (gen.Comment, error) {
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
