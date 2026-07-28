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
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/valkey-io/valkey-go/valkeycompat"
)


// SettingDef defines the metadata for a single setting within a category.
type SettingDef struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // "text", "number", "toggle", "select", "multiselect", "color", "file", "textarea"
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
	DB       *db.DB
	Queries  *gen.Queries
	RBAC     *rbac.RBAC
	Valkey   valkeycompat.Cmdable
	Settings *config.Settings
	GDPR     GDPRService
}

// GDPRService is the interface the admin hard-delete handler needs from the GDPR service.
type GDPRService interface {
	HardDeleteUser(ctx context.Context, userID, tenantID pgtype.UUID) error
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

// SetGDPR injects the GDPR service for admin hard-delete operations.
func (h *Handlers) SetGDPR(g GDPRService) {
	h.GDPR = g
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
			{Key: "card_style", Label: "Card Style", Type: "select", Options: []string{"reddit", "compact", "grid", "magazine"}},
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
			{Key: "logo_url", Label: "Logo URL", Type: "file", Accept: "image/*"},
			{Key: "favicon_url", Label: "Favicon URL", Type: "file", Accept: "image/*"},
			{
				Key:   "auth_brand_slides",
				Label: "Auth Brand Slides",
				Type:  "brand-slides",
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
			{Key: "feed_card_style", Label: "Feed Card Style", Type: "select", Options: []string{"reddit", "compact", "grid", "magazine"}, Value: "reddit"},
			{Key: "default_editor", Label: "Default Editor Mode", Type: "select", Options: []string{"block", "wysiwyg", "markdown"}, Value: "block", Description: "Default editor mode for the post composer"},
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
		},
	},
	"payment": {
		Key:   "payment",
		Label: "Payment",
		Settings: []SettingDef{
			{Key: "currency", Label: "Currency", Type: "select", Options: []string{"NGN", "USD", "GBP", "EUR"}},
			// Removed: paystack_public_key / paystack_secret_key /
			//   stripe_public_key / stripe_secret_key — provider credentials are
			//   secrets that belong in JC_* env vars (Rule 3), not system_settings.
			//   No backend consumer read them; the payment service uses env config.
		},
	},
	"media": {
		Key:   "media",
		Label: "Media",
		Settings: []SettingDef{
			// Removed: imgproxy_enabled / imgproxy_key / imgproxy_salt — imgproxy
			//   credentials are env-only (JC_IMGPROXY_*); secrets must not live in
			//   system_settings (Rule 3).
			// Removed: max_image_width / max_image_height — no backend consumer.
			{Key: "allowed_video_types", Label: "Allowed Video Types", Type: "multiselect", Options: []string{"mp4", "webm", "mov", "avi", "mkv"}},
			{Key: "max_upload_mb", Label: "Max Upload Size (MB)", Type: "number"},
			{Key: "storage.provider", Label: "Storage Provider", Type: "select", Options: []string{"seaweedfs", "s3", "r2", "b2"}},
			{Key: "storage.endpoint", Label: "Storage Endpoint", Type: "text", Placeholder: "https://s3.amazonaws.com"},
			{Key: "storage.bucket", Label: "Storage Bucket", Type: "text", Placeholder: "japaconnect"},
			{Key: "storage.region", Label: "Storage Region", Type: "text", Placeholder: "us-east-1"},
			{Key: "storage.access_key", Label: "Storage Access Key", Type: "text", Masked: true},
			{Key: "storage.secret_key", Label: "Storage Secret Key", Type: "text", Masked: true},
			{Key: "cdn.base_url", Label: "CDN Base URL", Type: "text", Placeholder: "https://cdn.example.com"},
		},
	},
	"seo": {
		Key:   "seo",
		Label: "SEO",
		Settings: []SettingDef{
			{Key: "meta_title_template", Label: "Meta Title Template", Type: "text"},
			{Key: "meta_description_template", Label: "Meta Description Template", Type: "textarea"},
			{Key: "og_image", Label: "Default OG Image", Type: "file", Accept: "image/*"},
			{Key: "twitter_handle", Label: "Twitter/X Handle", Type: "text", Placeholder: "@username"},
			{Key: "json_ld_type", Label: "Default JSON-LD Type", Type: "select", Options: []string{"Organization", "WebSite", "LocalBusiness"}},
			{Key: "google_analytics_id", Label: "Google Analytics ID", Type: "text", Placeholder: "G-XXXXXXXXXX"},
			{Key: "google_tag_manager_id", Label: "Google Tag Manager ID", Type: "text", Placeholder: "GTM-XXXXXXX"},
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
		},
	},
	"spaces": {
		Key:   "spaces",
		Label: "Spaces",
		Settings: []SettingDef{
			{Key: "bubble_depth", Label: "Bubble Depth", Type: "number", Description: "How many levels posts bubble up"},
			{Key: "max_pinned_posts", Label: "Max Pinned Posts", Type: "number", Description: "Max pinned posts per space"},
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
		},
	},
	"wallet": {
		Key:   "wallet",
		Label: "Wallet",
		Settings: []SettingDef{
			{Key: "min_convert_points", Label: "Min Convert Points", Type: "number"},
			{Key: "points_to_credit_rate", Label: "Points to Credit Rate", Type: "number", Description: "1 point = N credits"},
			{Key: "max_daily_earn_points", Label: "Max Daily Earn Points", Type: "number"},
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
			{Key: "deletion_grace_days", Label: "Deletion Grace Period (days)", Type: "number"},
			{Key: "export_expiry_days", Label: "Export Expiry (days)", Type: "number"},
			{Key: "hard_delete_enabled", Label: "Enable Hard Delete", Type: "toggle"},
			{Key: "retention_days", Label: "Retention Days", Type: "number", Description: "0 = immediate"},
			{Key: "deleted_user_placeholder_uuid", Label: "Deleted User Placeholder UUID", Type: "text"},
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
		},
	},
}

// settingCategoryOrder controls the response order sent to the frontend.
// Removed categories (features, email, integrations, advanced) are intentionally
// absent — see the NOTE comments above each removal in defaultSettings.
var settingCategoryOrder = []string{
	"general", "auth", "feed", "security",
	"appearance", "users", "content", "payment",
	"media", "seo", "ai",
	"spaces", "moderation", "wallet", "api", "ads",
	"notifications", "tickets", "audit", "i18n", "gdpr",
	"search", "affiliate",
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

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
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to load settings")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
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
		writeError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}

	// Validate that the category exists in our schema.
	def, ok := defaultSettings[category]
	if !ok {
		writeError(w, http.StatusNotFound, "invalid_category", "Unknown settings category: "+category)
		return
	}

	// Parse the request body as key-value pairs.
	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "empty_body", "No settings provided")
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
			writeError(w, http.StatusBadRequest, "invalid_key", "Unknown setting key: "+key)
			return
		}
		valueJSON, err := json.Marshal(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_value", "Cannot serialize value for key: "+key)
			return
		}
		fullKey := category + "." + key
		pending[fullKey] = valueJSON
		isPublic[fullKey] = control.IsPublicSetting(fullKey)
	}

	if err := h.Settings.UpdateSettingsBatchWithPublic(ctx, tenantID, pending, isPublic); err != nil {
		slog.Error("admin: failed to save settings", "category", category, "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to save settings")
		return
	}

	// Invalidate link preview cache when the blacklist is updated, since
	// already-cached previews for now-blocklisted domains would survive their 24h TTL.
	if category == "content" && h.Valkey != nil {
		invalidateLinkPreviewCache(r.Context(), h.Valkey)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":   "saved",
			"category": category,
		},
	})
}
