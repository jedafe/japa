// Package users provides HTTP handlers and business logic for user profile
// and preference management. The store layer wraps sqlc-generated queries
// with read/write pool separation.
package users

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

// UpdateProfileRequest is the JSON body for PATCH /api/v1/users/me.
// Supports both URL-based (avatar_url/cover_url) and media-system
// (avatar_media_id/header_media_id) fields per Section 50 (BUG-004).
// When media IDs are provided, the service resolves them to URLs.
// ADR-064a: origin_country and destination_country are saved as custom
// field values (entity_type='user_profile') after the profile upsert.
type UpdateProfileRequest struct {
	DisplayName        *string `json:"display_name"`
	AvatarURL          *string `json:"avatar_url"`
	AvatarMediaID      *string `json:"avatar_media_id"`
	CoverURL           *string `json:"cover_url"`
	HeaderMediaID      *string `json:"header_media_id"`
	Bio                *string `json:"bio"`
	Timezone           *string `json:"timezone"`
	WebsiteURL         *string `json:"website_url"`
	Education          *string `json:"education"`
	Profession         *string `json:"profession"`
	MaritalStatus      *string `json:"marital_status"`
	DateOfBirth        *string `json:"date_of_birth"`
	Gender             *string `json:"gender"`
	OriginCountry      *string `json:"origin_country"`
	OriginStateID      *string `json:"origin_state_id"`
	OriginCityID       *string `json:"origin_city_id"`
	DestinationCountry *string `json:"destination_country"`
}

// UpdateAccountRequest is the JSON body for PATCH /api/v1/me/account.
// Per Section 50: username may be changed once per 30 days; email change
// triggers re-verification (email_verified_at reset to NULL); display_name
// is also editable here. All fields are optional (partial update).
// §03 alias amendment: public_username (alias handle) and public_group_id
// (public role) are super_admin-gated presentation fields — the handler
// rejects non-super_admin callers when either is present.
type UpdateAccountRequest struct {
	Username       *string `json:"username"`
	Email          *string `json:"email"`
	DisplayName    *string `json:"display_name"`
	PublicUsername *string `json:"public_username"`
	PublicGroupID  *string `json:"public_group_id"`
}

// UpdatePreferencesRequest is the JSON body for PUT /api/v1/users/me/preferences.
type UpdatePreferencesRequest struct {
	DarkMode *string `json:"dark_mode"` // "light", "dark", "system"
	Language *string `json:"language"`  // "en-NG", "en-US", "en-GB", "en-CA"
}

// SyntheticUserInput holds the fields required to create or retrieve a
// synthetic user channel for an imported video.
type SyntheticUserInput struct {
	SourceType    string
	Username      string
	Email         string
	DisplayName   string
	Bio           string
	AvatarURL     string
	CoverURL      string
	Location      string
	BannerMediaID pgtype.UUID
}

// UpdateJourneyRequest is the JSON body for PATCH /api/v1/me/journey.
// All fields are optional. Values are normalised before being persisted as
// user_profile custom field values.
type UpdateJourneyRequest struct {
	OriginCountry      *string `json:"origin_country"`
	DestinationCountry *string `json:"destination_country"`
	JapaStage          *string `json:"japa_stage"`
	VisaRoute          *string `json:"visa_route"`
	Purpose            *string `json:"purpose"`
	TravelDate         *string `json:"travel_date"`
}

// UserProfileResponse is the API response for a user profile.
type UserProfileResponse struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Username      string   `json:"username"`
	DisplayName   *string  `json:"display_name"`
	AvatarURL     *string  `json:"avatar_url"`
	AvatarMediaID *string  `json:"avatar_media_id"`
	CoverURL      *string  `json:"cover_url"`
	HeaderMediaID *string  `json:"header_media_id"`
	Bio           *string  `json:"bio"`
	Timezone      *string  `json:"timezone"`
	WebsiteURL    *string  `json:"website_url"`
	Education     *string  `json:"education"`
	Profession    *string  `json:"profession"`
	MaritalStatus *string  `json:"marital_status"`
	DateOfBirth   *string  `json:"date_of_birth"`
	Gender        *string  `json:"gender"`
	ProfileType   string   `json:"profile_type"`
	Status        string   `json:"status"`
	FollowerCount int32    `json:"follower_count"`
	UserGroupName string   `json:"user_group_name"`
	Groups        []string `json:"groups"`
	LastLoginAt   *string  `json:"last_login_at"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	// ContentCountryPreference is the user's geo-aware feed personalization
	// country (ADR-064). Surfaced on /me so the feed header can render the
	// inline "Showing: [Country] · Change" control without an extra fetch.
	ContentCountryPreference string `json:"content_country_preference"`
	// ADR-064a: ISO 3166-1 alpha-2 origin/destination country codes (custom
	// field values resolved from custom_field_values). Surfaced on /me so the
	// settings/location page can pre-populate without an extra fetch.
	OriginCountry      string `json:"origin_country"`
	OriginStateID      string `json:"origin_state_id"`
	OriginCityID       string `json:"origin_city_id"`
	DestinationCountry string `json:"destination_country"`
	// The user's current geo space ID (migration 000200, ADR-064a). Surfaced
	// on /me so sidebar widgets can fetch geo-relevant content (Option A,
	// ADR-064) without an extra round-trip. Empty string = not set.
	CurrentGeoSpaceID string `json:"current_geo_space_id"`
	// EffectivePermissions is the caller's resolved RBAC permission node set
	// (e.g. "forum.ama.create", "content.edit_own"). Surfaced on /me so any
	// frontend page can gate UI on the actual permission node instead of
	// hardcoded role names (§26 RBAC wiring, Rule 16). Nil for public
	// profiles (only populated by GetMyProfile). Sorted for stable output.
	EffectivePermissions []string `json:"permissions,omitempty"`
	// §03 alias amendment — super_admin public identity fields. Surfaced on
	// /me so account settings can pre-populate; nil/empty when unset.
	PublicUsername *string `json:"public_username"`
	PublicGroupID  *string `json:"public_group_id"`
	PublicRole     *string `json:"public_role"`
	// IsOwner marks the JC_OWNER_EMAIL platform owner account.
	IsOwner bool `json:"is_owner"`
	// §07 verification badge (ADR-029) — surfaced on /me so account settings
	// can gate the "Request verification" action.
	Verified         bool    `json:"verified"`
	VerificationType *string `json:"verification_type"`
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

// PublicUserListResponse is the paginated API response for the public user
// directory at GET /users. Contains PII-safe PublicProfileResponse items.
type PublicUserListResponse struct {
	Items      []PublicProfileResponse `json:"items"`
	Page       int32                   `json:"page"`
	TotalPages int32                   `json:"total_pages"`
	Total      int32                   `json:"total"`
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
	Verified      bool     `json:"verified"`
	FollowerCount int32    `json:"follower_count"`
	Groups        []string `json:"groups"`
	IsOwner       bool     `json:"is_owner"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// PublicProfileResponse is the PII-safe API response for GET /users/{username}.
// It deliberately omits email, status, last_login_at, and groups (Section 19
// / ADR-014) and adds following_count alongside the existing follower_count
// (Section 31 / ADR-031).
//
// Points is included only when the user has opted in via
// show_points_publicly (Section 28). Credits are never included.
// When absent, the field is nil and omitempty drops it from the JSON.
type PublicProfileResponse struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name"`
	AvatarURL      *string `json:"avatar_url"`
	CoverURL       *string `json:"cover_url"`
	Bio            *string `json:"bio"`
	WebsiteURL     *string `json:"website_url"`
	ProfileType    string  `json:"profile_type"`
	FollowerCount  int32   `json:"follower_count"`
	FollowingCount int32   `json:"following_count"`
	CreatedAt      string  `json:"created_at"`
	Points         *int64  `json:"points,omitempty"`
	// Video channel metadata (migration 000633 / §106). IsSynthetic marks
	// imported video channels; SyntheticSource identifies the origin
	// platform (e.g. "youtube"); Location and BannerURL carry the channel's
	// public profile metadata (BannerURL is resolved from
	// user_profiles.banner_media_id via the media resolver, best-effort).
	IsSynthetic     bool    `json:"is_synthetic"`
	SyntheticSource *string `json:"synthetic_source"`
	Location        *string `json:"location"`
	BannerURL       *string `json:"banner_url"`
	// Role is the display name of the user's chosen public group
	// (users.public_group_id → user_groups.name). Null when unset.
	// Ref: §03 alias amendment — super_admin public role.
	Role *string `json:"role"`
	// §07 verification badge (ADR-029): verified flag + badge type.
	Verified         bool    `json:"verified"`
	VerificationType *string `json:"verification_type"`
	// §41 viewer-relative block state (BLK-006): only populated when the
	// request is authenticated and the viewer is not the profile owner.
	// IsBlocked = viewer blocked this user; BlockedBy = this user blocked
	// the viewer. Omitted for guests and own-profile responses.
	IsBlocked *bool `json:"is_blocked,omitempty"`
	BlockedBy *bool `json:"blocked_by,omitempty"`

	IsFollowing bool `json:"is_following"`
}

// DraftResponse is the API response for GET /api/v1/me/drafts/{id}, used by
// the /create composer to restore an existing draft for editing. It mirrors
// the field set the composer's ContentRecord consumes (id, slug, content_type,
// title, body, body_html, field_values, canonical_url, status, visibility,
// publish_at, space_id) plus timestamps, the post-as-org organization_id, and
// the SEO/feature-image fields (migration 000165) so a draft restored into the
// composer round-trips every field the editor can mutate. FieldValues is a
// json.RawMessage so the JSONB column serializes as a proper JSON object (not
// base64), which the composer's field_values: Record<string, unknown> expects.
type DraftResponse struct {
	ID              string          `json:"id"`
	Slug            string          `json:"slug"`
	ContentType     string          `json:"content_type"`
	Title           string          `json:"title"`
	Body            string          `json:"body"`
	BodyHTML        string          `json:"body_html"`
	FieldValues     json.RawMessage `json:"field_values"`
	CanonicalURL    *string         `json:"canonical_url"`
	HeaderImageID   *string         `json:"header_image_id"`
	FeaturedImageID *string         `json:"featured_image_id"`
	VideoSourceID   *string         `json:"video_source_id"`
	MetaTitle       *string         `json:"meta_title"`
	MetaDescription *string         `json:"meta_description"`
	OgImageURL      *string         `json:"og_image_url"`
	SeoIndexable    *bool           `json:"seo_indexable"`
	OrganizationID  *string         `json:"organization_id"`
	Status          string          `json:"status"`
	Visibility      string          `json:"visibility"`
	SpaceID         *string         `json:"space_id"`
	GeoSpaceID      *string         `json:"geo_space_id"`
	GuideCategory   *string         `json:"guide_category"`
	JourneyStage    *string         `json:"journey_stage"`
	VisaRoute       *string         `json:"visa_route"`
	SourceCountry   *string         `json:"source_country"`
	TargetCountry   *string         `json:"target_country"`
	PublishAt       *string         `json:"publish_at"`
	PublishedAt     *string         `json:"published_at"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}
