package feed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for feed operations.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// insertFeedItem creates a new feed row (idempotent via ON CONFLICT DO NOTHING).
//
//nolint:unused // reserved for future features
func (s *Store) insertFeedItem(ctx context.Context, params gen.InsertFeedItemParams) error {
	if err := s.write.InsertFeedItem(ctx, params); err != nil {
		return fmt.Errorf("inserting feed item: %w", err)
	}

	return nil
}

// getExploreFeed returns trending published content for guest users, ranked by
// the visitor's geo proximity.
//
//nolint:unused // reserved for future features
func (s *Store) getExploreFeed(ctx context.Context, tenantID pgtype.UUID, visitorCountryCode, visitorRegion string, limit, offset int32) ([]gen.GetExploreFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetExploreFeed(ctx, gen.GetExploreFeedParams{
		TenantID:           tenantID,
		VisitorCountryCode: &visitorCountryCode,
		VisitorRegion:      &visitorRegion,
		PageLimit:          limit,
		PageOffset:         offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting explore feed: %w", err)
	}

	return rows, nil
}

// getPrimaryFeed returns the user's personalized feed from the feeds table.
//
//nolint:unused // reserved for future features
func (s *Store) getPrimaryFeed(ctx context.Context, tenantID, userID pgtype.UUID, sort string, limit, offset int32) ([]gen.GetPrimaryFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetPrimaryFeed(ctx, gen.GetPrimaryFeedParams{
		TenantID:   tenantID,
		UserID:     userID,
		Sort:       sort,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting primary feed: %w", err)
	}

	return rows, nil
}

// getSpaceFeed returns published posts in a specific space.
// viewerID is nullable — when valid, mutual user_blocks are excluded (§41).
//
//nolint:unused // reserved for future features
func (s *Store) getSpaceFeed(ctx context.Context, tenantID, spaceID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]gen.GetSpaceFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetSpaceFeed(ctx, gen.GetSpaceFeedParams{
		TenantID:   tenantID,
		SpaceID:    spaceID,
		ViewerID:   viewerID,
		Sort:       sort,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting space feed: %w", err)
	}

	return rows, nil
}

// getProfileFeed returns all public published posts by a specific user.
// viewerID is nullable — when valid, mutual user_blocks are excluded (§41).
//
//nolint:unused // reserved for future features
func (s *Store) getProfileFeed(ctx context.Context, tenantID, authorID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]gen.GetProfileFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetProfileFeed(ctx, gen.GetProfileFeedParams{
		TenantID:   tenantID,
		AuthorID:   authorID,
		ViewerID:   viewerID,
		Sort:       sort,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting profile feed: %w", err)
	}

	return rows, nil
}

// getTopFeed returns top published content from the 7-day rolling window.
//
//nolint:unused // reserved for future features
func (s *Store) getTopFeed(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.GetTopFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetTopFeed(ctx, gen.GetTopFeedParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting top feed: %w", err)
	}

	return rows, nil
}

// getRisingFeed returns rising content sorted by velocity score.
//
//nolint:unused // reserved for future features
func (s *Store) getRisingFeed(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.GetRisingFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetRisingFeed(ctx, gen.GetRisingFeedParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting rising feed: %w", err)
	}

	return rows, nil
}

// updateContentScore updates a single content's score columns.
//
//nolint:unused // reserved for future features
func (s *Store) updateContentScore(ctx context.Context, params gen.UpdateContentScoreParams) error {
	if err := s.write.UpdateContentScore(ctx, params); err != nil {
		return fmt.Errorf("updating content score: %w", err)
	}

	return nil
}

// updateVelocityScores batch updates velocity scores for published content
// < 30 days old. gravity is the decay exponent (default 1.8).
//
//nolint:unused // reserved for future features
func (s *Store) updateVelocityScores(ctx context.Context, tenantID pgtype.UUID, gravity float64) error {
	if err := s.write.UpdateVelocityScores(ctx, gen.UpdateVelocityScoresParams{
		Gravity:  gravity,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("updating velocity scores: %w", err)
	}

	return nil
}

// deleteFeedItemsByContent removes all feed rows referencing a content ID.
//
//nolint:unused // reserved for future features
func (s *Store) deleteFeedItemsByContent(ctx context.Context, tenantID, contentID pgtype.UUID) error {
	if err := s.write.DeleteFeedItemsByContent(ctx, gen.DeleteFeedItemsByContentParams{
		ContentID: contentID, TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting feed items by content: %w", err)
	}

	return nil
}

// deleteFeedItemsByUser removes all feed rows for a user.
//
//nolint:unused // reserved for future features
func (s *Store) deleteFeedItemsByUser(ctx context.Context, tenantID, userID pgtype.UUID) error {
	if err := s.write.DeleteFeedItemsByUser(ctx, gen.DeleteFeedItemsByUserParams{
		UserID: userID, TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting feed items by user: %w", err)
	}

	return nil
}

// getFeedItemCount returns the total number of feed items for a user.
//
//nolint:unused // reserved for future features
func (s *Store) getFeedItemCount(ctx context.Context, tenantID, userID pgtype.UUID) (int32, error) {
	count, err := s.read.GetFeedItemCount(ctx, gen.GetFeedItemCountParams{
		UserID: userID, TenantID: tenantID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting feed items: %w", err)
	}

	return count, nil
}

// getFollowedEntityIDs returns the IDs of all entities of one type the user follows.
func (s *Store) getFollowedEntityIDs(
	ctx context.Context, tenantID, followerID pgtype.UUID, entityType string,
) ([]pgtype.UUID, error) {
	ids, err := s.read.GetFollowedEntityIDsByType(ctx, gen.GetFollowedEntityIDsByTypeParams{
		TenantID:   tenantID,
		FollowerID: followerID,
		EntityType: entityType,
	})
	if err != nil {
		return nil, fmt.Errorf("getting followed %s ids: %w", entityType, err)
	}

	return ids, nil
}

// getMemberSpaceIDs returns the IDs of spaces the user is a member of.
func (s *Store) getMemberSpaceIDs(
	ctx context.Context, tenantID, userID pgtype.UUID,
) ([]pgtype.UUID, error) {
	ids, err := s.read.GetMemberSpaceIDs(ctx, gen.GetMemberSpaceIDsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting member space ids: %w", err)
	}

	return ids, nil
}

// getForYouCandidates runs the bounded personalized candidate query.
func (s *Store) getForYouCandidates(
	ctx context.Context, params gen.GetForYouCandidatesParams,
) ([]gen.GetForYouCandidatesRow, error) {
	rows, err := s.read.GetForYouCandidates(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("getting for-you candidates: %w", err)
	}

	return rows, nil
}
