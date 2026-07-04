package follows

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// FollowerInfo extends a follow row with the follower's profile data.
type FollowerInfo struct {
	gen.Follow
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// FollowingInfo extends a follow row with the entity's display name.
type FollowingInfo struct {
	gen.Follow
	EntityName string `json:"entity_name"`
}

// ToggleFollow handles follow/unfollow logic. Returns true if the user is now
// following the entity, false if they unfollowed.
func (s *Service) ToggleFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	if !isValidEntityType(entityType) {
		return false, fmt.Errorf("invalid entity type: %s", entityType)
	}

	if entityType == EntityTypeUser && followerID == entityID {
		return false, errors.New("cannot follow yourself")
	}

	followed, err := s.store.toggleFollow(ctx, tenantID, followerID, entityType, entityID)
	if err != nil {
		return false, fmt.Errorf("toggling follow: %w", err)
	}

	// Update denormalized follower count on the entity table.
	delta := int32(-1)
	if followed {
		delta = 1
	}

	if err := s.updateFollowerCount(ctx, entityType, entityID, delta); err != nil {
		// Log but don't fail the follow operation.
		slog.Warn("failed to update follower count", "entity_type", entityType, "entity_id", entityID, "error", err)
	}

	return followed, nil
}

// GetFollow returns the follow row for a specific follower-entity pair.
func (s *Service) GetFollow(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.Follow, error) {
	return s.store.getFollow(ctx, followerID, entityType, entityID)
}

// IsFollowing returns true if the follower is following the entity.
func (s *Service) IsFollowing(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	return s.store.isFollowing(ctx, followerID, entityType, entityID)
}

// IsMutualFollow checks if two users mutually follow each other.
func (s *Service) IsMutualFollow(ctx context.Context, followerID, entityID pgtype.UUID) (bool, error) {
	return s.store.isMutualFollow(ctx, followerID, entityID)
}

// GetFollowers returns paginated followers with user profile data.
func (s *Service) GetFollowers(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, limit, offset int32) ([]FollowerInfo, error) {
	rows, err := s.store.getFollowers(ctx, tenantID, entityType, entityID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting followers: %w", err)
	}

	result := make([]FollowerInfo, len(rows))
	for i, r := range rows {
		result[i] = FollowerInfo{
			Follow: gen.Follow{
				ID:         r.ID,
				TenantID:   r.TenantID,
				FollowerID: r.FollowerID,
				EntityType: r.EntityType,
				EntityID:   r.EntityID,
				CreatedAt:  r.CreatedAt,
			},
			Username:    r.Username,
			DisplayName: r.DisplayName,
			AvatarURL:   r.AvatarUrl,
		}
	}

	return result, nil
}

// GetFollowing returns paginated entities that a user follows.
func (s *Service) GetFollowing(ctx context.Context, tenantID, followerID pgtype.UUID, limit, offset int32) ([]FollowingInfo, error) {
	rows, err := s.store.getFollowing(ctx, tenantID, followerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting following: %w", err)
	}

	result := make([]FollowingInfo, len(rows))
	for i, r := range rows {
		result[i] = FollowingInfo{
			Follow: gen.Follow{
				ID:         r.ID,
				TenantID:   r.TenantID,
				FollowerID: r.FollowerID,
				EntityType: r.EntityType,
				EntityID:   r.EntityID,
				CreatedAt:  r.CreatedAt,
			},
			EntityName: r.EntityName,
		}
	}

	return result, nil
}

// CountFollowers returns the follower count for an entity.
func (s *Service) CountFollowers(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) (int32, error) {
	return s.store.countFollowers(ctx, tenantID, entityType, entityID)
}

// CountFollowing returns the following count for a user.
func (s *Service) CountFollowing(ctx context.Context, tenantID, followerID pgtype.UUID) (int32, error) {
	return s.store.countFollowing(ctx, tenantID, followerID)
}

// GetBulkFollowingStatus returns follow status for multiple entities of the same type.
func (s *Service) GetBulkFollowingStatus(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityIDs []pgtype.UUID) (map[pgtype.UUID]bool, error) {
	return s.store.getBulkFollowingStatus(ctx, tenantID, followerID, entityType, entityIDs)
}

// DeleteFollowsByFollower removes all follows by a user (for account deletion).
func (s *Service) DeleteFollowsByFollower(ctx context.Context, followerID pgtype.UUID) error {
	return s.store.deleteFollowsByFollower(ctx, followerID)
}

// DeleteFollowsByEntity removes all follows for an entity (for entity deletion).
func (s *Service) DeleteFollowsByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) error {
	return s.store.deleteFollowsByEntity(ctx, entityType, entityID)
}

// updateFollowerCount adjusts the denormalized follower_count on the entity's
// table by delta (+1 or -1). Uses a direct SQL UPDATE because not all entity
// tables share the same column name, and triggers may not exist yet.
func (s *Service) updateFollowerCount(ctx context.Context, entityType string, entityID pgtype.UUID, delta int32) error {
	var table string

	switch entityType {
	case EntityTypeUser:
		table = "users"
	case EntityTypeSpace:
		table = "spaces"
	case EntityTypeTag:
		table = "tags"
	default:
		// Organizations and professional_profiles don't have tables yet.
		return nil
	}

	query := fmt.Sprintf(
		"UPDATE %s SET follower_count = GREATEST(0, follower_count + $1) WHERE id = $2",
		table,
	)

	_, err := s.db.Writer.Exec(ctx, query, delta, entityID)
	if err != nil {
		return fmt.Errorf("updating follower count on %s: %w", table, err)
	}

	return nil
}

// isValidEntityType returns true if the entity type is a known follow target.
func isValidEntityType(entityType string) bool {
	switch entityType {
	case EntityTypeUser, EntityTypeSpace, EntityTypeTag, EntityTypeOrganization, EntityTypeProfessionalProfile:
		return true
	default:
		return false
	}
}
