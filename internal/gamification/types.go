package gamification

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// BadgeInfo is the JSON-serialisable representation of a badge definition.
type BadgeInfo struct {
	ID                 string `json:"id"`
	Slug               string `json:"slug"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	IconMediaID        string `json:"icon_media_id,omitempty"`
	TriggerType        string `json:"trigger_type"`
	TriggerValue       int    `json:"trigger_value,omitempty"`
	TriggerActivityKey string `json:"trigger_activity_key,omitempty"`
	IsActive           bool   `json:"is_active"`
	IsSystem           bool   `json:"is_system"`
	DisplayOrder       int    `json:"display_order"`
	CreatedAt          string `json:"created_at"`
}

// BadgeWithDetails is a user's earned badge with the badge definition details.
type BadgeWithDetails struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	IconMediaID *string `json:"icon_media_id,omitempty"`
	AwardedAt   string  `json:"awarded_at"`
}

// ChallengeInfo is the JSON-serialisable representation of a challenge.
type ChallengeInfo struct {
	ID            string  `json:"id"`
	Slug          string  `json:"slug"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	ImageMediaID  string  `json:"image_media_id,omitempty"`
	ActivityKey   string  `json:"activity_key"`
	TargetCount   int     `json:"target_count"`
	RewardPoints  int     `json:"reward_points"`
	RewardBadgeID string  `json:"reward_badge_id,omitempty"`
	StartsAt      string  `json:"starts_at"`
	EndsAt        *string `json:"ends_at,omitempty"`
	IsActive      bool    `json:"is_active"`
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
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Points   int64  `json:"points"`
	Rank     int    `json:"rank"`
}

// CreateBadgeInput is the admin request body for creating a badge.
type CreateBadgeInput struct {
	Slug               string  `json:"slug"`
	Name               string  `json:"name"`
	Description        *string `json:"description,omitempty"`
	IconMediaID        *string `json:"icon_media_id,omitempty"`
	TriggerType        string  `json:"trigger_type"`
	TriggerValue       *int32  `json:"trigger_value,omitempty"`
	TriggerActivityKey *string `json:"trigger_activity_key,omitempty"`
	IsActive           *bool   `json:"is_active,omitempty"`
	DisplayOrder       *int32  `json:"display_order,omitempty"`
}

// UpdateBadgeInput is the admin request body for updating a badge.
type UpdateBadgeInput struct {
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	IconMediaID        *string `json:"icon_media_id,omitempty"`
	TriggerType        *string `json:"trigger_type,omitempty"`
	TriggerValue       *int32  `json:"trigger_value,omitempty"`
	TriggerActivityKey *string `json:"trigger_activity_key,omitempty"`
	IsActive           *bool   `json:"is_active,omitempty"`
	DisplayOrder       *int32  `json:"display_order,omitempty"`
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
}

// UpdateChallengeInput is the admin request body for updating a challenge.
type UpdateChallengeInput struct {
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	ImageMediaID  *string `json:"image_media_id,omitempty"`
	ActivityKey   *string `json:"activity_key,omitempty"`
	TargetCount   *int32  `json:"target_count,omitempty"`
	RewardPoints  *int32  `json:"reward_points,omitempty"`
	RewardBadgeID *string `json:"reward_badge_id,omitempty"`
	StartsAt      *string `json:"starts_at,omitempty"`
	EndsAt        *string `json:"ends_at,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}
