// (follow_mute.go) — space mute helpers. The legacy non-transactional
// ToggleFollowSpace fallback was removed in the follows audit: the canonical
// follows.Service path is always wired and the check-then-write version
// could drift follower_count.
package spaces

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// MuteSpace mutes a space and invalidates the
// jc:muted_spaces:{}:{user_id} Valkey cache (Section 52 /
// VALKEY-001, BUG-003: platform-scoped).
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
// VALKEY-001, BUG-003: platform-scoped).
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
