package follows

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for follows. The Service layer
// wraps Store methods with business logic (follower count maintenance, etc.).
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// getSpaceJoiningPerm returns the joining_perm value for a space. Used by
// ToggleFollow to decide whether following a space should auto-join the
// follower as a member (BL-010 / Section 31: open-joining spaces auto-join).
func (s *Store) getSpaceJoiningPerm(ctx context.Context, spaceID pgtype.UUID) (string, error) {
	space, err := s.read.GetSpaceByID(ctx,
		spaceID,
	)
	if err != nil {
		return "", fmt.Errorf("getting space joining_perm: %w", err)
	}

	return space.JoiningPerm, nil
}

// addSpaceMember adds the user as a member of the space, returning whether
// the row was actually inserted (ON CONFLICT DO NOTHING → 0 rows when the
// user is already a member). Callers use the flag to drive member_count.
func (s *Store) addSpaceMember(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error) {
	inserted, err := s.write.AddSpaceMember(ctx, gen.AddSpaceMemberParams{
		SpaceID: spaceID,
		UserID:  userID,
	})
	if err != nil {
		return false, fmt.Errorf("adding space member: %w", err)
	}

	return inserted > 0, nil
}

// toggleFollow inserts a follow row. Returns true if a new row was inserted,
// false if the row already existed (ON CONFLICT DO NOTHING).
func (s *Store) toggleFollow(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	return s.toggleFollowTx(ctx, s.write, followerID, entityType, entityID)
}

// toggleFollowTx is the transaction-scoped version of toggleFollow. It uses
// the provided gen.Queries (which may be backed by a transaction) for both
// the read check and the write, ensuring the toggle + count update are atomic.
func (s *Store) toggleFollowTx(ctx context.Context, q *gen.Queries, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	// Race-safe toggle: attempt the insert first. If the row lands we are
	// following (the ON CONFLICT row count is authoritative — no read-check
	// that a concurrent request could also pass). If the insert is a no-op
	// the follow already exists, so delete it.
	inserted, err := q.InsertFollow(ctx, gen.InsertFollowParams{
		FollowerID: followerID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return false, fmt.Errorf("inserting follow: %w", err)
	}

	if inserted > 0 {
		return true, nil
	}

	if _, err := q.DeleteFollowByFollowerEntity(ctx, gen.DeleteFollowByFollowerEntityParams{
		FollowerID: followerID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return false, fmt.Errorf("deleting follow: %w", err)
	}

	return false, nil
}

// insertFollowIfAbsentTx inserts a follow only when the row is absent.
// Returns true when the row was actually inserted (state changed).
func (s *Store) insertFollowIfAbsentTx(ctx context.Context, q *gen.Queries, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	inserted, err := q.InsertFollow(ctx, gen.InsertFollowParams{
		FollowerID: followerID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return false, fmt.Errorf("inserting follow: %w", err)
	}

	return inserted > 0, nil
}

// deleteFollowIfPresentTx deletes a follow only when the row exists.
// Returns true when a row was actually deleted (state changed).
func (s *Store) deleteFollowIfPresentTx(ctx context.Context, q *gen.Queries, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	deleted, err := q.DeleteFollowByFollowerEntity(ctx, gen.DeleteFollowByFollowerEntityParams{
		FollowerID: followerID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return false, fmt.Errorf("deleting follow: %w", err)
	}

	return deleted > 0, nil
}

// getFollow returns the follow row for a specific follower-entity pair.
func (s *Store) getFollow(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.Follow, error) {
	follow, err := s.read.GetFollow(ctx, gen.GetFollowParams{
		FollowerID: followerID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return gen.Follow{}, fmt.Errorf("getting follow: %w", err)
	}

	return follow, nil
}

// isFollowing returns true if the follower is following the entity.
func (s *Store) isFollowing(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	result, err := s.read.IsFollowing(ctx, gen.IsFollowingParams{
		FollowerID: followerID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return false, fmt.Errorf("checking follow status: %w", err)
	}

	return result, nil
}

// isMutualFollow returns true if the entityID follows the followerID (single
// direction). The Service.IsMutualFollow wrapper calls this twice to check
// both directions for a true mutual connection (Section 31).
func (s *Store) isMutualFollow(ctx context.Context, followerID, entityID pgtype.UUID) (bool, error) {
	result, err := s.read.IsMutualFollow(ctx, gen.IsMutualFollowParams{
		EntityID:   entityID,
		FollowerID: followerID,
	})
	if err != nil {
		return false, fmt.Errorf("checking mutual follow: %w", err)
	}

	return result, nil
}

// getFollowers returns paginated followers of an entity with user info.
// viewerID is nullable: when invalid the mutual user_blocks exclusion is
// skipped (guests and workers see the unfiltered list). Section 41 (BLK-006).
func (s *Store) getFollowers(ctx context.Context, viewerID pgtype.UUID, entityType string, entityID pgtype.UUID, limit, offset int32) ([]gen.GetFollowersByEntityRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetFollowersByEntity(ctx, gen.GetFollowersByEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
		ViewerID:   viewerID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("getting followers: %w", err)
	}

	return rows, nil
}

// getFollowing returns paginated entities that a user follows.
// viewerID filters followed users with a mutual block against the viewer
// (BLK-006); pass an invalid UUID to skip the filter.
func (s *Store) getFollowing(ctx context.Context, followerID, viewerID pgtype.UUID, limit, offset int32) ([]gen.GetFollowingByUserRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetFollowingByUser(ctx, gen.GetFollowingByUserParams{
		FollowerID: followerID,
		ViewerID:   viewerID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("getting following: %w", err)
	}

	return rows, nil
}

// getFollowingByType returns paginated entities of one entity_type that a
// user follows (catalog: GET /me/followed-tags uses entity_type "tag").
func (s *Store) getFollowingByType(ctx context.Context, followerID pgtype.UUID, entityType string, viewerID pgtype.UUID, limit, offset int32) ([]gen.GetFollowingByUserAndTypeRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetFollowingByUserAndType(ctx, gen.GetFollowingByUserAndTypeParams{
		FollowerID: followerID,
		EntityType: entityType,
		ViewerID:   viewerID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("getting following by type: %w", err)
	}

	return rows, nil
}

// countFollowers returns the number of followers for an entity.
func (s *Store) countFollowers(ctx context.Context, entityType string, entityID pgtype.UUID) (int32, error) {
	count, err := s.read.CountFollowersByEntity(ctx, gen.CountFollowersByEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting followers: %w", err)
	}

	return count, nil
}

// getBulkFollowingStatus returns follow status for multiple entities of the same type.
func (s *Store) getBulkFollowingStatus(ctx context.Context, followerID pgtype.UUID, entityType string, entityIDs []pgtype.UUID) (map[pgtype.UUID]bool, error) {
	if len(entityIDs) == 0 {
		return map[pgtype.UUID]bool{}, nil
	}

	rows, err := s.read.GetBulkFollowingStatus(ctx, gen.GetBulkFollowingStatusParams{
		FollowerID: followerID,

		EntityType: entityType,
		EntityIds:  entityIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("getting bulk following status: %w", err)
	}

	status := make(map[pgtype.UUID]bool, len(rows))
	for _, r := range rows {
		status[r.EntityID] = r.IsFollowing
	}

	return status, nil
}

// deleteFollowsByFollower removes all follows by a user (used for account deletion).
func (s *Store) deleteFollowsByFollower(ctx context.Context, followerID pgtype.UUID) error {
	if err := s.write.DeleteFollowsByFollower(ctx, followerID); err != nil {
		return fmt.Errorf("deleting follows by follower: %w", err)
	}

	return nil
}

// deleteFollowsByEntity removes all follows for an entity (used for entity deletion).
func (s *Store) deleteFollowsByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) error {
	if err := s.write.DeleteFollowsByEntity(ctx, gen.DeleteFollowsByEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("deleting follows by entity: %w", err)
	}

	return nil
}
