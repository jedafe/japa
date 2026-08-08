package interactions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ToggleBlock adds or removes a user block. Returns true if blocked, false if unblocked.
// Tenant-scoped per JEOS §9 (BUG-006).
func (s *Service) ToggleBlock(ctx context.Context, tenantID, blockerID, blockedID pgtype.UUID) (bool, error) {
	if blockerID == blockedID {
		return false, errors.New("cannot block yourself")
	}

	existing, err := s.read.GetUserBlocks(ctx, blockerID)
	if err != nil {
		return false, fmt.Errorf("listing user blocks: %w", err)
	}

	for _, b := range existing {
		if b.BlockedID == blockedID {
			if err := s.write.DeleteUserBlock(ctx, gen.DeleteUserBlockParams{
				BlockerID: blockerID,
				BlockedID: blockedID,
				TenantID:  tenantID,
			}); err != nil {
				return false, fmt.Errorf("deleting user block: %w", err)
			}

			return false, nil
		}
	}

	if err := s.write.CreateUserBlock(ctx, gen.CreateUserBlockParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
		TenantID:  tenantID,
	}); err != nil {
		return false, fmt.Errorf("creating user block: %w", err)
	}

	return true, nil
}

// BlockedUserInfo holds the blocked user's ID and creation timestamp.
type BlockedUserInfo struct {
	BlockedID pgtype.UUID        `json:"blocked_id"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

// ListBlockedUsers returns users blocked by the given user.
func (s *Service) ListBlockedUsers(ctx context.Context, blockerID pgtype.UUID) ([]BlockedUserInfo, error) {
	rows, err := s.read.GetUserBlocks(ctx, blockerID)
	if err != nil {
		return nil, fmt.Errorf("listing blocked users: %w", err)
	}

	result := make([]BlockedUserInfo, len(rows))
	for i, r := range rows {
		result[i] = BlockedUserInfo{
			BlockedID: r.BlockedID,
			CreatedAt: r.CreatedAt,
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
		TenantID:  tenantID,
	})
}

// =====================
// User Mutes (Section 52)
// =====================

// MuteUser mutes a user. Returns true if muted, false if already muted (idempotent).
// Invalidates the jc:muted_users:{user_id} Valkey cache (Section 52 / VALKEY-001).
func (s *Service) MuteUser(ctx context.Context, tenantID, userID, mutedUserID pgtype.UUID) error {
	if err := s.write.MuteUser(ctx, gen.MuteUserParams{
		TenantID:    tenantID,
		UserID:      userID,
		MutedUserID: mutedUserID,
	}); err != nil {
		return err
	}
	s.invalidateMutedUsersCache(ctx, userID)
	return nil
}

// UnmuteUser removes a mute.
// Invalidates the jc:muted_users:{user_id} Valkey cache (Section 52 / VALKEY-001).
func (s *Service) UnmuteUser(ctx context.Context, tenantID, userID, mutedUserID pgtype.UUID) error {
	if err := s.write.UnmuteUser(ctx, gen.UnmuteUserParams{
		TenantID:    tenantID,
		UserID:      userID,
		MutedUserID: mutedUserID,
	}); err != nil {
		return err
	}
	s.invalidateMutedUsersCache(ctx, userID)
	return nil
}

// IsUserMuted checks if a user is muted.
func (s *Service) IsUserMuted(ctx context.Context, tenantID, userID, mutedUserID pgtype.UUID) (bool, error) {
	return s.read.IsUserMuted(ctx, gen.IsUserMutedParams{
		TenantID:    tenantID,
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
		TenantID:   tenantID,
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
// Space Mutes (Section 52)
// =====================

// MuteSpace mutes a space.
// Invalidates the jc:muted_spaces:{user_id} Valkey cache (Section 52 / VALKEY-001).
func (s *Service) MuteSpace(ctx context.Context, tenantID, userID, spaceID pgtype.UUID) error {
	if err := s.write.MuteSpace(ctx, gen.MuteSpaceParams{
		TenantID: tenantID,
		UserID:   userID,
		SpaceID:  spaceID,
	}); err != nil {
		return err
	}
	s.invalidateMutedSpacesCache(ctx, userID)
	return nil
}

// UnmuteSpace removes a space mute.
// Invalidates the jc:muted_spaces:{user_id} Valkey cache (Section 52 / VALKEY-001).
func (s *Service) UnmuteSpace(ctx context.Context, tenantID, userID, spaceID pgtype.UUID) error {
	if err := s.write.UnmuteSpace(ctx, gen.UnmuteSpaceParams{
		TenantID: tenantID,
		UserID:   userID,
		SpaceID:  spaceID,
	}); err != nil {
		return err
	}
	s.invalidateMutedSpacesCache(ctx, userID)
	return nil
}

// IsSpaceMuted checks if a space is muted.
func (s *Service) IsSpaceMuted(ctx context.Context, tenantID, userID, spaceID pgtype.UUID) (bool, error) {
	return s.read.IsSpaceMuted(ctx, gen.IsSpaceMutedParams{
		TenantID: tenantID,
		UserID:   userID,
		SpaceID:  spaceID,
	})
}

// MutedSpaceInfo holds space-mute list entry data.
type MutedSpaceInfo struct {
	ID        pgtype.UUID        `json:"id"`
	SpaceID   pgtype.UUID        `json:"space_id"`
	SpaceName string             `json:"space_name"`
	SpaceSlug string             `json:"space_slug"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

// ListMutedSpaces returns paginated muted spaces for the caller.
func (s *Service) ListMutedSpaces(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]MutedSpaceInfo, error) {
	rows, err := s.read.ListMutedSpaces(ctx, gen.ListMutedSpacesParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing muted spaces: %w", err)
	}

	result := make([]MutedSpaceInfo, len(rows))
	for i, r := range rows {
		result[i] = MutedSpaceInfo{
			ID:        r.ID,
			SpaceID:   r.SpaceID,
			SpaceName: r.SpaceName,
			SpaceSlug: r.SpaceSlug,
			CreatedAt: r.CreatedAt,
		}
	}

	return result, nil
}
