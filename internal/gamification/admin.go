package gamification

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Admin operations
// ---------------------------------------------------------------------------

// ErrBadgeNotFound is returned when a badge definition does not exist —
// handlers map it to 404 (PTS-012).
var ErrBadgeNotFound = errors.New("gamification: badge not found")

// ErrChallengeNotFound is returned when a challenge definition does not
// exist — handlers map it to 404.
var ErrChallengeNotFound = errors.New("gamification: challenge not found")

// ErrUserNotFound is returned when an award targets a user id that does
// not exist — handlers map it to 404.
var ErrUserNotFound = errors.New("gamification: user not found")

// ErrInvalidInput marks admin create/update validation failures so handlers
// can surface the specific message in the error envelope instead of a
// generic "failed" string.
var ErrInvalidInput = errors.New("gamification: invalid input")

// ErrSlugExists marks a unique-slug violation on create — handlers map it
// to 409.
var ErrSlugExists = errors.New("gamification: slug already exists")

// invalidInput wraps a validation detail as ErrInvalidInput.
func invalidInput(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// isUniqueViolation reports whether err wraps a Postgres 23505 unique
// constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// validateBadgeTrigger checks the trigger_type → trigger_value /
// trigger_activity_key combination required by §64 (lifetime_points and
// activity_count need a threshold; activity_count also needs the key).
func validateBadgeTrigger(triggerType string, value *int32, activityKey *string) error {
	switch triggerType {
	case TriggerTypeLifetimePoints:
		if value == nil || *value <= 0 {
			return invalidInput("trigger_value must be a positive point threshold for lifetime_points badges")
		}
	case TriggerTypeActivityCount:
		if value == nil || *value <= 0 {
			return invalidInput("trigger_value must be a positive count for activity_count badges")
		}
		if activityKey == nil || *activityKey == "" {
			return invalidInput("trigger_activity_key is required for activity_count badges")
		}
	case TriggerTypeManual:
		// Manual badges carry no trigger requirements.
	default:
		return invalidInput("invalid trigger_type %q", triggerType)
	}
	return nil
}

// CreateBadge creates a new badge definition (admin).
func (s *Service) CreateBadge(ctx context.Context, in CreateBadgeInput) (BadgeInfo, error) {
	if in.Slug == "" || in.Name == "" {
		return BadgeInfo{}, invalidInput("slug and name are required")
	}

	if err := validateBadgeTrigger(in.TriggerType, in.TriggerValue, in.TriggerActivityKey); err != nil {
		return BadgeInfo{}, err
	}

	arg := gen.CreateBadgeDefinitionParams{
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
		if isUniqueViolation(err) {
			return BadgeInfo{}, ErrSlugExists
		}
		return BadgeInfo{}, fmt.Errorf("gamification: create badge: %w", err)
	}

	return badgeToInfo(badge), nil
}

// UpdateBadge updates a badge definition's mutable fields (admin). Only
// non-nil fields in the input are updated; the clear_* flags NULL out the
// matching nullable column.
func (s *Service) UpdateBadge(ctx context.Context, badgeID pgtype.UUID, in UpdateBadgeInput) (BadgeInfo, error) {
	arg := gen.UpdateBadgeDefinitionParams{
		ID: badgeID,
	}
	if in.Name != nil {
		if *in.Name == "" {
			return BadgeInfo{}, invalidInput("name cannot be empty")
		}
		arg.Name = in.Name
	}

	if in.Description != nil {
		arg.Description = in.Description
	}

	if in.IconMediaID != nil {
		id, err := scanUUID(*in.IconMediaID)
		if err != nil {
			return BadgeInfo{}, invalidInput("invalid icon_media_id")
		}

		arg.IconMediaID = id
	}
	arg.ClearIconMediaID = &in.ClearIconMediaID

	if in.TriggerType != nil {
		arg.TriggerType = in.TriggerType
	}

	if in.TriggerValue != nil {
		arg.TriggerValue = in.TriggerValue
	}
	arg.ClearTriggerValue = &in.ClearTriggerValue

	if in.TriggerActivityKey != nil {
		arg.TriggerActivityKey = in.TriggerActivityKey
	}
	arg.ClearTriggerActivityKey = &in.ClearTriggerActivityKey

	if in.IsActive != nil {
		arg.IsActive = in.IsActive
	}

	if in.DisplayOrder != nil {
		arg.DisplayOrder = in.DisplayOrder
	}

	// Validate the effective trigger combination — a partial update may
	// change trigger_type while leaving the old value/key in place, or
	// clear a field the new trigger still requires.
	current, err := s.store.GetBadgeDefinitionByID(ctx, badgeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BadgeInfo{}, ErrBadgeNotFound
		}
		return BadgeInfo{}, fmt.Errorf("gamification: get badge: %w", err)
	}

	effectiveType := current.TriggerType
	if in.TriggerType != nil {
		effectiveType = *in.TriggerType
	}
	effectiveValue := current.TriggerValue
	if in.ClearTriggerValue {
		effectiveValue = nil
	} else if in.TriggerValue != nil {
		effectiveValue = in.TriggerValue
	}
	effectiveKey := current.TriggerActivityKey
	if in.ClearTriggerActivityKey {
		effectiveKey = nil
	} else if in.TriggerActivityKey != nil {
		effectiveKey = in.TriggerActivityKey
	}
	if err := validateBadgeTrigger(effectiveType, effectiveValue, effectiveKey); err != nil {
		return BadgeInfo{}, err
	}

	badge, err := s.store.UpdateBadgeDefinition(ctx, arg)
	if err != nil {
		return BadgeInfo{}, fmt.Errorf("gamification: update badge: %w", err)
	}

	return badgeToInfo(badge), nil
}

// AwardBadge manually awards a badge to a user (admin). Returns the awarded
// badge details. If the user already has the badge, returns the existing row.
func (s *Service) AwardBadge(ctx context.Context, badgeID, userID, awardedBy pgtype.UUID) (BadgeWithDetails, error) {
	// Verify the badge exists.
	badge, err := s.store.GetBadgeDefinitionByID(ctx, badgeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BadgeWithDetails{}, ErrBadgeNotFound
		}

		return BadgeWithDetails{}, fmt.Errorf("gamification: get badge: %w", err)
	}

	_, err = s.store.AwardBadge(ctx, gen.AwardBadgeParams{
		UserID:    userID,
		BadgeID:   badgeID,
		AwardedBy: awardedBy,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return BadgeWithDetails{}, ErrUserNotFound
		}
		// ON CONFLICT DO NOTHING returns no rows if duplicate — check existing.
		_, getErr := s.store.GetUserBadge(ctx, userID, badgeID)
		if getErr != nil {
			return BadgeWithDetails{}, fmt.Errorf("gamification: award badge: %w", err)
		}
	} else {
		// Fresh award — notify the recipient (fire-and-forget, Rule 8).
		s.notifyBadgeAwarded(ctx, userID, badge.Name)
	}

	var iconID *string

	if badge.IconMediaID.Valid {
		s := badge.IconMediaID.String()
		iconID = &s
	}

	// Fetch the awarded_at timestamp from the user_badge row.
	ub, err := s.store.GetUserBadge(ctx, userID, badgeID)
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
func (s *Service) ListAllBadges(ctx context.Context, limit, offset int32) ([]BadgeInfo, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	badges, total, err := s.store.ListBadgeDefinitionsPaginated(ctx, false, limit, offset)
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
func (s *Service) ListAllChallenges(ctx context.Context, limit, offset int32) ([]ChallengeInfo, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	challenges, total, err := s.store.ListAllChallenges(ctx, limit, offset)
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
func (s *Service) CreateChallenge(ctx context.Context, in CreateChallengeInput) (ChallengeInfo, error) {
	if in.Slug == "" || in.Name == "" || in.ActivityKey == "" {
		return ChallengeInfo{}, invalidInput("slug, name, and activity_key are required")
	}

	if in.TargetCount <= 0 {
		return ChallengeInfo{}, invalidInput("target_count must be positive")
	}

	startsAt, err := scanTimestamp(in.StartsAt)
	if err != nil {
		return ChallengeInfo{}, invalidInput("invalid starts_at: expected RFC 3339 timestamp")
	}

	if !startsAt.Valid {
		return ChallengeInfo{}, invalidInput("starts_at is required")
	}

	arg := gen.CreateChallengeParams{
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
			return ChallengeInfo{}, invalidInput("invalid image_media_id")
		}

		arg.ImageMediaID = id
	}

	if in.RewardBadgeID != nil {
		id, err := scanUUID(*in.RewardBadgeID)
		if err != nil {
			return ChallengeInfo{}, invalidInput("invalid reward_badge_id")
		}

		arg.RewardBadgeID = id
	}

	if in.EndsAt != nil {
		endsAt, err := scanTimestamp(*in.EndsAt)
		if err != nil {
			return ChallengeInfo{}, invalidInput("invalid ends_at: expected RFC 3339 timestamp")
		}

		if endsAt.Valid && endsAt.Time.Before(startsAt.Time) {
			return ChallengeInfo{}, invalidInput("ends_at must be after starts_at")
		}

		arg.EndsAt = endsAt
	}

	if in.IsActive != nil {
		arg.IsActive = *in.IsActive
	}

	challenge, err := s.store.CreateChallenge(ctx, arg)
	if err != nil {
		if isUniqueViolation(err) {
			return ChallengeInfo{}, ErrSlugExists
		}
		return ChallengeInfo{}, fmt.Errorf("gamification: create challenge: %w", err)
	}

	return challengeToInfo(challenge), nil
}

// UpdateChallenge updates a challenge definition's mutable fields (admin).
// Only non-nil fields are updated; the clear_* flags NULL out the matching
// nullable column.
func (s *Service) UpdateChallenge(ctx context.Context, challengeID pgtype.UUID, in UpdateChallengeInput) (ChallengeInfo, error) {
	arg := gen.UpdateChallengeParams{
		ID: challengeID,
	}
	if in.Name != nil {
		if *in.Name == "" {
			return ChallengeInfo{}, invalidInput("name cannot be empty")
		}
		arg.Name = in.Name
	}

	if in.Description != nil {
		arg.Description = in.Description
	}

	if in.ImageMediaID != nil {
		id, err := scanUUID(*in.ImageMediaID)
		if err != nil {
			return ChallengeInfo{}, invalidInput("invalid image_media_id")
		}

		arg.ImageMediaID = id
	}
	arg.ClearImageMediaID = &in.ClearImageMediaID

	if in.ActivityKey != nil {
		if *in.ActivityKey == "" {
			return ChallengeInfo{}, invalidInput("activity_key cannot be empty")
		}
		arg.ActivityKey = in.ActivityKey
	}

	if in.TargetCount != nil {
		if *in.TargetCount <= 0 {
			return ChallengeInfo{}, invalidInput("target_count must be positive")
		}
		arg.TargetCount = in.TargetCount
	}

	if in.RewardPoints != nil {
		arg.RewardPoints = in.RewardPoints
	}

	if in.RewardBadgeID != nil {
		id, err := scanUUID(*in.RewardBadgeID)
		if err != nil {
			return ChallengeInfo{}, invalidInput("invalid reward_badge_id")
		}

		arg.RewardBadgeID = id
	}
	arg.ClearRewardBadgeID = &in.ClearRewardBadgeID

	if in.StartsAt != nil {
		startsAt, err := scanTimestamp(*in.StartsAt)
		if err != nil {
			return ChallengeInfo{}, invalidInput("invalid starts_at: expected RFC 3339 timestamp")
		}

		arg.StartsAt = startsAt
	}

	if in.EndsAt != nil {
		endsAt, err := scanTimestamp(*in.EndsAt)
		if err != nil {
			return ChallengeInfo{}, invalidInput("invalid ends_at: expected RFC 3339 timestamp")
		}

		arg.EndsAt = endsAt
	}
	arg.ClearEndsAt = &in.ClearEndsAt

	if in.IsActive != nil {
		arg.IsActive = in.IsActive
	}

	// Validate the effective start/end ordering — a partial update may move
	// ends_at before starts_at.
	if !in.ClearEndsAt && (in.StartsAt != nil || in.EndsAt != nil) {
		current, err := s.store.GetChallengeByID(ctx, challengeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ChallengeInfo{}, ErrChallengeNotFound
			}
			return ChallengeInfo{}, fmt.Errorf("gamification: get challenge: %w", err)
		}

		effectiveStart := current.StartsAt
		if arg.StartsAt.Valid {
			effectiveStart = arg.StartsAt
		}
		effectiveEnd := current.EndsAt
		if in.EndsAt != nil {
			effectiveEnd = arg.EndsAt
		}
		if effectiveEnd.Valid && effectiveStart.Valid && effectiveEnd.Time.Before(effectiveStart.Time) {
			return ChallengeInfo{}, invalidInput("ends_at must be after starts_at")
		}
	}

	challenge, err := s.store.UpdateChallenge(ctx, arg)
	if err != nil {
		return ChallengeInfo{}, fmt.Errorf("gamification: update challenge: %w", err)
	}

	return challengeToInfo(challenge), nil
}

// DeleteBadge deletes a non-system badge definition (admin). System badges
// cannot be deleted — the query filters them out and returns 0 rows affected.
func (s *Service) DeleteBadge(ctx context.Context, badgeID pgtype.UUID) error {
	count, err := s.store.DeleteBadgeDefinition(ctx, badgeID)
	if err != nil {
		return fmt.Errorf("gamification: delete badge: %w", err)
	}

	if count == 0 {
		return errors.New("gamification: badge not found or is a system badge")
	}

	return nil
}

// DeleteChallenge deletes a challenge definition (admin).
func (s *Service) DeleteChallenge(ctx context.Context, challengeID pgtype.UUID) error {
	count, err := s.store.DeleteChallenge(ctx, challengeID)
	if err != nil {
		return fmt.Errorf("gamification: delete challenge: %w", err)
	}

	if count == 0 {
		return errors.New("gamification: challenge not found")
	}

	return nil
}
