// Package admin — HTTP handlers for admin-only platform management endpoints.
//
// Settings handlers manage the system_settings table, providing a structured
// API for reading and writing platform-wide configuration organized by category.
package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/control"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// SettingDef defines the metadata for a single setting within a category.
type SettingDef struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // "text", "number", "toggle", "select", "multiselect", "color", "file", "media", "textarea"
	Value       any      `json:"value,omitempty"`
	Options     []string `json:"options,omitempty"`
	Accept      string   `json:"accept,omitempty"`      // file type filter for "file" type
	Placeholder string   `json:"placeholder,omitempty"` // placeholder text
	Description string   `json:"description,omitempty"` // helper text below label
	Masked      bool     `json:"masked,omitempty"`      // true for secrets — API returns "***"
}

// SettingCategory groups related settings with display metadata.
type SettingCategory struct {
	Key      string       `json:"key"`
	Label    string       `json:"label"`
	Settings []SettingDef `json:"settings"`
}

// SettingValue holds a raw JSONB value read from the database.
type SettingValue struct {
	Key   string
	Value any
}

// Handlers holds dependencies for admin HTTP handlers.
type Handlers struct {
	DB                  *db.DB
	Queries             *gen.Queries
	RBAC                *rbac.RBAC
	Valkey              valkeycompat.Cmdable
	Settings            *config.Settings
	GDPR                GDPRService
	Orgs                OrgVerifier
	Tasks               TaskEnqueuer
	StudioHook          StudioReviewHook
	DefaultFolderSetter auth.DefaultFolderProvider // Optional — wired by server after bookmarkfolders.Service is constructed (§50)
	Catalog             *literals.Catalog          // Optional — caches active tenant literals and resolves maps.
}

// SetDefaultFolderProvider injects the default bookmark folder seeder.
func (h *Handlers) SetDefaultFolderProvider(p auth.DefaultFolderProvider) { h.DefaultFolderSetter = p }

// SetLiteralCatalog wires the shared literal catalog for cache invalidation.
func (h *Handlers) SetLiteralCatalog(c *literals.Catalog) { h.Catalog = c }

// StudioReviewHook is the subset of the studio service the moderation
// handler needs to transition studio_jobs status when a source='studio'
// HITL review is approved or rejected (§75, ADR-076 §3). Without this,
// ModerateItem updates content_reviews but studio_jobs stays 'in_review'
// forever — the same defect class as the persona_action BUG-001 fix.
type StudioReviewHook interface {
	OnStudioReviewApproved(ctx context.Context, tenantID, reviewID, reviewerID pgtype.UUID) error
	OnStudioReviewRejected(ctx context.Context, tenantID, reviewID, reviewerID pgtype.UUID) error
}

// TaskEnqueuer is the subset of the worker enqueuer that admin needs to
// dispatch follow-up tasks (e.g. persona:dispatch_action when a persona
// action review is approved, §76/ADR-077 §4).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// GDPRService is the interface the admin hard-delete handler needs from the GDPR service.
type GDPRService interface {
	HardDeleteUser(ctx context.Context, userID, tenantID pgtype.UUID) error
}

// OrgVerifier is the subset of the organizations service the admin verify
// handler needs to mark an org verified/unverified and emit the
// organization.verified / organization.unverified webhook events (Section 12).
// Local interface so admin does not import organizations (which would create
// a cycle via server wiring).
type OrgVerifier interface {
	VerifyOrganization(ctx context.Context, id, tenantID, adminID pgtype.UUID) error
	UnverifyOrganization(ctx context.Context, id, tenantID, adminID pgtype.UUID) error
}

// NewHandlers creates Handlers with the given database pool.
func NewHandlers(database *db.DB, queries *gen.Queries) *Handlers {
	return &Handlers{DB: database, Queries: queries}
}

// SetRBAC injects the RBAC resolver for permission cache invalidation.
func (h *Handlers) SetRBAC(r *rbac.RBAC) {
	h.RBAC = r
}

// SetValkey injects the Valkey client for permission cache invalidation.
func (h *Handlers) SetValkey(v valkeycompat.Cmdable) {
	h.Valkey = v
}

// SetSettings injects the hot-reloadable settings reader for cache invalidation.
func (h *Handlers) SetSettings(s *config.Settings) {
	h.Settings = s
}

// SetTaskEnqueuer injects the worker task enqueuer for dispatching follow-up
// tasks (e.g. persona:dispatch_action on persona-action review approval, §76).
func (h *Handlers) SetTaskEnqueuer(e TaskEnqueuer) {
	h.Tasks = e
}

// SetGDPR injects the GDPR service for admin hard-delete operations.
func (h *Handlers) SetGDPR(g GDPRService) {
	h.GDPR = g
}

// SetOrgVerifier injects the organizations service for the admin org-verify
// flow (which emits the organization.verified webhook event, Section 12).
func (h *Handlers) SetOrgVerifier(o OrgVerifier) {
	h.Orgs = o
}

// SetStudioReviewHook injects the studio service hook so ModerateItem can
// transition studio_jobs status when a source='studio' HITL review is
// approved or rejected (§75, ADR-076 §3).
func (h *Handlers) SetStudioReviewHook(s StudioReviewHook) {
	h.StudioHook = s
}

// defaultSettings defines every available setting with its metadata.
// The first segment of the dot-notation key (e.g. "general" from
// "general.site_name") determines the category.
var defaultSettings = map[string]SettingCategory{
	"general": {
		Key:   "general",
		Label: "General",
		Settings: []SettingDef{
			{Key: "site_name", Label: "Site Name", Type: "text"},
			{Key: "site_description", Label: "Site Description", Type: "textarea"},
			{Key: "maintenance_mode", Label: "Maintenance Mode", Type: "toggle"},
			{Key: "registration_open", Label: "Open Registration", Type: "toggle"},
		},
	},
	// NOTE: The "features" category was removed from the admin Settings page.
	// Those toggles duplicated the real feature-flag system (feature_flags table,
	// keys feature.blog / feature.forum / ... edited via the Features admin page
	// at GET /api/v1/features). Writing features.* to system_settings had no
	// consumer, so admin edits silently did nothing. Use the Features page to
	// toggle modules.
	"feed": {
		Key:   "feed",
		Label: "Feed",
		Settings: []SettingDef{
			{Key: "fanout_threshold", Label: "Fanout Threshold", Type: "number", Description: "Max space members for push fan-out"},
			{Key: "velocity.gravity", Label: "Velocity Gravity", Type: "number", Description: "HN gravity constant for Rising sort"},
			{Key: "max_followed_spaces_in_feed", Label: "Max Followed Spaces", Type: "number", Description: "Cap on spaces in feed"},
			{Key: "card_style", Label: "Card Style", Type: "card-style", Description: "Per-content-type listing card style. Managed on the Content Types page."},
			{Key: "follower_fanout_cap", Label: "Follower Fanout Cap", Type: "number", Description: "Max author followers receiving a push feed row on publish (BUG-006). Authors with more followers have their fan-out truncated to the most recent N.", Value: 1000},
			{Key: "for_you_affinity_cap", Label: "For You Affinity Cap", Type: "number", Description: "Max affinity-sourced candidates in the for-you pool (BUG-008)", Value: 300},
			{Key: "for_you_cache_ttl_seconds", Label: "For You Cache Ttl Seconds", Type: "number", Description: "For-you ranked feed cache TTL in seconds (BUG-002)", Value: 90},
			{Key: "for_you_candidate_cap", Label: "For You Candidate Cap", Type: "number", Description: "Total candidate cap after union of affinity + velocity pools (BUG-008)", Value: 600},
			{Key: "for_you_geo_boost_cap", Label: "For You Geo Boost Cap", Type: "number", Description: "Maximum multiplier applied to base score for geo-matched content (ADR-064, BUG-007)", Value: 1.5},
			{Key: "for_you_geo_weight", Label: "For You Geo Weight", Type: "number", Description: "Geo boost weight added when content space country matches visitor country (ADR-064, BUG-007)", Value: 1.0},
			{Key: "for_you_half_life_hours", Label: "For You Half Life Hours", Type: "number", Description: "Recency decay half-life in hours for for-you ranking (BUG-007)", Value: 48.0},
			{Key: "for_you_journey_boost_cap", Label: "For You Journey Boost Cap", Type: "number", Description: "Maximum combined journey multiplier applied to a candidate score", Value: 1.8},
			{Key: "for_you_journey_weight_destination", Label: "For You Journey Weight Destination", Type: "number", Description: "Journey boost for content whose target country matches the visitor's destination", Value: 1.5},
			{Key: "for_you_journey_weight_roadmap", Label: "For You Journey Weight Roadmap", Type: "number", Description: "Journey boost for content whose slug appears in the readiness roadmap", Value: 1.4},
			{Key: "for_you_journey_weight_source", Label: "For You Journey Weight Source", Type: "number", Description: "Journey boost for content whose source country matches the visitor's origin", Value: 0.8},
			{Key: "for_you_journey_weight_stage", Label: "For You Journey Weight Stage", Type: "number", Description: "Journey boost for content whose journey stage matches the visitor's stage", Value: 1.2},
			{Key: "for_you_journey_weight_visa", Label: "For You Journey Weight Visa", Type: "number", Description: "Journey boost for content whose visa route matches the visitor's visa route", Value: 1.0},
			{Key: "for_you_tag_overlap_cap", Label: "For You Tag Overlap Cap", Type: "number", Description: "Cap on followed-tag overlap count when computing the tag signal (BUG-007)", Value: 3},
			{Key: "for_you_velocity_cap", Label: "For You Velocity Cap", Type: "number", Description: "Max velocity-sourced candidates in the for-you pool (BUG-008)", Value: 300},
			{Key: "for_you_weight_author", Label: "For You Weight Author", Type: "number", Description: "For-you ranking weight for followed-author signal (BUG-007)", Value: 3.0},
			{Key: "for_you_weight_recency", Label: "For You Weight Recency", Type: "number", Description: "For-you ranking weight for recency decay (BUG-007)", Value: 2.0},
			{Key: "for_you_weight_space", Label: "For You Weight Space", Type: "number", Description: "For-you ranking weight for followed-space signal (BUG-007)", Value: 2.5},
			{Key: "for_you_weight_tag", Label: "For You Weight Tag", Type: "number", Description: "For-you ranking weight for followed-tag overlap signal (BUG-007)", Value: 1.5},
			{Key: "for_you_weight_velocity", Label: "For You Weight Velocity", Type: "number", Description: "For-you ranking weight for normalized velocity (BUG-007)", Value: 1.0},
			{Key: "for_you_window_days", Label: "For You Window Days", Type: "number", Description: "For-you candidate pool lookback window in days (BUG-003)", Value: 30},
			{Key: "max_page_size", Label: "Max Page Size", Type: "number", Description: "Maximum page size accepted by feed list endpoints (BUG-012)", Value: 100},
			{Key: "prune_interval_days", Label: "Prune Interval Days", Type: "number", Description: "Age threshold in days for pruning old feed rows (BUG-010)", Value: 30},
			{Key: "top_window_days", Label: "Top Window Days", Type: "number", Description: "Rolling window in days for the top sort mode (BUG-011)", Value: 7},
			{Key: "velocity_window_days", Label: "Velocity Window Days", Type: "number", Description: "Lookback window in days for velocity score recalculation (BUG-009)", Value: 30},
		},
	},
	// NOTE: The "email" category was removed. SMTP credentials are secrets that
	// belong in JC_* environment variables (Rule 3), not in system_settings.
	// Storing SMTP passwords in a hot-reloadable settings table is a secret-in-DB
	// risk and no backend consumer read these keys.
	"security": {
		Key:   "security",
		Label: "Security",
		Settings: []SettingDef{
			{Key: "max_login_attempts", Label: "Max Login Attempts", Type: "number"},
			{Key: "fail_window_minutes", Label: "Fail Window (min)", Type: "number", Description: "Lockout window in minutes"},
			// Removed: cors.allowed_origins (CORS is env-only via JC_CORS_ORIGINS;
			//   stored under the wrong key and no consumer read it).
			// Removed: lockout_duration_minutes (dead — bruteforce backoff is
			//   computed from max_login_attempts + fail_window_minutes).
			// Removed: require_email_verification (duplicate of
			//   auth.email.verify_required in the Authentication category, which
			//   is the consumed key).
		},
	},
	"appearance": {
		Key:   "appearance",
		Label: "Appearance",
		Settings: []SettingDef{
			{Key: "primary_color", Label: "Primary Color", Type: "color"},
			{Key: "logo_url", Label: "Logo URL", Type: "media", Accept: "image/*"},
			{Key: "favicon_url", Label: "Favicon URL", Type: "media", Accept: "image/*"},
			{
				Key:         "auth_brand_slides",
				Label:       "Auth Brand Slides",
				Type:        "brand-slides",
				Description: "Carousel slides for the login/signup brand panel. Pick images from the media library and add a headline + subheading per slide.",
				Value: []map[string]string{
					{"image": "/brand/kigali-airplane-window.jpg", "headline": "Connecting the diaspora,", "subheading": "one journey at a time"},
					{"image": "/brand/kigali-skyline.jpg", "headline": "Build your future abroad,", "subheading": "with your community behind you"},
					{"image": "/brand/study-abroad.jpg", "headline": "Learn, grow, and thrive,", "subheading": "wherever your path leads"},
					{"image": "/brand/networking.jpg", "headline": "Your network, your roots,", "subheading": "your bridge to opportunity"},
					{"image": "/brand/opportunity.jpg", "headline": "From Kigali to the world,", "subheading": "we grow together"},
				},
			},
			{Key: "dark_mode_default", Label: "Dark Mode Default", Type: "toggle"},
			{Key: "default_editor", Label: "Default Editor Mode", Type: "select", Options: []string{"block", "wysiwyg", "markdown"}, Value: "block", Description: "Default editor mode for the post composer"},
			{Key: "feed_card_style", Label: "Feed Card Style", Type: "text", Description: "Default feed card style: `reddit`|`compact`|`grid`|`magazine`", Value: "reddit"},
		},
	},
	"users": {
		Key:   "users",
		Label: "Users",
		Settings: []SettingDef{
			{Key: "allow_social_login", Label: "Allow Social Login", Type: "toggle"},
			{Key: "require_profile_completion", Label: "Require Profile Completion", Type: "toggle"},
			{Key: "max_avatar_size_mb", Label: "Max Avatar Size (MB)", Type: "number"},
			{Key: "allowed_avatar_types", Label: "Allowed Avatar Types", Type: "multiselect", Options: []string{"jpeg", "jpg", "png", "gif", "webp", "svg"}},
			{Key: "username_cooldown_days", Label: "Username Cooldown Days", Type: "number", Description: "Minimum days between username changes (§50)", Value: 30},
		},
	},
	"content": {
		Key:   "content",
		Label: "Content",
		Settings: []SettingDef{
			{Key: "posts_per_page", Label: "Posts Per Page", Type: "number"},
			{Key: "allow_anonymous_posts", Label: "Allow Anonymous Posts", Type: "toggle"},
			{Key: "require_content_review", Label: "Require Content Review", Type: "toggle"},
			{Key: "max_upload_size_mb", Label: "Max Upload Size (MB)", Type: "number"},
			{Key: "allowed_upload_types", Label: "Allowed Upload Types", Type: "multiselect", Options: []string{"jpeg", "jpg", "png", "gif", "webp", "svg", "pdf", "mp4", "webm"}},
			{Key: "link_preview.blacklist", Label: "Link Preview Blacklist", Type: "textarea", Placeholder: `["example.com", "spam.com"]`},
			{Key: "edit_lock_ttl_seconds", Label: "Edit Lock TTL (seconds)", Type: "number", Description: "Editor lock TTL in seconds"},
			{Key: "edit_window_minutes", Label: "Edit Window (minutes)", Type: "number", Description: "Content edit window (24hr default)"},
			{Key: "max_tags", Label: "Max Tags", Type: "number", Description: "Max tags per content item"},
			{Key: "min_length", Label: "Min Content Length", Type: "number", Description: "Minimum content body length"},
			{Key: "comments.edit_window_minutes", Label: "Comment Edit Window (min)", Type: "number"},
			{Key: "forum.edit_window_minutes", Label: "Forum Edit Window (min)", Type: "number"},
			{Key: "pagination.style", Label: "Pagination Style", Type: "text", Description: "Default feed/list pagination style: `infinite_scroll`|`load_more`|`pagination`", Value: "infinite_scroll"},
		},
	},
	"payment": {
		Key:   "payment",
		Label: "Payment",
		Settings: []SettingDef{
			// NOTE: Payment provider credentials (Stripe/Paystack keys) are
			// secrets that belong in JC_* env vars (Rule 3). Only the default
			// payment currency is exposed here; see payment.currency.
			{Key: "currency", Label: "Payment Currency", Type: "select", Options: []string{"NGN", "USD", "GBP", "EUR"}, Value: "NGN", Description: "Default payment currency for payment and credit operations"},
		},
	},
	"media": {
		Key:   "media",
		Label: "Media",
		Settings: []SettingDef{
			// NOTE: Media storage infrastructure (storage.*, cdn.*) managed via Control Panel.
			// Only content-level media settings belong in admin.
			{Key: "allowed_video_types", Label: "Allowed Video Types", Type: "multiselect", Options: []string{"mp4", "webm", "mov", "avi", "mkv"}},
			{Key: "max_upload_mb", Label: "Max Upload Size (MB)", Type: "number"},
		},
	},
	"seo": {
		Key:   "seo",
		Label: "SEO",
		Settings: []SettingDef{
			{Key: "meta_title_template", Label: "Meta Title Template", Type: "text"},
			{Key: "meta_description_template", Label: "Meta Description Template", Type: "textarea"},
			{Key: "og_image", Label: "Default OG Image", Type: "media", Accept: "image/*"},
			{Key: "twitter_handle", Label: "Twitter/X Handle", Type: "text", Placeholder: "@username"},
			{Key: "json_ld_type", Label: "Default JSON-LD Type", Type: "select", Options: []string{"Organization", "WebSite", "LocalBusiness"}},
			{Key: "google_analytics_id", Label: "Google Analytics ID", Type: "text", Placeholder: "G-XXXXXXXXXX"},
			{Key: "google_tag_manager_id", Label: "Google Tag Manager ID", Type: "text", Placeholder: "GTM-XXXXXXX"},
			{Key: "sitemap.space_min_members", Label: "Sitemap: Min Space Members", Type: "number", Description: "Minimum member count for a space to be included in the sitemap."},
			{Key: "sitemap.space_min_posts", Label: "Sitemap: Min Space Posts (total)", Type: "number", Description: "Minimum total post count for a space to be included in the sitemap."},
			{Key: "sitemap.space_min_forums", Label: "Sitemap: Min Space Forums", Type: "number", Description: "Minimum forum thread count for a space to be included in the sitemap."},
			{Key: "sitemap.space_min_questions", Label: "Sitemap: Min Space Questions", Type: "number", Description: "Minimum solved question count for a space to be included in the sitemap."},
			{Key: "sitemap.content_min_words", Label: "Sitemap: Min Content Words (fallback)", Type: "number", Description: "Fallback minimum word count when no per-type threshold is set."},
			{Key: "sitemap.blog_min_words", Label: "Sitemap: Min Blog Words", Type: "number", Description: "Minimum word count for blog posts (overrides fallback)."},
			{Key: "sitemap.forum_min_words", Label: "Sitemap: Min Forum Words", Type: "number", Description: "Minimum word count for forum threads (overrides fallback)."},
			{Key: "sitemap.discussion_min_words", Label: "Sitemap: Min Discussion Words", Type: "number", Description: "Minimum word count for discussions (overrides fallback)."},
			{Key: "sitemap.guide_min_words", Label: "Sitemap: Min Guide Words", Type: "number", Description: "Minimum word count for guides (overrides fallback)."},
			{Key: "sitemap.comparison_min_words", Label: "Sitemap: Min Comparison Words", Type: "number", Description: "Minimum word count for comparison content (overrides fallback)."},
			{Key: "sitemap.blog_min_engagement", Label: "Sitemap: Min Blog Engagement", Type: "number", Description: "Minimum engagement (comments + shares + reactions) for blog posts."},
			{Key: "sitemap.forum_min_replies", Label: "Sitemap: Min Forum Replies", Type: "number", Description: "Minimum reply count for forum threads to be indexed."},
			{Key: "sitemap.question_min_answers", Label: "Sitemap: Min Question Answers", Type: "number", Description: "Minimum answer count for questions (solved threads). Set 0 to include unsolved."},
			{Key: "sitemap.discussion_min_comments", Label: "Sitemap: Min Discussion Comments", Type: "number", Description: "Minimum comment count for discussions to be indexed."},
			// Removed: enable_sitemap / enable_robots_txt / enable_json_ld —
			//   sitemap & robots.txt are always served (robots.txt is generated
			//   from the bot registry per ADR-043, not a system_settings toggle),
			//   and JSON-LD is emitted by the SSR shell. These toggles had no
			//   consumer and misled admins into thinking they could disable SEO
			//   endpoints. Bot policy is managed in the Bot Management page.
		},
	},
	// NOTE: The "integrations" category was removed. Meilisearch/Honcho endpoints
	// are infrastructure configured via JC_* env vars (Rule 3), not hot-reloadable
	// system_settings, and no backend consumer read these keys.
	"ai": {
		Key:   "ai",
		Label: "AI",
		Settings: []SettingDef{
			// Removed: ai_assist_enabled (duplicate of ai.content_assist_enabled),
			//   ai_model / ai_max_tokens (provider config, env-only per Rule 3),
			//   ai_content_moderation (use moderation.ai_auto_flag_threshold in
			//   the Moderation category). None had a backend consumer.
			{Key: "monthly_budget_usd", Label: "Monthly Budget (USD)", Type: "number", Description: "Alert threshold for AI spend"},
			{Key: "japabot_enabled", Label: "JapaBot Enabled", Type: "toggle"},
			{Key: "content_assist_enabled", Label: "Content Assist Enabled", Type: "toggle"},
			{Key: "humanize.enabled", Label: "Humanize Enabled", Type: "toggle", Description: "Global toggle for AI text humanization (natural-language style injection)", Value: false},
			{Key: "humanize.level", Label: "Humanize Level", Type: "text", Description: "Humanization intensity: low | medium | high", Value: "medium"},
		},
	},
	"agent": {
		Key:   "agent",
		Label: "AI Agent",
		Settings: []SettingDef{
			{Key: "enabled", Label: "Agent Enabled", Type: "toggle", Value: true, Description: "Global AI agent toggle"},
			{Key: "max_tool_calls", Label: "Max Tool Calls", Type: "number", Value: 20, Description: "Max tool calls per agent request"},
			{Key: "timeout_seconds", Label: "Timeout (seconds)", Type: "number", Value: 60, Description: "Agent request timeout"},
			{Key: "rate_limit_per_day", Label: "Rate Limit Per Day", Type: "number", Value: 50, Description: "Max agent requests per user per day"},
			{Key: "daily_budget_usd", Label: "Daily Budget (USD)", Type: "number", Value: 5.00, Description: "Per-user daily AI spend hard cap; 0 disables"},
			{Key: "cost_limit_per_request_usd", Label: "Cost Limit Per Request (USD)", Type: "number", Value: 0.50, Description: "Per-request cost limit"},
			{Key: "temperature", Label: "LLM Temperature", Type: "number", Value: 0.3, Description: "Sampling temperature (0.0–2.0)"},
			{Key: "guest_enabled", Label: "Guest Agent Enabled", Type: "toggle", Value: false, Description: "Allow guest (unauthenticated) agent prompts"},
			{Key: "default_persona_id", Label: "Default Persona ID", Type: "text", Description: "Default AI persona for draft authorship"},
			{Key: "external_search_provider", Label: "External Search Provider", Type: "select", Options: []string{"tavily", "brave", "none"}, Value: "tavily", Description: "Pre-search provider"},
			{Key: "presearch_full_threshold", Label: "Pre-search Full Threshold", Type: "number", Value: 0.85, Description: "Relevance threshold for FULL pre-search"},
			{Key: "bulk_create_max_items", Label: "Bulk Create Max Items", Type: "number", Value: 50, Description: "Max items per content.bulk_create"},
			{Key: "bulk_publish_max_items", Label: "Bulk Publish Max Items", Type: "number", Value: 50, Description: "Max items per content.bulk_publish"},
		},
	},
	// NOTE: The "advanced" category was removed. debug_mode / log_level are
	// process-startup concerns configured via JC_* env vars (Rule 3); they cannot
	// be hot-reloaded from system_settings. cache_ttl_seconds / rate_limit_per_minute
	// were vague and had no consumer.
	"auth": {
		Key:   "auth",
		Label: "Authentication",
		Settings: []SettingDef{
			{Key: "password.min_length", Label: "Min Password Length", Type: "number"},
			{Key: "password.require_uppercase", Label: "Require Uppercase", Type: "toggle"},
			{Key: "session.refresh_days", Label: "Session Refresh (days)", Type: "number"},
			{Key: "session.access_token_ttl_minutes", Label: "Access Token TTL (min)", Type: "number"},
			{Key: "totp.required_for_admin", Label: "Require 2FA for Admin", Type: "toggle"},
			{Key: "email.verify_required", Label: "Require Email Verification", Type: "toggle"},
			{Key: "password.check_breach", Label: "Password Check Breach", Type: "toggle", Description: "Check password against HIBP breach database before accepting", Value: true},
			{Key: "password.history_count", Label: "Password History Count", Type: "number", Description: "Number of recent password hashes to check against reuse", Value: 5},
			{Key: "password.max_length", Label: "Password Max Length", Type: "number", Description: "Maximum password length", Value: 128},
			{Key: "password.require_number", Label: "Password Require Number", Type: "toggle", Description: "Enforce numeric digit requirement", Value: true},
			{Key: "password.require_symbol", Label: "Password Require Symbol", Type: "toggle", Description: "Enforce symbol requirement", Value: false},
		},
	},
	"spaces": {
		Key:   "spaces",
		Label: "Spaces",
		Settings: []SettingDef{
			{Key: "bubble_depth", Label: "Bubble Depth", Type: "number", Description: "How many levels posts bubble up"},
			{Key: "max_pinned_posts", Label: "Max Pinned Posts", Type: "number", Description: "Max pinned posts per space"},
			{Key: "wiki_rate_interval_ms", Label: "Wiki Rate Interval Ms", Type: "number", Description: "Minimum interval (ms) between Wiki API calls (BUG-005)", Value: 2000},
		},
	},
	"moderation": {
		Key:   "moderation",
		Label: "Moderation",
		Settings: []SettingDef{
			{Key: "auto_hide_report_count", Label: "Auto-Hide After N Reports", Type: "number"},
			{Key: "ai_auto_flag_threshold", Label: "AI Auto-Flag Threshold", Type: "number", Description: "Confidence threshold (0-1)"},
			{Key: "max_links_per_post", Label: "Max Links Per Post", Type: "number"},
			{Key: "new_account_threshold_days", Label: "New Account Threshold (days)", Type: "number"},
			{Key: "new_user_min_posts_for_links", Label: "New User Min Posts For Links", Type: "number", Description: "Min total posts before a new user may include external links (§41 Layer 3)", Value: 3},
			{Key: "report_reasons", Label: "Report Reasons", Type: "textarea", Description: "Available report reason codes for the report dialog (Rule 16 compliance)", Value: `["spam","harassment","hate_speech","misinformation","illegal_content","self_harm","copyright","other"]`},
		},
	},
	"wallet": {
		Key:   "wallet",
		Label: "Wallet",
		Settings: []SettingDef{
			{Key: "min_convert_points", Label: "Min Convert Points", Type: "number"},
			{Key: "points_to_credit_rate", Label: "Points to Credit Rate", Type: "number", Description: "1 point = N credits"},
			{Key: "max_daily_earn_points", Label: "Max Daily Earn Points", Type: "number"},
			{Key: "credit_purchase_enabled", Label: "Credit Purchase Enabled", Type: "toggle", Description: "Killswitch for credit purchasing. When false, `POST /wallet/purchase-credits` returns 403; spending existing credits still works. Ref: Section 58.", Value: true},
			{Key: "default_currency", Label: "Default Currency", Type: "text", Description: "Fallback currency code when user has no country/preference (ISO 4217). Ref: Section 58, ADR-065.", Value: "NGN"},
			{Key: "min_purchase_credits", Label: "Min Purchase Credits", Type: "number", Description: "Minimum credits for a custom-amount credit purchase. Ref: Section 58.", Value: 100},
		},
	},
	"api": {
		Key:   "api",
		Label: "API",
		Settings: []SettingDef{
			{Key: "rate_limit.basic", Label: "Basic Rate Limit", Type: "number", Description: "Requests per minute"},
			{Key: "rate_limit.standard", Label: "Standard Rate Limit", Type: "number"},
			{Key: "rate_limit.premium", Label: "Premium Rate Limit", Type: "number"},
			{Key: "keys.max_per_user", Label: "Max API Keys Per User", Type: "number"},
			{Key: "idempotency_ttl_hours", Label: "Idempotency TTL (hours)", Type: "number"},
		},
	},
	"ads": {
		Key:   "ads",
		Label: "Ads",
		Settings: []SettingDef{
			{Key: "frequency_cap_per_day", Label: "Frequency Cap Per Day", Type: "number"},
			{Key: "min_campaign_budget", Label: "Min Campaign Budget", Type: "number", Description: "Minimum credits"},
		},
	},
	"notifications": {
		Key:   "notifications",
		Label: "Notifications",
		Settings: []SettingDef{
			{Key: "max_delivery_attempts", Label: "Max Delivery Attempts", Type: "number"},
			{Key: "batch_window_minutes", Label: "Batch Window (minutes)", Type: "number"},
		},
	},
	"tickets": {
		Key:   "tickets",
		Label: "Tickets",
		Settings: []SettingDef{
			{Key: "default_sla_hours", Label: "Default SLA (hours)", Type: "number"},
			{Key: "auto_close_resolved_days", Label: "Auto-Close Resolved (days)", Type: "number"},
		},
	},
	"audit": {
		Key:   "audit",
		Label: "Audit",
		Settings: []SettingDef{
			{Key: "log_retention_days", Label: "Log Retention (days)", Type: "number"},
		},
	},
	"i18n": {
		Key:   "i18n",
		Label: "Languages",
		Settings: []SettingDef{
			{Key: "default_language", Label: "Default Language", Type: "select", Options: []string{"en", "fr"}},
			{Key: "enabled_languages", Label: "Enabled Languages", Type: "multiselect", Options: []string{"en", "fr", "yo", "ha", "ig", "pcm"}},
		},
	},
	"gdpr": {
		Key:   "gdpr",
		Label: "GDPR",
		Settings: []SettingDef{
			// BUG-GDPR-006: deletion_grace_days removed — superseded by
			// retention_days (ADR-057). No code reads it.
			{Key: "export_expiry_days", Label: "Export Expiry (days)", Type: "number"},
			{Key: "hard_delete_enabled", Label: "Enable Hard Delete", Type: "toggle"},
			{Key: "retention_days", Label: "Retention Days", Type: "number", Description: "0 = immediate"},
			{Key: "deleted_user_placeholder_uuid", Label: "Deleted User Placeholder UUID", Type: "text"},
			{Key: "policy_version", Label: "Policy Version", Type: "number", Description: "Current privacy policy version stamped on consent records (BUG-GDPR-005)", Value: 1.0},
		},
	},
	"search": {
		Key:   "search",
		Label: "Search",
		Settings: []SettingDef{
			{Key: "autocomplete_max", Label: "Autocomplete Max", Type: "number"},
		},
	},
	"affiliate": {
		Key:   "affiliate",
		Label: "Affiliate",
		Settings: []SettingDef{
			{Key: "min_account_days", Label: "Min Account Days", Type: "number"},
			{Key: "attribution_window_days", Label: "Attribution Window (days)", Type: "number"},
			{Key: "commission.pro_monthly", Label: "Pro Monthly Commission %", Type: "number"},
			{Key: "commission.pro_annual", Label: "Pro Annual Commission %", Type: "number"},
			{Key: "commission.course", Label: "Course Commission %", Type: "number"},
			{Key: "max_links_per_user", Label: "Max Links Per User", Type: "number"},
			{Key: "author_commission_default", Label: "Author Commission Default", Type: "number", Description: "Default % of external affiliate commission shared with content author (when provider has no override)", Value: 25},
			{Key: "scan_on_publish", Label: "Scan On Publish", Type: "toggle", Description: "Whether to scan content for unregistered affiliate links on publish", Value: true},
			{Key: "unregistered_link_policy", Label: "Unregistered Link Policy", Type: "text", Description: "How to handle unregistered affiliate links: `\"flag\"` (log only), `\"block\"` (prevent publish), `\"strip\"` (remove link)", Value: "flag"},
		},
	},
	"organizations": {
		Key:   "organizations",
		Label: "Organizations",
		Settings: []SettingDef{
			{Key: "max_per_user", Label: "Max Per User", Type: "number", Description: "Max organizations a user can create (0 = none, -1 = unlimited)", Value: 5},
		},
	},

	"page": {
		Key:   "page",
		Label: "Page",
		Settings: []SettingDef{
			{Key: "cache_ttl_secs", Label: "Cache Ttl Secs", Type: "number", Description: "TTL for the published-page Valkey cache (`jc:page:{tenant_id}:{slug}`)", Value: 300},
		},
	},

	"site": {
		Key:   "site",
		Label: "Site",
		Settings: []SettingDef{
			{Key: "custom_css", Label: "Custom CSS", Type: "textarea", Description: "Raw CSS injected inside `<style id=\"jc-custom-css\">` in `<head>`. Max 50 KB."},
			{Key: "custom_js", Label: "Custom JS", Type: "textarea", Description: "Raw JS injected as `<script id=\"jc-custom-js\">` before `</body>`. Max 100 KB."},
		},
	},

	"trending": {
		Key:   "trending",
		Label: "Trending",
		Settings: []SettingDef{
			{Key: "expiry.24h_hours", Label: "Expiry 24h Hours", Type: "number", Description: "TTL (hours) for 24h trending snapshots", Value: 2},
			{Key: "expiry.30d_hours", Label: "Expiry 30d Hours", Type: "number", Description: "TTL (hours) for 30d trending snapshots", Value: 32},
			{Key: "expiry.7d_hours", Label: "Expiry 7d Hours", Type: "number", Description: "TTL (hours) for 7d trending snapshots", Value: 8},
			{Key: "gravity", Label: "Gravity", Type: "number", Description: "HN gravity exponent for trending velocity (ADR-032)", Value: 1.8},
			{Key: "weight.bookmarks", Label: "Weight Bookmarks", Type: "number", Description: "Content/user velocity: bookmarks multiplier", Value: 3},
			{Key: "weight.comments", Label: "Weight Comments", Type: "number", Description: "Content/user velocity: comments multiplier", Value: 5},
			{Key: "weight.shares", Label: "Weight Shares", Type: "number", Description: "Content/user velocity: shares multiplier", Value: 8},
			{Key: "weight.space_member", Label: "Weight Space Member", Type: "number", Description: "Space trending: member count multiplier", Value: 1},
			{Key: "weight.space_post", Label: "Weight Space Post", Type: "number", Description: "Space trending: post count multiplier", Value: 2},
			{Key: "weight.space_recent", Label: "Weight Space Recent", Type: "number", Description: "Space trending: recent post count multiplier", Value: 5},
			{Key: "weight.tag_content", Label: "Weight Tag Content", Type: "number", Description: "Tag trending: recent content count multiplier", Value: 3},
			{Key: "weight.tag_follower", Label: "Weight Tag Follower", Type: "number", Description: "Tag trending: follower count multiplier", Value: 1},
			{Key: "weight.views", Label: "Weight Views", Type: "number", Description: "Content/user velocity: views multiplier", Value: 1},
			{Key: "weight.votes", Label: "Weight Votes", Type: "number", Description: "Content/user velocity: votes multiplier", Value: 10},
		},
	},
}

// settingCategoryOrder controls the response order sent to the frontend.
// Removed categories (features, email, integrations, advanced) are intentionally
// absent — see the NOTE comments above each removal in defaultSettings.
var settingCategoryOrder = []string{
	"general", "auth", "feed", "security",
	"appearance", "users", "content",
	"media", "seo", "ai", "agent",
	"spaces", "moderation", "wallet", "payment", "api", "ads",
	"notifications", "tickets", "audit", "i18n", "gdpr",
	"search", "affiliate",
	"organizations",
	"page",
	"site",
	"trending",
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// invalidateLinkPreviewCache removes all cached link previews so that an
// updated domain blacklist takes effect immediately instead of after the 24h TTL.
func invalidateLinkPreviewCache(ctx context.Context, vk valkeycompat.Cmdable) {
	var cursor uint64
	for {
		keys, nextCursor, err := vk.Scan(ctx, cursor, "jc:link_preview:*", 100).Result()
		if err != nil {
			slog.Warn("settings: failed to scan link preview cache for invalidation", "error", err)
			return
		}
		if len(keys) > 0 {
			_ = vk.Del(ctx, keys...).Err()
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// requireTenant resolves the tenant from the request context.
func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// ---------------------------------------------------------------------------
// ListSettings — GET /admin/settings
// ---------------------------------------------------------------------------

// ListSettings returns all platform settings for the current tenant,
// organized by category. The response shape matches the frontend's
// expected format: an array of categories, each containing an array
// of settings with their current values.
func (h *Handlers) ListSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	ctx := r.Context()

	// Fetch all settings for this tenant from the database.
	rows, err := h.DB.Reader.Query(
		ctx,
		`SELECT key, value FROM system_settings WHERE tenant_id = $1`,
		tenantID,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load settings")
		return
	}
	defer rows.Close()

	// Build a map of stored values keyed by the short key (after the dot).
	stored := make(map[string]any)

	for rows.Next() {
		var fullKey string

		var value any
		if err := rows.Scan(&fullKey, &value); err != nil {
			continue
		}
		// The stored key is dot-notation (e.g. "general.site_name");
		// the short key is the part after the first dot.
		parts := strings.SplitN(fullKey, ".", 2)
		if len(parts) == 2 {
			stored[parts[1]] = value
		}
	}

	// Assemble the response by iterating over defined categories in order,
	// merging stored values with the schema defaults.
	result := make([]SettingCategory, 0, len(settingCategoryOrder))

	for _, catKey := range settingCategoryOrder {
		def, ok := defaultSettings[catKey]
		if !ok {
			continue
		}

		settings := make([]SettingDef, len(def.Settings))
		for i, s := range def.Settings {
			settings[i] = s
			if val, exists := stored[s.Key]; exists {
				if s.Masked {
					settings[i].Value = "***"
				} else {
					settings[i].Value = val
				}
			}
		}

		result = append(result, SettingCategory{
			Key:      def.Key,
			Label:    def.Label,
			Settings: settings,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ---------------------------------------------------------------------------
// UpdateSettings — PUT /admin/settings/{category}
// ---------------------------------------------------------------------------

// UpdateSettings saves settings for a single category. The request body
// contains a flat map of key-value pairs (e.g. {"site_name": "New Name"}).
// Each pair is upserted into the system_settings table using the full
// dot-notation key (category.setting_key).
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	category := chi.URLParam(r, "category")
	if category == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}

	// Validate that the category exists in our schema.
	def, ok := defaultSettings[category]
	if !ok {
		errpkg.WriteError(w, http.StatusNotFound, "invalid_category", "Unknown settings category: "+category)
		return
	}

	// Parse the request body as key-value pairs.
	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if len(updates) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_body", "No settings provided")
		return
	}

	// Build a set of valid keys for this category so we can reject unknown keys.
	validKeys := make(map[string]bool, len(def.Settings))
	for _, s := range def.Settings {
		validKeys[s.Key] = true
	}

	ctx := r.Context()

	// Validate every key up front and serialize values to JSON before delegating
	// the transactional upsert to the settings service (TX-001: tx belongs to service).
	// is_public is resolved from the control settings catalog so appearance keys
	// are exposed via GET /site/settings/public without a separate control-panel call.
	pending := make(map[string][]byte, len(updates))
	isPublic := make(map[string]bool, len(updates))
	for key, value := range updates {
		if !validKeys[key] {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_key", "Unknown setting key: "+key)
			return
		}
		valueJSON, err := json.Marshal(value)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", "Cannot serialize value for key: "+key)
			return
		}

		// Section 57: custom CSS/JS have size caps (50 KB / 100 KB raw value).
		fullKey := category + "." + key
		if category == "site" {
			var raw string
			if err := json.Unmarshal(valueJSON, &raw); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", key+" must be a string")
				return
			}
			if key == "custom_css" && len(raw) > 50*1024 {
				errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Custom CSS exceeds 50 KB limit")
				return
			}
			if key == "custom_js" && len(raw) > 100*1024 {
				errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Custom JS exceeds 100 KB limit")
				return
			}
		}

		pending[fullKey] = valueJSON
		isPublic[fullKey] = control.IsPublicSetting(fullKey)
	}

	if err := h.Settings.UpdateSettingsBatchWithPublic(ctx, tenantID, pending, isPublic); err != nil {
		slog.Error("admin: failed to save settings", "category", category, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to save settings")
		return
	}

	// Invalidate link preview cache when the blacklist is updated, since
	// already-cached previews for now-blocklisted domains would survive their 24h TTL.
	if category == "content" && h.Valkey != nil {
		invalidateLinkPreviewCache(r.Context(), h.Valkey)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":   "saved",
			"category": category,
		},
	})
}
