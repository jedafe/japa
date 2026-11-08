package gamification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
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

// ErrSeasonNotFound is returned when a season does not exist — handlers
// map it to 404.
var ErrSeasonNotFound = errors.New("gamification: season not found")

// ErrTierNotFound is returned when a badge-tier row does not exist —
// handlers map it to 404.
var ErrTierNotFound = errors.New("gamification: tier not found")

// ErrTierExists marks a unique (badge_group, tier) violation on create —
// handlers map it to 409.
var ErrTierExists = errors.New("gamification: tier already exists for this badge group")

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
	case TriggerTypeStreakDays:
		if value == nil || *value <= 0 {
			return invalidInput("trigger_value must be a positive day count for streak_days badges")
		}
		if activityKey == nil || *activityKey == "" {
			return invalidInput("trigger_activity_key is required for streak_days badges")
		}
	case TriggerTypeManual:
		// Manual badges carry no trigger requirements.
	default:
		return invalidInput("invalid trigger_type %q", triggerType)
	}
	return nil
}

// validateFeatureFlag checks an optional feature_flag link against the §42
// registry — an unregistered key would resolve disabled forever and silently
// hide the item, so it is rejected up front. NULL/empty is always valid
// (item is not tied to a feature).
func validateFeatureFlag(flag *string) error {
	if flag == nil || *flag == "" {
		return nil
	}
	if _, ok := features.RegistryLookup(*flag); !ok {
		return invalidInput("unknown feature_flag %q", *flag)
	}
	return nil
}

// normalizeFeatureFlag collapses "" to nil — an empty flag means "not
// linked", identical to NULL everywhere downstream; the column should not
// accumulate ” next to NULL rows.
func normalizeFeatureFlag(flag *string) *string {
	if flag != nil && *flag == "" {
		return nil
	}
	return flag
}

// validRarity reports whether s is a catalogue rarity value.
func validRarity(s string) bool {
	switch s {
	case "common", "uncommon", "rare", "epic", "legendary", "mythic":
		return true
	}
	return false
}

// validTier reports whether s is a progression tier value.
func validTier(s string) bool {
	switch s {
	case "bronze", "silver", "gold", "platinum", "diamond":
		return true
	}
	return false
}

// CreateBadge creates a new badge definition (admin).
func (s *Service) CreateBadge(ctx context.Context, in CreateBadgeInput) (BadgeInfo, error) {
	if in.Slug == "" || in.Name == "" {
		return BadgeInfo{}, invalidInput("slug and name are required")
	}

	if err := validateBadgeTrigger(in.TriggerType, in.TriggerValue, in.TriggerActivityKey); err != nil {
		return BadgeInfo{}, err
	}

	if err := validateFeatureFlag(in.FeatureFlag); err != nil {
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
		Rarity:             "common",
	}

	if in.Rarity != nil {
		if !validRarity(*in.Rarity) {
			return BadgeInfo{}, invalidInput("invalid rarity %q", *in.Rarity)
		}
		arg.Rarity = *in.Rarity
	}

	if in.Tier != nil {
		if !validTier(*in.Tier) {
			return BadgeInfo{}, invalidInput("invalid tier %q", *in.Tier)
		}
		arg.Tier = in.Tier
	}

	arg.Category = in.Category
	arg.BadgeGroup = in.BadgeGroup
	arg.AccentColor = in.AccentColor
	arg.FeatureFlag = normalizeFeatureFlag(in.FeatureFlag)

	if in.IsSecret != nil {
		arg.IsSecret = *in.IsSecret
	}
	if in.IsLimited != nil {
		arg.IsLimited = *in.IsLimited
	}
	if in.IsFeatured != nil {
		arg.IsFeatured = *in.IsFeatured
	}
	if in.PointsReward != nil {
		if *in.PointsReward < 0 {
			return BadgeInfo{}, invalidInput("points_reward cannot be negative")
		}
		arg.PointsReward = *in.PointsReward
	}
	if in.AvailableFrom != nil {
		ts, err := scanTimestamp(*in.AvailableFrom)
		if err != nil {
			return BadgeInfo{}, invalidInput("invalid available_from: expected RFC 3339 timestamp")
		}
		arg.AvailableFrom = ts
	}
	if in.AvailableUntil != nil {
		ts, err := scanTimestamp(*in.AvailableUntil)
		if err != nil {
			return BadgeInfo{}, invalidInput("invalid available_until: expected RFC 3339 timestamp")
		}
		arg.AvailableUntil = ts
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

	if in.IconMediaID != nil {
		s.knockoutArtwork(ctx, *in.IconMediaID, badgeIconMaxDim)
	}

	info := s.badgeToInfo(ctx, badge)
	info.IconURL = s.mediaURL(ctx, badge.IconMediaID)
	return info, nil
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

	if in.Category != nil {
		arg.Category = in.Category
	}
	arg.ClearCategory = &in.ClearCategory

	if in.Rarity != nil {
		if !validRarity(*in.Rarity) {
			return BadgeInfo{}, invalidInput("invalid rarity %q", *in.Rarity)
		}
		arg.Rarity = in.Rarity
	}

	if in.Tier != nil {
		if !validTier(*in.Tier) {
			return BadgeInfo{}, invalidInput("invalid tier %q", *in.Tier)
		}
		arg.Tier = in.Tier
	}
	arg.ClearTier = &in.ClearTier

	if in.BadgeGroup != nil {
		arg.BadgeGroup = in.BadgeGroup
	}
	arg.ClearBadgeGroup = &in.ClearBadgeGroup
	arg.AccentColor = in.AccentColor
	arg.ClearAccentColor = &in.ClearAccentColor

	if in.IsSecret != nil {
		arg.IsSecret = in.IsSecret
	}
	if in.IsLimited != nil {
		arg.IsLimited = in.IsLimited
	}
	if in.IsFeatured != nil {
		arg.IsFeatured = in.IsFeatured
	}
	if in.PointsReward != nil {
		if *in.PointsReward < 0 {
			return BadgeInfo{}, invalidInput("points_reward cannot be negative")
		}
		arg.PointsReward = in.PointsReward
	}
	if in.AvailableFrom != nil {
		ts, err := scanTimestamp(*in.AvailableFrom)
		if err != nil {
			return BadgeInfo{}, invalidInput("invalid available_from: expected RFC 3339 timestamp")
		}
		arg.AvailableFrom = ts
	}
	arg.ClearAvailableFrom = &in.ClearAvailableFrom
	if in.AvailableUntil != nil {
		ts, err := scanTimestamp(*in.AvailableUntil)
		if err != nil {
			return BadgeInfo{}, invalidInput("invalid available_until: expected RFC 3339 timestamp")
		}
		arg.AvailableUntil = ts
	}
	arg.ClearAvailableUntil = &in.ClearAvailableUntil

	if err := validateFeatureFlag(in.FeatureFlag); err != nil {
		return BadgeInfo{}, err
	}
	// An explicitly-sent empty string means "unlink" — same outcome as the
	// documented clear_feature_flag mechanism.
	if in.FeatureFlag != nil && *in.FeatureFlag == "" {
		in.ClearFeatureFlag = true
	}
	arg.FeatureFlag = normalizeFeatureFlag(in.FeatureFlag)
	arg.ClearFeatureFlag = &in.ClearFeatureFlag

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

	// Snapshot the icon before the update — comparing after the write
	// would read the new value and the change check could never fire.
	prevIcon, prevIconKnown := pgtype.UUID{}, false
	if in.IconMediaID != nil {
		if cur, cerr := s.store.GetBadgeDefinitionByID(ctx, badgeID); cerr == nil {
			prevIcon, prevIconKnown = cur.IconMediaID, true
		}
	}

	badge, err := s.store.UpdateBadgeDefinition(ctx, arg)
	if err != nil {
		return BadgeInfo{}, fmt.Errorf("gamification: update badge: %w", err)
	}

	if in.IconMediaID != nil && (!prevIconKnown || artworkChanged(prevIcon, arg.IconMediaID)) {
		s.knockoutArtwork(ctx, *in.IconMediaID, badgeIconMaxDim)
	}

	info := s.badgeToInfo(ctx, badge)
	info.IconURL = s.mediaURL(ctx, badge.IconMediaID)
	return info, nil
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
		// Fresh award — audit, notify, and grant the badge's points_reward
		// when configured (fire-and-forget, Rule 8).
		if err := s.store.InsertBadgeAwardEvent(ctx, gen.InsertBadgeAwardEventParams{
			UserID:    userID,
			BadgeID:   badgeID,
			Source:    "admin",
			AwardedBy: awardedBy,
		}); err != nil {
			slog.Warn("gamification: insert award event failed",
				"badge_id", uuidStr(badgeID), "error", err)
		}
		s.notifyBadgeAwarded(ctx, userID, badge.Name)
		s.grantBadgePoints(ctx, userID, badge)
	}

	// Fetch the awarded_at timestamp from the user_badge row.
	ub, err := s.store.GetUserBadge(ctx, userID, badgeID)
	if err != nil {
		d := s.badgeWithDetails(ctx, badge, pgtype.Timestamptz{})
		d.IconURL = s.mediaURL(ctx, badge.IconMediaID)
		return d, nil
	}

	d := s.badgeWithDetails(ctx, badge, ub.AwardedAt)
	d.IconURL = s.mediaURL(ctx, badge.IconMediaID)
	return d, nil
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
	iconIDs := make([]pgtype.UUID, 0, len(badges))
	for _, b := range badges {
		if b.IconMediaID.Valid {
			iconIDs = append(iconIDs, b.IconMediaID)
		}
		results = append(results, s.badgeToInfo(ctx, b))
	}
	urls := s.resolveIconURLs(ctx, iconIDs)
	for i := range results {
		results[i].IconURL = urls[results[i].IconMediaID]
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
	imgIDs := make([]pgtype.UUID, 0, len(challenges))
	for _, c := range challenges {
		if c.ImageMediaID.Valid {
			imgIDs = append(imgIDs, c.ImageMediaID)
		}
		results = append(results, s.challengeToInfo(ctx, c))
	}
	urls := s.resolveIconURLs(ctx, imgIDs)
	for i := range results {
		results[i].ImageURL = urls[results[i].ImageMediaID]
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

	if err := validateFeatureFlag(in.FeatureFlag); err != nil {
		return ChallengeInfo{}, err
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
		FeatureFlag:  normalizeFeatureFlag(in.FeatureFlag),
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

	if in.ImageMediaID != nil {
		s.knockoutArtwork(ctx, *in.ImageMediaID, challengeImageMaxDim)
	}

	info := s.challengeToInfo(ctx, challenge)
	info.ImageURL = s.mediaURL(ctx, challenge.ImageMediaID)
	return info, nil
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

	if err := validateFeatureFlag(in.FeatureFlag); err != nil {
		return ChallengeInfo{}, err
	}
	// An explicitly-sent empty string means "unlink" — same outcome as the
	// documented clear_feature_flag mechanism.
	if in.FeatureFlag != nil && *in.FeatureFlag == "" {
		in.ClearFeatureFlag = true
	}
	arg.FeatureFlag = normalizeFeatureFlag(in.FeatureFlag)
	arg.ClearFeatureFlag = &in.ClearFeatureFlag

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

	// Snapshot the image before the update — comparing after the write
	// would read the new value and the change check could never fire.
	prevImage, prevImageKnown := pgtype.UUID{}, false
	if in.ImageMediaID != nil {
		if cur, cerr := s.store.GetChallengeByID(ctx, challengeID); cerr == nil {
			prevImage, prevImageKnown = cur.ImageMediaID, true
		}
	}

	challenge, err := s.store.UpdateChallenge(ctx, arg)
	if err != nil {
		return ChallengeInfo{}, fmt.Errorf("gamification: update challenge: %w", err)
	}

	if in.ImageMediaID != nil && (!prevImageKnown || artworkChanged(prevImage, arg.ImageMediaID)) {
		s.knockoutArtwork(ctx, *in.ImageMediaID, challengeImageMaxDim)
	}

	info := s.challengeToInfo(ctx, challenge)
	info.ImageURL = s.mediaURL(ctx, challenge.ImageMediaID)
	return info, nil
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

// --- Admin v2 surfaces: award events, seasons, tiers ---

// ListAwardEvents returns the badge-award audit trail across all users,
// optionally scoped to one badge. q filters username/badge/source;
// sortBy is whitelisted to username|badge_name|source|created_at.
func (s *Service) ListAwardEvents(ctx context.Context, badgeID pgtype.UUID, q, sortBy, sortDir string, limit, offset int32) ([]AwardEventInfo, int64, error) {
	rows, total, err := s.store.ListAdminBadgeAwardEvents(ctx, badgeID, q, sortBy, sortDir, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("gamification: list award events: %w", err)
	}
	results := make([]AwardEventInfo, 0, len(rows))
	for _, r := range rows {
		ev := AwardEventInfo{
			ID:        uuidStr(r.ID),
			UserID:    uuidStr(r.UserID),
			BadgeID:   uuidStr(r.BadgeID),
			BadgeName: r.BadgeName,
			BadgeSlug: r.BadgeSlug,
			Source:    r.Source,
		}
		if r.Username != nil {
			ev.Username = *r.Username
		}
		if r.AwardedBy.Valid {
			by := r.AwardedBy.String()
			ev.AwardedBy = &by
		}
		if r.AwardedByUsername != nil {
			ev.AwardedByUsername = r.AwardedByUsername
		}
		ev.CreatedAt = rfc3339(r.CreatedAt)
		results = append(results, ev)
	}
	return results, total, nil
}

// ListSeasons returns every collection season (newest window first).
func (s *Service) ListSeasons(ctx context.Context) ([]SeasonInfo, error) {
	rows, err := s.store.ListBadgeSeasons(ctx)
	if err != nil {
		return nil, fmt.Errorf("gamification: list seasons: %w", err)
	}
	return seasonsToInfo(rows), nil
}

// CreateSeason creates a season window (admin).
func (s *Service) CreateSeason(ctx context.Context, in CreateSeasonInput) (SeasonInfo, error) {
	if strings.TrimSpace(in.Slug) == "" || strings.TrimSpace(in.Name) == "" {
		return SeasonInfo{}, invalidInput("slug and name are required")
	}
	startsAt, err := parseTimestamp(in.StartsAt)
	if err != nil || !startsAt.Valid {
		return SeasonInfo{}, invalidInput("valid starts_at is required")
	}
	var endsAt pgtype.Timestamptz
	if in.EndsAt != nil && *in.EndsAt != "" {
		endsAt, err = parseTimestamp(*in.EndsAt)
		if err != nil {
			return SeasonInfo{}, invalidInput("invalid ends_at")
		}
	}
	var description *string
	if in.Description != nil {
		desc := strings.TrimSpace(*in.Description)
		description = &desc
	}
	row, err := s.store.CreateBadgeSeason(ctx, strings.TrimSpace(in.Slug), strings.TrimSpace(in.Name), description, startsAt, endsAt, in.IsActive)
	if err != nil {
		if isUniqueViolation(err) {
			return SeasonInfo{}, ErrSlugExists
		}
		return SeasonInfo{}, fmt.Errorf("gamification: create season: %w", err)
	}
	return seasonsToInfo([]gen.BadgeSeason{row})[0], nil
}

// UpdateSeason applies non-nil fields to an existing season (admin).
func (s *Service) UpdateSeason(ctx context.Context, id pgtype.UUID, in UpdateSeasonInput) (SeasonInfo, error) {
	arg := gen.UpdateBadgeSeasonParams{ID: id, Name: in.Name, IsActive: in.IsActive}
	if in.Description != nil {
		desc := strings.TrimSpace(*in.Description)
		arg.Description = &desc
	}
	if in.StartsAt != nil {
		ts, err := parseTimestamp(*in.StartsAt)
		if err != nil {
			return SeasonInfo{}, invalidInput("invalid starts_at")
		}
		arg.StartsAt = ts
	}
	if in.EndsAt != nil {
		ts, err := parseTimestamp(*in.EndsAt)
		if err != nil {
			return SeasonInfo{}, invalidInput("invalid ends_at")
		}
		arg.EndsAt = ts
	}
	row, err := s.store.UpdateBadgeSeason(ctx, id, arg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SeasonInfo{}, ErrSeasonNotFound
		}
		return SeasonInfo{}, fmt.Errorf("gamification: update season: %w", err)
	}
	return seasonsToInfo([]gen.BadgeSeason{row})[0], nil
}

// DeleteSeason removes a season row (admin).
func (s *Service) DeleteSeason(ctx context.Context, id pgtype.UUID) error {
	return s.store.DeleteBadgeSeason(ctx, id)
}

// ListTiers returns tiered-collection progression metadata.
func (s *Service) ListTiers(ctx context.Context) ([]TierInfo, error) {
	rows, err := s.store.queries.ListBadgeTiers(ctx)
	if err != nil {
		return nil, fmt.Errorf("gamification: list tiers: %w", err)
	}
	return tiersToInfo(rows), nil
}

// CreateTier inserts a progression row (admin).
func (s *Service) CreateTier(ctx context.Context, in CreateTierInput) (TierInfo, error) {
	if strings.TrimSpace(in.BadgeGroup) == "" || !validTier(in.Tier) || in.Threshold < 0 {
		return TierInfo{}, invalidInput("valid badge_group, tier, and non-negative threshold are required")
	}
	var badgeID pgtype.UUID
	if in.BadgeID != "" {
		if err := badgeID.Scan(in.BadgeID); err != nil {
			return TierInfo{}, invalidInput("invalid badge_id")
		}
	}
	row, err := s.store.CreateBadgeTier(ctx, strings.TrimSpace(in.BadgeGroup), in.Tier, strings.TrimSpace(in.Name), int32(in.Threshold), badgeID) // #nosec G115 -- bounded: pagination clamp or config/settings value, cannot reach int32 overflow
	if err != nil {
		if isUniqueViolation(err) {
			return TierInfo{}, ErrTierExists
		}
		return TierInfo{}, fmt.Errorf("gamification: create tier: %w", err)
	}
	return tiersToInfo([]gen.BadgeTier{row})[0], nil
}

// UpdateTier applies non-nil fields to a tier row (admin).
func (s *Service) UpdateTier(ctx context.Context, id pgtype.UUID, in UpdateTierInput) (TierInfo, error) {
	arg := gen.UpdateBadgeTierParams{ID: id, BadgeGroup: in.BadgeGroup, Tier: in.Tier, Name: in.Name, ClearBadgeID: in.ClearBadgeID}
	if in.Threshold != nil {
		v := int32(*in.Threshold) // #nosec G115 -- bounded: pagination clamp or config/settings value, cannot reach int32 overflow
		arg.Threshold = &v
	}
	if in.BadgeID != nil {
		var bid pgtype.UUID
		if err := bid.Scan(*in.BadgeID); err != nil {
			return TierInfo{}, invalidInput("invalid badge_id")
		}
		arg.BadgeID = bid
	}
	row, err := s.store.UpdateBadgeTier(ctx, id, arg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TierInfo{}, ErrTierNotFound
		}
		return TierInfo{}, fmt.Errorf("gamification: update tier: %w", err)
	}
	return tiersToInfo([]gen.BadgeTier{row})[0], nil
}

// DeleteTier removes a tier row (admin).
func (s *Service) DeleteTier(ctx context.Context, id pgtype.UUID) error {
	return s.store.DeleteBadgeTier(ctx, id)
}

// ListBadgeGroups returns the distinct badge_group values for the tier editor.
func (s *Service) ListBadgeGroups(ctx context.Context) ([]string, error) {
	return s.store.ListBadgeGroups(ctx)
}

func seasonsToInfo(rows []gen.BadgeSeason) []SeasonInfo {
	out := make([]SeasonInfo, 0, len(rows))
	for _, r := range rows {
		info := SeasonInfo{ID: uuidStr(r.ID), Slug: r.Slug, Name: r.Name, Description: r.Description, IsActive: r.IsActive}
		info.StartsAt = rfc3339(r.StartsAt)
		if e := rfc3339(r.EndsAt); e != "" {
			info.EndsAt = &e
		}
		info.CreatedAt = rfc3339(r.CreatedAt)
		out = append(out, info)
	}
	return out
}

func tiersToInfo(rows []gen.BadgeTier) []TierInfo {
	out := make([]TierInfo, 0, len(rows))
	for _, r := range rows {
		info := TierInfo{ID: uuidStr(r.ID), BadgeGroup: r.BadgeGroup, Tier: r.Tier, Name: r.Name, Threshold: int(r.Threshold)}
		if r.BadgeID.Valid {
			s := r.BadgeID.String()
			info.BadgeID = &s
		}
		info.CreatedAt = rfc3339(r.CreatedAt)
		out = append(out, info)
	}
	return out
}
