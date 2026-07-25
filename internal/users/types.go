// Package users provides HTTP handlers and business logic for user profile
// and preference management. The store layer wraps sqlc-generated queries
// with read/write pool separation.
package users

import "encoding/json"

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
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Username      string   `json:"username"`
	DisplayName   *string  `json:"display_name"`
	AvatarURL     *string  `json:"avatar_url"`
	Bio           *string  `json:"bio"`
	Timezone      *string  `json:"timezone"`
	ProfileType   string   `json:"profile_type"`
	Status        string   `json:"status"`
	FollowerCount int32    `json:"follower_count"`
	UserGroupName string   `json:"user_group_name"`
	Groups        []string `json:"groups"`
	LastLoginAt   *string  `json:"last_login_at"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// UserPreferencesResponse is the API response for user preferences.
type UserPreferencesResponse struct {
	DarkMode string `json:"dark_mode"`
	Language string `json:"language"`
}

// ListUsersResponse is the paginated API response for listing users.
type ListUsersResponse struct {
	Items      []UserListItem `json:"items"`
	Page       int32          `json:"page"`
	TotalPages int32          `json:"total_pages"`
	Total      int32          `json:"total"`
}

// UserActivityItem is one row in the admin user-activity feed: a content
// item the user authored (content = title) or a comment (content = body,
// truncated server-side).
type UserActivityItem struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// UserActivityResponse is the API response for GET /admin/users/{id}/activity.
type UserActivityResponse struct {
	Activities []UserActivityItem `json:"activities"`
	Total      int32              `json:"total"`
}

// UserListItem is a condensed user record for list endpoints.
type UserListItem struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Username      string   `json:"username"`
	ProfileType   string   `json:"profile_type"`
	Status        string   `json:"status"`
	FollowerCount int32    `json:"follower_count"`
	Groups        []string `json:"groups"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// PublicProfileResponse is the PII-safe API response for GET /users/{username}.
// It deliberately omits email, status, last_login_at, and groups (Section 19
// / ADR-014) and adds following_count alongside the existing follower_count
// (Section 31 / ADR-031).
type PublicProfileResponse struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name"`
	AvatarURL      *string `json:"avatar_url"`
	Bio            *string `json:"bio"`
	ProfileType    string  `json:"profile_type"`
	FollowerCount  int32   `json:"follower_count"`
	FollowingCount int32   `json:"following_count"`
	CreatedAt      string  `json:"created_at"`
}

// DraftResponse is the API response for GET /api/v1/me/drafts/{id}, used by
// the /create composer to restore an existing draft for editing. It mirrors
// the field set the composer's ContentRecord consumes (id, slug, content_type,
// title, body, body_html, field_values, canonical_url, status, visibility,
// publish_at, space_id) plus timestamps. FieldValues is a json.RawMessage so
// the JSONB column serializes as a proper JSON object (not base64), which the
// composer's field_values: Record<string, unknown> expects.
type DraftResponse struct {
	ID            string          `json:"id"`
	Slug          string          `json:"slug"`
	ContentType   string          `json:"content_type"`
	Title         string          `json:"title"`
	Body          string          `json:"body"`
	BodyHTML      string          `json:"body_html"`
	FieldValues   json.RawMessage `json:"field_values"`
	CanonicalURL  *string         `json:"canonical_url"`
	Status        string          `json:"status"`
	Visibility    string          `json:"visibility"`
	SpaceID       *string         `json:"space_id"`
	GeoSpaceID    *string         `json:"geo_space_id"`
	PublishAt     *string         `json:"publish_at"`
	PublishedAt   *string         `json:"published_at"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}
