package interactions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
)

// BlockUser idempotently blocks a user and cancels side-effects per Section 41
// line 147: "block a user (also cancels any DM/follow)". The block insert,
// follow deletion (both directions), and pending DM request decline run in a
// single transaction so a partial failure leaves no orphaned follow/DM.
// Platform-scoped per JEOS §9 (BUG-006). Ref: §41 — POST /users/:id/block.
func (s *Service) BlockUser(ctx context.Context, blockerID, blockedID pgtype.UUID) error {
	if blockerID == blockedID {
		return errors.New("cannot block yourself")
	}

	// BLK-007: the target must exist — otherwise CreateUserBlock hits an FK
	// violation and surfaces a raw 500 instead of a 404.
	exists, err := s.read.ActiveUserExistsByID(ctx, blockedID)
	if err != nil {
		return fmt.Errorf("checking block target: %w", err)
	}

	if !exists {
		return fmt.Errorf("%w: user", errpkg.ErrNotFound)
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning block transaction: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after commit

	q := gen.New(tx)

	// 1. Insert the block (idempotent — ON CONFLICT DO NOTHING).
	if err := q.CreateUserBlock(ctx, gen.CreateUserBlockParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
	}); err != nil {
		return fmt.Errorf("creating user block: %w", err)
	}

	// 2. Cancel follows in both directions (BE-1 / Section 41).
	if err := q.DeleteFollowsBetweenUsers(ctx, gen.DeleteFollowsBetweenUsersParams{
		FollowerID: blockerID,
		EntityID:   blockedID,
	}); err != nil {
		return fmt.Errorf("deleting follows between users: %w", err)
	}

	// 3. BLK-003: recompute both users' denormalized follower_count inside
	// the same tx — DeleteFollowsBetweenUsers bypasses the follows service's
	// count updater, so without this every block leaves phantom followers.
	if err := q.SyncFollowerCountAfterBlock(ctx, gen.SyncFollowerCountAfterBlockParams{
		ID:   blockerID,
		ID_2: blockedID,
	}); err != nil {
		return fmt.Errorf("syncing follower counts: %w", err)
	}

	// 4. Decline pending DM requests between the two users (BE-1 / Section 41).
	if err := q.DeclinePendingDMRequestsBetweenUsers(ctx, gen.DeclinePendingDMRequestsBetweenUsersParams{
		RequesterID: blockerID,
		RecipientID: blockedID,
	}); err != nil {
		return fmt.Errorf("declining pending DM requests: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing block transaction: %w", err)
	}

	s.invalidateBlockSideEffectCaches(ctx, blockerID, blockedID)

	if s.followInv != nil {
		s.followInv(blockerID, blockedID)
		s.followInv(blockedID, blockerID)
	}

	return nil
}

// invalidateBlockSideEffectCaches clears the follow-related caches whose
// state changed inside the block transaction (BLK-016): the is_following
// lookups for both directions (jc:is_following:*, 60s) and the ranked
// for-you feed slices for both users (jc:cache:feed:foryou:*). Mirrors the
// invalidation the follows service runs on toggle — best-effort, the TTLs
// self-heal on failure. Key patterns per conventions/valkey-keys-catalog.md.
func (s *Service) invalidateBlockSideEffectCaches(ctx context.Context, blockerID, blockedID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	_ = s.vk.Del(ctx,
		valkey.MustKey("is_following:%s:%s:%s", blockerID.String(), "user", blockedID.String()),
		valkey.MustKey("is_following:%s:%s:%s", blockedID.String(), "user", blockerID.String()),
	).Err()

	for _, uid := range []pgtype.UUID{blockerID, blockedID} {
		pattern := valkey.MustKey("cache:feed:foryou:%s:*", uid.String())

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
}

// UnblockUser idempotently removes a user block. Returns nil if not blocked.
// Platform-scoped per JEOS §9 (BUG-006). Ref: §41 — DELETE /users/:id/block.
func (s *Service) UnblockUser(ctx context.Context, blockerID, blockedID pgtype.UUID) error {
	if blockerID == blockedID {
		return errors.New("cannot block yourself")
	}

	return s.write.DeleteUserBlock(ctx, gen.DeleteUserBlockParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
	})
}

// BlockedUserInfo holds the blocked user's ID, profile, and creation timestamp.
type BlockedUserInfo struct {
	BlockedID   pgtype.UUID        `json:"blocked_id"`
	Username    string             `json:"username"`
	DisplayName string             `json:"display_name"`
	AvatarURL   string             `json:"avatar_url"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
}

// ListBlockedUsers returns users blocked by the given user, platform-scoped
// with profile enrichment (BUG-003, BUG-004).
// Ref: japaconnect-engineering/sections/41-spam-content-safety.md.
// CountBlockedUsers returns the caller's total block count — backs the
// meta.total sidecar on GET /me/blocked-users.
func (s *Service) CountBlockedUsers(ctx context.Context, blockerID pgtype.UUID) (int32, error) {
	return s.read.CountBlockedUsers(ctx, blockerID)
}

func (s *Service) ListBlockedUsers(ctx context.Context, blockerID pgtype.UUID, limit, offset int32) ([]BlockedUserInfo, error) {
	rows, err := s.read.ListBlockedUsers(ctx, gen.ListBlockedUsersParams{
		BlockerID: blockerID,

		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing blocked users: %w", err)
	}

	result := make([]BlockedUserInfo, len(rows))
	for i, r := range rows {
		result[i] = BlockedUserInfo{
			BlockedID:   r.BlockedID,
			Username:    r.Username,
			DisplayName: r.DisplayName,
			AvatarURL:   r.AvatarUrl,
			CreatedAt:   r.CreatedAt,
		}
	}

	return result, nil
}

// IsBlocked returns true if either user has blocked the other.
// Platform-scoped per JEOS §9 (BUG-006).
func (s *Service) IsBlocked(ctx context.Context, userA, userB pgtype.UUID) (bool, error) {
	return s.read.IsBlocked(ctx, gen.IsBlockedParams{
		BlockerID: userA,
		BlockedID: userB,
	})
}

// BlockStatus reports the directional block state between the viewer and a
// target user (BLK-005 / BLK-013 — backs GET /users/{id}/block-status):
// Blocked = viewer blocked target (drives the toggle label), BlockedBy =
// target blocked viewer (drives the profile's interaction gating).
type BlockStatus struct {
	Blocked   bool `json:"blocked"`
	BlockedBy bool `json:"blocked_by"`
}

// GetBlockStatus resolves both directions with two indexed EXISTS lookups.
func (s *Service) GetBlockStatus(ctx context.Context, viewerID, targetID pgtype.UUID) (BlockStatus, error) {
	blocked, err := s.read.IsUserBlockedDirectional(ctx, gen.IsUserBlockedDirectionalParams{
		BlockerID: viewerID,
		BlockedID: targetID,
	})
	if err != nil {
		return BlockStatus{}, fmt.Errorf("checking block status: %w", err)
	}

	blockedBy, err := s.read.IsUserBlockedDirectional(ctx, gen.IsUserBlockedDirectionalParams{
		BlockerID: targetID,
		BlockedID: viewerID,
	})
	if err != nil {
		return BlockStatus{}, fmt.Errorf("checking block status: %w", err)
	}

	return BlockStatus{Blocked: blocked, BlockedBy: blockedBy}, nil
}

// =====================
// User Mutes (Section 52)
// =====================

// MuteUser mutes a user. Returns true if muted, false if already muted (idempotent).
// Invalidates the jc:muted_users:{}:{user_id} Valkey cache (Section 52
// / VALKEY-001, BUG-003: platform-scoped).
func (s *Service) MuteUser(ctx context.Context, userID, mutedUserID pgtype.UUID) error {
	if err := s.write.MuteUser(ctx, gen.MuteUserParams{
		UserID:      userID,
		MutedUserID: mutedUserID,
	}); err != nil {
		return err
	}

	s.invalidateMutedUsersCache(ctx, userID)

	return nil
}

// UnmuteUser removes a mute.
// Invalidates the jc:muted_users:{}:{user_id} Valkey cache (Section 52
// / VALKEY-001, BUG-003: platform-scoped).
func (s *Service) UnmuteUser(ctx context.Context, userID, mutedUserID pgtype.UUID) error {
	if err := s.write.UnmuteUser(ctx, gen.UnmuteUserParams{
		UserID:      userID,
		MutedUserID: mutedUserID,
	}); err != nil {
		return err
	}

	s.invalidateMutedUsersCache(ctx, userID)

	return nil
}

// IsUserMuted checks if a user is muted.
func (s *Service) IsUserMuted(ctx context.Context, userID, mutedUserID pgtype.UUID) (bool, error) {
	return s.read.IsUserMuted(ctx, gen.IsUserMutedParams{
		UserID:      userID,
		MutedUserID: mutedUserID,
	})
}

// MutedUserInfo holds user-mute list entry data.
type MutedUserInfo struct {
	ID          pgtype.UUID        `json:"id"`
	MutedUserID pgtype.UUID        `json:"muted_user_id"`
	Username    string             `json:"username"`
	DisplayName string             `json:"display_name"`
	AvatarURL   string             `json:"avatar_url"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
}

// ListMutedUsers returns paginated muted users for the caller.
func (s *Service) ListMutedUsers(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]MutedUserInfo, error) {
	rows, err := s.read.ListMutedUsers(ctx, gen.ListMutedUsersParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing muted users: %w", err)
	}

	result := make([]MutedUserInfo, len(rows))
	for i, r := range rows {
		result[i] = MutedUserInfo{
			ID:          r.ID,
			MutedUserID: r.MutedUserID,
			Username:    r.Username,
			DisplayName: r.DisplayName,
			AvatarURL:   r.AvatarUrl,
			CreatedAt:   r.CreatedAt,
		}
	}

	return result, nil
}

// =====================
// Space Mutes (Section 52) — list only
// =====================
// Space mute/unmute *actions* and IsSpaceMuted live in internal/spaces, which
// resolves the space by slug. This package only owns the list query below.

// MutedSpaceInfo holds space-mute list entry data.
type MutedSpaceInfo struct {
	ID          pgtype.UUID        `json:"id"`
	SpaceID     pgtype.UUID        `json:"space_id"`
	SpaceName   string             `json:"space_name"`
	SpaceSlug   string             `json:"space_slug"`
	Level       string             `json:"level"`
	SpaceTypeID pgtype.UUID        `json:"space_type_id"`
	IsPrimary   *bool              `json:"is_primary"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
}

// ListMutedSpaces returns paginated muted spaces for the caller.
func (s *Service) ListMutedSpaces(ctx context.Context, userID pgtype.UUID, search, level string, spaceTypeID pgtype.UUID, limit, offset int32) ([]MutedSpaceInfo, error) {
	rows, err := s.read.ListMutedSpaces(ctx, gen.ListMutedSpacesParams{
		UserID:      userID,
		Search:      search,
		Level:       level,
		SpaceTypeID: spaceTypeID,
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing muted spaces: %w", err)
	}

	result := make([]MutedSpaceInfo, len(rows))
	for i, r := range rows {
		result[i] = MutedSpaceInfo{
			ID:          r.ID,
			SpaceID:     r.SpaceID,
			SpaceName:   r.SpaceName,
			SpaceSlug:   r.SpaceSlug,
			Level:       r.Level,
			SpaceTypeID: r.SpaceTypeID,
			IsPrimary:   r.IsPrimary,
			CreatedAt:   r.CreatedAt,
		}
	}

	return result, nil
}
