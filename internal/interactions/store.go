package interactions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// BlockUser idempotently blocks a user and cancels side-effects per Section 41
// line 147: "block a user (also cancels any DM/follow)". The block insert,
// follow deletion (both directions), and pending DM request decline run in a
// single transaction so a partial failure leaves no orphaned follow/DM.
// Tenant-scoped per JEOS §9 (BUG-006). Ref: §41 — POST /users/:id/block.
func (s *Service) BlockUser(ctx context.Context, tenantID, blockerID, blockedID pgtype.UUID) error {
	if blockerID == blockedID {
		return errors.New("cannot block yourself")
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

	// 3. Decline pending DM requests between the two users (BE-1 / Section 41).
	if err := q.DeclinePendingDMRequestsBetweenUsers(ctx, gen.DeclinePendingDMRequestsBetweenUsersParams{
		RequesterID: blockerID,
		RecipientID: blockedID,
	}); err != nil {
		return fmt.Errorf("declining pending DM requests: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing block transaction: %w", err)
	}

	return nil
}

// UnblockUser idempotently removes a user block. Returns nil if not blocked.
// Tenant-scoped per JEOS §9 (BUG-006). Ref: §41 — DELETE /users/:id/block.
func (s *Service) UnblockUser(ctx context.Context, tenantID, blockerID, blockedID pgtype.UUID) error {
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

// ListBlockedUsers returns users blocked by the given user, tenant-scoped
// with profile enrichment (BUG-003, BUG-004).
// Ref: japaconnect-engineering/sections/41-spam-content-safety.md.
func (s *Service) ListBlockedUsers(ctx context.Context, tenantID, blockerID pgtype.UUID, limit, offset int32) ([]BlockedUserInfo, error) {
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
// Tenant-scoped per JEOS §9 (BUG-006).
func (s *Service) IsBlocked(ctx context.Context, tenantID, userA, userB pgtype.UUID) (bool, error) {
	return s.read.IsBlocked(ctx, gen.IsBlockedParams{
		BlockerID: userA,
		BlockedID: userB,
	})
}

// =====================
// User Mutes (Section 52)
// =====================

// MuteUser mutes a user. Returns true if muted, false if already muted (idempotent).
// Invalidates the jc:muted_users:{}:{user_id} Valkey cache (Section 52
// / VALKEY-001, BUG-003: tenant-scoped).
func (s *Service) MuteUser(ctx context.Context, tenantID, userID, mutedUserID pgtype.UUID) error {
	if err := s.write.MuteUser(ctx, gen.MuteUserParams{

		UserID:      userID,
		MutedUserID: mutedUserID,
	}); err != nil {
		return err
	}

	s.invalidateMutedUsersCache(ctx, tenantID, userID)

	return nil
}

// UnmuteUser removes a mute.
// Invalidates the jc:muted_users:{}:{user_id} Valkey cache (Section 52
// / VALKEY-001, BUG-003: tenant-scoped).
func (s *Service) UnmuteUser(ctx context.Context, tenantID, userID, mutedUserID pgtype.UUID) error {
	if err := s.write.UnmuteUser(ctx, gen.UnmuteUserParams{

		UserID:      userID,
		MutedUserID: mutedUserID,
	}); err != nil {
		return err
	}

	s.invalidateMutedUsersCache(ctx, tenantID, userID)

	return nil
}

// IsUserMuted checks if a user is muted.
func (s *Service) IsUserMuted(ctx context.Context, tenantID, userID, mutedUserID pgtype.UUID) (bool, error) {
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
func (s *Service) ListMutedUsers(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]MutedUserInfo, error) {
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
func (s *Service) ListMutedSpaces(ctx context.Context, tenantID, userID pgtype.UUID, search, level string, spaceTypeID pgtype.UUID, limit, offset int32) ([]MutedSpaceInfo, error) {
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
