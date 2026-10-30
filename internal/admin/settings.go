// Package admin — HTTP handlers for admin-only platform management endpoints.
// // Settings handlers manage the system_settings table, providing a structured
// API for reading and writing platform-wide configuration organized by category.
package admin

import (
	"context"
	"encoding/json"
	"github.com/jedafe/japa/internal/brand"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/control"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// SettingDef defines the metadata for a single setting within a category.
type SettingDef struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // "text", "number", "toggle", "select", "multiselect", "color", "file", "media", "textarea", "json"
	Value       any      `json:"value,omitempty"`
	Options     []string `json:"options,omitempty"`
	Max         int      `json:"max,omitempty"`         // item cap for repeater types (link-list, link-cards)
	Accept      string   `json:"accept,omitempty"`      // file type filter for "file" type
	Placeholder string   `json:"placeholder,omitempty"` // placeholder text
	Description string   `json:"description,omitempty"` // helper text below label
	Masked      bool     `json:"masked,omitempty"`      // true for secrets — API returns "***"
	// DefaultValue echoes the catalog default so admin surfaces can render a
	// per-setting "reset to system default" affordance. It is populated by
	// ListSettings before any stored override is merged into Value.
	DefaultValue any `json:"default_value,omitempty"`
	// Overridden reports whether a system_settings row exists for the key —
	// i.e. the effective value is a customization, not the catalog default.
	Overridden bool `json:"overridden,omitempty"`
	// Hidden keeps the key writable via PUT /admin/settings/{category} but
	// omits it from ListSettings — for settings managed on a dedicated admin
	// surface (e.g. docs.categories on /docs/categories) rather than the
	// generic settings page.
	Hidden bool `json:"-"`
	// StorageKey overrides the system_settings row this definition reads and
	// writes when it differs from category+"."+Key — for settings grouped
	// under an admin category whose canonical key lives in another namespace
	// (e.g. "Comment Edit Window" under Content stores to
	// comments.edit_window_minutes, the key the comments service reads).
	StorageKey string `json:"-"`
}

// settingStorageKey resolves the system_settings row a definition persists
// to — StorageKey when set, otherwise the conventional category+"."+Key.
func settingStorageKey(category string, s SettingDef) string {
	if s.StorageKey != "" {
		return s.StorageKey
	}
	return category + "." + s.Key
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
	Users               UserVerifier
	Tasks               TaskEnqueuer
	StudioHook          StudioReviewHook
	DraftPublishHook    DraftPublishHook           // Optional — publishes non-studio ai_hitl_reviews targets on approve (§103)
	ToolReviewHook      ToolReviewHook             // Optional — §§107–110 tool entities in the unified queue
	DefaultFolderSetter auth.DefaultFolderProvider // Optional — wired by server after bookmarkfolders.Service is constructed (§50)
	TierManager         auth.TierManager           // Optional — wired by server after tiers.Service is constructed (§unified-tiers)
	Catalog             *literals.Catalog          // Optional — caches active platform literals and resolves maps.
	AppealResolver      AppealResolver             // Optional — §22 appeal resolution + per-type overturn effects
	ownerEmail          string                     // JC_OWNER_EMAIL — platform owner designation (§03 owner amendment)
}

// AppealResolver is the subset of the §22 appeals service the moderation
// appeal handler delegates to — status validation, subject-type overturn
// effects, and linked-ticket resolution live there.
type AppealResolver interface {
	Resolve(ctx context.Context, appealID, reviewerID pgtype.UUID, status, note string) error
}

// SetAppealResolver injects the appeals service used by PATCH
// /admin/appeals/{id} so overturns apply their domain reversal.
func (h *Handlers) SetAppealResolver(r AppealResolver) { h.AppealResolver = r }

// SetTierManager injects the tier manager used to assign the regular tier on
// admin-created users.
func (h *Handlers) SetTierManager(m auth.TierManager) { h.TierManager = m }

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
	OnStudioReviewApproved(ctx context.Context, reviewID, reviewerID pgtype.UUID) error
	OnStudioReviewRejected(ctx context.Context, reviewID, reviewerID pgtype.UUID) error
}

// DraftPublishHook is the subset of the HITL target publisher the
// moderation handler needs to take a non-studio ai_hitl_reviews row's
// target live on approve — contents rows get status='published';
// tool-table targets route through the dedicated tool publisher (§103).
type DraftPublishHook interface {
	PublishHITLTarget(ctx context.Context, reviewerID pgtype.UUID, targetTable string, targetID pgtype.UUID) error
}

// ToolReviewHook is the dispatcher the unified moderation queue calls to
// approve or reject §§107–110 tool entities (crs_rulesets,
// pof_requirements, eca_guides, eca_community_updates, sop_samples,
// sop_templates). It must return handled=false when the ID does not match
// a tool row so ModerateItem falls through to the generic updates.
// appliedStatus reports the entity-level status actually written
// ('active' for CRS rulesets, 'published' for guides/samples, 'approved'
// for community updates) so the response reflects the real outcome.
type ToolReviewHook interface {
	HandleToolReview(ctx context.Context, id, reviewerID pgtype.UUID, action, reason string) (handled bool, appliedStatus string, err error)
}

// TaskEnqueuer is the subset of the worker enqueuer that admin needs to
// dispatch follow-up tasks (e.g. persona:dispatch_action when a persona
// action review is approved, §76/ADR-077 §4).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// GDPRService is the interface the admin hard-delete handler needs from the GDPR service.
type GDPRService interface {
	HardDeleteUser(ctx context.Context, userID pgtype.UUID) error
}

// OrgVerifier is the subset of the organizations service the admin verify
// handler needs to mark an org verified/unverified and emit the
// organization.verified / organization.unverified webhook events (Section 12).
// Local interface so admin does not import organizations (which would create
// a cycle via server wiring).
type OrgVerifier interface {
	VerifyOrganization(ctx context.Context, id, adminID pgtype.UUID, vtype *string) error
	UnverifyOrganization(ctx context.Context, id, adminID pgtype.UUID) error
}

// UserVerifier is the subset of the users service the admin verify handlers
// need to grant/revoke the §07 profile verification badge and emit the
// user.verified / user.unverified webhook events (Section 12).
// Local interface so admin does not import users (cycle-safe).
type UserVerifier interface {
	VerifyUser(ctx context.Context, id, adminID pgtype.UUID, vtype *string) error
	UnverifyUser(ctx context.Context, id, adminID pgtype.UUID) error
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

// SetUserVerifier injects the users service for the admin profile-verify
// flow (which emits the user.verified webhook event, Section 12).
func (h *Handlers) SetUserVerifier(u UserVerifier) {
	h.Users = u
}

// SetStudioReviewHook injects the studio service hook so ModerateItem can
// transition studio_jobs status when a source='studio' HITL review is
// approved or rejected (§75, ADR-076 §3).
func (h *Handlers) SetStudioReviewHook(s StudioReviewHook) {
	h.StudioHook = s
}

// SetDraftPublishHook injects the publisher that takes a non-studio
// ai_hitl_reviews target live on approve (§103 "Approve → publish" —
// agent_draft_request drafts enter the queue unpublished).
func (h *Handlers) SetDraftPublishHook(d DraftPublishHook) {
	h.DraftPublishHook = d
}

// SetToolReviewHook injects the §§107–110 tool review dispatcher so the
// unified moderation queue can approve/reject CRS rulesets, POF
// requirements, ECA guides + community updates, and SOP samples/templates.
func (h *Handlers) SetToolReviewHook(t ToolReviewHook) {
	h.ToolReviewHook = t
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
			// stored under the wrong key and no consumer read it).
			// Removed: lockout_duration_minutes (dead — bruteforce backoff is
			// computed from max_login_attempts + fail_window_minutes).
			// Removed: require_email_verification (duplicate of
			// auth.email.verify_required in the Authentication category, which
			// is the consumed key).
		},
	},
	"appearance": {
		Key:   "appearance",
		Label: "Appearance",
		Settings: []SettingDef{
			{Key: "primary_color", Label: "Primary Color", Type: "color"},
			{Key: "primary_color_dark", Label: "Primary Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = primary color applies to both themes"},
			{Key: "secondary_color", Label: "Secondary Color", Type: "color", Description: "Accent color mapped to --color-secondary"},
			{Key: "secondary_color_dark", Label: "Secondary Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = theme default"},
			{Key: "background_color", Label: "Background Color", Type: "color", Description: "Page background, light theme only"},
			{Key: "background_color_dark", Label: "Background Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = theme default"},
			{Key: "surface_color", Label: "Surface Color", Type: "color", Description: "Cards and panels, light theme only"},
			{Key: "surface_color_dark", Label: "Surface Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = theme default"},
			{Key: "surface_subtle_color", Label: "Subtle Surface Color", Type: "color", Description: "Subtle/hover surface tone, light theme only"},
			{Key: "surface_subtle_color_dark", Label: "Subtle Surface Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = theme default"},
			{Key: "text_color", Label: "Text Color", Type: "color", Description: "Primary text, light theme only"},
			{Key: "text_color_dark", Label: "Text Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = theme default"},
			{Key: "text_subtle_color", Label: "Subtle Text Color", Type: "color", Description: "Secondary/caption text, light theme only"},
			{Key: "text_subtle_color_dark", Label: "Subtle Text Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = theme default"},
			{Key: "muted_color", Label: "Muted Color", Type: "color", Description: "Muted/disabled tone, light theme only"},
			{Key: "muted_color_dark", Label: "Muted Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = theme default"},
			{Key: "border_color", Label: "Border Color", Type: "color", Description: "Borders and dividers, light theme only"},
			{Key: "border_color_dark", Label: "Border Color (Dark)", Type: "color", Description: "Optional dark-mode override; empty = theme default"},
			{Key: "font_family", Label: "Font Family", Type: "select", Options: []string{"inter", "lora", "poppins", "roboto", "open-sans", "montserrat", "system-ui", "georgia", "monospace"}, Value: "inter", Description: "Body/UI font"},
			{Key: "font_family_serif", Label: "Serif Font Family", Type: "select", Options: []string{"inter", "lora", "poppins", "roboto", "open-sans", "montserrat", "system-ui", "georgia", "monospace"}, Value: "lora", Description: "Reading/prose font for long-form content"},
			{Key: "heading_font_family", Label: "Heading Font Family", Type: "select", Options: []string{"inherit", "inter", "lora", "poppins", "roboto", "open-sans", "montserrat", "system-ui", "georgia", "monospace"}, Value: "inherit", Description: "Font applied to h1–h6; inherit follows the body font"},
			{Key: "font_size", Label: "Font Size", Type: "select", Options: []string{"14", "15", "16", "17", "18"}, Value: "16", Description: "Base body font size in px"},
			{Key: "font_weight", Label: "Font Weight", Type: "select", Options: []string{"400", "500", "600"}, Value: "400", Description: "Base body font weight"},
			{Key: "heading_size", Label: "Heading Size", Type: "select", Options: []string{"compact", "default", "comfortable", "large", "xl"}, Value: "default", Description: "Heading size scale for h1–h6 (desktop)"},
			{Key: "heading_size_tablet", Label: "Heading Size (Tablet)", Type: "select", Options: []string{"inherit", "compact", "default", "comfortable", "large", "xl"}, Value: "inherit", Description: "Heading size scale at ≤1024px"},
			{Key: "heading_size_mobile", Label: "Heading Size (Mobile)", Type: "select", Options: []string{"inherit", "compact", "default", "comfortable", "large", "xl"}, Value: "inherit", Description: "Heading size scale at ≤640px"},
			{Key: "heading_weight", Label: "Heading Weight", Type: "select", Options: []string{"500", "600", "700", "800", "900"}, Value: "700", Description: "Heading font weight for h1–h6 (desktop)"},
			{Key: "heading_weight_tablet", Label: "Heading Weight (Tablet)", Type: "select", Options: []string{"inherit", "500", "600", "700", "800", "900"}, Value: "inherit", Description: "Heading weight at ≤1024px"},
			{Key: "heading_weight_mobile", Label: "Heading Weight (Mobile)", Type: "select", Options: []string{"inherit", "500", "600", "700", "800", "900"}, Value: "inherit", Description: "Heading weight at ≤640px"},
			{Key: "border_radius", Label: "Border Radius", Type: "select", Options: []string{"0", "4", "8", "12", "16"}, Value: "8", Description: "Corner radius in px (cards derive +4)"},
			{Key: "logo_url", Label: "Logo URL", Type: "media", Accept: "image/*"},
			{Key: "logo_url_dark", Label: "Logo URL (Dark)", Type: "media", Accept: "image/*", Description: "Optional dark-theme logo; falls back to Logo URL"},
			{Key: "favicon_url", Label: "Favicon URL", Type: "media", Accept: "image/*"},
			{Key: "favicon_url_dark", Label: "Favicon URL (Dark)", Type: "media", Accept: "image/*", Description: "Optional dark-theme favicon; falls back to Favicon URL"},
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
			// Legacy scalar superseded by feed.card_style (JSON {default, by_type}),
			// which the public GET /feed/card-style endpoint actually serves and
			// the Content Types page manages (feed category → card-style row).
			// Hidden keeps the key writable for compatibility without rendering a
			// dead control in this category.
			{Key: "feed_card_style", Label: "Feed Card Style", Type: "text", Description: "Default feed card style: `reddit`|`compact`|`grid`|`magazine`", Value: "reddit", Hidden: true},
		},
	},
	// Progressive Web App (§124) — public keys consumed by each app's dynamic
	// /manifest.json endpoint, service worker, and install UX.
	"pwa": {
		Key:   "pwa",
		Label: "Progressive Web App",
		Settings: []SettingDef{
			{Key: "name", Label: "App Name", Type: "text", Description: "Installed app name; empty falls back to Site Name"},
			{Key: "short_name", Label: "Short Name", Type: "text", Description: "Launcher short name; empty falls back to App Name"},
			{Key: "description", Label: "Description", Type: "textarea", Description: "Manifest description; empty falls back to Site Description"},
			{Key: "theme_color", Label: "Theme Color", Type: "color", Description: "Manifest theme_color + browser chrome tint; empty falls back to Primary Color"},
			{Key: "background_color", Label: "Background Color", Type: "color", Value: "#ffffff", Description: "Splash/launch screen background"},
			{Key: "display", Label: "Display Mode", Type: "select", Options: []string{"standalone", "minimal-ui", "browser", "fullscreen"}, Value: "standalone"},
			{Key: "start_url", Label: "Start URL", Type: "text", Value: "/", Description: "Main-app manifest start_url (docs falls back to /docs when '/')"},
			{Key: "icon_192", Label: "Icon 192px", Type: "media", Accept: "image/*", Description: "Empty uses the bundled /icon-192.png"},
			{Key: "icon_512", Label: "Icon 512px", Type: "media", Accept: "image/*", Description: "Empty uses the bundled /favicon.png"},
			{Key: "maskable_icon", Label: "Maskable Icon", Type: "media", Accept: "image/*", Description: "Safe-zone padded icon for adaptive launchers; empty uses /icon-maskable-512.png"},
			{Key: "shortcuts", Label: "App Shortcuts", Type: "link-list", Max: 4, Description: "Manifest shortcuts on the main app (label + url)"},
			{Key: "screenshots", Label: "Screenshots", Type: "json", Placeholder: `[{"src":"/screenshots/home.png","sizes":"1280x720","type":"image/png"}]`, Description: "JSON array for the richer install dialog"},
			{Key: "categories", Label: "Categories", Type: "text", Value: "social", Description: "Comma-separated manifest categories"},
			{Key: "offline_enabled", Label: "Offline Support", Type: "toggle", Value: true, Description: "Service-worker caching + offline fallback page"},
			{Key: "install_prompt_enabled", Label: "Install Prompt", Type: "toggle", Value: true, Description: "Show the install banner and Devices install row when the browser offers install"},
			{Key: "badge_enabled", Label: "App Icon Badge", Type: "toggle", Value: true, Description: "Mirror unread notifications onto the installed app icon"},
			{Key: "share_target_enabled", Label: "Share Target", Type: "toggle", Value: false, Description: "Accept OS-level shares into the /studio/create composer"},
		},
	},
	"users": {
		Key:   "users",
		Label: "Users",
		Settings: []SettingDef{
			{Key: "allow_social_login", Label: "Allow Social Login", Type: "toggle"},
			// require_profile_completion / max_avatar_size_mb removed (dead
			// controls): no gate consults the former and avatar uploads share
			// media.max_upload_mb.
			{Key: "allowed_avatar_types", Label: "Allowed Avatar Types", Type: "multiselect", Options: []string{"jpeg", "jpg", "png", "gif", "webp", "svg"}},
			{Key: "username_cooldown_days", Label: "Username Cooldown Days", Type: "number", Description: "Minimum days between username changes (§50)", Value: 30},
		},
	},
	"content": {
		Key:   "content",
		Label: "Content",
		Settings: []SettingDef{
			{Key: "posts_per_page", Label: "Posts Per Page", Type: "number"},
			// allow_anonymous_posts removed (dead control): content create is
			// behind RequireAuth + content.create — no guest-posting path
			// exists to gate. max_upload_size_mb removed (dead duplicate):
			// uploads share media.max_upload_mb under Media settings.
			{Key: "require_content_review", Label: "Require Content Review", Type: "toggle"},
			{Key: "allowed_upload_types", Label: "Allowed Upload Types", Type: "multiselect", Options: []string{"jpeg", "jpg", "png", "gif", "webp", "svg", "pdf", "mp4", "webm", "mov", "mp3", "ogg", "wav", "m4a"}},
			{Key: "link_preview.blacklist", Label: "Link Preview Blacklist", Type: "json", Placeholder: `["example.com", "spam.com"]`},
			{Key: "edit_lock_ttl_seconds", Label: "Edit Lock TTL (seconds)", Type: "number", Description: "Editor lock TTL in seconds"},
			{Key: "edit_window_minutes", Label: "Edit Window (minutes)", Type: "number", Description: "Content edit window (24hr default)"},
			{Key: "max_tags", Label: "Max Tags", Type: "number", Description: "Max tags per content item"},
			{Key: "min_length", Label: "Min Content Length", Type: "number", Description: "Minimum content body length"},
			// These two store to the canonical comments.*/forum.* keys the
			// comments and forum services read — without StorageKey the writes
			// would land on dead content.* duplicates that nothing reads.
			{Key: "comments.edit_window_minutes", Label: "Comment Edit Window (min)", Type: "number", StorageKey: "comments.edit_window_minutes"},
			{Key: "forum.edit_window_minutes", Label: "Forum Edit Window (min)", Type: "number", StorageKey: "forum.edit_window_minutes"},
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
			{Key: "sitemap.story_min_words", Label: "Sitemap: Min Story Words", Type: "number", Description: "Minimum word count for community stories (overrides fallback)."},
			{Key: "sitemap.list_min_words", Label: "Sitemap: Min List Words", Type: "number", Description: "Minimum word count for listicle content (overrides fallback)."},
			{Key: "sitemap.wiki_min_words", Label: "Sitemap: Min Wiki Words", Type: "number", Description: "Minimum word count across a wiki page's approved revision (wiki indexing floor)."},
			{Key: "sitemap.forum_min_words", Label: "Sitemap: Min Forum Words", Type: "number", Description: "Minimum word count for forum threads (overrides fallback)."},
			{Key: "sitemap.discussion_min_words", Label: "Sitemap: Min Discussion Words", Type: "number", Description: "Minimum word count for discussions (overrides fallback)."},
			{Key: "sitemap.guide_min_words", Label: "Sitemap: Min Guide Words", Type: "number", Description: "Minimum word count for guides (overrides fallback)."},
			{Key: "sitemap.comparison_min_words", Label: "Sitemap: Min Comparison Words", Type: "number", Description: "Minimum word count for comparison content (overrides fallback)."},
			{Key: "sitemap.blog_min_engagement", Label: "Sitemap: Min Blog Engagement", Type: "number", Description: "Minimum engagement (comments + shares + reactions) for blog posts."},
			{Key: "sitemap.forum_min_replies", Label: "Sitemap: Min Forum Replies", Type: "number", Description: "Minimum reply count for forum threads to be indexed."},
			{Key: "sitemap.question_min_answers", Label: "Sitemap: Min Question Answers", Type: "number", Description: "Minimum answer count for questions (solved threads). Set 0 to include unsolved."},
			{Key: "sitemap.discussion_min_comments", Label: "Sitemap: Min Discussion Comments", Type: "number", Description: "Minimum comment count for discussions to be indexed."},
			// Removed: enable_sitemap / enable_robots_txt / enable_json_ld —
			// sitemap & robots.txt are always served (robots.txt is generated
			// from the bot registry per ADR-043, not a system_settings toggle),
			// and JSON-LD is emitted by the SSR shell. These toggles had no
			// consumer and misled admins into thinking they could disable SEO
			// endpoints. Bot policy is managed in the Bot Management page.
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
			// ai_model / ai_max_tokens (provider config, env-only per Rule 3),
			// ai_content_moderation (use moderation.ai_auto_flag_threshold in
			// the Moderation category). None had a backend consumer.
			{Key: "monthly_budget_usd", Label: "Monthly Budget (USD)", Type: "number", Description: "Alert threshold for AI spend"},
			// japabot_enabled removed: reachability is owned by the
			// feature.japabot feature flag (Control Panel → Extensions), same
			// as agent.enabled — the system_settings row is only a deprecated
			// nil-checker fallback, not an editable control.
			{Key: "content_assist_enabled", Label: "Content Assist Enabled", Type: "toggle"},
			{Key: "humanize.enabled", Label: "Humanize Enabled", Type: "toggle", Description: "Global toggle for AI text humanization (natural-language style injection)", Value: false},
			{Key: "humanize.level", Label: "Humanize Level", Type: "text", Description: "Humanization intensity: low | medium | high", Value: "medium"},
		},
	},
	// NOTE: The "agent" category was removed. Every agent.* key is classified
	// Panel "control" in internal/control/settings_catalog.go (§103, ADR-111)
	// and is managed under Control Panel → System Settings, so listing them
	// here duplicated the same system_settings rows in both UIs. agent.enabled
	// is additionally a deprecated reachability toggle owned by the
	// feature.ai_agent feature flag (Control Panel → Extensions), not an
	// editable system setting.
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
			{Key: "report_reasons", Label: "Report Reasons", Type: "json", Description: "Available report reason codes for the report dialog (Rule 16 compliance)", Value: `["spam","harassment","hate_speech","misinformation","illegal_content","self_harm","copyright","other"]`},
		},
	},
	"wallet": {
		Key:   "wallet",
		Label: "Wallet",
		Settings: []SettingDef{
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
			{Key: "cache_ttl_secs", Label: "Cache Ttl Secs", Type: "number", Description: "TTL for the published-page Valkey cache (`jc:page:{the id}:{slug}`)", Value: 300},
		},
	},

	"site": {
		Key:   "site",
		Label: "Site",
		Settings: []SettingDef{
			{Key: "custom_css", Label: "Custom CSS", Type: "textarea", Description: "Raw CSS injected inside `<style id=\"jc-custom-css\">` in `<head>`. Max 50 KB."},
			{Key: "custom_js", Label: "Custom JS", Type: "textarea", Description: "Raw JS injected as `<script id=\"jc-custom-js\">` before `</body>`. Max 100 KB. Renders ONLY when custom_js_approved is true."},
			{Key: "custom_js_approved", Label: "Approve Custom JS", Type: "toggle", Description: "Two-person approval for Custom JS: a super_admin must set this true before the JS renders. Fail-closed default.", Value: false},
		},
	},

	"docs": {
		Key:   "docs",
		Label: "Documentation",
		Settings: []SettingDef{
			// Managed on the dedicated /docs/categories admin page — hidden
			// from the generic settings UI but still writable via
			// PUT /admin/settings/docs.
			{Key: "categories", Label: "Categories & Subcategories", Type: "docs-categories", Hidden: true, Description: "Managed category catalog for the docs portal: display order, card descriptions/icons, and canonical subcategory names. Categories pages actually use stay free-text; this catalog curates presentation and suggestions. Empty derives categories from published pages."},
			{Key: "help_links", Label: "Need Help Links", Type: "link-list", Max: 3, Description: "Links shown in the docs portal 'Need help' card. Empty uses the default Help Center / Support links."},
			{Key: "contribute_links", Label: "Contribute to Docs Links", Type: "link-list", Max: 3, Description: "Links shown in the docs portal 'Contribute to Docs' card. Removing all links hides the card."},
			{Key: "dev_resources", Label: "Developer Resources", Type: "link-cards", Max: 8, Description: "Resource cards shown at the bottom of the docs portal landing page. Empty uses the built-in defaults."},
			{Key: "pinned_pages", Label: "Get Started Pinned Pages", Type: "pinned-pages", Description: "Pinned pages powering the sidebar GET STARTED section and the 'New to " + brand.Get() + "?' card. Unresolvable pins are skipped."},
			{Key: "versions", Label: "Version Switcher Entries", Type: "docs-versions", Description: "Version entries powering the docs header version dropdown. Empty hides it."},
			{Key: "changelog_category", Label: "Changelog Categories", Type: "text", Description: "Comma-separated doc page categories surfaced in the docs portal 'What's new' section.", Value: "Changelog"},
			{Key: "changelog_new_days", Label: "NEW Badge Days", Type: "number", Description: "Changelog entries newer than this many days get the NEW badge.", Value: 14},
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
	"media", "seo", "ai",
	"spaces", "moderation", "wallet", "payment", "api", "ads",
	"notifications", "tickets", "audit", "i18n", "gdpr",
	"search", "affiliate",
	"organizations",
	"page",
	"site",
	"docs",
	"trending",
	"pwa",
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

// ListSettings returns all platform settings, organized by category.
// The response shape matches the frontend's expected format: an array of
// categories, each containing an array of settings with their current values.
func (h *Handlers) ListSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	ctx := r.Context()

	// Fetch all settings from the database.
	rows, err := h.DB.Reader.Query(ctx, `SELECT key, value FROM system_settings `)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load settings")
		return
	}
	defer rows.Close()

	// Currency-valued fields take their options from the managed currencies
	// table — the static option lists predate operator-managed currencies
	// (§58, Rule 16).
	currencyCodes := h.currencySettingCodes(ctx)

	// Build a map of stored values keyed by the full dot-notation key
	// (e.g. "general.site_name"). Keying by the full key — not the suffix
	// after the first dot — keeps cross-namespace lookups unambiguous: a
	// setting whose StorageKey lives in another namespace resolves to its
	// own row, and two keys that merely share a suffix cannot collide.
	stored := make(map[string]any)

	for rows.Next() {
		var fullKey string

		var value any
		if err := rows.Scan(&fullKey, &value); err != nil {
			continue
		}
		stored[fullKey] = value
	}

	// Assemble the response by iterating over defined categories in order,
	// merging stored values with the schema defaults.
	result := make([]SettingCategory, 0, len(settingCategoryOrder))

	for _, catKey := range settingCategoryOrder {
		def, ok := defaultSettings[catKey]
		if !ok {
			continue
		}

		settings := make([]SettingDef, 0, len(def.Settings))

		for _, s := range def.Settings {
			// Hidden settings stay writable via UpdateSettings but are managed
			// on a dedicated admin surface — omit them from this response.
			if s.Hidden {
				continue
			}

			if len(currencyCodes) > 0 && isCurrencySetting(catKey, s.Key) {
				s.Type = "select"
				s.Options = currencyCodes
			}

			// Capture the catalog default before merging the stored override —
			// keys with no default at all stay "unset" (their default IS unset).
			s.DefaultValue = s.Value

			if val, exists := stored[settingStorageKey(catKey, s)]; exists {
				s.Overridden = true
				if s.Masked {
					s.Value = "***"
				} else {
					s.Value = val
				}
			}

			settings = append(settings, s)
		}

		result = append(result, SettingCategory{
			Key:      def.Key,
			Label:    def.Label,
			Settings: settings,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// isCurrencySetting reports whether a category/key pair holds an ISO 4217
// currency code drawn from the managed currencies table (§58). Keys here are
// the short form (after the first dot) within their category.
func isCurrencySetting(category, key string) bool {
	switch category {
	case "payment":
		return key == "currency" // payment.currency
	case "wallet":
		return key == "default_currency" // wallet.default_currency
	}

	return false
}

// currencySettingCodes returns the active currency codes for select-type
// currency settings. Empty slice when the table can't be read — callers keep
// the static options/type declared in defaultSettings.
func (h *Handlers) currencySettingCodes(ctx context.Context) []string {
	currencies, err := gen.New(h.DB.Reader).ListActiveCurrencies(ctx)
	if err != nil || len(currencies) == 0 {
		return nil
	}

	codes := make([]string, 0, len(currencies))
	for _, c := range currencies {
		codes = append(codes, c.Code)
	}

	return codes
}

// ---------------------------------------------------------------------------
// UpdateSettings — PUT /admin/settings/{category}
// ---------------------------------------------------------------------------

// UpdateSettings saves settings for a single category. The request body
// contains a flat map of key-value pairs (e.g. {"site_name": "New Name"}).
// Each pair is upserted into the system_settings table using the full
// dot-notation key (category.setting_key).
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUser(w, r)
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

	// Build a key→definition map so unknown keys are rejected and each
	// accepted key resolves to its storage row (StorageKey when the canonical
	// key lives outside this category's namespace).
	defsByKey := make(map[string]SettingDef, len(def.Settings))
	for _, s := range def.Settings {
		defsByKey[s.Key] = s
	}

	ctx := r.Context()

	// Validate every key up front and serialize values to JSON before delegating
	// the transactional upsert to the settings service (TX-001: tx belongs to service).
	// is_public is resolved from the control settings catalog so appearance keys
	// are exposed via GET /site/settings/public without a separate control-panel call.
	pending := make(map[string][]byte, len(updates))
	isPublic := make(map[string]bool, len(updates))

	for key, value := range updates {
		settingDef, known := defsByKey[key]
		if !known {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_key", "Unknown setting key: "+key)
			return
		}

		valueJSON, err := json.Marshal(value)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", "Cannot serialize value for key: "+key)
			return
		}

		// Section 57: custom CSS/JS have size caps (50 KB / 100 KB raw value).
		fullKey := settingStorageKey(category, settingDef)

		if category == "site" {
			// Two-person approval for custom JS: only a super_admin may set
			// custom_js_approved, and the JS renders only while it is true.
			// A single compromised admin account (or a stored-XSS payload
			// smuggled through another surface) cannot ship site-wide
			// script execution alone.
			if key == "custom_js_approved" {
				want := false
				if b, ok := value.(bool); ok {
					want = b
				}

				if want && (h.RBAC == nil || !h.RBAC.IsSuperAdmin(ctx, actor.UserID)) {
					errpkg.WriteError(w, http.StatusForbidden, "super_admin_required", "Approving custom JS requires super_admin")
					return
				}
			}

			var raw string
			if key != "custom_js_approved" {
				if err := json.Unmarshal(valueJSON, &raw); err != nil {
					errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", key+" must be a string")
					return
				}
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

	if err := h.Settings.UpdateSettingsBatchWithPublic(ctx, pending, isPublic); err != nil {
		slog.Error("admin: failed to save settings", "category", category, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to save settings")

		return
	}

	// Invalidate link preview cache when the blacklist is updated, since
	// already-cached previews for now-blocklisted domains would survive their 24h TTL.
	if category == "content" && h.Valkey != nil {
		invalidateLinkPreviewCache(r.Context(), h.Valkey)
	}

	// Audit-trail every settings write — custom_css/custom_js reach every
	// visitor's browser, so who changed what and when must be traceable.
	// Best-effort (Rule 8): a failed audit write never blocks the save.
	if h.DB != nil {
		keys := make([]string, 0, len(pending))
		for k := range pending {
			keys = append(keys, k)
		}

		meta, _ := json.Marshal(map[string]any{"category": category, "keys": keys})
		ip := clientip.FromRequest(r)

		var ipArg any

		if ip != "" {
			ipArg = ip
		}

		if _, err := h.DB.Writer.Exec(ctx, `
			INSERT INTO audit_logs (actor_user_id, action_type, target_type, metadata, ip_address)
			VALUES ($1, 'admin.settings.update', 'system_setting', $2::jsonb, $3::inet)
		`, actor.UserID, meta, ipArg); err != nil {
			slog.Warn("admin: failed to write settings audit log", "category", category, "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":   "saved",
			"category": category,
		},
	})
}

// ---------------------------------------------------------------------------
// ResetSettings — POST /admin/settings/{category}/reset
// ---------------------------------------------------------------------------

// ResetSettings removes every stored override for a category, restoring the
// catalog defaults declared in defaultSettings. ListSettings falls back to the
// catalog only when no system_settings row exists, so deleting the rows —
// rather than writing defaults back — is what actually resets the category
// (several appearance keys intentionally have no catalog value; "unset" IS
// their default).
func (h *Handlers) ResetSettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUser(w, r)
	if !ok {
		return
	}

	category := chi.URLParam(r, "category")
	if category == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}

	def, ok := defaultSettings[category]
	if !ok {
		errpkg.WriteError(w, http.StatusNotFound, "invalid_category", "Unknown settings category: "+category)
		return
	}

	// Every catalog key in the category is deleted — including Hidden keys,
	// which are still category-scoped customizations even though they render
	// on a dedicated surface. Settings with a StorageKey outside the
	// category namespace drop that row plus the legacy category-scoped
	// alias left by writes that predated the override.
	keys := make([]string, 0, len(def.Settings))
	for _, s := range def.Settings {
		keys = append(keys, settingStorageKey(category, s))
		if legacy := category + "." + s.Key; s.StorageKey != "" && legacy != s.StorageKey {
			keys = append(keys, legacy)
		}
	}

	ctx := r.Context()

	if err := h.Settings.DeleteSettingsBatch(ctx, keys); err != nil {
		slog.Error("admin: failed to reset settings", "category", category, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to reset settings")
		return
	}

	// Same post-write hook as UpdateSettings — a content reset can clear the
	// link-preview blacklist.
	if category == "content" && h.Valkey != nil {
		invalidateLinkPreviewCache(r.Context(), h.Valkey)
	}

	// Audit-trail the reset — same traceability bar as a settings write.
	// Best-effort (Rule 8): a failed audit write never blocks the reset.
	if h.DB != nil {
		meta, _ := json.Marshal(map[string]any{"category": category, "keys": keys})
		ip := clientip.FromRequest(r)

		var ipArg any

		if ip != "" {
			ipArg = ip
		}

		if _, err := h.DB.Writer.Exec(ctx, `
			INSERT INTO audit_logs (actor_user_id, action_type, target_type, metadata, ip_address)
			VALUES ($1, 'admin.settings.reset', 'system_setting', $2::jsonb, $3::inet)
		`, actor.UserID, meta, ipArg); err != nil {
			slog.Warn("admin: failed to write settings reset audit log", "category", category, "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":   "reset",
			"category": category,
		},
	})
}

// ---------------------------------------------------------------------------
// ResetSetting — POST /admin/settings/{category}/reset/{key}
// ---------------------------------------------------------------------------

// ResetSetting removes the stored override for a single setting, restoring
// that key's catalog default. Same delete-not-write semantics as
// ResetSettings, scoped to one key — the response echoes the catalog default
// so the caller can update its local state without a refetch.
func (h *Handlers) ResetSetting(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUser(w, r)
	if !ok {
		return
	}

	category := chi.URLParam(r, "category")
	if category == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Setting key is required")
		return
	}

	def, ok := defaultSettings[category]
	if !ok {
		errpkg.WriteError(w, http.StatusNotFound, "invalid_category", "Unknown settings category: "+category)
		return
	}

	// The key must exist in the catalog — including Hidden keys, which are
	// still category-scoped customizations even though they render on a
	// dedicated surface (same rule as the category-wide reset).
	var defValue any
	var foundDef SettingDef
	found := false

	for _, s := range def.Settings {
		if s.Key == key {
			defValue = s.Value
			foundDef = s
			found = true
			break
		}
	}

	if !found {
		errpkg.WriteError(w, http.StatusNotFound, "invalid_key", "Unknown setting key: "+key)
		return
	}

	// Drop the canonical storage row plus the legacy category-scoped alias —
	// writes that predated a StorageKey override left rows under
	// category.key that must not resurrect the old effective value.
	fullKey := settingStorageKey(category, foundDef)
	deleteKeys := []string{fullKey}
	if legacy := category + "." + key; legacy != fullKey {
		deleteKeys = append(deleteKeys, legacy)
	}
	ctx := r.Context()

	if err := h.Settings.DeleteSettingsBatch(ctx, deleteKeys); err != nil {
		slog.Error("admin: failed to reset setting", "category", category, "key", key, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to reset setting")
		return
	}

	// Same post-write hook as UpdateSettings — resetting a content key can
	// clear the link-preview blacklist.
	if category == "content" && h.Valkey != nil {
		invalidateLinkPreviewCache(r.Context(), h.Valkey)
	}

	// Audit-trail the reset — same traceability bar as a settings write.
	// Best-effort (Rule 8): a failed audit write never blocks the reset.
	if h.DB != nil {
		meta, _ := json.Marshal(map[string]any{"category": category, "keys": []string{fullKey}})
		ip := clientip.FromRequest(r)

		var ipArg any

		if ip != "" {
			ipArg = ip
		}

		if _, err := h.DB.Writer.Exec(ctx, `
			INSERT INTO audit_logs (actor_user_id, action_type, target_type, metadata, ip_address)
			VALUES ($1, 'admin.settings.reset', 'system_setting', $2::jsonb, $3::inet)
		`, actor.UserID, meta, ipArg); err != nil {
			slog.Warn("admin: failed to write setting reset audit log", "category", category, "key", key, "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status":   "reset",
			"category": category,
			"key":      key,
			"value":    defValue,
		},
	})
}
