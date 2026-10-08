// Package advertising implements the Ad Placement system (Section 25).
//
// Handles ad slot types, campaign management, batched impression/click
// ingestion, daily analytics aggregation, and organization boosts.
// V1 is admin-only campaign management; V2 will add self-serve for users.
package advertising

import (
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
)

// Slot type names matching the predefined ad_slot_types registry.
const (
	SlotFeedInline          = "feed_inline"
	SlotSidebar             = "sidebar"
	SlotSearchSponsored     = "search_sponsored"
	SlotComparisonSponsored = "comparison_sponsored"
	SlotForumInline         = "forum_inline"

	// §25 amendment — house-fallback + new placements (migration 000814).
	SlotLeftRail             = "left_rail"
	SlotArticleInline        = "article_inline"
	SlotArticleBelowComments = "article_below_comments"
	SlotCommentsBelow        = "comments_below"
	SlotNativeFeed           = "native_feed"
	SlotNativeDiscover       = "native_discover"
	SlotNativeSpaceFeed      = "native_space_feed"
	SlotNativeForum          = "native_forum"
	SlotStickyAnchor         = "sticky_anchor"
	SlotSidebarSticky        = "sidebar_sticky"
)

// Campaign statuses.
const (
	CampaignStatusDraft         = "draft"
	CampaignStatusActive        = "active"
	CampaignStatusPaused        = "paused"
	CampaignStatusEnded         = "ended"
	CampaignStatusRejected      = "rejected"
	CampaignStatusPendingReview = "pending_review"
)

// Ad event types.
const (
	EventTypeImpression = "impression"
	EventTypeClick      = "click"
	EventTypeConversion = "conversion"
)

// Organization boost statuses.
const (
	BoostStatusActive   = "active"
	BoostStatusEnded    = "ended"
	BoostStatusCanceled = "canceled"
)

// Boost types.
const (
	BoostTypeFeaturedPlacement = "featured_placement"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Service provides advertising operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	settings *config.Settings
}

// NewService creates a Service backed by the given database pool.
// Until sqlc generates advertising query code, the store uses raw
// pgxpool connections from db.Reader and db.Writer.
func NewService(database *db.DB, settings *config.Settings) *Service {
	return &Service{
		store:    NewStore(database),
		db:       database,
		settings: settings,
	}
}
