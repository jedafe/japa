package gamification

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Admin operations
// ---------------------------------------------------------------------------

// CreateBadge creates a new badge definition (admin).
func (s *Service) CreateBadge(ctx context.Context, tenantID pgtype.UUID, in CreateBadgeInput) (BadgeInfo, error) {
	if in.TriggerType != TriggerTypeLifetimePoints && in.TriggerType != TriggerTypeActivityCount && in.TriggerType != TriggerTypeManual {
		return BadgeInfo{}, fmt.Errorf("gamification: invalid trigger_type %q", in.TriggerType)
	}

	if in.Slug == "" || in.Name == "" {
		return BadgeInfo{}, errors.New("gamification: slug and name are required")
	}

	arg := gen.CreateBadgeDefinitionParams{
		TenantID:           tenantID,
		Slug:               in.Slug,
		Name:               in.Name,
		Description:        in.Description,
		TriggerType:        in.TriggerType,
		TriggerValue:       in.TriggerValue,
		TriggerActivityKey: in.TriggerActivityKey,
		IsSystem:           false,
		DisplayOrder:       0,
		IsActive:           true,
	}

	if in.IconMediaID != nil {
		id, err := scanUUID(*in.IconMediaID)
		if err != nil {
			return BadgeInfo{}, fmt.Errorf("gamification: invalid icon_media_id: %w", err)
		}

		arg.IconMediaID = id
	}

	if in.IsActive != nil {
		arg.IsActive = *in.IsActive
	}

	if in.DisplayOrder != nil {
		arg.DisplayOrder = *in.DisplayOrder
	}

	badge, err := s.store.CreateBadgeDefinition(ctx, arg)
	if err != nil {
		return BadgeInfo{}, fmt.Errorf("gamification: create badge: %w", err)
	}

	return badgeToInfo(badge), nil
}

// UpdateBadge updates a badge definition's mutable fields (admin). Only
// non-nil fields in the input are updated.
func (s *Service) UpdateBadge(ctx context.Context, tenantID, badgeID pgtype.UUID, in UpdateBadgeInput) (BadgeInfo, error) {
	arg := gen.UpdateBadgeDefinitionParams{
		TenantID: tenantID,
		ID:       badgeID,
	}
	if in.Name != nil {
		arg.Name = in.Name
	}

	if in.Description != nil {
		arg.Description = in.Description
	}

	if in.IconMediaID != nil {
		id, err := scanUUID(*in.IconMediaID)
		if err != nil {
			return BadgeInfo{}, fmt.Errorf("gamification: invalid icon_media_id: %w", err)
		}

		arg.IconMediaID = id
	}

	if in.TriggerType != nil {
		arg.TriggerType = in.TriggerType
	}

	if in.TriggerValue != nil {
		arg.TriggerValue = in.TriggerValue
	}

	if in.TriggerActivityKey != nil {
		arg.TriggerActivityKey = in.TriggerActivityKey
	}

	if in.IsActive != nil {
		arg.IsActive = in.IsActive
	}

	if in.DisplayOrder != nil {
		arg.DisplayOrder = in.DisplayOrder
	}

	badge, err := s.store.UpdateBadgeDefinition(ctx, arg)
	if err != nil {
		return BadgeInfo{}, fmt.Errorf("gamification: update badge: %w", err)
	}

	return badgeToInfo(badge), nil
}

// AwardBadge manually awards a badge to a user (admin). Returns the awarded
// badge details. If the user already has the badge, returns the existing row.
func (s *Service) AwardBadge(ctx context.Context, tenantID, badgeID, userID, awardedBy pgtype.UUID) (BadgeWithDetails, error) {
	// Verify the badge exists.
	badge, err := s.store.GetBadgeDefinitionByID(ctx, tenantID, badgeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BadgeWithDetails{}, errors.New("gamification: badge not found")
		}

		return BadgeWithDetails{}, fmt.Errorf("gamification: get badge: %w", err)
	}

	_, err = s.store.AwardBadge(ctx, gen.AwardBadgeParams{
		TenantID:  tenantID,
		UserID:    userID,
		BadgeID:   badgeID,
		AwardedBy: awardedBy,
	})
	if err != nil {
		// ON CONFLICT DO NOTHING returns no rows if duplicate — check existing.
		_, getErr := s.store.GetUserBadge(ctx, tenantID, userID, badgeID)
		if getErr != nil {
			return BadgeWithDetails{}, fmt.Errorf("gamification: award badge: %w", err)
		}
	}

	var iconID *string

	if badge.IconMediaID.Valid {
		s := badge.IconMediaID.String()
		iconID = &s
	}

	// Fetch the awarded_at timestamp from the user_badge row.
	ub, err := s.store.GetUserBadge(ctx, tenantID, userID, badgeID)
	if err != nil {
		return BadgeWithDetails{
			ID:          uuidStr(badge.ID),
			Slug:        badge.Slug,
			Name:        badge.Name,
			Description: ptrStr(badge.Description),
			IconMediaID: iconID,
		}, nil
	}

	return BadgeWithDetails{
		ID:          uuidStr(badge.ID),
		Slug:        badge.Slug,
		Name:        badge.Name,
		Description: ptrStr(badge.Description),
		IconMediaID: iconID,
		AwardedAt:   rfc3339(ub.AwardedAt),
	}, nil
}

// ListAllBadges returns all badge definitions (admin view, includes inactive).
func (s *Service) ListAllBadges(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]BadgeInfo, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	badges, total, err := s.store.ListBadgeDefinitionsPaginated(ctx, tenantID, false, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: list all badges: %w", err)
	}

	results := make([]BadgeInfo, 0, len(badges))
	for _, b := range badges {
		results = append(results, badgeToInfo(b))
	}

	return results, total, nil
}

// ListAllChallenges returns all challenges (admin view, includes inactive), paginated.
func (s *Service) ListAllChallenges(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]ChallengeInfo, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	challenges, total, err := s.store.ListAllChallenges(ctx, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: list all challenges: %w", err)
	}

	results := make([]ChallengeInfo, 0, len(challenges))
	for _, c := range challenges {
		results = append(results, challengeToInfo(c))
	}

	return results, total, nil
}

// CreateChallenge creates a new challenge definition (admin).
func (s *Service) CreateChallenge(ctx context.Context, tenantID pgtype.UUID, in CreateChallengeInput) (ChallengeInfo, error) {
	if in.Slug == "" || in.Name == "" || in.ActivityKey == "" {
		return ChallengeInfo{}, errors.New("gamification: slug, name, and activity_key are required")
	}

	if in.TargetCount <= 0 {
		return ChallengeInfo{}, errors.New("gamification: target_count must be positive")
	}

	startsAt, err := scanTimestamp(in.StartsAt)
	if err != nil {
		return ChallengeInfo{}, err
	}

	if !startsAt.Valid {
		return ChallengeInfo{}, errors.New("gamification: starts_at is required")
	}

	arg := gen.CreateChallengeParams{
		TenantID:     tenantID,
		Slug:         in.Slug,
		Name:         in.Name,
		Description:  in.Description,
		ActivityKey:  in.ActivityKey,
		TargetCount:  in.TargetCount,
		RewardPoints: in.RewardPoints,
		StartsAt:     startsAt,
		IsActive:     true,
	}

	if in.ImageMediaID != nil {
		id, err := scanUUID(*in.ImageMediaID)
		if err != nil {
			return ChallengeInfo{}, fmt.Errorf("gamification: invalid image_media_id: %w", err)
		}

		arg.ImageMediaID = id
	}

	if in.RewardBadgeID != nil {
		id, err := scanUUID(*in.RewardBadgeID)
		if err != nil {
			return ChallengeInfo{}, fmt.Errorf("gamification: invalid reward_badge_id: %w", err)
		}

		arg.RewardBadgeID = id
	}

	if in.EndsAt != nil {
		endsAt, err := scanTimestamp(*in.EndsAt)
		if err != nil {
			return ChallengeInfo{}, err
		}

		arg.EndsAt = endsAt
	}

	if in.IsActive != nil {
		arg.IsActive = *in.IsActive
	}

	challenge, err := s.store.CreateChallenge(ctx, arg)
	if err != nil {
		return ChallengeInfo{}, fmt.Errorf("gamification: create challenge: %w", err)
	}

	return challengeToInfo(challenge), nil
}

// UpdateChallenge updates a challenge definition's mutable fields (admin).
func (s *Service) UpdateChallenge(ctx context.Context, tenantID, challengeID pgtype.UUID, in UpdateChallengeInput) (ChallengeInfo, error) {
	arg := gen.UpdateChallengeParams{
		TenantID: tenantID,
		ID:       challengeID,
	}
	if in.Name != nil {
		arg.Name = in.Name
	}

	if in.Description != nil {
		arg.Description = in.Description
	}

	if in.ImageMediaID != nil {
		id, err := scanUUID(*in.ImageMediaID)
		if err != nil {
			return ChallengeInfo{}, fmt.Errorf("gamification: invalid image_media_id: %w", err)
		}

		arg.ImageMediaID = id
	}

	if in.ActivityKey != nil {
		arg.ActivityKey = in.ActivityKey
	}

	if in.TargetCount != nil {
		arg.TargetCount = in.TargetCount
	}

	if in.RewardPoints != nil {
		arg.RewardPoints = in.RewardPoints
	}

	if in.RewardBadgeID != nil {
		id, err := scanUUID(*in.RewardBadgeID)
		if err != nil {
			return ChallengeInfo{}, fmt.Errorf("gamification: invalid reward_badge_id: %w", err)
		}

		arg.RewardBadgeID = id
	}

	if in.StartsAt != nil {
		startsAt, err := scanTimestamp(*in.StartsAt)
		if err != nil {
			return ChallengeInfo{}, err
		}

		arg.StartsAt = startsAt
	}

	if in.EndsAt != nil {
		endsAt, err := scanTimestamp(*in.EndsAt)
		if err != nil {
			return ChallengeInfo{}, err
		}

		arg.EndsAt = endsAt
	}

	if in.IsActive != nil {
		arg.IsActive = in.IsActive
	}

	challenge, err := s.store.UpdateChallenge(ctx, arg)
	if err != nil {
		return ChallengeInfo{}, fmt.Errorf("gamification: update challenge: %w", err)
	}

	return challengeToInfo(challenge), nil
}

// DeleteBadge deletes a non-system badge definition (admin). System badges
// cannot be deleted — the query filters them out and returns 0 rows affected.
func (s *Service) DeleteBadge(ctx context.Context, tenantID, badgeID pgtype.UUID) error {
	count, err := s.store.DeleteBadgeDefinition(ctx, tenantID, badgeID)
	if err != nil {
		return fmt.Errorf("gamification: delete badge: %w", err)
	}

	if count == 0 {
		return errors.New("gamification: badge not found or is a system badge")
	}

	return nil
}

// DeleteChallenge deletes a challenge definition (admin).
func (s *Service) DeleteChallenge(ctx context.Context, tenantID, challengeID pgtype.UUID) error {
	count, err := s.store.DeleteChallenge(ctx, tenantID, challengeID)
	if err != nil {
		return fmt.Errorf("gamification: delete challenge: %w", err)
	}

	if count == 0 {
		return errors.New("gamification: challenge not found")
	}

	return nil
}
