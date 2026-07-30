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
func (s *Store) getSpaceJoiningPerm(ctx context.Context, spaceID, tenantID pgtype.UUID) (string, error) {
	space, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
		ID:       spaceID,
		TenantID: tenantID,
	})
	if err != nil {
		return "", fmt.Errorf("getting space joining_perm: %w", err)
	}
	return space.JoiningPerm, nil
}

// addSpaceMember adds the user as a member of the space. The underlying
// query is INSERT ... ON CONFLICT DO NOTHING, so calling this when the user
// is already a member is a safe no-op (BL-010).
func (s *Store) addSpaceMember(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) error {
	if err := s.write.AddSpaceMember(ctx, gen.AddSpaceMemberParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("adding space member: %w", err)
	}
	return nil
}

// toggleFollow inserts a follow row. Returns true if a new row was inserted,
// false if the row already existed (ON CONFLICT DO NOTHING).
func (s *Store) toggleFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	return s.toggleFollowTx(ctx, s.write, tenantID, followerID, entityType, entityID)
}

// toggleFollowTx is the transaction-scoped version of toggleFollow. It uses
// the provided gen.Queries (which may be backed by a transaction) for both
// the read check and the write, ensuring the toggle + count update are atomic.
func (s *Store) toggleFollowTx(ctx context.Context, q *gen.Queries, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	// First check if the follow already exists.
	existing, err := q.GetFollow(ctx, gen.GetFollowParams{
		FollowerID: followerID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err == nil && existing.ID.Valid {
		// Already following — delete (unfollow).
		if err := q.DeleteFollowByFollowerEntity(ctx, gen.DeleteFollowByFollowerEntityParams{
			FollowerID: followerID,
			EntityType: entityType,
			EntityID:   entityID,
		}); err != nil {
			return false, fmt.Errorf("deleting follow: %w", err)
		}

		return false, nil
	}

	// Not following — insert.
	if err := q.InsertFollow(ctx, gen.InsertFollowParams{
		TenantID:   tenantID,
		FollowerID: followerID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return false, fmt.Errorf("inserting follow: %w", err)
	}

	return true, nil
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

// isMutualFollow returns true if two users mutually follow each other.
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
func (s *Store) getFollowers(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, limit, offset int32) ([]gen.GetFollowersByEntityRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetFollowersByEntity(ctx, gen.GetFollowersByEntityParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("getting followers: %w", err)
	}

	return rows, nil
}

// getFollowing returns paginated entities that a user follows.
func (s *Store) getFollowing(ctx context.Context, tenantID, followerID pgtype.UUID, limit, offset int32) ([]gen.GetFollowingByUserRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetFollowingByUser(ctx, gen.GetFollowingByUserParams{
		TenantID:   tenantID,
		FollowerID: followerID,
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
func (s *Store) getFollowingByType(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, limit, offset int32) ([]gen.GetFollowingByUserAndTypeRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetFollowingByUserAndType(ctx, gen.GetFollowingByUserAndTypeParams{
		TenantID:   tenantID,
		FollowerID: followerID,
		EntityType: entityType,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("getting following by type: %w", err)
	}

	return rows, nil
}

// countFollowers returns the number of followers for an entity.
func (s *Store) countFollowers(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) (int32, error) {
	count, err := s.read.CountFollowersByEntity(ctx, gen.CountFollowersByEntityParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting followers: %w", err)
	}

	return count, nil
}

// countFollowing returns the number of entities a user follows.
func (s *Store) countFollowing(ctx context.Context, tenantID, followerID pgtype.UUID) (int32, error) {
	count, err := s.read.CountFollowingByUser(ctx, gen.CountFollowingByUserParams{
		TenantID:   tenantID,
		FollowerID: followerID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting following: %w", err)
	}

	return count, nil
}

// getBulkFollowingStatus returns follow status for multiple entities of the same type.
func (s *Store) getBulkFollowingStatus(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityIDs []pgtype.UUID) (map[pgtype.UUID]bool, error) {
	if len(entityIDs) == 0 {
		return map[pgtype.UUID]bool{}, nil
	}

	rows, err := s.read.GetBulkFollowingStatus(ctx, gen.GetBulkFollowingStatusParams{
		FollowerID: followerID,
		TenantID:   tenantID,
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
