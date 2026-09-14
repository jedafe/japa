package spaces

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ToggleFollowSpace is an exported method.
func (s *Service) ToggleFollowSpace(ctx context.Context, userID, spaceID pgtype.UUID) (bool, error) {
	isFollowing, err := s.read.IsFollowing(ctx, gen.IsFollowingParams{

		FollowerID: userID,
		EntityType: "space",
		EntityID:   spaceID,
	})
	if err != nil {
		return false, fmt.Errorf("checking follow status: %w", err)
	}

	if isFollowing {
		err = s.write.DeleteFollowByFollowerEntity(ctx, gen.DeleteFollowByFollowerEntityParams{

			FollowerID: userID,
			EntityType: "space",
			EntityID:   spaceID,
		})
		if err != nil {
			return false, fmt.Errorf("unfollowing space: %w", err)
		}

		if err := s.write.DecrementSpaceFollowerCount(ctx,
			spaceID,
		); err != nil {
			return false, fmt.Errorf("decrementing follower count: %w", err)
		}

		return false, nil
	}

	err = s.write.InsertFollow(ctx, gen.InsertFollowParams{

		FollowerID: userID,
		EntityType: "space",
		EntityID:   spaceID,
	})
	if err != nil {
		return false, fmt.Errorf("following space: %w", err)
	}

	if err := s.write.IncrementSpaceFollowerCount(ctx,
		spaceID,
	); err != nil {
		return false, fmt.Errorf("incrementing follower count: %w", err)
	}

	return true, nil
}

// MuteSpace mutes a space and invalidates the
// jc:muted_spaces:{}:{user_id} Valkey cache (Section 52 /
// VALKEY-001, BUG-003: tenant-scoped).
func (s *Service) MuteSpace(ctx context.Context, userID, spaceID pgtype.UUID) error {
	if err := s.write.MuteSpace(ctx, gen.MuteSpaceParams{

		UserID:  userID,
		SpaceID: spaceID,
	}); err != nil {
		return err
	}

	s.invalidateMutedSpacesCache(ctx, userID)

	return nil
}

// UnmuteSpace removes a space mute and invalidates the
// jc:muted_spaces:{}:{user_id} Valkey cache (Section 52 /
// VALKEY-001, BUG-003: tenant-scoped).
func (s *Service) UnmuteSpace(ctx context.Context, userID, spaceID pgtype.UUID) error {
	if err := s.write.UnmuteSpace(ctx, gen.UnmuteSpaceParams{

		UserID:  userID,
		SpaceID: spaceID,
	}); err != nil {
		return err
	}

	s.invalidateMutedSpacesCache(ctx, userID)

	return nil
}

// IsSpaceMuted is an exported method.
func (s *Service) IsSpaceMuted(ctx context.Context, userID, spaceID pgtype.UUID) (bool, error) {
	return s.read.IsSpaceMuted(ctx, gen.IsSpaceMutedParams{

		UserID:  userID,
		SpaceID: spaceID,
	})
}
