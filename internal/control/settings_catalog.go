package control

// settingMeta holds the authoritative metadata for one system setting.
// The Key is the full dot-notation key (e.g. "auth.password.min_length").
type settingMeta struct {
	Key          string  `json:"-"`
	Type         string  `json:"type"`         // "int", "float", "bool", "string", "csv", "json"
	DefaultValue string  `json:"default_value"`
	Description  string  `json:"description"`
	Min          float64 `json:"min,omitempty"`
	Max          float64 `json:"max,omitempty"`
	Sensitive    bool    `json:"sensitive"`
	// Options is a JSON-encoded array of {value, label} objects for select/multi-select.
	Options string `json:"options,omitempty"`
}

// settingsCatalog maps every known dot-notation key to its metadata.
// This is the same catalog documented in .jeos/conventions/system-settings-catalog.md.
// When a setting is not in the catalog, it falls back to type "string" with no metadata.
var settingsCatalog = map[string]settingMeta{
	// Authentication
	"auth.password.min_length":               {Type: "int", DefaultValue: "8", Description: "Minimum password length."},
	"auth.password.max_length":               {Type: "int", DefaultValue: "128", Description: "Maximum password length."},
	"auth.password.require_uppercase":        {Type: "bool", DefaultValue: "false", Description: "Enforce uppercase requirement."},
	"auth.password.require_number":           {Type: "bool", DefaultValue: "true", Description: "Enforce numeric digit requirement."},
	"auth.password.require_symbol":           {Type: "bool", DefaultValue: "false", Description: "Enforce symbol requirement."},
	"auth.password.check_breach":             {Type: "bool", DefaultValue: "true", Description: "Check password against HIBP breach database before accepting."},
	"auth.password.history_count":            {Type: "int", DefaultValue: "5", Description: "Number of recent password hashes to check against reuse."},
	"auth.session.refresh_days":              {Type: "int", DefaultValue: "30", Description: "Refresh token TTL in days."},
	"auth.session.access_token_ttl_minutes":  {Type: "int", DefaultValue: "15", Description: "Access token TTL in minutes."},
	"auth.totp.required_for_admin":           {Type: "bool", DefaultValue: "false", Description: "Force 2FA for admin roles."},
	"auth.email.verify_required":             {Type: "bool", DefaultValue: "true", Description: "Require email verification before posting."},
	// Feed & Content
	"feed.fanout_threshold":            {Type: "int", DefaultValue: "5000", Description: "Max space members for push fan-out."},
	"feed.velocity.gravity":            {Type: "float", DefaultValue: "1.8", Description: "HN gravity constant for Rising sort."},
	"feed.max_followed_spaces_in_feed": {Type: "int", DefaultValue: "50", Description: "Cap on spaces contributing to feed."},
	"feed.card_style":                  {Type: "json", DefaultValue: `{"default":"reddit","by_type":{}}`, Description: "{default, by_type} map; each style ∈ reddit|compact|grid|magazine — active feed card style, resolved per content type (tolerates a legacy scalar)."},
	"content.edit_window_minutes":      {Type: "int", DefaultValue: "1440", Description: "Content edit window (24hr default)."},
	"content.link_preview.blacklist":   {Type: "json", DefaultValue: "[]", Description: "JSON array of blocked domains for link previews."},
	"content.min_length":               {Type: "int", DefaultValue: "10", Description: "Minimum content body length."},
	"content.max_tags":                 {Type: "int", DefaultValue: "5", Description: "Max tags per content item."},
	"content.edit_lock_ttl_seconds":    {Type: "int", DefaultValue: "120", Description: "Editor lock TTL in seconds."},
	"comments.edit_window_minutes":     {Type: "int", DefaultValue: "60", Description: "Comment edit window."},
	"forum.edit_window_minutes":        {Type: "int", DefaultValue: "30", Description: "Editable window (minutes) after posting a forum thread/reply."},
	// Spaces
	"spaces.bubble_depth":       {Type: "int", DefaultValue: "2", Description: "How many levels posts bubble up the space hierarchy."},
	"spaces.max_pinned_posts":   {Type: "int", DefaultValue: "5", Description: "Max pinned posts per space."},
	// Moderation
	"moderation.auto_hide_report_count":      {Type: "int", DefaultValue: "5", Description: "Auto-hide content after N reports."},
	"moderation.ai_auto_flag_threshold":      {Type: "float", DefaultValue: "0.85", Description: "AI classifier confidence for auto-flag."},
	"moderation.max_links_per_post":          {Type: "int", DefaultValue: "3", Description: "Max links per post (spam detection)."},
	"moderation.new_account_threshold_days":  {Type: "int", DefaultValue: "7", Description: "Days before account is no longer 'new'."},
	// Security
	"security.max_login_attempts": {Type: "int", DefaultValue: "5", Description: "Max failed login attempts before lockout."},
	"security.fail_window_minutes": {Type: "int", DefaultValue: "15", Description: "Lockout window in minutes."},
	// Wallet
	"wallet.min_convert_points":    {Type: "int", DefaultValue: "100", Description: "Minimum points for conversion."},
	"wallet.points_to_credit_rate": {Type: "float", DefaultValue: "0.01", Description: "1 point = 0.01 credits by default."},
	"wallet.max_daily_earn_points": {Type: "int", DefaultValue: "500", Description: "Cap on points earned per user per day."},
	// API Rate Limits
	"api.rate_limit.basic":    {Type: "int", DefaultValue: "100", Description: "Requests per minute — basic tier."},
	"api.rate_limit.standard": {Type: "int", DefaultValue: "1000", Description: "Requests per minute — standard tier."},
	"api.rate_limit.premium":  {Type: "int", DefaultValue: "10000", Description: "Requests per minute — premium tier."},
	"api.keys.max_per_user":   {Type: "int", DefaultValue: "5", Description: "Max API keys per user."},
	"api.idempotency_ttl_hours": {Type: "int", DefaultValue: "24", Description: "Idempotency key TTL in hours."},
	// CORS
	"cors.allowed_origins": {Type: "csv", DefaultValue: "*", Description: "Comma-separated list of allowed CORS origins."},
	// Ads
	"ads.frequency_cap_per_day": {Type: "int", DefaultValue: "3", Description: "Max ad impressions per user per day (global default)."},
	"ads.min_campaign_budget":   {Type: "float", DefaultValue: "10", Description: "Minimum campaign budget in credits."},
	// AI
	"ai.monthly_budget_usd":        {Type: "float", DefaultValue: "100", Description: "Alert threshold for AI spend (not hard cap)."},
	"ai.japabot_enabled":           {Type: "bool", DefaultValue: "true", Description: "Global JapaBot toggle."},
	"ai.content_assist_enabled":    {Type: "bool", DefaultValue: "true", Description: "In-editor AI assist toggle."},
	// Media
	"media.storage.provider":    {Type: "string", DefaultValue: "seaweedfs", Description: "seaweedfs | s3 | r2 | b2 — Storage backend provider."},
	"media.storage.endpoint":    {Type: "string", DefaultValue: "", Description: "S3-compatible endpoint URL."},
	"media.storage.bucket":      {Type: "string", DefaultValue: "japaconnect", Description: "Default bucket name."},
	"media.storage.region":      {Type: "string", DefaultValue: "us-east-1", Description: "S3 region (SeaweedFS ignores this)."},
	"media.storage.access_key":  {Type: "string", DefaultValue: "", Description: "S3 access key.", Sensitive: true},
	"media.storage.secret_key":  {Type: "string", DefaultValue: "", Description: "S3 secret key.", Sensitive: true},
	"media.cdn.base_url":        {Type: "string", DefaultValue: "", Description: "CDN base URL (empty = use imgproxy directly)."},
	"media.max_upload_mb":       {Type: "int", DefaultValue: "50", Description: "Max file size in MB."},
	"media.allowed_video_types": {Type: "csv", DefaultValue: "mp4,webm,mov", Description: "Allowed video MIME extensions."},
	// Notifications
	"notifications.max_delivery_attempts": {Type: "int", DefaultValue: "5", Description: "Max retry attempts for notification delivery."},
	"notifications.batch_window_minutes":  {Type: "int", DefaultValue: "5", Description: "Notification dedup batch window."},
	// Tickets
	"tickets.default_sla_hours":         {Type: "int", DefaultValue: "48", Description: "Default SLA response time."},
	"tickets.auto_close_resolved_days":  {Type: "int", DefaultValue: "7", Description: "Auto-close resolved tickets after N days."},
	// Audit
	"audit.log_retention_days": {Type: "int", DefaultValue: "365", Description: "Days to retain audit_logs before pruning."},
	// i18n
	"i18n.default_language":  {Type: "string", DefaultValue: "en", Description: "Platform default language."},
	"i18n.enabled_languages": {Type: "csv", DefaultValue: "en,fr", Description: "Comma-separated active language codes."},
	// GDPR
	"gdpr.deletion_grace_days":            {Type: "int", DefaultValue: "30", Description: "Days before hard deletion after request."},
	"gdpr.export_expiry_days":             {Type: "int", DefaultValue: "7", Description: "Days before export download link expires."},
	"gdpr.hard_delete_enabled":            {Type: "bool", DefaultValue: "false", Description: "Enable physical user erasure after anonymization (ADR-057)."},
	"gdpr.retention_days":                 {Type: "int", DefaultValue: "0", Description: "Days after anonymization before hard delete (0 = immediate, ADR-057)."},
	"gdpr.deleted_user_placeholder_uuid":  {Type: "string", DefaultValue: "", Description: "UUID assigned to orphaned content authorship after hard delete."},
	// Search
	"search.autocomplete_max": {Type: "int", DefaultValue: "8", Description: "Max autocomplete suggestions returned."},
	// Pricing Plans (SETTING-001 / Rule 3 — prices must be configurable).
	"pricing.plan.pro.monthly_amount":     {Type: "int", DefaultValue: "500000", Description: "Pro plan monthly price in kobo (NGN)."},
	"pricing.plan.pro.yearly_amount":      {Type: "int", DefaultValue: "5000000", Description: "Pro plan yearly price in kobo (NGN)."},
	"pricing.plan.business.monthly_amount": {Type: "int", DefaultValue: "1500000", Description: "Business plan monthly price in kobo (NGN)."},
	"pricing.plan.business.yearly_amount":  {Type: "int", DefaultValue: "15000000", Description: "Business plan yearly price in kobo (NGN)."},
	"pricing.currency":                    {Type: "string", DefaultValue: "NGN", Description: "Currency code for plan prices.", Options: `[{"value":"NGN","label":"Nigerian Naira (NGN)"},{"value":"USD","label":"US Dollar (USD)"},{"value":"GBP","label":"British Pound (GBP)"},{"value":"EUR","label":"Euro (EUR)"}]`},
	// Affiliate
	"affiliate.min_account_days":        {Type: "int", DefaultValue: "30", Description: "Minimum account age (days) to enroll in affiliate program."},
	"affiliate.attribution_window_days": {Type: "int", DefaultValue: "30", Description: "Click attribution window in days."},
	"affiliate.commission.pro_monthly":  {Type: "float", DefaultValue: "15", Description: "Commission % on Pro monthly subscription purchase."},
	"affiliate.commission.pro_annual":   {Type: "float", DefaultValue: "20", Description: "Commission % on Pro annual subscription purchase."},
	"affiliate.commission.course":       {Type: "float", DefaultValue: "10", Description: "Commission % on course access purchase."},
	"affiliate.max_links_per_user":      {Type: "int", DefaultValue: "10", Description: "Max active affiliate links per enrolled user."},
	// AUDIT-BE-SETTING-001/002/003: affiliate enhancement settings (Section 55).
	"affiliate.author_commission_default": {Type: "float", DefaultValue: "25", Description: "Default % of external affiliate commission shared with content author (when provider has no override)."},
	"affiliate.unregistered_link_policy":  {Type: "string", DefaultValue: "flag", Description: "How to handle unregistered affiliate links: flag (log only), block (prevent publish), strip (remove link).", Options: `[{"value":"flag","label":"Flag (log only)"},{"value":"block","label":"Block (prevent publish)"},{"value":"strip","label":"Strip (remove link)"}]`},
	"affiliate.scan_on_publish":           {Type: "bool", DefaultValue: "true", Description: "Whether to scan content for unregistered affiliate links on publish."},
	// AUDIT-BE-SETTING-004: Japa University creator economy settings (ADR-063, Section 36).
	"university.subdomain":                {Type: "string", DefaultValue: "university", Description: "Host label for the Japa University shell."},
	"university.custom_domain":            {Type: "string", DefaultValue: "", Description: "Optional custom domain (e.g. japauni.com); empty = use subdomain."},
	"university.creator_percentage_default": {Type: "int", DefaultValue: "75", Description: "Default creator revenue share % (platform takes the remainder)."},
	"university.platform_percentage_default": {Type: "int", DefaultValue: "25", Description: "Default platform revenue share %."},
	"university.payout_schedule":          {Type: "string", DefaultValue: "monthly", Description: "Payout cadence: weekly | monthly.", Options: `[{"value":"weekly","label":"Weekly"},{"value":"monthly","label":"Monthly"}]`},
	"university.min_payout_kobo":          {Type: "int", DefaultValue: "500000", Description: "Minimum accrued net (kobo) before a payout is issued."},
	"university.pro_stream_rate_kobo":     {Type: "int", DefaultValue: "5000", Description: "Amount (kobo) accrued to a creator per Pro-enrolled learner per period."},
	// AUDIT-BE-SETTING-005: geo-aware personalization master switch (ADR-064).
	"geo_personalization.enabled":         {Type: "bool", DefaultValue: "true", Description: "Master switch for geo-aware feed/search/discovery weighting."},
	// AUDIT-BE2-SEC-013: link preview fetch timeout (Rule 3 — was hardcoded 10s).
	"linkpreview.fetch_timeout_seconds":   {Type: "int", DefaultValue: "10", Min: 1, Max: 120, Description: "HTTP timeout for link preview fetches in seconds."},
	// AUDIT-BE2-BL-014: configurable iframe embed allowlist for HTML sanitization (Section 16).
	"content.sanitization.allowed_iframe_hosts": {Type: "csv", DefaultValue: "youtube.com,vimeo.com,codepen.io,jsfiddle.net,embed.ted.com", Description: "Comma-separated domains permitted in <iframe> src in user-generated content."},
	// AUDIT-BE2-SEC-011: webhook delivery max attempts (Rule 3 — was hardcoded 5).
	// Reuses notifications.max_delivery_attempts (already catalogued above) — no
	// separate key needed; documented here for discoverability.
	// Onboarding promoted entities (SETTING-005 — JEOS catalog alignment).
	"onboarding.promoted_spaces": {Type: "json", DefaultValue: "[]", Description: "JSON array of space UUIDs to promote in recommendations."},
	"onboarding.promoted_users":  {Type: "json", DefaultValue: "[]", Description: "JSON array of user UUIDs to promote in recommendations."},
	"onboarding.promoted_tags":   {Type: "json", DefaultValue: "[]", Description: "JSON array of tag UUIDs to promote in recommendations."},
	"onboarding.min_spaces_join": {Type: "int", DefaultValue: "3", Description: "Minimum number of spaces a new user must join during onboarding."},
	"onboarding.min_tags_follow": {Type: "int", DefaultValue: "5", Description: "Minimum number of tags a new user must follow during onboarding."},
	"onboarding.min_users_follow": {Type: "int", DefaultValue: "3", Description: "Minimum number of users a new user must follow during onboarding."},
	// General platform settings (exist in DB, now catalogued for type metadata).
	"general.site_name":              {Type: "string", DefaultValue: "JapaConnect", Description: "Platform display name."},
	"general.site_description":       {Type: "string", DefaultValue: "", Description: "Short platform description for SEO and meta tags."},
	"general.maintenance_mode":       {Type: "bool", DefaultValue: "false", Description: "When true, the platform shows a maintenance banner to non-admin users."},
	"general.registration_open":      {Type: "bool", DefaultValue: "true", Description: "Whether new user self-registration is allowed."},
	// Appearance / branding.
	"appearance.dark_mode_default":   {Type: "bool", DefaultValue: "false", Description: "Default theme for new visitors (false = light)."},
	"appearance.feed_card_style":     {Type: "string", DefaultValue: "reddit", Description: "Default feed card style: reddit | compact | grid | magazine.", Options: `[{"value":"reddit","label":"Reddit"},{"value":"compact","label":"Compact"},{"value":"grid","label":"Grid"},{"value":"magazine","label":"Magazine"}]`},
	"appearance.logo_url":            {Type: "string", DefaultValue: "", Description: "URL to the platform logo image."},
	"appearance.primary_color":       {Type: "string", DefaultValue: "#c615a6", Description: "Primary brand color (hex)."},
	// Advanced / operational.
	"advanced.cache_ttl_seconds":     {Type: "int", DefaultValue: "20", Description: "Default Valkey cache TTL in seconds for non-explicit keys."},
	"advanced.log_level":             {Type: "string", DefaultValue: "info", Description: "Application log level.", Options: `[{"value":"debug","label":"Debug"},{"value":"info","label":"Info"},{"value":"warn","label":"Warn"},{"value":"error","label":"Error"}]`},
	// Content settings not yet in the JEOS catalog but present in the DB.
	"content.allow_anonymous_posts":  {Type: "bool", DefaultValue: "false", Description: "Whether anonymous (logged-out) users can create posts."},
	"content.allowed_upload_types":   {Type: "csv", DefaultValue: "jpeg,png,jpg", Description: "Comma-separated allowed file extensions for content uploads.", Options: `[{"value":"jpeg","label":"JPEG"},{"value":"jpg","label":"JPG"},{"value":"png","label":"PNG"},{"value":"gif","label":"GIF"},{"value":"webp","label":"WebP"},{"value":"svg","label":"SVG"},{"value":"pdf","label":"PDF"},{"value":"doc","label":"DOC"},{"value":"docx","label":"DOCX"},{"value":"mp4","label":"MP4"},{"value":"webm","label":"WebM"},{"value":"mov","label":"MOV"}]`},
	"content.max_upload_size_mb":     {Type: "int", DefaultValue: "5", Description: "Maximum upload file size in MB for content attachments."},
	"content.posts_per_page":         {Type: "int", DefaultValue: "12", Description: "Number of posts per page in feed/list views."},
	"content.require_content_review": {Type: "bool", DefaultValue: "true", Description: "Whether new content requires admin review before publishing."},
	// Feature flags (platform-level toggles).
	"features.advertising_enabled":   {Type: "bool", DefaultValue: "true", Description: "Master toggle for the advertising module."},
	"features.blog_enabled":          {Type: "bool", DefaultValue: "true", Description: "Master toggle for the blog module."},
	"features.comparisons_enabled":   {Type: "bool", DefaultValue: "true", Description: "Master toggle for the comparison module."},
	"features.forum_enabled":         {Type: "bool", DefaultValue: "true", Description: "Master toggle for the forum module."},
	"features.reviews_enabled":       {Type: "bool", DefaultValue: "true", Description: "Master toggle for the reviews module."},
	// Security settings not in the JEOS catalog.
	"security.lockout_duration_minutes":    {Type: "int", DefaultValue: "15", Description: "Account lockout duration in minutes after max login attempts."},
	"security.require_email_verification":  {Type: "bool", DefaultValue: "true", Description: "Whether email verification is required before posting."},
	// SEO settings.
	"seo.enable_json_ld":     {Type: "bool", DefaultValue: "true", Description: "Generate JSON-LD structured data on content pages."},
	"seo.enable_robots_txt":  {Type: "bool", DefaultValue: "true", Description: "Serve a robots.txt file."},
	"seo.enable_sitemap":     {Type: "bool", DefaultValue: "true", Description: "Generate and serve an XML sitemap."},
	// User profile settings.
	"users.allowed_avatar_types":         {Type: "csv", DefaultValue: "png,jpg,gif,webp", Description: "Comma-separated allowed file extensions for user avatars.", Options: `[{"value":"png","label":"PNG"},{"value":"jpg","label":"JPG"},{"value":"jpeg","label":"JPEG"},{"value":"gif","label":"GIF"},{"value":"webp","label":"WebP"},{"value":"svg","label":"SVG"}]`},
	"users.allow_social_login":           {Type: "bool", DefaultValue: "false", Description: "Whether OAuth social login providers are enabled."},
	"users.require_profile_completion":   {Type: "bool", DefaultValue: "false", Description: "Whether users must complete their profile before posting."},
}

// getSettingMeta returns the metadata for a given dot-notation key.
// If the key is not in the catalog, it returns a best-effort guess based on
// the value content (bool-like, number-like, etc.) or falls back to "string".
func getSettingMeta(key string, currentValue string) settingMeta {
	if meta, ok := settingsCatalog[key]; ok {
		return meta
	}
	// No catalog entry — return a minimal fallback with no type.
	return settingMeta{
		Type:    "",
		DefaultValue: "",
		Description:  "",
	}
}
