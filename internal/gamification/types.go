package gamification

import "errors"

// ErrGamificationDisabled is returned when the requested gamification
// surface is disabled via system_settings (gamification.badges_enabled /
// challenges_enabled / leaderboard_enabled). Handlers map it to a 404
// gamification_disabled envelope per the feature-flag convention (ADR-042).
var ErrGamificationDisabled = errors.New("gamification: feature disabled")

// Notification type keys — seeded in notification_types by migration
// 000627. Keep in sync with that seed so user preferences resolve (§35).
const (
	NotifTypeBadgeAwarded       = "gamification.badge_awarded"
	NotifTypeChallengeCompleted = "gamification.challenge_completed"
)

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// BadgeInfo is the JSON-serialisable representation of a badge definition.
type BadgeInfo struct {
	ID                 string  `json:"id"`
	Slug               string  `json:"slug"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	IconMediaID        string  `json:"icon_media_id,omitempty"`
	IconURL            string  `json:"icon_url,omitempty"`
	AccentColor        string  `json:"accent_color,omitempty"`
	TriggerType        string  `json:"trigger_type"`
	TriggerValue       int     `json:"trigger_value,omitempty"`
	TriggerActivityKey string  `json:"trigger_activity_key,omitempty"`
	IsActive           bool    `json:"is_active"`
	IsSystem           bool    `json:"is_system"`
	DisplayOrder       int     `json:"display_order"`
	Category           string  `json:"category,omitempty"`
	Rarity             string  `json:"rarity"`
	Tier               string  `json:"tier,omitempty"`
	BadgeGroup         string  `json:"badge_group,omitempty"`
	IsSecret           bool    `json:"is_secret"`
	IsLimited          bool    `json:"is_limited"`
	IsFeatured         bool    `json:"is_featured"`
	PointsReward       int     `json:"points_reward"`
	AvailableFrom      *string `json:"available_from,omitempty"`
	AvailableUntil     *string `json:"available_until,omitempty"`
	FeatureFlag        *string `json:"feature_flag,omitempty"`
	CreatedAt          string  `json:"created_at"`
}

// BadgeWithDetails is a user's earned badge with the badge definition details.
type BadgeWithDetails struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	IconMediaID *string `json:"icon_media_id,omitempty"`
	IconURL     string  `json:"icon_url,omitempty"`
	AccentColor string  `json:"accent_color,omitempty"`
	Category    string  `json:"category,omitempty"`
	Rarity      string  `json:"rarity,omitempty"`
	Tier        string  `json:"tier,omitempty"`
	BadgeGroup  string  `json:"badge_group,omitempty"`
	AwardedAt   string  `json:"awarded_at"`
}

// CollectionBadge is a single catalogue entry in the achievement-centre
// collection view: the badge definition plus the caller's earned state and
// progress. Secret badges are masked when unearned (Name/Description and
// trigger fields are replaced by placeholders).
type CollectionBadge struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	IconMediaID  string  `json:"icon_media_id,omitempty"`
	IconURL      string  `json:"icon_url,omitempty"`
	AccentColor  string  `json:"accent_color,omitempty"`
	Category     string  `json:"category,omitempty"`
	Rarity       string  `json:"rarity"`
	Tier         string  `json:"tier,omitempty"`
	BadgeGroup   string  `json:"badge_group,omitempty"`
	IsSecret     bool    `json:"is_secret"`
	IsLimited    bool    `json:"is_limited"`
	IsFeatured   bool    `json:"is_featured"`
	PointsReward int     `json:"points_reward"`
	Earned       bool    `json:"earned"`
	AwardedAt    *string `json:"awarded_at,omitempty"`
	Progress     int     `json:"progress"`
	Goal         int     `json:"goal,omitempty"`
	FeatureFlag  *string `json:"feature_flag,omitempty"`
}

// BadgeCollectionStats summarises the user's collection progress.
type BadgeCollectionStats struct {
	Earned    int `json:"earned"`
	Total     int `json:"total"`
	Remaining int `json:"remaining"`
	Percent   int `json:"percent"`
}

// BadgeCollectionCategory groups collection badges under a category.
type BadgeCollectionCategory struct {
	Category string            `json:"category"`
	Earned   int               `json:"earned"`
	Total    int               `json:"total"`
	Badges   []CollectionBadge `json:"badges"`
}

// BadgeCollection is the achievement-centre payload.
type BadgeCollection struct {
	Stats      BadgeCollectionStats      `json:"stats"`
	Categories []BadgeCollectionCategory `json:"categories"`
}

// ChallengeInfo is the JSON-serialisable representation of a challenge.
type ChallengeInfo struct {
	ID            string  `json:"id"`
	Slug          string  `json:"slug"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	ImageMediaID  string  `json:"image_media_id,omitempty"`
	ImageURL      string  `json:"image_url,omitempty"`
	ActivityKey   string  `json:"activity_key"`
	TargetCount   int     `json:"target_count"`
	RewardPoints  int     `json:"reward_points"`
	RewardBadgeID string  `json:"reward_badge_id,omitempty"`
	StartsAt      string  `json:"starts_at"`
	EndsAt        *string `json:"ends_at,omitempty"`
	IsActive      bool    `json:"is_active"`
	FeatureFlag   *string `json:"feature_flag,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

// ChallengeWithProgress is a user's challenge progress with challenge details.
type ChallengeWithProgress struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	TargetCount  int     `json:"target_count"`
	CurrentCount int     `json:"current_count"`
	Completed    bool    `json:"completed"`
	CompletedAt  *string `json:"completed_at,omitempty"`
	EndsAt       *string `json:"ends_at,omitempty"`
}

// LeaderboardEntry is a single row in the leaderboard.
type LeaderboardEntry struct {
	UserID      string  `json:"user_id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Points      int64   `json:"points"`
	Rank        int     `json:"rank"`
}

// CreateBadgeInput is the admin request body for creating a badge.
type CreateBadgeInput struct {
	Slug               string  `json:"slug"`
	Name               string  `json:"name"`
	Description        *string `json:"description,omitempty"`
	IconMediaID        *string `json:"icon_media_id,omitempty"`
	AccentColor        *string `json:"accent_color,omitempty"`
	TriggerType        string  `json:"trigger_type"`
	TriggerValue       *int32  `json:"trigger_value,omitempty"`
	TriggerActivityKey *string `json:"trigger_activity_key,omitempty"`
	IsActive           *bool   `json:"is_active,omitempty"`
	DisplayOrder       *int32  `json:"display_order,omitempty"`
	Category           *string `json:"category,omitempty"`
	Rarity             *string `json:"rarity,omitempty"`
	Tier               *string `json:"tier,omitempty"`
	BadgeGroup         *string `json:"badge_group,omitempty"`
	IsSecret           *bool   `json:"is_secret,omitempty"`
	IsLimited          *bool   `json:"is_limited,omitempty"`
	IsFeatured         *bool   `json:"is_featured,omitempty"`
	PointsReward       *int32  `json:"points_reward,omitempty"`
	AvailableFrom      *string `json:"available_from,omitempty"`
	AvailableUntil     *string `json:"available_until,omitempty"`
	FeatureFlag        *string `json:"feature_flag,omitempty"`
}

// UpdateBadgeInput is the admin request body for updating a badge.
// The clear_* flags set nullable columns back to NULL — COALESCE-style
// updates cannot distinguish "leave unchanged" from "set to null".
type UpdateBadgeInput struct {
	Name                    *string `json:"name,omitempty"`
	Description             *string `json:"description,omitempty"`
	IconMediaID             *string `json:"icon_media_id,omitempty"`
	ClearIconMediaID        bool    `json:"clear_icon_media_id,omitempty"`
	AccentColor             *string `json:"accent_color,omitempty"`
	ClearAccentColor        bool    `json:"clear_accent_color,omitempty"`
	TriggerType             *string `json:"trigger_type,omitempty"`
	TriggerValue            *int32  `json:"trigger_value,omitempty"`
	ClearTriggerValue       bool    `json:"clear_trigger_value,omitempty"`
	TriggerActivityKey      *string `json:"trigger_activity_key,omitempty"`
	ClearTriggerActivityKey bool    `json:"clear_trigger_activity_key,omitempty"`
	IsActive                *bool   `json:"is_active,omitempty"`
	DisplayOrder            *int32  `json:"display_order,omitempty"`
	Category                *string `json:"category,omitempty"`
	ClearCategory           bool    `json:"clear_category,omitempty"`
	Rarity                  *string `json:"rarity,omitempty"`
	Tier                    *string `json:"tier,omitempty"`
	ClearTier               bool    `json:"clear_tier,omitempty"`
	BadgeGroup              *string `json:"badge_group,omitempty"`
	ClearBadgeGroup         bool    `json:"clear_badge_group,omitempty"`
	IsSecret                *bool   `json:"is_secret,omitempty"`
	IsLimited               *bool   `json:"is_limited,omitempty"`
	IsFeatured              *bool   `json:"is_featured,omitempty"`
	PointsReward            *int32  `json:"points_reward,omitempty"`
	AvailableFrom           *string `json:"available_from,omitempty"`
	ClearAvailableFrom      bool    `json:"clear_available_from,omitempty"`
	AvailableUntil          *string `json:"available_until,omitempty"`
	ClearAvailableUntil     bool    `json:"clear_available_until,omitempty"`
	FeatureFlag             *string `json:"feature_flag,omitempty"`
	ClearFeatureFlag        bool    `json:"clear_feature_flag,omitempty"`
}

// CreateChallengeInput is the admin request body for creating a challenge.
type CreateChallengeInput struct {
	Slug          string  `json:"slug"`
	Name          string  `json:"name"`
	Description   *string `json:"description,omitempty"`
	ImageMediaID  *string `json:"image_media_id,omitempty"`
	ActivityKey   string  `json:"activity_key"`
	TargetCount   int32   `json:"target_count"`
	RewardPoints  int32   `json:"reward_points,omitempty"`
	RewardBadgeID *string `json:"reward_badge_id,omitempty"`
	StartsAt      string  `json:"starts_at"`
	EndsAt        *string `json:"ends_at,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
	FeatureFlag   *string `json:"feature_flag,omitempty"`
}

// UpdateChallengeInput is the admin request body for updating a challenge.
// The clear_* flags set nullable columns back to NULL — COALESCE-style
// updates cannot distinguish "leave unchanged" from "set to null".
type UpdateChallengeInput struct {
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	ImageMediaID       *string `json:"image_media_id,omitempty"`
	ClearImageMediaID  bool    `json:"clear_image_media_id,omitempty"`
	ActivityKey        *string `json:"activity_key,omitempty"`
	TargetCount        *int32  `json:"target_count,omitempty"`
	RewardPoints       *int32  `json:"reward_points,omitempty"`
	RewardBadgeID      *string `json:"reward_badge_id,omitempty"`
	ClearRewardBadgeID bool    `json:"clear_reward_badge_id,omitempty"`
	StartsAt           *string `json:"starts_at,omitempty"`
	EndsAt             *string `json:"ends_at,omitempty"`
	ClearEndsAt        bool    `json:"clear_ends_at,omitempty"`
	IsActive           *bool   `json:"is_active,omitempty"`
	FeatureFlag        *string `json:"feature_flag,omitempty"`
	ClearFeatureFlag   bool    `json:"clear_feature_flag,omitempty"`
}

// --- Admin management surfaces (v2): award events, seasons, tiers ---

// AwardEventInfo is one row of the badge-award audit trail.
// AwardedByUsername resolves the admin who granted a manual award.
type AwardEventInfo struct {
	ID                string  `json:"id"`
	UserID            string  `json:"user_id"`
	Username          string  `json:"username,omitempty"`
	BadgeID           string  `json:"badge_id"`
	BadgeName         string  `json:"badge_name"`
	BadgeSlug         string  `json:"badge_slug"`
	Source            string  `json:"source"`
	AwardedBy         *string `json:"awarded_by,omitempty"`
	AwardedByUsername *string `json:"awarded_by_username,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

// SeasonInfo is a limited-edition collection window.
type SeasonInfo struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	StartsAt    string  `json:"starts_at"`
	EndsAt      *string `json:"ends_at,omitempty"`
	IsActive    bool    `json:"is_active"`
	CreatedAt   string  `json:"created_at"`
}

// TierInfo is progression metadata for a tiered badge collection.
type TierInfo struct {
	ID         string  `json:"id"`
	BadgeGroup string  `json:"badge_group"`
	Tier       string  `json:"tier"`
	Name       string  `json:"name"`
	Threshold  int     `json:"threshold"`
	BadgeID    *string `json:"badge_id,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

// CreateSeasonInput / UpdateSeasonInput / CreateTierInput / UpdateTierInput
// are the admin write payloads.
type CreateSeasonInput struct {
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	StartsAt    string  `json:"starts_at"`
	EndsAt      *string `json:"ends_at,omitempty"`
	IsActive    bool    `json:"is_active"`
}

type UpdateSeasonInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	StartsAt    *string `json:"starts_at,omitempty"`
	EndsAt      *string `json:"ends_at,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

type CreateTierInput struct {
	BadgeGroup string `json:"badge_group"`
	Tier       string `json:"tier"`
	Name       string `json:"name"`
	Threshold  int    `json:"threshold"`
	BadgeID    string `json:"badge_id,omitempty"`
}

type UpdateTierInput struct {
	BadgeGroup   *string `json:"badge_group,omitempty"`
	Tier         *string `json:"tier,omitempty"`
	Name         *string `json:"name,omitempty"`
	Threshold    *int    `json:"threshold,omitempty"`
	BadgeID      *string `json:"badge_id,omitempty"`
	ClearBadgeID bool    `json:"clear_badge_id,omitempty"`
}
