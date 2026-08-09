// Package onboarding provides the onboarding wizard state management
// and recommendation engine for new users.
package onboarding

import "encoding/json"

// OnboardingStatusResponse is the API response for GET /api/v1/onboarding/status.
type OnboardingStatusResponse struct { //nolint:revive // stuttering type name preserved for API compatibility
	CurrentStep   int             `json:"current_step"`
	Completed     bool            `json:"completed"`
	StepData      json.RawMessage `json:"step_data"`
	TourDismissed bool            `json:"tour_dismissed"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

// UpdateStepRequest is the JSON body for PUT /api/v1/onboarding/step.
// When Skip is true, the backend auto-fulfills any step minimum (tags for
// step 3, follows for step 4) using the ranked recommendation algorithm
// per Section 09 skip semantics, rather than bypassing the requirement.
type UpdateStepRequest struct {
	Step     int             `json:"step"`
	StepData json.RawMessage `json:"step_data"`
	Skip     bool            `json:"skip"`
}

// OnboardingRecommendation is a single recommended space or user.
type OnboardingRecommendation struct { //nolint:revive // stuttering type name preserved for API compatibility
	Type      string  `json:"type"` // "space" or "user"
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Slug      string  `json:"slug"`
	AvatarURL string  `json:"avatar_url,omitempty"`
	Reason    string  `json:"reason"` // "geo_match", "tag_overlap", "reputation"
	Score     float64 `json:"score"`
}

// OnboardingAdminSettings is the API response for GET /admin/onboarding/settings.
type OnboardingAdminSettings struct { //nolint:revive // stuttering type name preserved for API compatibility
	MinTagsFollow  int      `json:"min_tags_follow"`
	MinUsersFollow int      `json:"min_users_follow"`
	MinSpacesJoin  int      `json:"min_spaces_join"`
	PromotedSpaces []string `json:"promoted_spaces"`
	PromotedUsers  []string `json:"promoted_users"`
	PromotedTags   []string `json:"promoted_tags"`
}

// UpdateOnboardingSettingsRequest is the JSON body for PUT /admin/onboarding/settings.
type UpdateOnboardingSettingsRequest struct {
	MinTagsFollow  *int      `json:"min_tags_follow,omitempty"`
	MinUsersFollow *int      `json:"min_users_follow,omitempty"`
	MinSpacesJoin  *int      `json:"min_spaces_join,omitempty"`
	PromotedSpaces *[]string `json:"promoted_spaces,omitempty"`
	PromotedUsers  *[]string `json:"promoted_users,omitempty"`
	PromotedTags   *[]string `json:"promoted_tags,omitempty"`
}

// CreateTemplateRequest is the JSON body for POST /admin/onboarding/templates.
type CreateTemplateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Steps       any    `json:"steps"`
	IsActive    bool   `json:"is_active"`
}

// UpdateTemplateRequest is the JSON body for PATCH /admin/onboarding/templates/:id.
type UpdateTemplateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Steps       any    `json:"steps"`
	IsActive    bool   `json:"is_active"`
}
