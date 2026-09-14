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

// getContentsBySlugs returns published content for the given slugs in the order
// provided by the slugs array. Used for the readiness roadmap feed.
func (s *Store) getContentsBySlugs(ctx context.Context, requesterID pgtype.UUID, slugs []string, limit, offset int32) ([]gen.ListContentsBySlugsRow, error) {
	rows, err := s.read.ListContentsBySlugs(ctx, gen.ListContentsBySlugsParams{

		Slugs:             slugs,
		CanSeeUnpublished: false,
		CanSeePrivate:     false,
		RequesterID:       requesterID,
		CanSeeMembers:     true,
		PageLimit:         limit,
		PageOffset:        offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting contents by slugs: %w", err)
	}

	return rows, nil
}

// getExploreFeed returns trending published content for guest users, ranked by
// the visitor's geo proximity.
//

func (s *Store) getExploreFeed(ctx context.Context, visitorCountryCode, visitorRegion string, limit, offset int32) ([]gen.GetExploreFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetExploreFeed(ctx, gen.GetExploreFeedParams{

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

func (s *Store) getPrimaryFeed(ctx context.Context, userID pgtype.UUID, sort string, limit, offset int32) ([]gen.GetPrimaryFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetPrimaryFeed(ctx, gen.GetPrimaryFeedParams{

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

func (s *Store) getSpaceFeed(ctx context.Context, spaceID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]gen.GetSpaceFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetSpaceFeed(ctx, gen.GetSpaceFeedParams{

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

func (s *Store) getProfileFeed(ctx context.Context, authorID, viewerID pgtype.UUID, sort string, limit, offset int32) ([]gen.GetProfileFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetProfileFeed(ctx, gen.GetProfileFeedParams{

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

func (s *Store) getTopFeed(ctx context.Context, visitorCountryCode, visitorRegion string, limit, offset int32) ([]gen.GetTopFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetTopFeed(ctx, gen.GetTopFeedParams{

		VisitorCountryCode: &visitorCountryCode,
		VisitorRegion:      &visitorRegion,
		PageLimit:          limit,
		PageOffset:         offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting top feed: %w", err)
	}

	return rows, nil
}

// getRisingFeed returns rising content sorted by velocity score.
//

func (s *Store) getRisingFeed(ctx context.Context, visitorCountryCode, visitorRegion string, limit, offset int32) ([]gen.GetRisingFeedRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetRisingFeed(ctx, gen.GetRisingFeedParams{

		VisitorCountryCode: &visitorCountryCode,
		VisitorRegion:      &visitorRegion,
		PageLimit:          limit,
		PageOffset:         offset,
	})
	if err != nil {
		return nil, fmt.Errorf("getting rising feed: %w", err)
	}

	return rows, nil
}

// updateContentScore updates a single content's score columns.
//

func (s *Store) updateContentScore(ctx context.Context, params gen.UpdateContentScoreParams) error {
	if err := s.write.UpdateContentScore(ctx, params); err != nil {
		return fmt.Errorf("updating content score: %w", err)
	}

	return nil
}

// updateVelocityScores batch updates velocity scores for published content
// < 30 days old. gravity is the decay exponent (default 1.8).
//

func (s *Store) updateVelocityScores(ctx context.Context, gravity float64) error {
	if err := s.write.UpdateVelocityScores(ctx,
		gravity,
	); err != nil {
		return fmt.Errorf("updating velocity scores: %w", err)
	}

	return nil
}

// deleteFeedItemsByContent removes all feed rows referencing a content ID.
//

func (s *Store) deleteFeedItemsByContent(ctx context.Context, contentID pgtype.UUID) error {
	if err := s.write.DeleteFeedItemsByContent(ctx,
		contentID,
	); err != nil {
		return fmt.Errorf("deleting feed items by content: %w", err)
	}

	return nil
}

// deleteFeedItemsByUser removes all feed rows for a user.
//

func (s *Store) deleteFeedItemsByUser(ctx context.Context, userID pgtype.UUID) error {
	if err := s.write.DeleteFeedItemsByUser(ctx,
		userID,
	); err != nil {
		return fmt.Errorf("deleting feed items by user: %w", err)
	}

	return nil
}

// getFeedItemCount returns the total number of feed items for a user.
//

func (s *Store) getFeedItemCount(ctx context.Context, userID pgtype.UUID) (int32, error) {
	count, err := s.read.GetFeedItemCount(ctx,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting feed items: %w", err)
	}

	return count, nil
}

// getFollowedEntityIDs returns the IDs of all entities of one type the user follows.
func (s *Store) getFollowedEntityIDs(
	ctx context.Context, followerID pgtype.UUID, entityType string,
) ([]pgtype.UUID, error) {
	ids, err := s.read.GetFollowedEntityIDsByType(ctx, gen.GetFollowedEntityIDsByTypeParams{

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
	ctx context.Context, userID pgtype.UUID,
) ([]pgtype.UUID, error) {
	ids, err := s.read.GetMemberSpaceIDs(ctx,

		userID,
	)
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

// listSourcesByContentIDs returns source attribution for a batch of content IDs.
func (s *Store) listSourcesByContentIDs(ctx context.Context, contentIDs []pgtype.UUID) (map[string]gen.ListSourcesByContentIDsRow, error) {
	if len(contentIDs) == 0 {
		return map[string]gen.ListSourcesByContentIDsRow{}, nil
	}

	rows, err := s.read.ListSourcesByContentIDs(ctx,

		contentIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("listing sources by content ids: %w", err)
	}

	out := make(map[string]gen.ListSourcesByContentIDsRow, len(rows))
	for _, r := range rows {
		out[r.ContentID.String()] = r
	}

	return out, nil
}
