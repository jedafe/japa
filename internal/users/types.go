// Package users provides HTTP handlers and business logic for user profile
// and preference management. The store layer wraps sqlc-generated queries
// with read/write pool separation.
package users

// UpdateProfileRequest is the JSON body for PATCH /api/v1/users/me.
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Bio         *string `json:"bio"`
	Timezone    *string `json:"timezone"`
}

// UpdatePreferencesRequest is the JSON body for PUT /api/v1/users/me/preferences.
type UpdatePreferencesRequest struct {
	DarkMode *string `json:"dark_mode"` // "light", "dark", "system"
	Language *string `json:"language"`  // "en-NG", "en-US", "en-GB", "en-CA"
}

// UserProfileResponse is the API response for a user profile.
type UserProfileResponse struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	Username      string  `json:"username"`
	DisplayName   *string `json:"display_name"`
	AvatarURL     *string `json:"avatar_url"`
	Bio           *string `json:"bio"`
	Timezone      *string `json:"timezone"`
	ProfileType   string  `json:"profile_type"`
	Status        string  `json:"status"`
	FollowerCount int32   `json:"follower_count"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// UserPreferencesResponse is the API response for user preferences.
type UserPreferencesResponse struct {
	DarkMode string `json:"dark_mode"`
	Language string `json:"language"`
}
