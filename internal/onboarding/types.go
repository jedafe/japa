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
type UpdateStepRequest struct {
	Step     int             `json:"step"`
	StepData json.RawMessage `json:"step_data"`
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
