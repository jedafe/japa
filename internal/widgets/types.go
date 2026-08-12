// Package widgets implements the Page Builder widget configuration system
// (Section 16). Widgets are data-source-driven dynamic blocks: each widget
// configuration references a widget_type (latest_content, trending_tags, etc.)
// and a config JSON blob. The public GET /widgets/:id endpoint returns
// rendered widget data, Valkey-cached by the widget's cache_ttl_secs.
//
// Ref: japaconnect-engineering/sections/16-page-builder.md
package widgets

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Widget is a widget_configurations row.
type Widget struct {
	ID           pgtype.UUID      `json:"id"`
	TenantID     pgtype.UUID      `json:"tenant_id"`
	Name         string           `json:"name"`
	WidgetType   string           `json:"widget_type"`
	Config       json.RawMessage  `json:"config"`
	CacheTTLSecs int32            `json:"cache_ttl_secs"`
	IsActive     bool             `json:"is_active"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// CreateWidgetInput is the body for POST /admin/widgets.
type CreateWidgetInput struct {
	Name         string          `json:"name"`
	WidgetType   string          `json:"widget_type"`
	Config       json.RawMessage `json:"config"`
	CacheTTLSecs *int32          `json:"cache_ttl_secs"`
	IsActive     *bool           `json:"is_active"`
}

// UpdateWidgetInput is the body for PATCH /admin/widgets/:id.
type UpdateWidgetInput struct {
	Name         *string          `json:"name"`
	WidgetType   *string          `json:"widget_type"`
	Config       *json.RawMessage `json:"config"`
	CacheTTLSecs *int32           `json:"cache_ttl_secs"`
	IsActive     *bool            `json:"is_active"`
}

// RenderedWidget is the response shape for GET /widgets/:id.
type RenderedWidget struct {
	WidgetID   string          `json:"widget_id"`
	WidgetType string          `json:"widget_type"`
	CardType   string          `json:"card_type,omitempty"`
	Data       json.RawMessage `json:"data"`
	CachedAt   time.Time       `json:"cached_at"`
}

// ValidWidgetTypes are the canonical widget types from Section 16, extended
// to cover all JapaConnect data sources. The original 7 types from the spec
// comment are supplemented with first-class renderers for every major content
// domain (forum, blog, learning, jobs, reviews, comparisons, etc.) so that
// landing pages can embed live data from any platform feature.
var ValidWidgetTypes = []string{
	// --- Original 7 (Section 16 spec comment) ---
	"latest_content",
	"trending_tags",
	"space_members",
	"user_leaderboard",
	"upcoming_events",
	"featured_spaces",
	"custom_html",
	// --- Extended: content-curation types ---
	"featured_content", // queries content_featured (admin-pinned posts)
	"blog_posts",        // contents WHERE content_type='blog'
	"forum_threads",     // forum_threads table (hot/recent discussions)
	"job_listings",      // contents WHERE content_type='job_listing'
	"achievements",      // contents + achievement_meta (visa approvals, etc.)
	// --- Extended: domain-specific types ---
	"learning_tracks",         // learning_tracks table
	"community_digest",        // digest_editions table (§84)
	"reviews",                 // reviews table (§29)
	"comparisons",             // comparison_items table (§30)
	"professional_profiles",   // professional_profiles table (§33)
	"community_organisations", // community_organisations table (§07/§101)
	"mentor_spotlight",        // mentorship_profiles table (§102)
	// --- Extended: form/widget types ---
	"newsletter_signup", // email capture widget (returns config, no DB query)
}

// IsValidWidgetType reports whether t is a recognized widget type.
func IsValidWidgetType(t string) bool {
	for _, v := range ValidWidgetTypes {
		if v == t {
			return true
		}
	}
	return false
}
