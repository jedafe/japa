package follows

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/wallet"
)

// FollowerInfo extends a follow row with the follower's profile data.
// IsFollowing indicates whether the authenticated caller follows this
// follower back (Section 31: list endpoints expose is_following per item).
type FollowerInfo struct {
	gen.Follow
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	ProfileType string `json:"profile_type"`
	IsFollowing bool   `json:"is_following"`
}

// FollowingInfo extends a follow row with the entity's display name + slug.
// For user-type follows the joined user profile fields (DisplayName,
// AvatarUrl) are populated from users; for other entity types they are empty
// strings (the SQL COALESCE), which the frontend treats as "no avatar".
// IsFollowing indicates whether the authenticated caller follows this entity
// (Section 31: list endpoints expose is_following per item).
type FollowingInfo struct {
	gen.Follow
	EntityName  string `json:"entity_name"`
	EntitySlug  string `json:"entity_slug"`
	DisplayName string `json:"entity_display_name"`
	AvatarUrl   string `json:"entity_avatar_url"`
	IsFollowing bool   `json:"is_following"`
}

// joiningPermOpen is the joining_perm value that allows anyone to auto-join
// a space on follow (BL-010 / Section 31). Kept as a local constant to avoid
// importing the spaces package (which would couple follows → spaces).
const joiningPermOpen = "open"

// ErrEntityNotFound is returned by ToggleFollow when the target entity does
// not exist (or does not belong to the caller's tenant). The polymorphic
// follows.entity_id column has no FK constraint, so without this check a
// follow row can be inserted for a non-existent entity (data integrity).
var ErrEntityNotFound = errors.New("entity not found")

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

	// Validate the target entity exists in the caller's tenant before
	// inserting a follow row. The polymorphic entity_id has no FK, so
	// without this check a follow row can dangle for a non-existent entity.
	if err := s.validateEntityExists(ctx, tenantID, entityType, entityId); err != nil {
		return false, err
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

	// following_count is computed on demand by the user profile queries
	// (subquery on follows), not a denormalized column on users.

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
		pattern := valkey.MustKey("cache:feed:foryou:%s:%s:*", tenantID, followerID)

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
			} else {
				// §64: auto-join via follow is a real membership — record
				// space.joined so badge/challenge triggers (e.g.
				// space_explorer) fire on this path too.
				s.emitActivity(ctx, tenantID, followerID, wallet.ActivityKeySpaceJoined, "space:"+entityId.String(), nil)
			}
		}
	}

	// C3: dispatch follow.received notification (Section 35). Fire-and-forget
	// per Rule 8 — only for user follows (the recipient is the followed user).
	if followed && entityType == EntityTypeUser && s.notify != nil {
		s.notify.DispatchFollowNotification(ctx, tenantID, entityId, followerID, entityType, entityId)
	}

	// §64: record the follow.created activity so badge/challenge triggers
	// (e.g. super_connector) can fire. The reference is per (entity_type,
	// entity_id) so unfollow/refollow cycles cannot farm the count.
	if followed {
		s.emitActivity(ctx, tenantID, followerID, wallet.ActivityKeyFollowCreated,
			fmt.Sprintf("follow:%s:%s", entityType, entityId.String()),
			map[string]interface{}{"entity_type": entityType})
	}

	return followed, nil
}

// EnsureFollow is an idempotent follow: it checks whether the caller already
// follows the entity, and only toggles on when not already following. This
// avoids onboarding (and other bulk-follow paths) accidentally unfollowing an
// entity the user is already following, while still preserving count updates,
// auto-join, notifications, and activity events via ToggleFollow.
func (s *Service) EnsureFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	following, err := s.IsFollowing(ctx, tenantID, followerID, entityType, entityID)
	if err != nil {
		return false, fmt.Errorf("checking follow status: %w", err)
	}
	if following {
		return true, nil
	}
	return s.ToggleFollow(ctx, tenantID, followerID, entityType, entityID)
}

// GetFollow returns the follow row for a specific follower-entity pair.
func (s *Service) GetFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.Follow, error) {
	return s.store.getFollow(ctx, tenantID, followerID, entityType, entityID)
}

// IsFollowing returns true if the follower is following the entity.
// Uses a read-through Valkey cache (jc:is_following:{...}, 60s TTL) to
// avoid a DB hit on every profile page render. On cache miss or when Valkey
// is not wired, falls back to the store query and populates the cache.
// Ref: japaconnect-engineering/sections/31-follow-system.md
//
//	conventions/valkey-keys-catalog.md (Follow System section)
func (s *Service) IsFollowing(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
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

	result, err := s.store.isFollowing(ctx, tenantID, followerID, entityType, entityID)
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

// IsMutualFollow checks if two users mutually follow each other. A
// "connection" (Section 31) requires BOTH directions: followerID follows
// entityID AND entityID follows followerID. The sqlc IsMutualFollow query
// checks a single direction (does entityID follow followerID?), so we call
// it twice and AND the results — mirroring the DM store pattern
// (internal/websocket/dm.go IsMutualFollow).
//
// Ref: japaconnect-engineering/sections/31-follow-system.md (Mutual Follow).
func (s *Service) IsMutualFollow(ctx context.Context, tenantID, followerID, entityID pgtype.UUID) (bool, error) {
	// Direction 1: does entityID follow followerID?
	dir1, err := s.store.isMutualFollow(ctx, tenantID, followerID, entityID)
	if err != nil {
		return false, fmt.Errorf("checking entity follows follower: %w", err)
	}

	if !dir1 {
		return false, nil
	}

	// Direction 2: does followerID follow entityID?
	dir2, err := s.store.isMutualFollow(ctx, tenantID, entityID, followerID)
	if err != nil {
		return false, fmt.Errorf("checking follower follows entity: %w", err)
	}

	return dir2, nil
}

// GetFollowers returns paginated followers with user profile data. When
// callerID is valid, each item includes is_following indicating whether the
// caller follows that follower back (Section 31: bulk is_following per item).
func (s *Service) GetFollowers(ctx context.Context, tenantID, callerID pgtype.UUID, entityType string, entityID pgtype.UUID, limit, offset int32) ([]FollowerInfo, error) {
	rows, err := s.store.getFollowers(ctx, tenantID, entityType, entityID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting followers: %w", err)
	}

	// Section 31: resolve is_following per item in a single bulk query
	// rather than N+1. Followers are always users, so the entity type is
	// EntityTypeUser.
	var bulkStatus map[pgtype.UUID]bool

	if callerID.Valid && len(rows) > 0 {
		ids := make([]pgtype.UUID, len(rows))
		for i, r := range rows {
			ids[i] = r.FollowerID
		}

		bulkStatus, err = s.store.getBulkFollowingStatus(ctx, tenantID, callerID, EntityTypeUser, ids)
		if err != nil {
			return nil, fmt.Errorf("bulk is_following for followers: %w", err)
		}
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
			ProfileType: r.ProfileType,
		}
		if bulkStatus != nil {
			result[i].IsFollowing = bulkStatus[r.FollowerID]
		}
	}

	return result, nil
}

// GetFollowing returns paginated entities that a user follows. When callerID
// is valid, each item includes is_following indicating whether the caller
// also follows that entity (Section 31: bulk is_following per item).
func (s *Service) GetFollowing(ctx context.Context, tenantID, callerID, followerID pgtype.UUID, limit, offset int32) ([]FollowingInfo, error) {
	rows, err := s.store.getFollowing(ctx, tenantID, followerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting following: %w", err)
	}

	bulkStatus := s.resolveBulkFollowingByType(ctx, tenantID, callerID, rows)

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
			EntityName:  r.EntityName,
			EntitySlug:  r.EntitySlug,
			DisplayName: r.EntityDisplayName,
			AvatarUrl:   r.EntityAvatarUrl,
		}
		if bulkStatus != nil {
			result[i].IsFollowing = bulkStatus[r.EntityType][r.EntityID]
		}
	}

	return result, nil
}

// GetFollowingByType returns paginated entities of one entity_type that a
// user follows (catalog: GET /me/followed-tags passes entityType "tag").
// When callerID is valid, each item includes is_following.
func (s *Service) GetFollowingByType(ctx context.Context, tenantID, callerID, followerID pgtype.UUID, entityType string, limit, offset int32) ([]FollowingInfo, error) {
	rows, err := s.store.getFollowingByType(ctx, tenantID, followerID, entityType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting following by type: %w", err)
	}

	// All items share the same entity_type, so a single bulk query suffices.
	var bulkStatus map[pgtype.UUID]bool

	if callerID.Valid && len(rows) > 0 {
		ids := make([]pgtype.UUID, len(rows))
		for i, r := range rows {
			ids[i] = r.EntityID
		}

		bulkStatus, err = s.store.getBulkFollowingStatus(ctx, tenantID, callerID, entityType, ids)
		if err != nil {
			return nil, fmt.Errorf("bulk is_following for following: %w", err)
		}
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
			EntityName:  r.EntityName,
			EntitySlug:  r.EntitySlug,
			DisplayName: r.EntityDisplayName,
			AvatarUrl:   r.EntityAvatarUrl,
		}
		if bulkStatus != nil {
			result[i].IsFollowing = bulkStatus[r.EntityID]
		}
	}

	return result, nil
}

// resolveBulkFollowingByType resolves is_following for a mixed-type following
// list. It groups entity_ids by entity_type and issues one bulk query per
// type (at most 5 — one per entity type present in the page), avoiding N+1.
// Returns nil when callerID is invalid (no authenticated caller).
func (s *Service) resolveBulkFollowingByType(ctx context.Context, tenantID, callerID pgtype.UUID, rows []gen.GetFollowingByUserRow) map[string]map[pgtype.UUID]bool {
	if !callerID.Valid || len(rows) == 0 {
		return nil
	}

	byType := make(map[string][]pgtype.UUID)
	for _, r := range rows {
		byType[r.EntityType] = append(byType[r.EntityType], r.EntityID)
	}

	result := make(map[string]map[pgtype.UUID]bool, len(byType))

	for et, ids := range byType {
		status, err := s.store.getBulkFollowingStatus(ctx, tenantID, callerID, et, ids)
		if err != nil {
			slog.Warn("bulk is_following lookup failed", "entity_type", et, "error", err)
			continue
		}

		result[et] = status
	}

	return result
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

// validateEntityExists checks that the target entity exists in the caller's
// tenant before a follow row is inserted. The polymorphic follows.entity_id
// column has no FK constraint, so without this check a follow row can dangle
// for a non-existent entity. Returns ErrEntityNotFound when the entity does
// not exist.
func (s *Service) validateEntityExists(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	var table string

	switch entityType {
	case EntityTypeUser:
		table = "users"
	case EntityTypeSpace:
		table = "spaces"
	case EntityTypeTag:
		table = "tags"
	case EntityTypeOrganization:
		table = "organizations"
	default:
		return fmt.Errorf("invalid entity type: %s", entityType)
	}

	var exists bool

	err := s.db.Reader.QueryRow(
		ctx,
		fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1 AND tenant_id = $2) AS exists", table),
		entityID, tenantID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking entity existence: %w", err)
	}

	if !exists {
		return ErrEntityNotFound
	}

	return nil
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
	default:
		return nil
	}

	return nil
}

// isValidEntityType returns true if the entity type is a known follow target.
func isValidEntityType(entityType string) bool {
	switch entityType {
	case EntityTypeUser, EntityTypeSpace, EntityTypeTag, EntityTypeOrganization:
		return true
	default:
		return false
	}
}
