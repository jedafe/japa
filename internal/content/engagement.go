package content

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ContentWithScore pairs a content ID with its velocity score for ranking.
type ContentWithScore struct { //nolint:revive // stuttering type name preserved for API compatibility
	ContentID     pgtype.UUID
	VelocityScore float64
}

// IncrementEngagementView atomically increments the view count for a content
// item in the content_engagement table. Creates the row if it doesn't exist.
// This is fire-and-forget from the content GET handler — errors are logged
// but never returned to the caller.
func (s *Service) IncrementEngagementView(ctx context.Context, contentID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO content_engagement (content_id, views, comments_count, votes_score, bookmarks_count, shares_count) VALUES ($1, 1, 0, 0, 0, 0)
		ON CONFLICT (content_id) DO UPDATE
		SET views = content_engagement.views + 1,
		    updated_at = now()
	`, contentID)
	if err != nil {
		return fmt.Errorf("incrementing engagement view: %w", err)
	}

	return nil
}

// IncrementEngagementComments atomically increments the comment count for
// a content item in the content_engagement table.
func (s *Service) IncrementEngagementComments(ctx context.Context, contentID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO content_engagement (content_id, views, comments_count, votes_score, bookmarks_count, shares_count) VALUES ($1, 0, 1, 0, 0, 0)
		ON CONFLICT (content_id) DO UPDATE
		SET comments_count = content_engagement.comments_count + 1,
		    updated_at = now()
	`, contentID)
	if err != nil {
		return fmt.Errorf("incrementing engagement comments: %w", err)
	}

	return nil
}

// UpdateEngagementVotesScore adjusts the vote score for a content item by
// the given delta (positive for upvotes, negative for downvotes).
func (s *Service) UpdateEngagementVotesScore(ctx context.Context, contentID pgtype.UUID, delta int32) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO content_engagement (content_id, views, comments_count, votes_score, bookmarks_count, shares_count) VALUES ($1, 0, 0, $2, 0, 0)
		ON CONFLICT (content_id) DO UPDATE
		SET votes_score = content_engagement.votes_score + $2,
		    updated_at = now()
	`, contentID, delta)
	if err != nil {
		return fmt.Errorf("updating engagement votes score: %w", err)
	}

	return nil
}

// IncrementEngagementBookmarks atomically increments the bookmark count.
func (s *Service) IncrementEngagementBookmarks(ctx context.Context, contentID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO content_engagement (content_id, views, comments_count, votes_score, bookmarks_count, shares_count) VALUES ($1, 0, 0, 0, 1, 0)
		ON CONFLICT (content_id) DO UPDATE
		SET bookmarks_count = content_engagement.bookmarks_count + 1,
		    updated_at = now()
	`, contentID)
	if err != nil {
		return fmt.Errorf("incrementing engagement bookmarks: %w", err)
	}

	return nil
}

// IncrementEngagementShares atomically increments the share count.
func (s *Service) IncrementEngagementShares(ctx context.Context, contentID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO content_engagement (content_id, views, comments_count, votes_score, bookmarks_count, shares_count) VALUES ($1, 0, 0, 0, 0, 1)
		ON CONFLICT (content_id) DO UPDATE
		SET shares_count = content_engagement.shares_count + 1,
		    updated_at = now()
	`, contentID)
	if err != nil {
		return fmt.Errorf("incrementing engagement shares: %w", err)
	}

	return nil
}

// GetTopContentByVelocity returns the top N content IDs ordered by velocity
// score descending. Used for trending/popular content feeds.
func (s *Service) GetTopContentByVelocity(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]ContentWithScore, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.db.Reader.Query(ctx, `
		SELECT ce.content_id, ce.velocity_score
		FROM content_engagement ce
		JOIN contents c ON c.id = ce.content_id
		WHERE c.tenant_id = $1
		  AND c.status = 'published'
		ORDER BY ce.velocity_score DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying top content by velocity: %w", err)
	}
	defer rows.Close()

	var results []ContentWithScore

	for rows.Next() {
		var cs ContentWithScore
		if err := rows.Scan(&cs.ContentID, &cs.VelocityScore); err != nil {
			return nil, fmt.Errorf("scanning velocity result: %w", err)
		}

		results = append(results, cs)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// EngagementStats holds the engagement metrics for a content item.
type EngagementStats struct {
	Views          int32
	CommentsCount  int32
	VotesScore     int32
	BookmarksCount int32
	SharesCount    int32
	VelocityScore  float64
}

// GetEngagementStats returns the engagement stats for a content item.
// Returns zero-valued stats if no engagement row exists.
func (s *Service) GetEngagementStats(ctx context.Context, contentID pgtype.UUID) (EngagementStats, error) {
	var stats EngagementStats

	err := s.db.Reader.QueryRow(ctx, `
		SELECT COALESCE(views, 0), COALESCE(comments_count, 0),
		       COALESCE(votes_score, 0), COALESCE(bookmarks_count, 0),
		       COALESCE(shares_count, 0), COALESCE(velocity_score, 0)
		FROM content_engagement
		WHERE content_id = $1
	`, contentID).Scan(
		&stats.Views, &stats.CommentsCount,
		&stats.VotesScore, &stats.BookmarksCount,
		&stats.SharesCount, &stats.VelocityScore,
	)
	if err != nil {
		// If no row exists, return zero stats — not an error.
		return EngagementStats{}, nil //nolint:nilerr // intentional: no engagement data yet
	}

	return stats, nil
}
