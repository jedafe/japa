package trending

import "time"

// TrendingItem represents a trending entity in API responses.
//
//nolint:revive // exported: stutter naming is intentional
type TrendingItem struct {
	ID         string    `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Score      float64   `json:"score"`
	Window     string    `json:"window"`
	GeoCode    *string   `json:"geo_code,omitempty"`
	ComputedAt time.Time `json:"computed_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// ContentTrending extends TrendingItem with content details.
type ContentTrending struct {
	TrendingItem
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	ContentType string  `json:"content_type"`
	AuthorID    string  `json:"author_id"`
	SpaceID     *string `json:"space_id,omitempty"`
	ViewCount   int64   `json:"view_count"`
}

// TagTrending extends TrendingItem with tag details.
type TagTrending struct {
	TrendingItem
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	FollowerCount int64  `json:"follower_count"`
}

// SpaceTrending extends TrendingItem with space details.
type SpaceTrending struct {
	TrendingItem
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	MemberCount int64  `json:"member_count"`
	PostCount   int64  `json:"post_count"`
}

// UserTrending extends TrendingItem with user details.
type UserTrending struct {
	TrendingItem
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Bio         *string `json:"bio,omitempty"`
}

// SuggestedUser represents a suggested user to follow.
type SuggestedUser struct {
	ID           string  `json:"id"`
	Username     string  `json:"username"`
	DisplayName  string  `json:"display_name"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
	Bio          *string `json:"bio,omitempty"`
	SharedSpaces int64   `json:"shared_spaces"`
}

// SuggestedSpace represents a suggested space to join.
type SuggestedSpace struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	MemberCount   int64  `json:"member_count"`
	PostCount     int64  `json:"post_count"`
	SharedMembers int64  `json:"shared_members"`
}

// SuggestedTag represents a suggested tag to follow.
type SuggestedTag struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	FollowerCount int64  `json:"follower_count"`
}
