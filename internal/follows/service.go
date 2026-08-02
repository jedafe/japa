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

// FollowingInfo extends a follow row with the entity's display name + slug.
type FollowingInfo struct {
	gen.Follow
	EntityName string `json:"entity_name"`
	EntitySlug string `json:"entity_slug"`
}

// joiningPermOpen is the joining_perm value that allows anyone to auto-join
// a space on follow (BL-010 / Section 31). Kept as a local constant to avoid
// importing the spaces package (which would couple follows → spaces).
const joiningPermOpen = "open"

// ToggleFollow handles follow/unfollow logic. Returns true if the user is now
// following the entity, false if they unfollowed. When following a space with
// `joining_perm = 'open'`, the follower is auto-joined as a member (BL-010 /
// Section 31). Unfollowing does NOT auto-leave (membership is independent).
//
// The follow row toggle and the denormalized follower_count update run in a
// single transaction so a count-update failure rolls back the follow (H1 —
// prevents count drift). Notification dispatch (Section 35, type
// follow.received) is fire-and-forget per Rule 8 — it runs after the
// transaction commits and never blocks or fails the follow.
func (s *Service) ToggleFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityId pgtype.UUID) (bool, error) {
	if !isValidEntityType(entityType) {
		return false, fmt.Errorf("invalid entity type: %s", entityType)
	}

	if entityType == EntityTypeUser && followerID == entityId {
		return false, errors.New("cannot follow yourself")
	}

	// H1: wrap toggle + count update in a transaction so a count-update
	// failure rolls back the follow row (prevents denormalized count drift).
	var followed bool
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after commit

	q := gen.New(tx)
	followed, err = s.store.toggleFollowTx(ctx, q, tenantID, followerID, entityType, entityId)
	if err != nil {
		return false, fmt.Errorf("toggling follow: %w", err)
	}

	delta := int32(-1)
	if followed {
		delta = 1
	}

	if err := s.updateFollowerCountTx(ctx, tx, entityType, entityId, delta); err != nil {
		return false, fmt.Errorf("updating follower count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("committing transaction: %w", err)
	}

	// L3: invalidate the is_following cache for this follower-entity pair so
	// the next IsFollowing call refetches the authoritative state from DB.
	// Best-effort — a failed DEL degrades to a stale read that self-heals
	// after the 60s TTL.
	if s.vk != nil {
		_ = s.vk.Del(ctx, isFollowingKey(followerID, entityType, entityId)).Err()

		// BUG-012: invalidate the follower's for-you feed cache so the new
		// follow/unfollow is reflected immediately in their ranked feed.
		// The cache key pattern is
		// jc:cache:feed:foryou:{tenant_id}:{user_id}:{sort_mode}:{content_country}
		// — we SCAN + DEL all variants for this user. Ref: Section 32.
		pattern := fmt.Sprintf("jc:cache:feed:foryou:%s:%s:*", tenantID, followerID)
		var cursor uint64
		var keys []string
		for {
			scanKeys, nextCursor, err := s.vk.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				break
			}
			keys = append(keys, scanKeys...)
			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
		if len(keys) > 0 {
			_ = s.vk.Del(ctx, keys...).Err()
		}
	}

	// BL-010 / Section 31: following a space with joining_perm='open' auto-joins
	// the follower as a member. Only auto-join on follow (not unfollow). Best-effort:
	// a failure to join does not roll back the follow.
	if followed && entityType == EntityTypeSpace {
		perm, permErr := s.store.getSpaceJoiningPerm(ctx, entityId, tenantID)
		if permErr != nil {
			slog.Warn("auto-join: failed to read space joining_perm",
				"space_id", entityId, "error", permErr)
		} else if perm == joiningPermOpen {
			if joinErr := s.store.addSpaceMember(ctx, entityId, followerID, tenantID); joinErr != nil {
				slog.Warn("auto-join: failed to add space member",
					"space_id", entityId, "user_id", followerID, "error", joinErr)
			}
		}
	}

	// C3: dispatch follow.received notification (Section 35). Fire-and-forget
	// per Rule 8 — only for user follows (the recipient is the followed user).
	if followed && entityType == EntityTypeUser && s.notify != nil {
		s.notify.DispatchFollowNotification(ctx, tenantID, entityId, followerID, entityType, entityId)
	}

	return followed, nil
}

// GetFollow returns the follow row for a specific follower-entity pair.
func (s *Service) GetFollow(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.Follow, error) {
	return s.store.getFollow(ctx, followerID, entityType, entityID)
}

// IsFollowing returns true if the follower is following the entity.
// Uses a read-through Valkey cache (jc:is_following:{...}, 60s TTL) to
// avoid a DB hit on every profile page render. On cache miss or when Valkey
// is not wired, falls back to the store query and populates the cache.
// Ref: japaconnect-engineering/sections/31-follow-system.md
//      conventions/valkey-keys-catalog.md (Follow System section)
func (s *Service) IsFollowing(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	// L3: try Valkey cache first (best-effort — never block on cache).
	if s.vk != nil {
		key := isFollowingKey(followerID, entityType, entityID)
		if cached, err := s.vk.Get(ctx, key).Result(); err == nil {
			switch cached {
			case "1":
				return true, nil
			case "0":
				return false, nil
			}
			// Stale/unexpected value falls through to DB.
		}
	}

	result, err := s.store.isFollowing(ctx, followerID, entityType, entityID)
	if err != nil {
		return false, err
	}

	// Populate cache (best-effort).
	if s.vk != nil {
		key := isFollowingKey(followerID, entityType, entityID)
		val := "0"
		if result {
			val = "1"
		}
		_ = s.vk.Set(ctx, key, val, isFollowingCacheTTL).Err()
	}

	return result, nil
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
			EntitySlug: r.EntitySlug,
		}
	}

	return result, nil
}

// GetFollowingByType returns paginated entities of one entity_type that a
// user follows (catalog: GET /me/followed-tags passes entityType "tag").
func (s *Service) GetFollowingByType(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, limit, offset int32) ([]FollowingInfo, error) {
	rows, err := s.store.getFollowingByType(ctx, tenantID, followerID, entityType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting following by type: %w", err)
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
			EntitySlug: r.EntitySlug,
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
	return s.updateFollowerCountTx(ctx, s.db.Writer, entityType, entityID, delta)
}

// updateFollowerCountTx is the transaction-scoped version that executes the
// UPDATE via the provided DBTX (a pgx.Tx or pgxpool.Pool). This ensures the
// count update is atomic with the follow row toggle (H1).
func (s *Service) updateFollowerCountTx(ctx context.Context, db gen.DBTX, entityType string, entityID pgtype.UUID, delta int32) error {
	switch entityType {
	case EntityTypeUser:
		_, err := db.Exec(ctx,
			"UPDATE users SET follower_count = GREATEST(0, follower_count + $1) WHERE id = $2",
			delta, entityID)
		if err != nil {
			return fmt.Errorf("updating follower count on users: %w", err)
		}
	case EntityTypeSpace:
		_, err := db.Exec(ctx,
			"UPDATE spaces SET follower_count = GREATEST(0, follower_count + $1) WHERE id = $2",
			delta, entityID)
		if err != nil {
			return fmt.Errorf("updating follower count on spaces: %w", err)
		}
	case EntityTypeTag:
		_, err := db.Exec(ctx,
			"UPDATE tags SET follower_count = GREATEST(0, follower_count + $1) WHERE id = $2",
			delta, entityID)
		if err != nil {
			return fmt.Errorf("updating follower count on tags: %w", err)
		}
	case EntityTypeOrganization:
		_, err := db.Exec(ctx,
			"UPDATE organizations SET follower_count = GREATEST(0, follower_count + $1) WHERE id = $2",
			delta, entityID)
		if err != nil {
			return fmt.Errorf("updating follower count on organizations: %w", err)
		}
	case EntityTypeProfessionalProfile:
		_, err := db.Exec(ctx,
			"UPDATE professional_profiles SET follower_count = GREATEST(0, follower_count + $1) WHERE id = $2",
			delta, entityID)
		if err != nil {
			return fmt.Errorf("updating follower count on professional_profiles: %w", err)
		}
	default:
		return nil
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
