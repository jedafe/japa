package control

import (
	"fmt"
	"strconv"
	"strings"
)

// settingMeta holds the authoritative metadata for one system setting.
// The Key is the full dot-notation key (e.g. "auth.password.min_length").
type settingMeta struct {
	Key          string  `json:"-"`
	Type         string  `json:"type"` // "int", "float", "bool", "string", "csv", "json"
	DefaultValue string  `json:"default_value"`
	Description  string  `json:"description"`
	Min          float64 `json:"min,omitempty"`
	Max          float64 `json:"max,omitempty"`
	Sensitive    bool    `json:"sensitive"`
	// Options is a JSON-encoded array of {value, label} objects for select/multi-select.
	Options string `json:"options,omitempty"`
	// Public indicates whether this setting should be exposed via the
	// GET /site/settings/public endpoint (is_public = true). Appearance and
	// general branding settings are public so the main site can render them.
	Public bool `json:"-"`
	// Panel determines which settings panel manages this setting.
	// "admin" = content/platform settings managed by regular admins.
	// "control" = infrastructure/system-level settings managed by super admins.
	// Empty string = shown in both panels (legacy/unclassified).
	Panel string `json:"-"`
	// Deprecated marks settings superseded by a feature_flags row (unified
	// rollout — see deprecatedReachabilitySettings). They remain valid reads
	// (services consult them only as a nil-checker fallback) but are excluded
	// from the editable settings list; manage them via /extensions.
	Deprecated bool `json:"-"`
}

// landingToolsDefault is the public landing-page tool card registry
// (moved from the frontend package so admins can configure it without a code change).
const landingToolsDefault = `[{"id":"visa-checklist","name":"Visa Checklist","slug":"visa-checklist","description":"Generate your visa document checklist by destination.","href":"/tools/visa-checklist","icon":"FileCheck","accent":"var(--chart-1)"},{"id":"readiness-assessment","name":"Japa Readiness Assessment","slug":"readiness-assessment","description":"Assess your readiness to relocate abroad.","href":"/tools/readiness","icon":"ClipboardCheck","accent":"var(--chart-2)"},{"id":"cost-of-living","name":"Cost of Living Calculator","slug":"cost-of-living","description":"Compare living costs across destination cities.","href":"/tools/cost-of-living","icon":"Calculator","accent":"var(--chart-3)"},{"id":"scholarship-finder","name":"Scholarship Finder","slug":"scholarship-finder","description":"Find scholarships for your destination and level.","href":"/scholarships","icon":"GraduationCap","accent":"var(--chart-4)"},{"id":"remittance-tracker","name":"Remittance Rate Tracker","slug":"remittance-tracker","description":"Track live remittance rates across corridors.","href":"/tools/remittance","icon":"TrendingUp","accent":"var(--chart-5)"},{"id":"japabot","name":"JapaBot","slug":"japabot","description":"AI assistant for your relocation questions.","href":"/ai/chat","icon":"Bot","badge":"AI","accent":"var(--color-primary)"},{"id":"jobs","name":"Jobs Board","slug":"jobs","description":"Browse jobs with visa sponsorship options.","href":"/jobs","icon":"Briefcase","accent":"var(--chart-1)"},{"id":"travel-search","name":"Travel Search","slug":"travel-search","description":"Search and compare flights and hotels.","href":"/tools/travel","icon":"Plane","accent":"var(--chart-2)"}]`

// landingStatsDefault is the public guest-homepage stats row.
const landingStatsDefault = `[{"value":"50K+","label":"Opportunities","icon":"Sparkles"},{"value":"185+","label":"Countries","icon":"Globe"},{"value":"20K+","label":"Success Stories","icon":"Star"},{"value":"500K+","label":"Community","icon":"Users"}]`

// landingCategoriesDefault is the public guest-homepage category card grid.
const landingCategoriesDefault = `[{"id":"jobs","title":"Find Jobs Abroad","description":"Discover verified job opportunities from top employers worldwide.","href":"/jobs","icon":"Briefcase","color":"green"},{"id":"study","title":"Study Abroad","description":"Find scholarships, schools, and resources to study in top countries.","href":"/scholarships","icon":"GraduationCap","color":"blue"},{"id":"visa","title":"Visa & Relocation","description":"Get visa guidance, relocation tips, and step-by-step support.","href":"/tools/visa-checklist","icon":"Plane","color":"purple"},{"id":"business","title":"Start a Business","description":"Learn, connect and grow your business beyond borders.","href":"/explore","icon":"TrendingUp","color":"orange"},{"id":"community","title":"Community Support","description":"Join a global community and get help from people who have been there.","href":"/stories","icon":"Heart","color":"green"}]`

// landingFeaturedOpportunitiesDefault is the public guest-homepage featured-opportunity carousel.
const landingFeaturedOpportunitiesDefault = `[{"id":"opp-1","type":"job","title":"Software Engineer","organization":"Microsoft","location":"Canada","tag":"Full-time","badge":"New","value":"$8,500/mo","href":"/jobs","published_at":"2025-01-15T00:00:00Z"},{"id":"opp-2","type":"scholarship","title":"Lester B. Pearson Scholarship","organization":"University of Toronto","location":"Canada","tag":"Scholarship","badge":"New","value":"Full Tuition","href":"/scholarships","published_at":"2025-01-14T00:00:00Z"},{"id":"opp-3","type":"visa","title":"Canada Express Entry Program","organization":"Government of Canada","location":"Canada","tag":"Visa Support","badge":"New","value":"Free","href":"/tools/visa-checklist","published_at":"2025-01-13T00:00:00Z"},{"id":"opp-4","type":"business","title":"Start Your E-commerce Business","organization":"Shopify","location":"Global","tag":"Business","badge":"New","value":"Free Guide","href":"/explore","published_at":"2025-01-12T00:00:00Z"},{"id":"opp-5","type":"internship","title":"Global Internship Program","organization":"Nestle","location":"Switzerland","tag":"Internship","badge":"New","value":"$3,000/mo","href":"/jobs","published_at":"2025-01-11T00:00:00Z"}]`

// landingTopDestinationsDefault is the public guest-homepage top-destinations card.
const landingTopDestinationsDefault = `[{"id":"ca","slug":"canada","name":"Canada"},{"id":"gb","slug":"united-kingdom","name":"United Kingdom"},{"id":"au","slug":"australia","name":"Australia"},{"id":"de","slug":"germany","name":"Germany"}]`

// settingsCatalog maps every known dot-notation key to its metadata.
// This is the same catalog documented in .jeos/conventions/system-settings-catalog.md.
// When a setting is not in the catalog, it falls back to type "string" with no metadata.
var settingsCatalog = map[string]settingMeta{
	// Authentication
	"auth.password.min_length":              {Type: "int", DefaultValue: "8", Description: "Minimum password length.", Panel: "admin"},
	"auth.password.max_length":              {Type: "int", DefaultValue: "128", Description: "Maximum password length.", Panel: "admin"},
	"auth.password.require_uppercase":       {Type: "bool", DefaultValue: "false", Description: "Enforce uppercase requirement.", Panel: "admin"},
	"auth.password.require_number":          {Type: "bool", DefaultValue: "true", Description: "Enforce numeric digit requirement.", Panel: "admin"},
	"auth.password.require_symbol":          {Type: "bool", DefaultValue: "false", Description: "Enforce symbol requirement.", Panel: "admin"},
	"auth.password.check_breach":            {Type: "bool", DefaultValue: "true", Description: "Check password against HIBP breach database before accepting.", Panel: "admin"},
	"auth.password.history_count":           {Type: "int", DefaultValue: "5", Description: "Number of recent password hashes to check against reuse.", Panel: "admin"},
	"auth.session.refresh_days":             {Type: "int", DefaultValue: "30", Description: "Refresh token TTL in days.", Panel: "admin"},
	"auth.session.access_token_ttl_minutes": {Type: "int", DefaultValue: "15", Description: "Access token TTL in minutes.", Panel: "admin"},
	"auth.totp.required_for_admin":          {Type: "bool", DefaultValue: "false", Description: "Force 2FA for admin roles.", Panel: "admin"},
	"auth.email.verify_required":            {Type: "bool", DefaultValue: "true", Description: "Require email verification before posting.", Panel: "admin"},
	// Feed & Content
	"feed.fanout_threshold":            {Type: "int", DefaultValue: "5000", Description: "Max space members for push fan-out.", Panel: "admin"},
	"feed.velocity.gravity":            {Type: "float", DefaultValue: "1.8", Description: "HN gravity constant for Rising sort.", Panel: "admin"},
	"feed.max_followed_spaces_in_feed": {Type: "int", DefaultValue: "50", Description: "Cap on spaces contributing to feed.", Panel: "admin"},
	"feed.card_style":                  {Type: "json", DefaultValue: `{"default":"daily","by_type":{}}`, Description: "{default, by_type} map; each style ∈ reddit|compact|grid|magazine|daily — active feed card style, resolved per content type (tolerates a legacy scalar).", Panel: "admin"},
	"content.edit_window_minutes":      {Type: "int", DefaultValue: "1440", Description: "Content edit window (24hr default).", Panel: "admin"},
	"content.link_preview.blacklist":   {Type: "json", DefaultValue: "[]", Description: "JSON array of blocked domains for link previews.", Panel: "admin"},
	"content.min_length":               {Type: "int", DefaultValue: "10", Description: "Minimum content body length.", Panel: "admin"},
	"content.max_tags":                 {Type: "int", DefaultValue: "5", Description: "Max tags per content item.", Public: true, Panel: "admin"},
	"content.edit_lock_ttl_seconds":    {Type: "int", DefaultValue: "120", Description: "Editor lock TTL in seconds.", Panel: "admin"},
	"comments.edit_window_minutes":     {Type: "int", DefaultValue: "60", Description: "Comment edit window.", Panel: "admin"},
	"forum.edit_window_minutes":        {Type: "int", DefaultValue: "30", Description: "Editable window (minutes) after posting a forum thread/reply.", Panel: "admin"},
	// Admin handler stores these under the "content" namespace (key prefix
	// "content.") so catalog them with the matching key to prevent uncatalogued
	// DB entries from leaking into the control panel.
	"content.comments.edit_window_minutes": {Type: "int", DefaultValue: "60", Description: "Comment edit window (stored under content.* by admin handler).", Panel: "admin"},
	"content.forum.edit_window_minutes":    {Type: "int", DefaultValue: "30", Description: "Forum edit window (stored under content.* by admin handler).", Panel: "admin"},
	// Spaces
	"spaces.bubble_depth":                   {Type: "int", DefaultValue: "2", Description: "How many levels posts bubble up the space hierarchy.", Panel: "admin"},
	"spaces.max_pinned_posts":               {Type: "int", DefaultValue: "5", Description: "Max pinned posts per space.", Panel: "admin"},
	"spaces.wiki_rate_interval_ms":          {Type: "int", DefaultValue: "2000", Description: "Minimum interval (ms) between Wiki API calls.", Panel: "admin"},
	"spaces.levels":                         {Type: "json", DefaultValue: `["country","state","city","custom"]`, Description: "Allowed space hierarchy levels for filters and creation.", Public: true, Panel: "admin"},
	"spaces.page_size":                      {Type: "int", DefaultValue: "24", Description: "Default page size for the /spaces directory.", Public: true, Panel: "admin"},
	"spaces.hub_page_size":                  {Type: "int", DefaultValue: "20", Description: "Default page size for the space hub feed/members.", Public: true, Panel: "admin"},
	"spaces.forum_page_size":                {Type: "int", DefaultValue: "50", Description: "Default page size for forum threads on the space hub.", Public: true, Panel: "admin"},
	"spaces.permission_axes":                {Type: "json", DefaultValue: `{"visibility":["public","members","private"],"posting":["open","members","verified","moderators","admins"],"joining":["open","request","invite"]}`, Description: "Allowed values for the space permission axes (Rule 16).", Public: true, Panel: "admin"},
	"spaces.default_member_space_type_slug": {Type: "string", DefaultValue: "community", Description: "Default space type slug for member-created spaces.", Public: true, Panel: "admin"},
	// Moderation
	"moderation.auto_hide_report_count":     {Type: "int", DefaultValue: "5", Description: "Auto-hide content after N reports.", Panel: "admin"},
	"moderation.ai_auto_flag_threshold":     {Type: "float", DefaultValue: "0.85", Description: "AI classifier confidence for auto-flag.", Panel: "admin"},
	"moderation.max_links_per_post":         {Type: "int", DefaultValue: "3", Description: "Max links per post (spam detection).", Panel: "admin"},
	"moderation.new_account_threshold_days": {Type: "int", DefaultValue: "7", Description: "Days before account is no longer 'new'.", Panel: "admin"},
	// Security
	"security.max_login_attempts":  {Type: "int", DefaultValue: "5", Description: "Max failed login attempts before lockout.", Panel: "admin"},
	"security.fail_window_minutes": {Type: "int", DefaultValue: "15", Description: "Lockout window in minutes.", Panel: "admin"},
	// Wallet
	"wallet.min_convert_points":    {Type: "int", DefaultValue: "100", Description: "Minimum points for conversion.", Panel: "admin"},
	"wallet.points_to_credit_rate": {Type: "float", DefaultValue: "0.01", Description: "1 point = 0.01 credits by default.", Panel: "admin"},
	"wallet.max_daily_earn_points": {Type: "int", DefaultValue: "500", Description: "Cap on points earned per user per day.", Panel: "admin"},
	// API Rate Limits
	"api.rate_limit.basic":      {Type: "int", DefaultValue: "100", Description: "Requests per minute — basic tier.", Panel: "admin"},
	"api.rate_limit.standard":   {Type: "int", DefaultValue: "1000", Description: "Requests per minute — standard tier.", Panel: "admin"},
	"api.rate_limit.premium":    {Type: "int", DefaultValue: "10000", Description: "Requests per minute — premium tier.", Panel: "admin"},
	"api.keys.max_per_user":     {Type: "int", DefaultValue: "5", Description: "Max API keys per user.", Panel: "admin"},
	"api.idempotency_ttl_hours": {Type: "int", DefaultValue: "24", Description: "Idempotency key TTL in hours.", Panel: "admin"},
	// Unified user tiers (regular / pro / business)
	"organizations.max_per_user.regular":    {Type: "int", DefaultValue: "1", Description: "Max organizations a regular-tier user can create.", Panel: "admin"},
	"organizations.max_per_user.pro":        {Type: "int", DefaultValue: "5", Description: "Max organizations a pro-tier user can create.", Panel: "admin"},
	"organizations.max_per_user.business":   {Type: "int", DefaultValue: "-1", Description: "Max organizations a business-tier user can create (-1 = unlimited).", Panel: "admin"},
	"ai.assistant.daily_queries.regular":    {Type: "int", DefaultValue: "10", Description: "Max AI assistant queries per day for regular-tier users.", Panel: "admin"},
	"ai.assistant.daily_queries.pro":        {Type: "int", DefaultValue: "100", Description: "Max AI assistant queries per day for pro-tier users.", Panel: "admin"},
	"ai.assistant.daily_queries.business":   {Type: "int", DefaultValue: "-1", Description: "Max AI assistant queries per day for business-tier users (-1 = unlimited).", Panel: "admin"},
	"content.posts_per_day.regular":         {Type: "int", DefaultValue: "5", Description: "Max content posts per day for regular-tier users.", Panel: "admin"},
	"content.posts_per_day.pro":             {Type: "int", DefaultValue: "-1", Description: "Max content posts per day for pro-tier users (-1 = unlimited).", Panel: "admin"},
	"content.posts_per_day.business":        {Type: "int", DefaultValue: "-1", Description: "Max content posts per day for business-tier users (-1 = unlimited).", Panel: "admin"},
	"credits.pack_discount_pct.regular":     {Type: "int", DefaultValue: "0", Description: "Credit pack discount percent for regular-tier users.", Panel: "admin"},
	"credits.pack_discount_pct.pro":         {Type: "int", DefaultValue: "0", Description: "Credit pack discount percent for pro-tier users.", Panel: "admin"},
	"credits.pack_discount_pct.business":    {Type: "int", DefaultValue: "10", Description: "Credit pack discount percent for business-tier users.", Panel: "admin"},
	"jce.comparisons.max_per_day.regular":   {Type: "int", DefaultValue: "5", Description: "Max JCE comparisons created per day for regular-tier users.", Panel: "admin"},
	"jce.comparisons.max_per_day.pro":       {Type: "int", DefaultValue: "25", Description: "Max JCE comparisons created per day for pro-tier users.", Panel: "admin"},
	"jce.comparisons.max_per_day.business":  {Type: "int", DefaultValue: "-1", Description: "Max JCE comparisons created per day for business-tier users (-1 = unlimited).", Panel: "admin"},
	"wallet.max_daily_earn_points.regular":  {Type: "int", DefaultValue: "100", Description: "Max points a regular-tier user can earn per day.", Panel: "admin"},
	"wallet.max_daily_earn_points.pro":      {Type: "int", DefaultValue: "500", Description: "Max points a pro-tier user can earn per day.", Panel: "admin"},
	"wallet.max_daily_earn_points.business": {Type: "int", DefaultValue: "-1", Description: "Max points a business-tier user can earn per day (-1 = unlimited).", Panel: "admin"},
	// CORS
	"cors.allowed_origins": {Type: "csv", DefaultValue: "*", Description: "Comma-separated list of allowed CORS origins.", Panel: "control"},
	// Ads
	"ads.frequency_cap_per_day": {Type: "int", DefaultValue: "3", Description: "Max ad impressions per user per day (global default).", Panel: "admin"},
	"ads.min_campaign_budget":   {Type: "float", DefaultValue: "10", Description: "Minimum campaign budget in credits.", Panel: "admin"},
	// AI
	"ai.monthly_budget_usd":      {Type: "float", DefaultValue: "100", Description: "Alert threshold for AI spend (not hard cap).", Panel: "admin"},
	"ai.japabot_enabled":         {Type: "bool", DefaultValue: "true", Description: "DEPRECATED — managed via feature_flags/feature.japabot. Global JapaBot toggle.", Panel: "admin"},
	"ai.content_assist_enabled":  {Type: "bool", DefaultValue: "true", Description: "DEPRECATED — managed via feature_flags/feature.ai_content_gen. In-editor AI assist toggle.", Panel: "admin"},
	"ai.daily_cost_limit_usd":    {Type: "float", DefaultValue: "0", Description: "Per-user daily AI spend hard cap in USD (UTC) shared across JapaBot chat, in-editor assist, and the AI agent. 0 = disabled.", Panel: "control"},
	"ai.cost_per_1k_tokens_usd":  {Type: "float", DefaultValue: "0.002", Description: "Default USD cost per 1,000 tokens for AI usage monitoring and cost limits.", Panel: "control"},
	"ai.cache_enabled":           {Type: "bool", DefaultValue: "false", Description: "Enable Valkey semantic cache for repeated AI prompts (generate, chat, assist).", Panel: "control"},
	"ai.cache_ttl_seconds":       {Type: "int", DefaultValue: "300", Description: "TTL for cached AI responses (seconds).", Panel: "control"},
	"ai.provider_cost_overrides": {Type: "json", DefaultValue: "{}", Description: `Per-provider/model price overrides. Keys are "provider" or "provider.model" (e.g. "xiaomi.mimo-v2-flash"), values are USD per 1,000 tokens.`, Panel: "control"},
	// AI Humanization — injects natural-language style instructions into every
	// AI generation surface (generate, chat, assist, persona/studio drafts).
	// Gated by the ai.humanize.manage RBAC node for the admin toggle; users
	// can also opt in/out per request via the humanize payload field.
	"ai.humanize.enabled": {Type: "bool", DefaultValue: "false", Description: "Global toggle for AI text humanization (natural-language style injection).", Panel: "admin"},
	"ai.humanize.level":   {Type: "string", DefaultValue: "medium", Description: "Humanization intensity: low | medium | high.", Options: `[{"value":"low","label":"Low"},{"value":"medium","label":"Medium"},{"value":"high","label":"High"}]`, Panel: "admin"},
	// AI Agent Executor (§103, ADR-111)
	"agent.enabled":                    {Type: "bool", DefaultValue: "true", Description: "Global AI agent toggle.", Panel: "control"},
	"agent.max_tool_calls":             {Type: "int", DefaultValue: "20", Min: 1, Max: 1000, Description: "Max tool calls per agent request.", Panel: "control"},
	"agent.timeout_seconds":            {Type: "int", DefaultValue: "60", Min: 1, Max: 3600, Description: "Agent request timeout in seconds.", Panel: "control"},
	"agent.default_persona_id":         {Type: "string", DefaultValue: "", Description: "Default AI persona whose backing bot user is stamped as contents.author_id for draft authorship.", Panel: "control"},
	"agent.external_search_provider":   {Type: "string", DefaultValue: "tavily", Description: "External search provider for pre-search: tavily, brave, or none.", Options: `[{"value":"tavily","label":"Tavily"},{"value":"brave","label":"Brave"},{"value":"none","label":"None"}]`, Panel: "control"},
	"agent.external_search_api_key":    {Type: "string", DefaultValue: "", Description: "API key for external search. Stored encrypted; rotate via secret management, not in UI.", Sensitive: true, Panel: "control"},
	"agent.presearch_full_threshold":   {Type: "float", DefaultValue: "0.85", Min: 0, Max: 1, Description: "Relevance score threshold for FULL pre-search classification.", Panel: "control"},
	"agent.cost_limit_per_request_usd": {Type: "float", DefaultValue: "0.50", Min: 0, Max: 1000, Description: "Per-request cost limit in USD — agent stops if exceeded.", Panel: "control"},
	"agent.cost_per_1k_tokens":         {Type: "float", DefaultValue: "0.002", Min: 0, Description: "Deprecated agent-specific price; use ai.cost_per_1k_tokens_usd instead. Retained as fallback.", Panel: "control"},
	"agent.guest_enabled":              {Type: "bool", DefaultValue: "false", Description: "Allow guest (unauthenticated) agent prompts (read-only, no draft creation).", Panel: "control"},
	"agent.rate_limit_per_day":         {Type: "int", DefaultValue: "50", Min: 0, Max: 100000, Description: "Max agent requests per user per day (UTC).", Panel: "control"},
	"agent.daily_budget_usd":           {Type: "float", DefaultValue: "5.00", Min: 0, Description: "Per-user daily AI spend hard cap in USD (UTC). Use 0 to disable budget enforcement.", Panel: "control"},
	"agent.bulk_create_max_items":      {Type: "int", DefaultValue: "50", Min: 1, Max: 1000, Description: "Maximum number of content items a single content.bulk_create tool call may process.", Panel: "control"},
	"agent.bulk_publish_max_items":     {Type: "int", DefaultValue: "50", Min: 1, Max: 1000, Description: "Maximum number of content IDs a single content.bulk_publish tool call may process.", Panel: "control"},
	"agent.temperature":                {Type: "float", DefaultValue: "0.3", Min: 0, Max: 2, Description: "LLM sampling temperature passed to the gateway ChatWithTools call.", Panel: "control"},
	// Media — storage.* keys are read at boot by ReconfigureS3FromSettings:
	// when endpoint+access_key+secret_key are all set here they override the
	// JC_S3_* env vars; otherwise the env-based client is retained. The
	// effective bucket also determines the StorageBucket recorded on media
	// rows and the bucket in generated serve/imgproxy URLs.
	"media.storage.provider":       {Type: "string", DefaultValue: "seaweedfs", Description: "Storage backend provider (env-only: JC_S3_ENDPOINT).", Options: `[{"value":"seaweedfs","label":"SeaweedFS"},{"value":"s3","label":"Amazon S3"},{"value":"r2","label":"Cloudflare R2"},{"value":"b2","label":"Backblaze B2"}]`, Panel: "control"},
	"media.storage.endpoint":       {Type: "string", DefaultValue: "", Description: "S3-compatible endpoint URL (env-only: JC_S3_ENDPOINT).", Panel: "control"},
	"media.storage.bucket":         {Type: "string", DefaultValue: "japaconnect", Description: "Default bucket name (env-only: JC_S3_BUCKET).", Panel: "control"},
	"media.storage.region":         {Type: "string", DefaultValue: "us-east-1", Description: "S3 region (env-only: JC_S3_REGION; SeaweedFS ignores this).", Panel: "control"},
	"media.storage.access_key":     {Type: "string", DefaultValue: "", Description: "S3 access key (env-only: JC_S3_ACCESS_KEY).", Sensitive: true, Panel: "control"},
	"media.storage.secret_key":     {Type: "string", DefaultValue: "", Description: "S3 secret key (env-only: JC_S3_SECRET_KEY).", Sensitive: true, Panel: "control"},
	"media.cdn.base_url":           {Type: "string", DefaultValue: "", Description: "CDN base URL (empty = use imgproxy directly).", Panel: "control"},
	"media.max_upload_mb":          {Type: "int", DefaultValue: "50", Description: "Max file size in MB.", Panel: "admin"},
	"media.allowed_video_types":    {Type: "csv", DefaultValue: "mp4,webm,mov", Description: "Allowed video MIME extensions.", Panel: "admin"},
	"media.deleted_retention_days": {Type: "int", DefaultValue: "30", Description: "Days a soft-deleted media row and its storage object are retained before the media:cleanup_deleted sweep purges them.", Panel: "control"},
	// Notifications
	"notifications.max_delivery_attempts": {Type: "int", DefaultValue: "5", Description: "Max retry attempts for notification delivery.", Panel: "admin"},
	"notifications.batch_window_minutes":  {Type: "int", DefaultValue: "5", Description: "Notification dedup batch window.", Panel: "admin"},
	// Tickets
	"tickets.default_sla_hours":        {Type: "int", DefaultValue: "48", Description: "Default SLA response time.", Panel: "admin"},
	"tickets.auto_close_resolved_days": {Type: "int", DefaultValue: "7", Description: "Auto-close resolved tickets after N days.", Panel: "admin"},
	// Audit
	"audit.log_retention_days": {Type: "int", DefaultValue: "365", Description: "Days to retain audit_logs before pruning.", Panel: "admin"},
	// i18n
	"i18n.default_language":  {Type: "string", DefaultValue: "en", Description: "Platform default language.", Public: true, Panel: "admin"},
	"i18n.enabled_languages": {Type: "csv", DefaultValue: "en,fr", Description: "Comma-separated active language codes.", Public: true, Panel: "admin"},
	// GDPR
	// BUG-GDPR-006: gdpr.deletion_grace_days is deprecated — superseded by
	// gdpr.retention_days (ADR-057). No code reads it; retained for backward
	// compatibility only. Catalogued as Panel: "admin" but not surfaced.
	"gdpr.deletion_grace_days":           {Type: "int", DefaultValue: "30", Description: "DEPRECATED — superseded by gdpr.retention_days (ADR-057). No code reads this key; retained for backward compatibility only.", Panel: "admin"},
	"gdpr.export_expiry_days":            {Type: "int", DefaultValue: "7", Description: "Days before export download link expires.", Panel: "admin"},
	"gdpr.hard_delete_enabled":           {Type: "bool", DefaultValue: "false", Description: "Enable physical user erasure after anonymization (ADR-057).", Panel: "admin"},
	"gdpr.retention_days":                {Type: "int", DefaultValue: "0", Description: "Days after anonymization before hard delete (0 = immediate, ADR-057).", Panel: "admin"},
	"gdpr.deleted_user_placeholder_uuid": {Type: "string", DefaultValue: "", Description: "UUID assigned to orphaned content authorship after hard delete.", Panel: "admin"},
	// Search
	"search.autocomplete_max": {Type: "int", DefaultValue: "8", Description: "Max autocomplete suggestions returned.", Panel: "admin"},
	// Pricing Plans (SETTING-001 / Rule 3 — prices must be configurable).
	"pricing.plan.pro.monthly_amount":      {Type: "int", DefaultValue: "500000", Description: "Pro plan monthly price in kobo (NGN).", Panel: "control"},
	"pricing.plan.pro.yearly_amount":       {Type: "int", DefaultValue: "5000000", Description: "Pro plan yearly price in kobo (NGN).", Panel: "control"},
	"pricing.plan.business.monthly_amount": {Type: "int", DefaultValue: "1500000", Description: "Business plan monthly price in kobo (NGN).", Panel: "control"},
	"pricing.plan.business.yearly_amount":  {Type: "int", DefaultValue: "15000000", Description: "Business plan yearly price in kobo (NGN).", Panel: "control"},
	"pricing.currency":                     {Type: "string", DefaultValue: "NGN", Description: "Currency code for plan prices.", Options: `[{"value":"NGN","label":"Nigerian Naira (NGN)"},{"value":"USD","label":"US Dollar (USD)"},{"value":"GBP","label":"British Pound (GBP)"},{"value":"EUR","label":"Euro (EUR)"}]`, Panel: "control"},
	// payment.currency is the default currency for payment/credit operations
	// (internal/payments/service.go and credit.go). It is separate from
	// pricing.currency, which controls plan-display currency in the Control
	// Panel. Ref: .jeos/conventions/system-settings-catalog.md §Payment.
	"payment.currency": {Type: "string", DefaultValue: "NGN", Description: "Default payment currency for payment and credit operations.", Panel: "admin", Options: `[{"value":"NGN","label":"Nigerian Naira (NGN)"},{"value":"USD","label":"US Dollar (USD)"},{"value":"GBP","label":"British Pound (GBP)"},{"value":"EUR","label":"Euro (EUR)"}]`},
	// Affiliate
	"affiliate.min_account_days":        {Type: "int", DefaultValue: "30", Description: "Minimum account age (days) to enroll in affiliate program.", Panel: "admin"},
	"affiliate.attribution_window_days": {Type: "int", DefaultValue: "30", Description: "Click attribution window in days.", Panel: "admin"},
	"affiliate.commission.pro_monthly":  {Type: "float", DefaultValue: "15", Description: "Commission % on Pro monthly subscription purchase.", Panel: "admin"},
	"affiliate.commission.pro_annual":   {Type: "float", DefaultValue: "20", Description: "Commission % on Pro annual subscription purchase.", Panel: "admin"},
	"affiliate.commission.course":       {Type: "float", DefaultValue: "10", Description: "Commission % on course access purchase.", Panel: "admin"},
	"affiliate.max_links_per_user":      {Type: "int", DefaultValue: "10", Description: "Max active affiliate links per enrolled user.", Panel: "admin"},
	// AUDIT-BE-SETTING-001/002/003: affiliate enhancement settings (Section 55).
	"affiliate.author_commission_default": {Type: "float", DefaultValue: "25", Description: "Default % of external affiliate commission shared with content author (when provider has no override).", Panel: "admin"},
	"affiliate.unregistered_link_policy":  {Type: "string", DefaultValue: "flag", Description: "How to handle unregistered affiliate links: flag (log only), block (prevent publish), strip (remove link).", Options: `[{"value":"flag","label":"Flag (log only)"},{"value":"block","label":"Block (prevent publish)"},{"value":"strip","label":"Strip (remove link)"}]`, Panel: "admin"},
	"affiliate.scan_on_publish":           {Type: "bool", DefaultValue: "true", Description: "Whether to scan content for unregistered affiliate links on publish.", Panel: "admin"},
	// AUDIT-BE-SETTING-004: Japa Academy creator economy settings (ADR-063, Section 36).
	"academy.subdomain":                   {Type: "string", DefaultValue: "academy", Description: "Host label for the Japa Academy shell.", Panel: "control"},
	"academy.custom_domain":               {Type: "string", DefaultValue: "", Description: "Optional custom domain (e.g. japaacademy.com); empty = use subdomain.", Panel: "control"},
	"academy.creator_percentage_default":  {Type: "int", DefaultValue: "75", Description: "Default creator revenue share % (platform takes the remainder).", Panel: "control"},
	"academy.platform_percentage_default": {Type: "int", DefaultValue: "25", Description: "Default platform revenue share %.", Panel: "control"},
	"academy.payout_schedule":             {Type: "string", DefaultValue: "monthly", Description: "Payout cadence: weekly | monthly.", Options: `[{"value":"weekly","label":"Weekly"},{"value":"monthly","label":"Monthly"}]`, Panel: "control"},
	"academy.min_payout_kobo":             {Type: "int", DefaultValue: "500000", Description: "Minimum accrued net (kobo) before a payout is issued.", Panel: "control"},
	"academy.pro_stream_rate_kobo":        {Type: "int", DefaultValue: "5000", Description: "Amount (kobo) accrued to a creator per Pro-enrolled learner per period.", Panel: "control"},
	// AUDIT-BE-SETTING-005: geo-aware personalization master switch (ADR-064).
	"geo_personalization.enabled": {Type: "bool", DefaultValue: "true", Description: "Master switch for geo-aware feed/search/discovery weighting.", Panel: "control"},
	// ADR-064 / Phase 38.03: config-driven archetype market lists (CSV of ISO
	// 3166-1 alpha-2 codes). The visitor archetype resolver classifies each
	// visitor as origin_visitor | diaspora | global_traveler based on whether
	// their current/origin country is in origin_markets or destination_markets.
	// Kept in config (not hardcoded) so ops can tune the lists without a deploy.
	"geo_personalization.origin_markets":      {Type: "csv", DefaultValue: "NG,GH,ZA,KE,EG", Description: "CSV of ISO 3166-1 alpha-2 codes of 'origin markets' — visitors here are classified origin_visitor (preparing to relocate) by the archetype resolver.", Panel: "control"},
	"geo_personalization.destination_markets": {Type: "csv", DefaultValue: "GB,CA,US,DE,AU,AE", Description: "CSV of ISO 3166-1 alpha-2 codes of 'destination markets' — visitors here (or with japa_status=settled) are classified diaspora by the archetype resolver.", Panel: "control"},
	// ADR-064a: recommendation engine config — the stage_tool_matrix drives
	// tool/feature suggestions by archetype + japa_stage; tool_labels and
	// tool_urls override the compiled-in display fallbacks (Rule 3).
	"geo_personalization.stage_tool_matrix":                 {Type: "json", DefaultValue: "", Description: "JSON mapping {japa_intent}:{japa_stage} or {archetype}:{japa_stage} keys to arrays of recommended tool/feature slugs. Resolution order: {intent}:{stage} → {intent}:* → {archetype}:{stage} → {archetype}:* → *:{japa_stage} → *:* (ADR-128).", Panel: "control"},
	"geo_personalization.intent_matrix":                     {Type: "json", DefaultValue: "", Description: "JSON mapping a japa_intent (or *) to an experience rule {hide_paths, pin_paths, demote_space_types} — applied to resolved menus, /suggestions/spaces, and onboarding recommendations (ADR-128). Gated by the geo_personalization.enabled feature flag.", Panel: "control"},
	"geo_personalization.tool_labels":                       {Type: "json", DefaultValue: "", Description: "JSON mapping tool/feature slugs to human-readable display labels for the recommendation engine.", Panel: "control"},
	"geo_personalization.tool_urls":                         {Type: "json", DefaultValue: "", Description: "JSON mapping tool/feature slugs to frontend URLs for the recommendation engine.", Panel: "control"},
	"geo_personalization.city_level_enabled":                {Type: "bool", DefaultValue: "true", Description: "Killswitch for city-level personalization (current_geo_space_id resolution, city-level filtering). When false, the resolver skips city-level enrichment.", Panel: "control"},
	"geo_personalization.nominatim_url":                     {Type: "string", DefaultValue: "https://nominatim.openstreetmap.org/reverse", Description: "Reverse-geocoding endpoint URL for browser geolocation.", Panel: "control"},
	"geo_personalization.nominatim_timeout_ms":              {Type: "int", DefaultValue: "5000", Min: 1000, Max: 30000, Description: "Reverse-geocoding request timeout in milliseconds.", Panel: "control"},
	"geo_personalization.recommendations_cache_ttl_seconds": {Type: "int", DefaultValue: "900", Min: 60, Max: 3600, Description: "TTL in seconds for the jc:recommendations:* cache keys (900 = 15 minutes).", Panel: "control"},
	"geo_personalization.recommendations_max_limit":         {Type: "int", DefaultValue: "20", Min: 1, Max: 100, Description: "Maximum value accepted for the ?limit query param on GET /recommendations.", Panel: "control"},
	"geo_personalization.recommendations_trending_window":   {Type: "string", DefaultValue: "7d", Description: "Trending-snapshot window for sourcing content/space recommendations.", Options: `[{"value":"24h","label":"24 hours"},{"value":"7d","label":"7 days"},{"value":"14d","label":"14 days"},{"value":"30d","label":"30 days"},{"value":"90d","label":"90 days"}]`, Panel: "control"},
	// AUDIT-BE2-SEC-013: link preview fetch timeout (Rule 3 — was hardcoded 10s).
	"linkpreview.fetch_timeout_seconds": {Type: "int", DefaultValue: "10", Min: 1, Max: 120, Description: "HTTP timeout for link preview fetches in seconds.", Panel: "control"},
	// AUDIT-BE2-BL-014: configurable iframe embed allowlist for HTML sanitization (Section 16).
	"content.sanitization.allowed_iframe_hosts": {Type: "csv", DefaultValue: "youtube.com,vimeo.com,codepen.io,jsfiddle.net,embed.ted.com", Description: "Comma-separated domains permitted in <iframe> src in user-generated content.", Panel: "control"},
	// AUDIT-BE2-SEC-011: webhook delivery max attempts (Rule 3 — was hardcoded 5).
	// Reuses notifications.max_delivery_attempts (already catalogued above) — no
	// separate key needed; documented here for discoverability.
	// Onboarding promoted entities (SETTING-005 — JEOS catalog alignment).
	"onboarding.promoted_spaces":  {Type: "json", DefaultValue: "[]", Description: "JSON array of space UUIDs to promote in recommendations.", Panel: "control"},
	"onboarding.promoted_users":   {Type: "json", DefaultValue: "[]", Description: "JSON array of user UUIDs to promote in recommendations.", Panel: "control"},
	"onboarding.promoted_tags":    {Type: "json", DefaultValue: "[]", Description: "JSON array of tag UUIDs to promote in recommendations.", Panel: "control"},
	"onboarding.min_spaces_join":  {Type: "int", DefaultValue: "3", Description: "Minimum number of spaces a new user must join during onboarding.", Public: true, Panel: "control"},
	"onboarding.min_tags_follow":  {Type: "int", DefaultValue: "5", Description: "Minimum number of tags a new user must follow during onboarding.", Public: true, Panel: "control"},
	"onboarding.min_users_follow": {Type: "int", DefaultValue: "3", Description: "Minimum number of users a new user must follow during onboarding.", Public: true, Panel: "control"},
	// Section 09 Product Tour: per-surface ordered step lists (data-tour
	// target keys). Frontends translate keys via their i18n dictionaries and
	// fall back to built-in defaults when the list is empty.
	"onboarding.tour_steps":                  {Type: "json", DefaultValue: `{"main":["primary-nav","home","notifications","messages","stories","forum","discover","explore","academy","jobs","achievements","bookmarks","search","space-switcher","compose","sidebar","account"],"docs":["docs-nav","docs-audience","docs-search","docs-topbar","docs-sidebar"],"academy":["primary-nav","academy-browse","my-learning","certificates","academy-bookmarks","instructor","academy-search","academy-sidebar","academy-account"]}`, Description: "Per-surface product tour step lists — JSON map of surface slug to ordered data-tour target keys.", Panel: "control"},
	"onboarding.reminder_enabled":            {Type: "bool", DefaultValue: "true", Description: "Daily in-app reminder for users who haven't finished onboarding.", Panel: "control"},
	"onboarding.reminder_max_count":          {Type: "int", DefaultValue: "5", Description: "Max onboarding reminder notifications sent per user.", Panel: "control"},
	"onboarding.reminder_window_days":        {Type: "int", DefaultValue: "14", Description: "Only accounts younger than this many days receive onboarding reminders.", Panel: "control"},
	"onboarding.reminder_min_interval_hours": {Type: "int", DefaultValue: "20", Description: "Minimum hours between onboarding reminder sends.", Panel: "control"},
	"onboarding.reminder_batch_limit":        {Type: "int", DefaultValue: "2000", Description: "Max users processed per onboarding reminder sweep run.", Panel: "control"},
	// General platform settings (exist in DB, now catalogued for type metadata).
	// Public so the main site can render the site name and description in SEO/meta.
	"general.site_name":         {Type: "string", DefaultValue: "JapaConnect", Description: "Platform display name.", Public: true, Panel: "admin"},
	"general.site_description":  {Type: "string", DefaultValue: "", Description: "Short platform description for SEO and meta tags.", Public: true, Panel: "admin"},
	"general.maintenance_mode":  {Type: "bool", DefaultValue: "false", Description: "When true, the platform shows a maintenance banner to non-admin users.", Public: true, Panel: "admin"},
	"general.registration_open": {Type: "bool", DefaultValue: "true", Description: "Whether new user self-registration is allowed.", Public: true, Panel: "admin"},

	// Landing page (Rule 16): the tool card registry is configurable via
	// public settings so the homepage never relies on hardcoded data.
	"landing.tools":                  {Type: "json", DefaultValue: landingToolsDefault, Description: "Public landing-page tool card registry (array of {id, name, slug, description, href, icon, accent?, badge?}).", Public: true, Panel: "admin"},
	"landing.stats":                  {Type: "json", DefaultValue: landingStatsDefault, Description: "Public guest-homepage stats row (array of {value, label, icon}).", Public: true, Panel: "admin"},
	"landing.categories":             {Type: "json", DefaultValue: landingCategoriesDefault, Description: "Public guest-homepage category card grid (array of {id, title, description, href, icon, color}).", Public: true, Panel: "admin"},
	"landing.featured_opportunities": {Type: "json", DefaultValue: landingFeaturedOpportunitiesDefault, Description: "Public guest-homepage featured-opportunity carousel (array of {id, type, title, organization, location, tag, badge?, value?, logo_url?, href, published_at?}).", Public: true, Panel: "admin"},
	"landing.top_destinations":       {Type: "json", DefaultValue: landingTopDestinationsDefault, Description: "Public guest-homepage top-destinations card (array of {id, slug, name, icon_url?}).", Public: true, Panel: "admin"},
	// Appearance / branding — public so the main site can apply primary color
	// and render the logo from the control panel settings.
	"appearance.dark_mode_default": {Type: "bool", DefaultValue: "false", Description: "Default theme for new visitors (false = light).", Public: true, Panel: "admin"},
	"appearance.default_editor":    {Type: "string", DefaultValue: "block", Description: "Default editor mode for the post composer: block | wysiwyg | markdown.", Options: `[{"value":"block","label":"Block"},{"value":"wysiwyg","label":"Rich text"},{"value":"markdown","label":"Markdown"}]`, Public: true, Panel: "admin"},
	"appearance.logo_url":          {Type: "string", DefaultValue: "", Description: "URL to the platform logo image for light theme.", Public: true, Panel: "admin"},
	"appearance.logo_url_dark":     {Type: "string", DefaultValue: "", Description: "URL to the platform logo image for dark theme; falls back to logo_url.", Public: true, Panel: "admin"},
	"appearance.favicon_url":       {Type: "string", DefaultValue: "", Description: "URL to the platform favicon image; empty = use default favicon.svg.", Public: true, Panel: "admin"},
	"appearance.primary_color":     {Type: "string", DefaultValue: "#c615a6", Description: "Primary brand color (hex).", Public: true, Panel: "admin"},
	"appearance.auth_brand_slides": {Type: "json", DefaultValue: "[]", Description: "Carousel slides for the auth brand panel: [{image, headline, subheading}].", Public: true, Panel: "admin"},
	// Documentation portal (docs app) — public so docs.japaconnect.com renders
	// its header chrome, sidebar sections, and landing cards from settings.
	"docs.pinned_pages":       {Type: "json", DefaultValue: "[]", Description: "Get Started pinned pages for the docs portal: [{audience, slug, label?}]. Label falls back to the page title; unresolvable pins are skipped.", Public: true, Panel: "admin"},
	"docs.versions":           {Type: "json", DefaultValue: "[]", Description: "Docs portal version switcher entries: [{label, url}]. Empty hides the version dropdown.", Public: true, Panel: "admin"},
	"docs.changelog_category": {Type: "csv", DefaultValue: "Changelog", Description: "Doc page categories surfaced in the docs portal 'What's new' section.", Public: true, Panel: "admin"},
	"docs.changelog_new_days": {Type: "int", DefaultValue: "14", Min: 0, Description: "Changelog entries newer than this many days get the NEW badge.", Public: true, Panel: "admin"},
	"docs.help_center_url":    {Type: "string", DefaultValue: "", Description: "Help center URL for the docs portal 'Need help' card; empty links to the main app.", Public: true, Panel: "admin"},
	"docs.support_url":        {Type: "string", DefaultValue: "", Description: "Support/contact URL for the docs portal 'Need help' card; empty hides the button.", Public: true, Panel: "admin"},
	"docs.repo_url":           {Type: "string", DefaultValue: "", Description: "Public docs repository URL powering the 'Contribute to Docs' card (suggest changes / report issue); empty hides the card.", Public: true, Panel: "admin"},
	"docs.help_links":         {Type: "json", DefaultValue: "[]", Description: "Docs portal 'Need help' card links: [{label, url}]. Empty/absent falls back to docs.help_center_url/docs.support_url defaults.", Public: true, Panel: "admin"},
	"docs.contribute_links":   {Type: "json", DefaultValue: "[]", Description: "Docs portal 'Contribute to Docs' card links: [{label, url}]. Absent falls back to docs.repo_url defaults; [] hides the card.", Public: true, Panel: "admin"},
	"docs.dev_resources":      {Type: "json", DefaultValue: "[]", Description: "Docs portal developer-resource cards: [{label, url, description?, icon?}]. Absent falls back to the built-in defaults.", Public: true, Panel: "admin"},
	"docs.categories":         {Type: "json", DefaultValue: "[]", Description: "Managed docs-portal category catalog: [{name, description?, icon?, subcategories[]}]. Curates display order/card metadata/subcategory suggestions; page-level taxonomy stays free-text.", Public: true, Panel: "admin"},
	// Content settings (§14, content type system).
	"content.allow_anonymous_posts": {Type: "bool", DefaultValue: "false", Description: "Whether anonymous (logged-out) users can create posts.", Panel: "admin"},
	// Default mirrors the media upload-context allowlist so a materialized
	// default is a no-op; the runtime gate only narrows the base allowlist.
	// Options are limited to extensions the media context can actually
	// permit (document types belong to the document upload context).
	"content.allowed_upload_types":   {Type: "csv", DefaultValue: "jpeg,jpg,png,gif,webp,svg,pdf,mp4,webm,mov,mp3,ogg,wav,m4a", Description: "Comma-separated allowed file extensions for content uploads.", Options: `[{"value":"jpeg","label":"JPEG"},{"value":"jpg","label":"JPG"},{"value":"png","label":"PNG"},{"value":"gif","label":"GIF"},{"value":"webp","label":"WebP"},{"value":"svg","label":"SVG"},{"value":"pdf","label":"PDF"},{"value":"mp4","label":"MP4"},{"value":"webm","label":"WebM"},{"value":"mov","label":"MOV"},{"value":"mp3","label":"MP3"},{"value":"ogg","label":"OGG"},{"value":"wav","label":"WAV"},{"value":"m4a","label":"M4A"}]`, Panel: "admin"},
	"content.max_upload_size_mb":     {Type: "int", DefaultValue: "5", Description: "Maximum upload file size in MB for content attachments.", Panel: "admin"},
	"content.posts_per_page":         {Type: "int", DefaultValue: "12", Description: "Number of posts per page in feed/list views.", Panel: "admin"},
	"content.require_content_review": {Type: "bool", DefaultValue: "true", Description: "Whether new content requires admin review before publishing.", Panel: "admin"},
	// Security settings (§02, authentication).
	// Note: security.lockout_duration_minutes and security.require_email_verification
	// were removed as dead keys — bruteforce backoff is computed from
	// max_login_attempts + fail_window_minutes, and email verification uses
	// auth.email.verify_required (the consumed key).
	// SEO settings.
	// SEO settings managed by the admin panel (not previously catalogued —
	// adding them with Panel: "admin" prevents DB entries from leaking into
	// the control panel).
	"seo.meta_title_template":             {Type: "string", DefaultValue: "", Description: "Template for meta titles.", Panel: "admin"},
	"seo.meta_description_template":       {Type: "string", DefaultValue: "", Description: "Template for meta descriptions.", Panel: "admin"},
	"seo.og_image":                        {Type: "string", DefaultValue: "", Description: "Default Open Graph image URL.", Panel: "admin"},
	"seo.twitter_handle":                  {Type: "string", DefaultValue: "", Description: "Twitter/X handle for meta cards.", Panel: "admin"},
	"seo.json_ld_type":                    {Type: "string", DefaultValue: "Organization", Description: "Default JSON-LD type.", Options: `[{"value":"Organization","label":"Organization"},{"value":"WebSite","label":"WebSite"},{"value":"LocalBusiness","label":"LocalBusiness"}]`, Panel: "admin"},
	"seo.google_analytics_id":             {Type: "string", DefaultValue: "", Description: "Google Analytics tracking ID.", Panel: "admin"},
	"seo.google_tag_manager_id":           {Type: "string", DefaultValue: "", Description: "Google Tag Manager container ID.", Panel: "admin"},
	"seo.sitemap.space_min_members":       {Type: "int", DefaultValue: "1", Min: 0, Description: "Minimum member count for a space to be included in the sitemap (Section 43 quality gate).", Panel: "admin"},
	"seo.sitemap.space_min_posts":         {Type: "int", DefaultValue: "1", Min: 0, Description: "Minimum total post count for a space to be included in the sitemap (Section 43 quality gate).", Panel: "admin"},
	"seo.sitemap.space_min_forums":        {Type: "int", DefaultValue: "1", Min: 0, Description: "Minimum forum thread count for a space to be included in the sitemap (Section 43 quality gate).", Panel: "admin"},
	"seo.sitemap.space_min_questions":     {Type: "int", DefaultValue: "1", Min: 0, Description: "Minimum solved question count for a space to be included in the sitemap (Section 43 quality gate).", Panel: "admin"},
	"seo.sitemap.content_min_words":       {Type: "int", DefaultValue: "100", Min: 0, Description: "Fallback minimum word count for content to be included in the sitemap when no per-type threshold is set (Section 43 quality gate).", Panel: "admin"},
	"seo.sitemap.blog_min_words":          {Type: "int", DefaultValue: "100", Min: 0, Description: "Minimum word count for blog posts (overrides content_min_words for blog_post).", Panel: "admin"},
	"seo.sitemap.forum_min_words":         {Type: "int", DefaultValue: "30", Min: 0, Description: "Minimum word count for forum threads (overrides content_min_words for forum_thread).", Panel: "admin"},
	"seo.sitemap.discussion_min_words":    {Type: "int", DefaultValue: "50", Min: 0, Description: "Minimum word count for discussions (overrides content_min_words for discussion).", Panel: "admin"},
	"seo.sitemap.guide_min_words":         {Type: "int", DefaultValue: "200", Min: 0, Description: "Minimum word count for guides (overrides content_min_words for guide).", Panel: "admin"},
	"seo.sitemap.comparison_min_words":    {Type: "int", DefaultValue: "50", Min: 0, Description: "Minimum word count for comparison content (overrides content_min_words for comparison).", Panel: "admin"},
	"seo.sitemap.blog_min_engagement":     {Type: "int", DefaultValue: "0", Min: 0, Description: "Minimum engagement (comments + shares + reactions) for blog posts to be included in the sitemap.", Panel: "admin"},
	"seo.sitemap.forum_min_replies":       {Type: "int", DefaultValue: "1", Min: 0, Description: "Minimum reply count for forum threads to be included in the sitemap.", Panel: "admin"},
	"seo.sitemap.question_min_answers":    {Type: "int", DefaultValue: "1", Min: 0, Description: "Minimum answer count for questions (solved forum threads) — set to 0 to include unsolved questions.", Panel: "admin"},
	"seo.sitemap.discussion_min_comments": {Type: "int", DefaultValue: "0", Min: 0, Description: "Minimum comment count for discussions to be included in the sitemap.", Panel: "admin"},
	// User profile settings.
	"users.allowed_avatar_types":       {Type: "csv", DefaultValue: "png,jpg,jpeg,gif,webp,svg", Description: "Comma-separated allowed file extensions for user avatars.", Options: `[{"value":"png","label":"PNG"},{"value":"jpg","label":"JPG"},{"value":"jpeg","label":"JPEG"},{"value":"gif","label":"GIF"},{"value":"webp","label":"WebP"},{"value":"svg","label":"SVG"}]`, Panel: "admin"},
	"users.allow_social_login":         {Type: "bool", DefaultValue: "false", Description: "Whether OAuth social login providers are enabled.", Panel: "admin"},
	"users.require_profile_completion": {Type: "bool", DefaultValue: "false", Description: "Whether users must complete their profile before posting.", Panel: "admin"},
	"users.max_avatar_size_mb":         {Type: "int", DefaultValue: "5", Description: "Max user avatar file size in MB.", Panel: "admin"},

	// ---------------------------------------------------------------------------
	// V2 Settings (JEOS §§60–78, ADRs 061–086)
	// All v2 keys are Panel: "control" — they are infrastructure-level toggles,
	// caps, schedules, and subdomain/domain configs managed by super admins.
	// Ref: .jeos/conventions/system-settings-catalog.md (V2 Settings section)
	// ---------------------------------------------------------------------------

	// RAG / JapaBot Knowledge Layer (§69, ADR-069)
	"rag.enabled":          {Type: "bool", DefaultValue: "true", Description: "Global RAG toggle — falls back to non-grounded LLM if false.", Panel: "control"},
	"rag.embedding_model":  {Type: "string", DefaultValue: "text-embedding-3-small", Description: "Embedding model name (via AI gateway).", Panel: "control"},
	"rag.retrieval_top_k":  {Type: "int", DefaultValue: "5", Description: "Number of chunks retrieved per query.", Panel: "control", Min: 1, Max: 50},
	"rag.chunk_max_tokens": {Type: "int", DefaultValue: "400", Description: "Max tokens per chunk during indexing.", Panel: "control", Min: 1, Max: 8000},

	// AI Content Studio (§75, ADR-076)
	"studio.enabled":                    {Type: "bool", DefaultValue: "true", Description: "Global studio toggle.", Panel: "control"},
	"studio.max_concurrent_jobs":        {Type: "int", DefaultValue: "3", Description: "Max parallel generation jobs (AI gateway throttle).", Panel: "control"},
	"studio.auto_publish_types":         {Type: "csv", DefaultValue: "", Description: "Content types that skip review (empty = all reviewed).", Panel: "control"},
	"studio.default_persona_id":         {Type: "string", DefaultValue: "", Description: "Pre-selected persona UUID in new-job form.", Panel: "control"},
	"studio.auto_promote_intel_records": {Type: "bool", DefaultValue: "false", Description: "When true, high-confidence intel_records are automatically promoted to Studio jobs for editorial generation (ADR-076 Amendment 1).", Panel: "control"},

	// Participation Orchestration (§76, ADR-077)
	"participation.enabled":              {Type: "bool", DefaultValue: "true", Description: "Global kill-switch for all persona activity.", Panel: "control"},
	"participation.run_interval_minutes": {Type: "int", DefaultValue: "60", Description: "Orchestration loop schedule (minutes).", Panel: "control"},
	"participation.global_daily_cap":     {Type: "int", DefaultValue: "500", Description: "Max total synthetic actions/day across all personas.", Panel: "control"},

	// Visa Document Checklist (§70, ADR-070)
	"checklist.enabled":           {Type: "bool", DefaultValue: "true", Description: "Global checklist tool toggle.", Panel: "control"},
	"checklist.completion_points": {Type: "int", DefaultValue: "0", Description: "Points on full completion (0 = disabled).", Panel: "control"},
	"checklist.subdomain":         {Type: "string", DefaultValue: "visa-checklist", Description: "Standalone subdomain (ADR-086).", Panel: "control"},
	"checklist.custom_domain":     {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// Japa Readiness Assessment (§71, ADR-071)
	"assessment.enabled":            {Type: "bool", DefaultValue: "true", Description: "Global assessment toggle.", Panel: "control"},
	"assessment.ai_roadmap_enabled": {Type: "bool", DefaultValue: "true", Description: "AI-assisted roadmap generation.", Panel: "control"},
	"assessment.completion_points":  {Type: "int", DefaultValue: "50", Description: "Points on completion.", Panel: "control"},
	"assessment.subdomain":          {Type: "string", DefaultValue: "readiness", Description: "Standalone subdomain (ADR-086).", Panel: "control"},
	"assessment.custom_domain":      {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// Cost of Living Calculator (§72, ADR-072)
	"col_calculator.enabled":       {Type: "bool", DefaultValue: "true", Description: "Global calculator toggle.", Panel: "control"},
	"col_calculator.stale_days":    {Type: "int", DefaultValue: "180", Description: "Days before \"data may be outdated\" notice.", Panel: "control"},
	"col_calculator.max_cities":    {Type: "int", DefaultValue: "3", Description: "Max destination cities per comparison.", Panel: "control"},
	"col_calculator.subdomain":     {Type: "string", DefaultValue: "cost-of-living", Description: "Standalone subdomain (ADR-086).", Panel: "control"},
	"col_calculator.custom_domain": {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// Scholarship Finder (§73, ADR-073)
	"scholarship_finder.enabled":               {Type: "bool", DefaultValue: "true", Description: "Global finder toggle.", Panel: "control"},
	"scholarship_finder.default_reminder_days": {Type: "int", DefaultValue: "14", Description: "Default days-before-deadline reminder.", Panel: "control"},
	"scholarship_finder.completion_points":     {Type: "int", DefaultValue: "10", Description: "Points for saving first scholarship.", Panel: "control"},
	"scholarship_finder.subdomain":             {Type: "string", DefaultValue: "scholarships", Description: "Standalone subdomain (ADR-086).", Panel: "control"},
	"scholarship_finder.custom_domain":         {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// Community Story Wall (§79, ADR-087)
	"story.enabled":       {Type: "bool", DefaultValue: "false", Description: "Global story wall toggle.", Panel: "control"},
	"story.subdomain":     {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"story.custom_domain": {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// Destination Wiki (§80, ADR-088)
	"wiki.enabled":                   {Type: "bool", DefaultValue: "false", Description: "Global wiki toggle.", Panel: "control"},
	"wiki.community_edits":           {Type: "bool", DefaultValue: "true", Description: "Allow community edits when wiki is enabled.", Panel: "control"},
	"wiki.min_reputation_to_edit":    {Type: "int", DefaultValue: "100", Description: "Reputation score required to submit a wiki edit.", Panel: "control"},
	"wiki.rollup_default_rows_limit": {Type: "int", DefaultValue: "50", Min: 1, Description: "Fallback cap on rollup descendant rows when a category's rollup.section_limit is unset or ≤0 (§80).", Panel: "control"},

	// Scam Alert Board (§81, ADR-089)
	"scam_alert.enabled":           {Type: "bool", DefaultValue: "false", Description: "Global scam alert board toggle.", Panel: "control"},
	"scam_alert.anonymous_reports": {Type: "bool", DefaultValue: "true", Description: "Allow anonymous scam reports.", Panel: "control"},

	// Japa Planning Groups (§82, ADR-090)
	"planning_group.enabled": {Type: "bool", DefaultValue: "false", Description: "Global planning groups toggle.", Panel: "control"},

	// Visa Timeline Tracker (§83, ADR-091)
	"visa_timeline.enabled":       {Type: "bool", DefaultValue: "false", Description: "Global visa timeline tracker toggle.", Panel: "control"},
	"visa_timeline.subdomain":     {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086, §93).", Panel: "control"},
	"visa_timeline.custom_domain": {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086, §93).", Panel: "control"},

	// Community Digest Widget (§84, ADR-092)
	"digest.enabled": {Type: "bool", DefaultValue: "false", Description: "Global community digest toggle.", Panel: "control"},
	"digest.email":   {Type: "bool", DefaultValue: "true", Description: "Send digest via email when published.", Panel: "control"},

	// Mutual Aid Board (§85, ADR-093)
	"mutual_aid.enabled":                   {Type: "bool", DefaultValue: "false", Description: "Global mutual aid board toggle.", Panel: "control"},
	"mutual_aid.need_reputation_threshold": {Type: "int", DefaultValue: "0", Description: "Minimum reputation (lifetime points) required to create a need post. 0 disables the gate.", Panel: "control"},

	// Community Calendar (§86, ADR-094)
	"calendar.enabled": {Type: "bool", DefaultValue: "false", Description: "Global community calendar toggle.", Panel: "control"},

	// Japa Stage Progress Profiles (§87, ADR-095)
	"japa_stage.enabled": {Type: "bool", DefaultValue: "false", Description: "Global japa stage profiles toggle.", Panel: "control"},
	"japa_stage.max_journeys": {Type: "int", DefaultValue: "10", Description: "Max non-deleted user_journeys rows a member may hold; POST /me/japa-stage/journeys returns journey_limit_reached at the cap (ADR-129).", Panel: "control"},

	// Ask an Expat AMA (§88, ADR-096)
	"forum.ama.enabled": {Type: "bool", DefaultValue: "false", Description: "Global AMA thread format toggle.", Panel: "control"},
	"ama_cooldown_days": {Type: "int", DefaultValue: "30", Description: "Minimum days between AMA sessions per user.", Panel: "control"},

	// Community Achievement Feed (§89, ADR-097)
	"achievement.enabled":   {Type: "bool", DefaultValue: "false", Description: "Global achievement feed toggle.", Panel: "control"},
	"achievement_reactions": {Type: "json", DefaultValue: `[{"emoji":"🎉","label":"Celebrate"},{"emoji":"🔥","label":"Inspired"},{"emoji":"💪","label":"Motivated"}]`, Description: "Curated reaction set for achievements (emoji + label pairs).", Panel: "control"},

	// Service Directory (§98, ADR-106)
	"service_directory.enabled":       {Type: "bool", DefaultValue: "false", Description: "Global service directory toggle.", Panel: "control"},
	"service_directory.subdomain":     {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"service_directory.custom_domain": {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// V2 functional tools (§§90–97, §§99–101) — geo-aware tool pattern
	// (ADR-088). Each toggle gates a tool on the Tools web surface (§78).
	// visa_timeline.enabled is catalogued above (§83/§93); the remaining
	// ten are registered here so the control-panel settings page exposes
	// form fields and the /extensions Tools tab resolves their defaults.
	"visa_routes.enabled":                     {Type: "bool", DefaultValue: "false", Description: "Global visa route comparison tool toggle.", Panel: "control"},
	"visa_routes.subdomain":                   {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"visa_routes.custom_domain":               {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"appointment_alert.enabled":               {Type: "bool", DefaultValue: "false", Description: "Global embassy appointment alerter toggle.", Panel: "control"},
	"appointment_alert.premium_poll_interval": {Type: "int", DefaultValue: "300", Description: "Polling interval (seconds) for embassies with premium-tier alerts (5 min default vs 15 min standard). §91, ADR-099.", Panel: "control"},
	"appointment_alert.subdomain":             {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"appointment_alert.custom_domain":         {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"arrival_checklist.enabled":               {Type: "bool", DefaultValue: "false", Description: "Global arrival checklist tool toggle.", Panel: "control"},
	"arrival_checklist.subdomain":             {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"arrival_checklist.custom_domain":         {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"credential_checker.enabled":              {Type: "bool", DefaultValue: "false", Description: "Global professional credential checker toggle.", Panel: "control"},
	"credential_checker.subdomain":            {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"credential_checker.custom_domain":        {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"cv_formatter.enabled":                    {Type: "bool", DefaultValue: "false", Description: "Global CV formatter tool toggle.", Panel: "control"},
	"cv_formatter.subdomain":                  {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"cv_formatter.custom_domain":              {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"cv_formatter.quota_standard":             {Type: "int", DefaultValue: "3", Description: "Monthly CV formatting quota for standard members (§95).", Panel: "control"},
	"housing_guide.enabled":                   {Type: "bool", DefaultValue: "false", Description: "Global housing guide tool toggle.", Panel: "control"},
	"housing_guide.subdomain":                 {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"housing_guide.custom_domain":             {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"attestation_guide.enabled":               {Type: "bool", DefaultValue: "false", Description: "Global attestation guide tool toggle.", Panel: "control"},
	"attestation_guide.subdomain":             {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"attestation_guide.custom_domain":         {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"language_test_hub.enabled":               {Type: "bool", DefaultValue: "false", Description: "Global language test hub tool toggle.", Panel: "control"},
	"language_test_hub.subdomain":             {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"language_test_hub.custom_domain":         {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"tax_calculator.enabled":                  {Type: "bool", DefaultValue: "false", Description: "Global tax calculator tool toggle.", Panel: "control"},
	"tax_calculator.subdomain":                {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"tax_calculator.custom_domain":            {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},
	"community_locator.enabled":               {Type: "bool", DefaultValue: "false", Description: "Global diaspora community locator toggle.", Panel: "control"},
	"community_locator.subdomain":             {Type: "string", DefaultValue: "", Description: "Optional standalone subdomain (ADR-086).", Panel: "control"},
	"community_locator.custom_domain":         {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// Remittance Rate Tracker (§74, ADR-075)
	"remittance.enabled":                {Type: "bool", DefaultValue: "true", Description: "Global tracker toggle.", Panel: "control"},
	"remittance.fetch_interval_minutes": {Type: "int", DefaultValue: "30", Description: "API rate-fetch schedule (minutes).", Panel: "control"},
	"remittance.manual_stale_hours":     {Type: "int", DefaultValue: "24", Description: "Hours before manual rate staleness warning.", Panel: "control"},
	"remittance.history_retention_days": {Type: "int", DefaultValue: "365", Description: "Days of rate history to retain.", Panel: "control"},
	"remittance.default_send_currency":  {Type: "string", DefaultValue: "GBP", Description: "Pre-selected send currency.", Panel: "control"},
	"remittance.subdomain":              {Type: "string", DefaultValue: "remittance", Description: "Standalone subdomain (ADR-086).", Panel: "control"},
	"remittance.custom_domain":          {Type: "string", DefaultValue: "", Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// FX Currency Converter (§112, ADR-125)
	// Public: true on the two host keys is load-bearing: IsPublicSetting feeds
	// is_public on admin upserts, and apps/tools host detection reads only
	// public settings.
	"fx.enabled":                {Type: "bool", DefaultValue: "true", Description: "Global FX converter toggle.", Panel: "control"},
	"fx.cache_ttl_minutes":      {Type: "int", DefaultValue: "120", Min: 5, Description: "Valkey TTL for the mid-market rate matrix (minutes).", Panel: "control"},
	"fx.stale_hours":            {Type: "int", DefaultValue: "48", Min: 1, Description: "Hours before a served FX matrix is flagged stale.", Panel: "control"},
	"fx.history_retention_days": {Type: "int", DefaultValue: "90", Min: 7, Description: "Days of fx_rates matrix history to retain.", Panel: "control"},
	"fx.default_base_currency":  {Type: "string", DefaultValue: "USD", Description: "Default base currency for /fx/rates.", Panel: "control"},
	"fx.subdomain":              {Type: "string", DefaultValue: "currency-converter", Public: true, Description: "Standalone subdomain (ADR-086).", Panel: "control"},
	"fx.custom_domain":          {Type: "string", DefaultValue: "", Public: true, Description: "Optional custom domain (ADR-086).", Panel: "control"},

	// Referral System (§60, ADR-078)
	"referral.enabled":                 {Type: "bool", DefaultValue: "true", Description: "Global toggle.", Panel: "control"},
	"referral.reward_credits":          {Type: "int", DefaultValue: "500", Description: "Credits awarded to referrer on qualification.", Panel: "control"},
	"referral.attribution_window_days": {Type: "int", DefaultValue: "30", Description: "Attribution TTL (days).", Panel: "control"},
	"referral.max_rewards_per_period":  {Type: "int", DefaultValue: "10", Description: "Monthly cap per referrer (fraud control).", Panel: "control"},

	// Self-Serve Advertising (§61, ADR-079)
	"ads.self_serve_enabled":      {Type: "bool", DefaultValue: "true", Description: "Global self-serve toggle.", Panel: "control"},
	"ads.min_campaign_days":       {Type: "int", DefaultValue: "3", Description: "Minimum campaign duration (days).", Panel: "control"},
	"ads.credit_refund_on_reject": {Type: "bool", DefaultValue: "true", Description: "Auto-refund credits on admin rejection.", Panel: "control"},

	// Ad placement rendering (§25 amendment) — Public: frontend components
	// (FeedList, InArticleAd) read these via /site/settings/public.
	"ads.native_feed_interval":       {Type: "int", DefaultValue: "5", Public: true, Description: "Insert a native ad after every N feed items (0 disables).", Panel: "control"},
	"ads.in_article_after_paragraph": {Type: "int", DefaultValue: "3", Public: true, Description: "Paragraph index for the first in-article ad.", Panel: "control"},
	"ads.in_article_every":           {Type: "int", DefaultValue: "6", Public: true, Description: "Repeat in-article ad every N paragraphs (0 = once).", Panel: "control"},

	// Newsletter & Digest (§62, ADR-080)
	"newsletter.enabled":         {Type: "bool", DefaultValue: "true", Description: "Global toggle.", Panel: "control"},
	"newsletter.digest_enabled":  {Type: "bool", DefaultValue: "true", Description: "Weekly digest toggle.", Panel: "control"},
	"newsletter.digest_day":      {Type: "int", DefaultValue: "1", Description: "Day of week (1 = Monday).", Panel: "control"},
	"newsletter.digest_hour_utc": {Type: "int", DefaultValue: "8", Description: "Hour UTC to send digest.", Panel: "control"},
	"newsletter.batch_size":      {Type: "int", DefaultValue: "500", Description: "Subscribers per send batch.", Panel: "control"},

	// Jobs Board (§63, ADR-081)
	"jobs.enabled":          {Type: "bool", DefaultValue: "true", Description: "Global jobs board toggle.", Panel: "control"},
	"jobs.featured_credits": {Type: "int", DefaultValue: "1000", Description: "Credits to feature a listing.", Panel: "control"},

	// Gamification (§64, ADR-082)
	"gamification.badges_enabled":      {Type: "bool", DefaultValue: "true", Description: "Badge system toggle.", Panel: "control"},
	"gamification.challenges_enabled":  {Type: "bool", DefaultValue: "true", Description: "Challenge system toggle.", Panel: "control"},
	"gamification.leaderboard_enabled": {Type: "bool", DefaultValue: "true", Description: "Leaderboard visibility toggle.", Panel: "control"},

	// Content Import (§65, ADR-083)
	"import.max_items_per_run": {Type: "int", DefaultValue: "50", Description: "Max posts imported per batch.", Panel: "control"},

	// Advanced Analytics (§66, ADR-084)
	"analytics.export_enabled": {Type: "bool", DefaultValue: "true", Description: "Enable CSV export.", Panel: "control"},

	// Travel Search (§67, ADR-085)
	"travel.enabled":           {Type: "bool", DefaultValue: "false", Description: "Off until a provider is configured.", Panel: "control"},
	"travel.cache_ttl_seconds": {Type: "int", DefaultValue: "300", Description: "Result cache TTL (seconds).", Panel: "control"},

	// Mobile Push (§77, ADR-074) — seeded for forward compatibility; not yet
	// consumed by Go code. Web push is gated by JC_VAPID_* env vars instead.
	"push.enabled":             {Type: "bool", DefaultValue: "false", Description: "Global push notification toggle (FCM/APNs for mobile; web push is gated by VAPID env vars).", Panel: "control"},
	"push.fcm_server_key":      {Type: "string", DefaultValue: "", Description: "FCM legacy server key (deprecated by Google June 2024; use push.fcm_service_account instead).", Sensitive: true, Panel: "control"},
	"push.fcm_service_account": {Type: "string", DefaultValue: "", Description: "FCM service account JSON for the HTTP v1 API (JSON key file content).", Sensitive: true, Panel: "control"},
	"push.apns_key_id":         {Type: "string", DefaultValue: "", Description: "APNs key ID (10-char hex).", Sensitive: true, Panel: "control"},
	"push.apns_team_id":        {Type: "string", DefaultValue: "", Description: "Apple Team ID (10-char hex).", Sensitive: true, Panel: "control"},
	"push.apns_private_key":    {Type: "string", DefaultValue: "", Description: "APNs private key PEM (p8, ES256).", Sensitive: true, Panel: "control"},

	// Tools Web Surface (§78, ADR-086)
	"tools.subdomain":       {Type: "string", DefaultValue: "tools", Description: "Subdomain for the tools hub.", Panel: "control"},
	"tools.custom_domain":   {Type: "string", DefaultValue: "", Description: "Custom domain for the tools hub.", Panel: "control"},
	"japabot.subdomain":     {Type: "string", DefaultValue: "japabot", Description: "JapaBot standalone subdomain.", Panel: "control"},
	"japabot.custom_domain": {Type: "string", DefaultValue: "", Description: "JapaBot custom domain.", Panel: "control"},

	// Mentorship Matching (§102, ADR-110)
	"mentorship.max_capacity": {Type: "int", DefaultValue: "10", Min: 1, Description: "Maximum mentee capacity per mentor (platform-wide cap).", Panel: "control"},

	// Security Scan Thresholds (§40, Rule 3)
	"security.scan.failed_login_threshold":       {Type: "int", DefaultValue: "50", Min: 0, Description: "Failed logins in 1h that triggers a warning finding.", Panel: "control"},
	"security.scan.sla_breach_threshold":         {Type: "int", DefaultValue: "5", Min: 0, Description: "Open tickets with breached SLA that triggers a critical finding.", Panel: "control"},
	"security.scan.stale_tokens_threshold":       {Type: "int", DefaultValue: "100", Min: 0, Description: "Unrevoked refresh tokens older than 30d that triggers a warning.", Panel: "control"},
	"security.scan.unverified_users_threshold":   {Type: "int", DefaultValue: "10", Min: 0, Description: "Users with unverified email older than 7d that triggers an info finding.", Panel: "control"},
	"security.scan.min_password_length":          {Type: "int", DefaultValue: "8", Min: 0, Description: "Recommended minimum password length for the weak-password scan.", Panel: "control"},
	"security.scan.excessive_api_keys_threshold": {Type: "int", DefaultValue: "20", Min: 0, Description: "Active API keys without expiration that triggers an info finding.", Panel: "control"},

	// Content Import (§65, ADR-083)
	"dataimport.max_file_mb": {Type: "int", DefaultValue: "100", Min: 1, Description: "Max import file size in MB (Section 67 platform data imports).", Panel: "control"},

	// Mobile Push (§77, ADR-074)
	"push.apns_bundle_id": {Type: "string", DefaultValue: "", Description: "APNs topic (app bundle ID, e.g. com.japaconnect.app).", Sensitive: true, Panel: "control"},

	// V2 Community Features & Tools (§§79–101)
	"visa_timeline.min_account_age_days": {Type: "int", DefaultValue: "30", Min: 0, Description: "Min account age to contribute to processing time aggregate (§83).", Panel: "control"},
	"visa_timeline.min_reputation":       {Type: "int", DefaultValue: "50", Min: 0, Description: "Min reputation to contribute to processing time aggregate (§83).", Panel: "control"},
	"calendar.categories":                {Type: "json", DefaultValue: `[{"value":"visa_window","label":"Visa window"},{"value":"embassy_closure","label":"Embassy closure"},{"value":"application_deadline","label":"Application deadline"},{"value":"exam_window","label":"Exam window"},{"value":"immigration_office","label":"Immigration office"},{"value":"other","label":"Other"}]`, Description: "Allowed calendar entry categories (§86). Array of {value, label} objects.", Panel: "control"},
	"attestation_guide.hague_members":    {Type: "json", DefaultValue: "[]", Description: "JSON array of destination space IDs that are Hague Convention members (§97).", Panel: "control"},

	// Claim Ownership (§07)
	"claim_ownership.form_slug":   {Type: "string", DefaultValue: "claim-ownership", Description: "Slug of the Form Builder form used for ownership claims (§07).", Panel: "control"},
	"claim_ownership.fee_credits": {Type: "int", DefaultValue: "0", Min: 0, Description: "Override fee for claim submissions in credits; 0 = use form's submission_fee_credits (§07).", Panel: "control"},

	// Webhooks (§12)
	"webhooks.event_catalog": {Type: "json", DefaultValue: "", Description: "JSON array of {key,label,description} objects defining the available webhook event types. Empty = use the built-in default catalog (§12).", Panel: "control"},

	// Intelligence System (§INTEL, ADR-112)
	"intel.enabled":                           {Type: "bool", DefaultValue: "true", Description: "Global intelligence system toggle.", Panel: "control"},
	"intel.schedule_batch_size":               {Type: "int", DefaultValue: "50", Min: 1, Max: 1000, Description: "Max sources selected per scheduler tick.", Panel: "control"},
	"intel.extraction_confidence_auto":        {Type: "float", DefaultValue: "0.85", Min: 0, Max: 1, Description: "Auto-publish threshold (confidence ≥ this AND source Tier 1-2 → AUTO_PUBLISH).", Panel: "control"},
	"intel.extraction_confidence_review":      {Type: "float", DefaultValue: "0.50", Min: 0, Max: 1, Description: "Review threshold (confidence ≥ this but below auto → REVIEW_REQUIRED).", Panel: "control"},
	"intel.extraction_confidence_reject":      {Type: "float", DefaultValue: "0.50", Min: 0, Max: 1, Description: "Reject threshold (confidence below this from Tier 3+ → reject).", Panel: "control"},
	"intel.extraction_max_retries":            {Type: "int", DefaultValue: "3", Min: 0, Max: 10, Description: "Malformed JSON retries before routing to REVIEW_REQUIRED.", Panel: "control"},
	"intel.extraction_model":                  {Type: "string", DefaultValue: "gpt-4o-mini", Description: "Extraction model via §20 AI gateway (cost-tuned; switchable).", Panel: "control"},
	"intel.freshness_decay_scholarship":       {Type: "string", DefaultValue: "slow", Description: "Freshness policy per entity type: slow | medium | fast.", Options: `[{"value":"slow","label":"Slow"},{"value":"medium","label":"Medium"},{"value":"fast","label":"Fast"}]`, Panel: "control"},
	"intel.credential.enabled":                {Type: "bool", DefaultValue: "true", Description: "Enable the credential_pathways intelligence domain.", Panel: "control"},
	"intel.credential.freshness_decay_policy": {Type: "string", DefaultValue: "slow", Description: "Freshness policy for credential_pathways: slow | medium | fast.", Options: `[{"value":"slow","label":"Slow"},{"value":"medium","label":"Medium"},{"value":"fast","label":"Fast"}]`, Panel: "control"},
	"intel.crawl_lock_ttl_secs":               {Type: "int", DefaultValue: "3600", Min: 60, Description: "Crawl lock TTL (prevents concurrent crawls of same source).", Panel: "control"},
	"intel.ratelimit_default_per_min":         {Type: "int", DefaultValue: "10", Min: 1, Description: "Default rate-limit tokens per domain per minute.", Panel: "control"},
	"intel.radar.default_page_size":           {Type: "int", DefaultValue: "20", Min: 1, Max: 100, Description: "Default page size for radar feed/counts endpoints (INTEL-016).", Panel: "control"},
	"intel.radar.max_page_size":               {Type: "int", DefaultValue: "100", Min: 1, Max: 1000, Description: "Maximum page size for radar feed/counts endpoints (INTEL-016).", Panel: "control"},
	"intel.radar.max_followed_spaces":         {Type: "int", DefaultValue: "100", Min: 1, Max: 1000, Description: "Max followed spaces returned by /intel/radar/followed (INTEL-016).", Panel: "control"},
	"intel.scholarships.max_page_size":        {Type: "int", DefaultValue: "50", Min: 1, Max: 1000, Description: "Maximum page size for /intel/scholarships search (INTEL-016).", Panel: "control"},
	"intel.default_persona_id":                {Type: "string", DefaultValue: "", Description: "Fallback AI persona whose backing bot user is stamped as contents.author_id when an intel source has no explicit persona_id.", Panel: "control"},
	"intel.radar.mixed_fetch_page_multiplier": {Type: "int", DefaultValue: "10", Min: 1, Description: "Multiplier on max page size for the mixed 'latest' source-items feed fetch before in-memory pagination (seeded in 000663).", Panel: "control"},
	// Intel channels — probe (Phase 1, spec §2)
	"intel.probe.max_requests":        {Type: "int", DefaultValue: "5", Min: 1, Description: "Preview probe request budget for POST /sources/probe (entry GET + detector fetches).", Panel: "control"},
	"intel.probe.verify_max_requests": {Type: "int", DefaultValue: "25", Min: 1, Description: "Verify-stage request budget for intel:probe_verify (sitemap walk + category-path probes + feed re-checks).", Panel: "control"},
	"intel.probe.max_urls":            {Type: "int", DefaultValue: "50", Min: 1, Description: "Max page URLs the verify prober's sitemap walk collects before fetching.", Panel: "control"},
	"intel.probe.rate_limit_per_hr":   {Type: "int", DefaultValue: "20", Min: 1, Description: "Per-user hourly quota for POST /sources/probe preview probes.", Panel: "control"},
	// Intel channels — category map + curate/relevance gates (Phase 1, spec §2/§4)
	"intel.category_method_map":                   {Type: "json", DefaultValue: `{"job":{"entity_types":["job"],"methods":["jsonld","api","tabular","render","static"],"pipeline":"extract","topics":["job","hiring","vacancy","career","remote work","recruitment"]},"internship":{"entity_types":["job","internship"],"methods":["jsonld","api","tabular","render","static"],"pipeline":"extract","topics":["internship","placement","co-op","graduate program","industrial attachment","trainee"]},"work_career":{"entity_types":["job","work_career"],"methods":["jsonld","api","tabular","render","static"],"pipeline":"extract","topics":["career","employment","work abroad","skilled worker","job seeker","labour market"]},"news":{"entity_types":["news"],"methods":["rss","jsonld","render","static"],"pipeline":"curate","topics":["migration","immigration","japa","diaspora","visa","relocation","travel"]},"blog":{"entity_types":["article"],"methods":["rss","render","static"],"pipeline":"curate","topics":["japa","relocation","study abroad","settlement","expat","diaspora"]},"event":{"entity_types":["event"],"methods":["ical","jsonld"],"pipeline":"curate","topics":["webinar","fair","embassy","orientation","deadline"]},"scholarship":{"entity_types":["scholarship"],"methods":["rss","jsonld","render","static"],"pipeline":"extract","topics":["scholarship","grant","fellowship","funding","tuition waiver","bursary"]},"visa":{"entity_types":["visa"],"methods":["static","pdf","render"],"pipeline":"extract","topics":["visa","immigration","embassy","residence permit","asylum","border"]},"dataset":{"entity_types":["dataset"],"methods":["tabular","git","api"],"pipeline":"curate","topics":["exchange rate","processing time","visa fee","tuition"]},"video":{"entity_types":[],"methods":["channel_sync"],"pipeline":"curate","topics":["japa","relocation","study abroad","visa"]}}`, Description: "Category→method map (spec §2): per-category {entity_types, methods, pipeline, topics}. Empty or malformed resolves against the embedded default (internal/intelligence/sources/category_map.json).", Panel: "control"},
	"intel.curate.auto_publish_trust_max":         {Type: "int", DefaultValue: "2", Min: 0, Max: 5, Description: "Max source trust_level whose curated records auto-publish; less-trusted sources route to review.", Panel: "control"},
	"intel.relevance.on_topic_threshold":          {Type: "float", DefaultValue: "0.15", Min: 0, Max: 1, Description: "Deterministic-match score cutoff — items scoring ≥ this are on_topic.", Panel: "control"},
	"intel.relevance.off_topic_threshold":         {Type: "float", DefaultValue: "0.03", Min: 0, Max: 1, Description: "Deterministic-reject score cutoff — items scoring below this are off_topic.", Panel: "control"},
	"intel.relevance.borderline_rate_cap":         {Type: "int", DefaultValue: "50", Min: 0, Description: "Max borderline-band classifier calls per channel per day; 0 disables the band's AI call (fail-open).", Panel: "control"},
	"intel.relevance.classifier_enabled":          {Type: "bool", DefaultValue: "true", Description: "Enable the borderline-band AI classifier in the relevance gate.", Panel: "control"},
	"intel.relevance.classifier_model":            {Type: "string", DefaultValue: "", Description: "Model override for the relevance classifier; empty = category-default routing via the §20 AI gateway.", Panel: "control"},
	"intel.consecutive_failure_disable_threshold": {Type: "int", DefaultValue: "5", Min: 1, Description: "Consecutive failed channel crawls before the channel auto-disables.", Panel: "control"},
	// Intel channels — Phase 2/3 placeholders (spec §System settings; no
	// Phase 1 consumer — registered so the keys are documented/editable).
	"intel.render.service_url":       {Type: "string", DefaultValue: "", Description: "Headless renderer endpoint for render-method channels (Phase 2 adapter; empty = disabled).", Panel: "control"},
	"intel.render.timeout_ms":        {Type: "int", DefaultValue: "30000", Min: 0, Description: "Render request timeout in milliseconds (Phase 2 adapter).", Panel: "control"},
	"intel.search.provider":          {Type: "string", DefaultValue: "", Description: "Search provider for the SERP crawl adapter: brave | google_cse | serpapi (Phase 2/3; empty = disabled).", Panel: "control"},
	"intel.search.api_key":           {Type: "string", DefaultValue: "", Sensitive: true, Description: "API key for intel.search.provider (Phase 2/3; stored as a normal setting — rotate via the settings UI).", Panel: "control"},
	"intel.email.inbound_domain":     {Type: "string", DefaultValue: "", Description: "Inbound email domain for the Phase 3 email inbound adapter (empty = disabled).", Panel: "control"},
	"intel.email.allowed_senders":    {Type: "json", DefaultValue: "[]", Description: "JSON array of sender addresses/domains allowed to submit intel items via the Phase 3 email inbound adapter.", Panel: "control"},
	"intel.social.enabled_providers": {Type: "json", DefaultValue: "[]", Description: "JSON array of social providers enabled for the Phase 3 social intel adapter (e.g. reddit, telegram).", Panel: "control"},
	// Intel channels — Phase 4 (spider + outbound source discovery)
	"intel.spider.max_children_per_crawl": {Type: "int", DefaultValue: "25", Min: 1, Description: "Cap on derived child fetches a changed spider manifest may enqueue per crawl (provider_config.max_urls overrides per channel).", Panel: "control"},
	"intel.discovery.enabled":             {Type: "bool", DefaultValue: "true", Description: "Outbound source discovery toggle — when false, intel:harvest_outbound enqueues are skipped and the handler no-ops.", Panel: "control"},
	"intel.discovery.max_trust_level":     {Type: "int", DefaultValue: "2", Min: 1, Max: 5, Description: "Only sources at or above this trust tier (lower = more trusted) contribute outbound-link source candidates.", Panel: "control"},
	"intel.discovery.domain_blocklist":    {Type: "json", DefaultValue: "[]", Description: "JSON array of hostnames never auto-proposed as sources (suffix match); supplements the built-in share/widget denylist.", Panel: "control"},
	"intel.discovery.max_per_crawl":       {Type: "int", DefaultValue: "10", Min: 1, Description: "Max proposed sources a single intel:harvest_outbound task may create.", Panel: "control"},
	"intel.rejected_retention_days":       {Type: "int", DefaultValue: "7", Min: 0, Description: "Days a rejected intel_records row is kept before the daily intel:prune_rejected_records sweep deletes it; 0 disables the sweep.", Panel: "control"},

	// Generic Comparison System (§30, ADR-030)
	"comparison.auto_dimension_hints":         {Type: "json", DefaultValue: `{"default":["Overview","Key features","Pros","Cons","Overall rating"],"groups":[{"hints":["price","cost","plan","package","version","edition","model"],"dimensions":["Price","Value for money"],"prepend":true},{"hints":["course","university","school","program","degree","certification"],"dimensions":["Curriculum","Duration","Cost","Accreditation"],"prepend":false}]}`, Description: "Heuristic hints used by POST /comparisons/auto-dimensions to derive dimension suggestions from item names. Admin-configurable per platform.", Panel: "control"},
	"appearance.feed_card_style":              {Type: "string", DefaultValue: "daily", Description: `Default feed card style: ` + "`" + `reddit` + "`" + `|` + "`" + `compact` + "`" + `|` + "`" + `grid` + "`" + `|` + "`" + `magazine` + "`" + `|` + "`" + `daily` + "`" + ``, Panel: "admin", Public: true},
	"campaign.enabled":                        {Type: "bool", DefaultValue: "true", Description: "Global campaign toggle", Panel: "control"},
	"campaign.max_count":                      {Type: "int", DefaultValue: "50", Description: "Max pieces per campaign", Panel: "control"},
	"content.pagination.style":                {Type: "string", DefaultValue: "infinite_scroll", Description: `Default feed/list pagination style: ` + "`" + `infinite_scroll` + "`" + `|` + "`" + `load_more` + "`" + `|` + "`" + `pagination` + "`" + ``, Panel: "admin"},
	"feed.follower_fanout_cap":                {Type: "int", DefaultValue: "1000", Description: "Max author followers receiving a push feed row on publish (BUG-006). Authors with more followers have their fan-out truncated to the most recent N.", Panel: "admin"},
	"feed.for_you_affinity_cap":               {Type: "int", DefaultValue: "300", Description: "Max affinity-sourced candidates in the for-you pool (BUG-008)", Panel: "admin"},
	"feed.for_you_cache_ttl_seconds":          {Type: "int", DefaultValue: "90", Description: "For-you ranked feed cache TTL in seconds (BUG-002)", Panel: "admin"},
	"feed.for_you_candidate_cap":              {Type: "int", DefaultValue: "600", Description: "Total candidate cap after union of affinity + velocity pools (BUG-008)", Panel: "admin"},
	"feed.for_you_geo_boost_cap":              {Type: "float", DefaultValue: "1.5", Description: "Maximum multiplier applied to base score for geo-matched content (ADR-064, BUG-007)", Panel: "admin"},
	"feed.for_you_geo_weight":                 {Type: "float", DefaultValue: "1.0", Description: "Geo boost weight added when content space country matches visitor country (ADR-064, BUG-007)", Panel: "admin"},
	"feed.for_you_half_life_hours":            {Type: "float", DefaultValue: "48.0", Description: "Recency decay half-life in hours for for-you ranking (BUG-007)", Panel: "admin"},
	"feed.for_you_journey_boost_cap":          {Type: "float", DefaultValue: "1.8", Description: "Maximum combined journey multiplier applied to a candidate score", Panel: "admin"},
	"feed.for_you_journey_weight_destination": {Type: "float", DefaultValue: "1.5", Description: "Journey boost for content whose target country matches the visitor's destination", Panel: "admin"},
	"feed.for_you_journey_weight_roadmap":     {Type: "float", DefaultValue: "1.4", Description: "Journey boost for content whose slug appears in the readiness roadmap", Panel: "admin"},
	"feed.for_you_journey_weight_source":      {Type: "float", DefaultValue: "0.8", Description: "Journey boost for content whose source country matches the visitor's origin", Panel: "admin"},
	"feed.for_you_journey_weight_stage":       {Type: "float", DefaultValue: "1.2", Description: "Journey boost for content whose journey stage matches the visitor's stage", Panel: "admin"},
	"feed.for_you_journey_weight_visa":        {Type: "float", DefaultValue: "1.0", Description: "Journey boost for content whose visa route matches the visitor's visa route", Panel: "admin"},
	"feed.for_you_tag_overlap_cap":            {Type: "int", DefaultValue: "3", Description: "Cap on followed-tag overlap count when computing the tag signal (BUG-007)", Panel: "admin"},
	"feed.for_you_velocity_cap":               {Type: "int", DefaultValue: "300", Description: "Max velocity-sourced candidates in the for-you pool (BUG-008)", Panel: "admin"},
	"feed.for_you_weight_author":              {Type: "float", DefaultValue: "3.0", Description: "For-you ranking weight for followed-author signal (BUG-007)", Panel: "admin"},
	"feed.for_you_weight_recency":             {Type: "float", DefaultValue: "2.0", Description: "For-you ranking weight for recency decay (BUG-007)", Panel: "admin"},
	"feed.for_you_weight_space":               {Type: "float", DefaultValue: "2.5", Description: "For-you ranking weight for followed-space signal (BUG-007)", Panel: "admin"},
	"feed.for_you_weight_tag":                 {Type: "float", DefaultValue: "1.5", Description: "For-you ranking weight for followed-tag overlap signal (BUG-007)", Panel: "admin"},
	"feed.for_you_weight_velocity":            {Type: "float", DefaultValue: "1.0", Description: "For-you ranking weight for normalized velocity (BUG-007)", Panel: "admin"},
	"feed.for_you_window_days":                {Type: "int", DefaultValue: "30", Description: "For-you candidate pool lookback window in days (BUG-003)", Panel: "admin"},
	"feed.max_page_size":                      {Type: "int", DefaultValue: "100", Description: "Maximum page size accepted by feed list endpoints (BUG-012)", Panel: "admin"},
	"feed.prune_interval_days":                {Type: "int", DefaultValue: "30", Description: "Age threshold in days for pruning old feed rows (BUG-010)", Panel: "admin"},
	"feed.top_window_days":                    {Type: "int", DefaultValue: "7", Description: "Rolling window in days for the top sort mode (BUG-011)", Panel: "admin"},
	"feed.velocity_window_days":               {Type: "int", DefaultValue: "30", Description: "Lookback window in days for velocity score recalculation (BUG-009)", Panel: "admin"},
	"gdpr.policy_version":                     {Type: "string", DefaultValue: "1.0", Description: "Current privacy policy version stamped on consent records (BUG-GDPR-005)", Panel: "admin"},
	"geo_personalization.default_country":     {Type: "string", DefaultValue: "NG", Description: "Fallback ISO 3166-1 alpha-2 code for visitors with no resolvable geo signal (GeoIP, preference, or archetype)", Panel: "control"},
	"geoip.anonymous_ip_enabled":              {Type: "bool", DefaultValue: "false", Description: `Master switch for VPN/anonymous-IP detection via GeoLite2-Anonymous-IP. When false (default), ` + "`" + `is_vpn` + "`" + ` in ` + "`" + `/geo/me` + "`" + ` and ` + "`" + `/geo/lookup` + "`" + ` is always ` + "`" + `false` + "`" + ` (GeoLite2-City carries no anonymous-IP data). When true, a second ` + "`" + `.mmdb` + "`" + ` is loaded and ` + "`" + `is_vpn` + "`" + ` is populated from the anonymous-IP record (Section 37)`, Panel: "control"},
	"jce.comparisons.page_size_options":       {Type: "json", DefaultValue: `[12,24,48,96]`, Description: "Allowed page-size options for JCE comparison lists.", Panel: "control"},
	"jce.comparisons.sort_options":            {Type: "json", DefaultValue: `["created_at","title","popularity"]`, Description: "Allowed sort fields for JCE comparison lists.", Panel: "control"},
	"jce.freshness_ai_estimate_weight":        {Type: "int", DefaultValue: "50", Description: "Weight numerator for AI-estimate sources in freshness scoring.", Panel: "control"},
	"jce.freshness_cache_ttl_minutes":         {Type: "int", DefaultValue: "10", Description: "Freshness summary Valkey TTL in minutes.", Panel: "control"},
	"jce.freshness_decay_per_hour":            {Type: "int", DefaultValue: "2", Description: "Freshness score decay per hour after a value becomes stale.", Panel: "control"},
	"jce.freshness_official_weight":           {Type: "int", DefaultValue: "100", Description: "Weight numerator for official sources in freshness scoring.", Panel: "control"},
	"jce.freshness_stale_hours":               {Type: "int", DefaultValue: "168", Description: "Hours before a comparison value is considered stale.", Panel: "control"},
	"jce.freshness_third_party_weight":        {Type: "int", DefaultValue: "75", Description: "Weight numerator for third-party sources in freshness scoring.", Panel: "control"},
	"jce.refresh_lock_ttl_minutes":            {Type: "int", DefaultValue: "5", Description: `Valkey TTL for the ` + "`" + `jc:jce:refresh_lock:*` + "`" + ` distributed lock.`, Panel: "control"},
	"jce.selection_token_ttl_minutes":         {Type: "int", DefaultValue: "15", Description: "Guest comparison selection token lifetime in minutes (Rule 3).", Panel: "control"},
	"jce.stale_refresh_sweep_hours":           {Type: "int", DefaultValue: "24", Description: "Minimum hours between completed refresh jobs for the same comparison in the stale sweep.", Panel: "control"},
	"jce.stale_refresh_sweep_limit":           {Type: "int", DefaultValue: "1000", Description: "Maximum number of comparisons to enqueue per stale refresh sweep.", Panel: "control"},
	"moderation.new_user_min_posts_for_links": {Type: "int", DefaultValue: "3", Description: "Min total posts before a new user may include external links (\u00a741 Layer 3)", Panel: "admin"},
	"moderation.report_reasons":               {Type: "json", DefaultValue: `["spam","harassment","hate_speech","misinformation","illegal_content","self_harm","copyright","other"]`, Description: "Available report reason codes for the report dialog (Rule 16 compliance)", Panel: "admin"},
	"organizations.max_per_user":              {Type: "int", DefaultValue: "5", Description: "Max organizations a user can create (0 = none, -1 = unlimited)", Panel: "admin"},
	"page.cache_ttl_secs":                     {Type: "int", DefaultValue: "300", Description: `TTL for the published-page Valkey cache (` + "`" + `jc:page:{the id}:{slug}` + "`" + `)`, Panel: "admin"},
	"persona.max_batch_size":                  {Type: "int", DefaultValue: "100", Description: "Max personas a single POST /admin/personas request can provision (BUG-006: caps batch to prevent long-tx DoS)", Panel: "control"},
	// Public Product Roadmap (§113, ADR-127). submissions_enabled and
	// custom_domain are Public so apps/main can gate the submit form and run
	// host detection off /site/settings/public (fx.custom_domain precedent).
	"roadmap.submissions_enabled":     {Type: "bool", DefaultValue: "true", Public: true, Description: "Public feature-request submissions open (§113).", Panel: "control"},
	"roadmap.custom_domain":           {Type: "string", DefaultValue: "", Public: true, Description: "Extra hostnames that render the roadmap surface (comma-separated; checked by apps/main host detection).", Panel: "control"},
	"roadmap.board_cache_ttl_seconds": {Type: "int", DefaultValue: "60", Description: `TTL for the public board Valkey cache (` + "`" + `jc:roadmap:board` + "`" + `).`, Panel: "control"},
	"roadmap.shipped_window_days":     {Type: "int", DefaultValue: "90", Description: "Days a shipped item stays on the public board before only appearing on the changelog.", Panel: "control"},
	"site.custom_css":                 {Type: "string", DefaultValue: "", Description: `Raw CSS injected inside ` + "`" + `<style id="jc-custom-css">` + "`" + ` in ` + "`" + `<head>` + "`" + `. Max 50 KB.`, Panel: "admin"},
	"site.custom_js":                  {Type: "string", DefaultValue: "", Description: `Raw JS injected as ` + "`" + `<script id="jc-custom-js">` + "`" + ` before ` + "`" + `</body>` + "`" + `. Max 100 KB. Renders ONLY when site.custom_js_approved is true (super_admin-gated two-person approval).`, Panel: "admin"},
	"site.custom_js_approved":         {Type: "bool", DefaultValue: "false", Public: true, Description: "Two-person approval for site.custom_js: a second super_admin must set this true before the JS renders. Fail-closed default — unapproved JS is inert. Public so the SSR layout can read the gate.", Panel: "admin"},
	"trending.expiry.24h_hours":       {Type: "int", DefaultValue: "2", Description: "TTL (hours) for 24h trending snapshots", Panel: "admin"},
	"trending.expiry.30d_hours":       {Type: "int", DefaultValue: "32", Description: "TTL (hours) for 30d trending snapshots", Panel: "admin"},
	"trending.expiry.7d_hours":        {Type: "int", DefaultValue: "8", Description: "TTL (hours) for 7d trending snapshots", Panel: "admin"},
	"trending.gravity":                {Type: "float", DefaultValue: "1.8", Description: "HN gravity exponent for trending velocity (ADR-032)", Panel: "admin"},
	"trending.weight.bookmarks":       {Type: "float", DefaultValue: "3", Description: "Content/user velocity: bookmarks multiplier", Panel: "admin"},
	"trending.weight.comments":        {Type: "float", DefaultValue: "5", Description: "Content/user velocity: comments multiplier", Panel: "admin"},
	"trending.weight.shares":          {Type: "float", DefaultValue: "8", Description: "Content/user velocity: shares multiplier", Panel: "admin"},
	"trending.weight.space_member":    {Type: "float", DefaultValue: "1", Description: "Space trending: member count multiplier", Panel: "admin"},
	"trending.weight.space_post":      {Type: "float", DefaultValue: "2", Description: "Space trending: post count multiplier", Panel: "admin"},
	"trending.weight.space_recent":    {Type: "float", DefaultValue: "5", Description: "Space trending: recent post count multiplier", Panel: "admin"},
	"trending.weight.tag_content":     {Type: "float", DefaultValue: "3", Description: "Tag trending: recent content count multiplier", Panel: "admin"},
	"trending.weight.tag_follower":    {Type: "float", DefaultValue: "1", Description: "Tag trending: follower count multiplier", Panel: "admin"},
	"trending.weight.views":           {Type: "float", DefaultValue: "1", Description: "Content/user velocity: views multiplier", Panel: "admin"},
	"trending.weight.votes":           {Type: "float", DefaultValue: "10", Description: "Content/user velocity: votes multiplier", Panel: "admin"},
	"users.username_cooldown_days":    {Type: "int", DefaultValue: "30", Description: "Minimum days between username changes (\u00a750)", Panel: "admin"},
	"wallet.credit_purchase_enabled":  {Type: "bool", DefaultValue: "true", Description: `Killswitch for credit purchasing. When false, ` + "`" + `POST /wallet/purchase-credits` + "`" + ` returns 403; spending existing credits still works. Ref: Section 58.`, Panel: "admin"},
	"wallet.default_currency":         {Type: "string", DefaultValue: "USD", Description: "Fallback currency code when user has no country/preference (ISO 4217). Ref: Section 58, ADR-065.", Panel: "admin"},
	"wallet.min_purchase_credits":     {Type: "int", DefaultValue: "100", Description: "Minimum credits for a custom-amount credit purchase. Ref: Section 58.", Panel: "admin"},
	// Video (§106, ADR-117)
	// Public because the main /create composer and studio surfaces gate
	// themselves on this setting (Rule 16).
	"video.enabled":                       {Type: "bool", DefaultValue: "true", Public: true, Description: "Feature flag for the video content type and /create composer.", Panel: "admin"},
	"video.import.allowed_platforms":      {Type: "json", DefaultValue: `["youtube","tiktok","instagram","vimeo"]`, Description: "Platform allowlist for single URL import.", Panel: "admin"},
	"video.import.youtube_api_key":        {Type: "string", DefaultValue: "", Description: "YouTube Data API v3 key for bulk imports (server-only).", Sensitive: true, Panel: "admin"},
	"video.import.instagram_access_token": {Type: "string", DefaultValue: "", Description: "Instagram oEmbed access token for Reel/post imports (server-only).", Sensitive: true, Panel: "admin"},
	"video.import.max_items_per_run":      {Type: "int", DefaultValue: "50", Description: "Max videos resolved per bulk import run.", Panel: "admin"},
	"video.ai_summary.enabled":            {Type: "bool", DefaultValue: "false", Public: true, Description: "Allow users to enqueue AI-generated drafts from video metadata.", Panel: "admin"},
	"video.claim_form_slug":               {Type: "string", DefaultValue: "video-channel-claim", Description: "Form Builder slug for claim/takedown evidence.", Panel: "admin"},
	"video.channel_sync.enabled":          {Type: "bool", DefaultValue: "true", Description: "Master switch for the recurring video_channel Atom-feed sync task.", Panel: "admin"},
	"video.channel_sync.interval_minutes": {Type: "int", DefaultValue: "360", Description: "Minimum minutes between syncs of the same channel source.", Panel: "admin"},
	"video.channel_sync.batch_size":       {Type: "int", DefaultValue: "50", Description: "Max channels processed per video:sync_channels run.", Panel: "admin"},
	"video.channel_sync.auto_publish":     {Type: "bool", DefaultValue: "true", Description: "Publish synced videos as contents when the channel author has a default space.", Panel: "admin"},
	// List (listicle) content type — §14 list amendment.
	// Public: the /create composer and /lists index gate on this setting
	// (same role as video.enabled). Missing = enabled.
	"list.enabled":   {Type: "bool", DefaultValue: "true", Public: true, Description: "Killswitch for the list content type, /lists index, and /create composer.", Panel: "admin"},
	"list.max_items": {Type: "int", DefaultValue: "50", Min: 1, Max: 500, Description: "Maximum number of items allowed in a list content.", Panel: "admin"},
}

// IsPublicSetting reports whether the given dot-notation key is marked Public
// in the catalog. Other packages (e.g. admin) use this to set is_public on upsert.
func IsPublicSetting(key string) bool {
	if meta, ok := settingsCatalog[key]; ok {
		return meta.Public
	}

	return false
}

// getSettingPanel returns the panel assignment ("admin" or "control") for a
// given dot-notation key. Empty string means unclassified (shown in both panels).
func getSettingPanel(key string) string {
	if meta, ok := settingsCatalog[key]; ok {
		return meta.Panel
	}

	return ""
}

// deprecatedReachabilitySettings maps system_settings reachability toggles
// migrated to feature_flags (Task 13 — unified rollout) to the flag key that
// now owns them. These keys remain catalogued for backward-compatible reads
// but are excluded from the editable settings list and carry a DEPRECATED
// description prefix pointing at the owning flag.
var deprecatedReachabilitySettings = map[string]string{
	"ai.japabot_enabled":          "feature.japabot",
	"agent.enabled":               "feature.ai_agent",
	"achievement.enabled":         "achievement.enabled",
	"appointment_alert.enabled":   "appointment_alert.enabled",
	"arrival_checklist.enabled":   "arrival_checklist.enabled",
	"assessment.enabled":          "assessment.enabled",
	"attestation_guide.enabled":   "attestation_guide.enabled",
	"calendar.enabled":            "calendar.enabled",
	"campaign.enabled":            "campaign.enabled",
	"checklist.enabled":           "checklist.enabled",
	"col_calculator.enabled":      "col_calculator.enabled",
	"community_locator.enabled":   "community_locator.enabled",
	"credential_checker.enabled":  "credential_checker.enabled",
	"crs_calculator.enabled":      "crs_calculator.enabled",
	"cv_formatter.enabled":        "cv_formatter.enabled",
	"digest.enabled":              "digest.enabled",
	"eca_guide.enabled":           "eca_guide.enabled",
	"forum.ama.enabled":           "forum.ama.enabled",
	"fx.enabled":                  "fx.enabled",
	"geo_personalization.enabled": "geo_personalization.enabled",
	"housing_guide.enabled":       "housing_guide.enabled",
	"intel.enabled":               "intel.enabled",
	"japa_stage.enabled":          "japa_stage.enabled",
	"jobs.enabled":                "jobs.enabled",
	"language_test_hub.enabled":   "language_test_hub.enabled",
	"list.enabled":                "list.enabled",
	"mutual_aid.enabled":          "mutual_aid.enabled",
	"newsletter.enabled":          "feature.newsletter",
	"participation.enabled":       "participation.enabled",
	"planning_group.enabled":      "planning_group.enabled",
	"pof_calculator.enabled":      "pof_calculator.enabled",
	"push.enabled":                "push.enabled",
	"rag.enabled":                 "rag.enabled",
	"referral.enabled":            "feature.referral",
	"remittance.enabled":          "remittance.enabled",
	"scam_alert.enabled":          "scam_alert.enabled",
	"scholarship_finder.enabled":  "scholarship_finder.enabled",
	"service_coupons.enabled":     "service_coupons.enabled",
	"service_directory.enabled":   "service_directory.enabled",
	"service_packages.enabled":    "service_packages.enabled",
	"sop_generator.enabled":       "sop_generator.enabled",
	"story.enabled":               "story.enabled",
	"studio.enabled":              "studio.enabled",
	"tax_calculator.enabled":      "tax_calculator.enabled",
	"travel.enabled":              "travel.enabled",
	"video.enabled":               "video.enabled",
	"visa_routes.enabled":         "visa_routes.enabled",
	"visa_timeline.enabled":       "visa_timeline.enabled",
}

// isDeprecatedSettingKey reports whether key is a migrated reachability
// toggle superseded by a feature_flags row. Unlike the Deprecated flag on
// settingMeta — which only fires for catalogued keys — this covers every
// entry in deprecatedReachabilitySettings, including keys with no catalog
// entry (a stored system_settings row still exists for some of them).
func isDeprecatedSettingKey(key string) bool {
	_, dep := deprecatedReachabilitySettings[key]
	return dep
}

// getSettingMeta returns the metadata for a given dot-notation key.
// If the key is not in the catalog, it returns a best-effort guess based on
// the value content (bool-like, number-like, etc.) or falls back to "string".
func getSettingMeta(key, currentValue string) settingMeta {
	if meta, ok := settingsCatalog[key]; ok {
		if flagKey, dep := deprecatedReachabilitySettings[key]; dep {
			meta.Deprecated = true
			meta.Description = "DEPRECATED — managed via feature_flags/" + flagKey + " — " + meta.Description
		}

		return meta
	}
	// No catalog entry — return a minimal fallback with no type.
	return settingMeta{
		Type:         "",
		DefaultValue: "",
		Description:  "",
	}
}

// validateSettingValue checks a raw string value against the catalog metadata
// for the given key. It verifies the type (int/float/bool) and, when the
// catalog declares Min/Max bounds, enforces them. Returns an error when the
// value is invalid, nil otherwise. Uncatalogued keys (empty Type) and
// string/csv/json types always pass — only numeric and bool types are checked.
func validateSettingValue(key, raw string) error {
	meta, ok := settingsCatalog[key]
	if !ok || meta.Type == "" {
		return nil
	}

	switch meta.Type {
	case "int", "float":
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return fmt.Errorf("setting %q must be a valid number", key)
		}

		n, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return fmt.Errorf("setting %q must be a valid number", key)
		}

		if meta.Type == "int" && n != float64(int64(n)) {
			return fmt.Errorf("setting %q must be a whole number", key)
		}

		if meta.Min != 0 || meta.Max != 0 {
			if n < meta.Min {
				return fmt.Errorf("setting %q must be >= %v", key, meta.Min)
			}

			if meta.Max > 0 && n > meta.Max {
				return fmt.Errorf("setting %q must be <= %v", key, meta.Max)
			}
		}
	case "bool":
		unquoted := strings.Trim(raw, `"`)

		lower := strings.ToLower(unquoted)
		if lower != "true" && lower != "false" {
			return fmt.Errorf("setting %q must be true or false", key)
		}
	}

	return nil
}
