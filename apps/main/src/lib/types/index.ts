/**
 * Shared domain + transport types for the Main App.
 * Shapes mirror the Go backend's JSON contract (verified against the live API).
 */

export type ThemeName = 'light' | 'dark';

/** Supported UI locale code (e.g. 'en', 'fr', 'yo'). Dynamic per ADR-013. */
export type Locale = string;

/* ------------------------------------------------------------------ *
 * Transport envelope
 * ------------------------------------------------------------------ */

/** Success envelope: every endpoint wraps its payload in `{ data }`. */
export interface ApiOk<T> {
	data: T;
	meta?: ListMeta;
	/** Optional offset pagination totals for list endpoints (e.g. /me/media). */
	total?: number;
	total_pages?: number;
	/** Keyset pagination sidecars emitted by cursor-paginated endpoints (§48 DM lists). */
	next_cursor?: string | null;
	has_more?: boolean;
	/** Optional JSON-LD structured data emitted by some GET endpoints (e.g. forum thread detail). */
	json_ld?: unknown;
	/** Optional capability flag emitted by some GET endpoints (e.g. forum can_solve). */
	can_solve?: boolean;
	/** Optional capability flag emitted by forum thread detail (can_edit). */
	can_edit?: boolean;
	/** Optional scope discriminator (e.g. /wiki/categories: 'geo' | 'university'). */
	scope?: string;
	/** Optional capability flag emitted by forum thread detail (can_delete). */
	can_delete?: boolean;
	/** Optional moderator flag for edit/delete any replies. */
	can_moderate?: boolean;
	/** Optional bookmark state emitted by forum thread detail (is_bookmarked + bookmark_id). */
	is_bookmarked?: boolean;
	bookmark_id?: string;
	/** Optional list of erased user UUIDs emitted by content/forum detail
	 * endpoints for GDPR/ADR-029 mention redaction (Section 15). */
	deleted_user_ids?: string[];
	/** Optional map of entity_id -> current user's vote value (1, -1, 0)
	 * emitted by forum detail and reply list endpoints (BUG-006). */
	user_votes?: Record<string, number>;
	/** Optional resolved template-literal map emitted by page and content endpoints. */
	literals?: Record<string, string>;
}

/** Error envelope: `{ error: { code, message } }`. */
export interface ApiErr {
	error: { code: string; message: string; status?: number };
}

export interface ListMeta {
	total?: number;
	page?: number;
	limit?: number;
	total_pages?: number;
	next_cursor?: string | null;
	has_more?: boolean;
	sort_options?: string[];
	page_size_options?: number[];
}

/* ------------------------------------------------------------------ *
 * Session / user
 * ------------------------------------------------------------------ */

export interface SessionUser {
	id: string;
	email: string;
	username: string;
	display_name?: string | null;
	avatar_url?: string | null;
	groups?: string[];
	/** Effective RBAC permission nodes resolved by the backend (§26). Present
	 *  on /me so frontend pages gate UI on the actual node (e.g.
	 *  `forum.ama.create`) instead of hardcoded role names (Rule 16). */
	permissions?: string[];
	primary_space?: SpaceRef | null;
	/** UI preferences persisted via PATCH /me/account, e.g. nav_collapsed. */
	ui?: { nav_collapsed?: boolean } & Record<string, unknown>;
	/** Geo-aware feed personalization country (ADR-064). Empty = use GeoIP default. */
	content_country_preference?: string;
	/** The user's current geo space ID (migration 000200). Used by sidebar
	 *  widgets to fetch geo-relevant content (Option A, ADR-064). Null for
	 *  guests or users who haven't set a geo space. */
	current_geo_space_id?: string | null;
}

export interface SpaceRef {
	id: string;
	slug: string;
	name: string;
}

/** GET /me/spaces item — a space the user belongs to. Lean shape for the nav switcher. */
export interface UserSpaceRef {
	id: string;
	slug: string;
	name: string;
}

/* ------------------------------------------------------------------ *
 * Navigation menus (ADR-054) — GET /menus/{slug}
 * ------------------------------------------------------------------ */

export type MenuPlacement = 'sidebar' | 'bottom_bar' | 'dropdown' | 'footer' | 'header' | 'dashboard_rail';
export type MenuVisibility = 'all' | 'authenticated' | 'guest';

export interface MenuItem {
	id: string;
	label: string;
	url: string;
	/** Lucide icon name in kebab-case, e.g. "graduation-cap". */
	icon: string | null;
	sort_order: number;
	visibility: MenuVisibility;
	target: '_self' | '_blank';
	permission_node?: string | null;
	is_ad_slot: boolean;
	ad_slot_config?: Record<string, unknown> | null;
	status: 'active' | 'inactive';
	/** Optional feature flag key; the item is hidden when the flag is disabled. */
	feature_flag?: string | null;
	children?: MenuItem[];
}

export interface Menu {
	id: string;
	slug: string;
	name: string;
	placement: MenuPlacement;
	items: MenuItem[];
}

/* ------------------------------------------------------------------ *
 * Right-sidebar widget data — GET /suggestions/*
 * ------------------------------------------------------------------ */

export interface TagSuggestion {
	id: string;
	slug: string;
	name: string;
	post_count?: number;
}

/* ------------------------------------------------------------------ *
 * Tag detail — GET /tags/{slug} (public)
 * ------------------------------------------------------------------ */

export interface TagDetail {
	id: string;
	slug: string;
	name: string;
	description?: string | null;
	rules?: string | null;
	color?: string | null;
	logo_url?: string | null;
	logo_media_id?: string | null;
	cover_url?: string | null;
	cover_media_id?: string | null;
	field_values?: Record<string, unknown> | null;
	/** Tag entity-class custom field definitions, embedded by
	 *  GET /tags/{slug} so guests can render labeled field_values. */
	field_defs?: Array<{
		key: string;
		label: string;
		field_type: string;
		placeholder?: string | null;
		default_value?: string | null;
		is_required: boolean;
		show_in_frontend?: boolean;
	}> | null;
	follower_count: number;
	created_at: string;
	updated_at: string;
}

export interface SpaceSuggestion {
	id: string;
	slug: string;
	name: string;
	member_count?: number;
	icon_url?: string | null;
}

/* ------------------------------------------------------------------ *
 * Feed items — GET /feed (auth) and GET /explore (guest)
 * ------------------------------------------------------------------ */

/** Lean organization badge surfaced on feed/profile cards. */
export interface FeedOrganization {
	id: string;
	name: string;
	slug: string;
	logo_url?: string | null;
}

/** Source attribution badge surfaced on feed cards for promoted content. */
export interface FeedSource {
	id: string;
	name: string;
	slug: string;
	logo_url?: string | null;
	source_url: string;
	trust_level: number;
}

export type ContentType =
	| 'blog_post'
	| 'discussion'
	| 'question'
	| 'forum_thread'
	| 'comparison'
	| 'list'
	| string;

export interface FeedItem {
	id: string;
	title: string;
	slug: string;
	content_type: ContentType;
	author_id: string;
	author_name?: string | null;
	author_username?: string | null;
	author_avatar_url?: string | null;
	space_id?: string | null;
	space_name?: string | null;
	space_slug?: string | null;
	space_avatar_url?: string | null;
	excerpt?: string;
	created_at: string;
	published_at?: string;
	view_count?: number;
	comment_count?: number;
	vote_score?: number;
	velocity_score?: number;
	reaction_count?: number;
	bookmark_count?: number;
	reading_time_minutes?: number;
	organization?: FeedOrganization | null;
	cover_image_url?: string | null;
	tags?: string[];
	// Journey-aware recommendation metadata surfaced on feed cards (§32).
	journey_stage?: string | null;
	visa_route?: string | null;
	source_country?: string | null;
	target_country?: string | null;
	/** §14 guide amendment — category slug when content_type === 'guide'. */
	guide_category?: string | null;
	/** For forum threads, the forum_threads.id (item.id is the backing
	 *  contents.id). The vote API keys on the thread id; feed surfaces leave
	 *  this unset and resolve it lazily from the slug. */
	forum_thread_id?: string | null;
	/** §106 video — linked video_sources row and clip length when the feed
	 *  query surfaces them (used for the play overlay / duration badge). */
	video_source_id?: string | null;
	duration_secs?: number | null;
	source?: FeedSource;
}

/* ------------------------------------------------------------------ *
 * Composer (/create) — Section 14/24/30
 * ------------------------------------------------------------------ */

export type Visibility = 'public' | 'members' | 'private';

/** GET /content-types item. */
export interface ContentTypeMeta {
	value: string;
	label: string;
	has_category?: boolean;
	categories?: GuideCategory[];
}

/** A guide category (Visa, Travel, Scholarship, etc.) — admin-configurable. */
export interface GuideCategory {
	Slug: string;
	Label: string;
}

/** GET /me/spaces item (the caller's joined spaces). */
export interface MeSpace {
	id: string;
	slug: string;
	name: string;
	is_primary?: boolean;
	level?: string;
	space_type_id?: string;
}

/** GET /me/muted-spaces item. */
export interface MutedSpace {
	id: string;
	space_id: string;
	space_slug: string;
	space_name: string;
	is_primary?: boolean;
	created_at: string;
	level?: string;
	space_type_id?: string;
}

/** One custom-field definition (from GET /custom-field-groups/{id}). */
export interface CustomFieldDef {
	id: string;
	key: string;
	label: string;
	field_type: string; // text|textarea|rich_text|number|boolean|date|select|suggest|multiselect|rating|url|...
	is_required: boolean;
	/** Options list for select/suggest/multiselect; object config for
	 *  repeater ({item_fields:[{key,label,type?}]}), number ({min,max,...}). */
	options?: (string | { value: string; label: string })[] | Record<string, unknown> | null;
	placeholder?: string | null;
	help_text?: string | null;
}

/** A custom-field group scoped to an entity type (the composer keeps entity_type === "content"). */
export interface CustomFieldGroup {
	id: string;
	name: string;
	slug?: string;
	entity_type: string;
	fields: CustomFieldDef[];
}

/** GET /blog/series item. */
export interface BlogSeries {
	id: string;

	slug: string;
	title: string;
	description?: string | null;
	cover_image_id?: string | null;
	cover_image_url?: string | null;
	author_user_id?: string;
	status?: string;
	created_at?: string;
	updated_at?: string;
}

/** Content record returned by POST /content and PATCH /content/{id}. */
export interface ContentRecord {
	id: string;
	slug: string;
	content_type: string;
	title: string;
	body?: string;
	body_html?: string;
	field_values?: Record<string, unknown>;
	canonical_url?: string | null;
	header_image_id?: string | null;
	featured_image_id?: string | null;
	/** §106 video — the resolved/uploaded video_sources row this content
	 *  embeds (content_type === 'video'). */
	video_source_id?: string | null;
	meta_title?: string | null;
	meta_description?: string | null;
	og_image_url?: string | null;
	seo_indexable?: boolean;
	organization_id?: string | null;
	status: string;
	visibility: string;
	publish_at?: string | null;
	space_id?: string | null;
	guide_category?: string | null;
	// Journey-aware recommendation metadata (§14 / 2026-08-21 journey design).
	journey_stage?: string | null;
	visa_route?: string | null;
	source_country?: string | null;
	target_country?: string | null;
}

/**
 * §106 video — a canonical video_sources row. Returned by
 * POST /videos/resolve, POST /videos/upload, GET /videos/{id}, and
 * GET /u/{username}/videos. `source_type` is one of youtube, vimeo,
 * tiktok, instagram_reel, upload, other.
 */
export interface VideoSource {
	id: string;

	/** §106 — public slug used by the canonical /video/{slug} route. */
	slug?: string;
	author_user_id?: string;
	/** §106 — the intel_sources row this video belongs to (channel source). */
	source_id?: string | null;
	source_type: string;
	source_url: string;
	external_id?: string | null;
	title?: string;
	description?: string;
	category?: string | null;
	thumbnail_url?: string | null;
	thumbnail_media_id?: string | null;
	duration_secs?: number | null;
	status?: string;
	field_values?: Record<string, unknown> | null;
	created_at?: string;
	updated_at?: string;
}

/** Uploaded media (POST /media/upload → confirm, or GET /me/media). */
export interface MediaAsset {
	id: string;
	url: string;
	filename?: string;
	mime_type?: string;
	file_size?: number;
	alt_text?: string | null;
	caption?: string | null;
	width?: number;
	height?: number;
	duration_secs?: number | null;
	status?: string;
	flagged?: boolean;
	created_at?: string;
	user_group_id?: string | null;
	// Stock attribution (present when media was imported from a stock provider)
	source_platform?: string;
	source_photographer?: string;
	source_photographer_url?: string;
	source_license?: string;
	source_license_url?: string;
}

export interface StockProvider {
	name: string;
	display_name: string;
}

export interface StockPhoto {
	id: string;
	provider: string;
	thumbnail_url: string;
	full_url: string;
	width: number;
	height: number;
	photographer: string;
	photographer_url: string;
	license?: string;
	license_url?: string;
	description?: string;
}

export interface StockSearchResult {
	photos: StockPhoto[];
	total: number;
	page: number;
	per_page: number;
	has_next_page: boolean;
}

export interface StockImportResult {
	media_id: string;
}

export interface StockProvidersResponse {
	providers: StockProvider[];
}

/** GET /me/bookmarks — a thin bookmark record (entity_type + entity_id). */
export interface Bookmark {
	id: string;
	entity_type: string;
	entity_id: string;
	folder?: string | null;
	folder_id?: string | null;
	created_at: string;
	title?: string | null;
	slug?: string | null;
	/** doc_page bookmarks only — the doc's audience slug (users, api, …). */
	audience?: string | null;
}

/** GET /me/bookmark-folders — a user's bookmark folder. */
export interface BookmarkFolder {
	id: string;
	name: string;
	position?: number;
	bookmark_count?: number;
	created_at: string;
	updated_at?: string;
}

/** One autocomplete result for @ / [[ / # triggers. */
export interface SearchHit {
	id: string;
	label: string; // display name / title / tag name
	slug?: string;
	handle?: string;
	avatar_url?: string | null;
	/** Public path the reference links to at render time (§15): /u/{handle} for
	 * users, /tags/{slug} for tags, the content path for [[ interlinks. */
	url?: string;
}

/** Autosave/publish lifecycle state. */
export type ComposerStatus =
	| 'pristine'
	| 'creating'
	| 'saved'
	| 'dirty'
	| 'saving'
	| 'published'
	| 'scheduled'
	| 'error';

/** How a failed save should be handled. */
export type SaveErrorClass = 'validation' | 'server' | 'network' | 'conflict';

/* ------------------------------------------------------------------ *
 * User hub (/settings, /dashboard)
 * ------------------------------------------------------------------ */

/** Rail grouping for the settings/dashboard registries. */
export type HubGroup =
	| 'account'
	| 'security'
	| 'notifications'
	| 'billing'
	| 'wallet'
	| 'developer'
	| 'activity'
	| 'danger';

/** Own profile+account (GET /me). Extends SessionUser with editable fields. */
export interface MeProfile {
	id: string;
	email: string;
	username: string;
	display_name?: string | null;
	bio?: string | null;
	/** IANA timezone — PATCH /me/profile accepts it (backend user_profiles.timezone). */
	timezone?: string | null;
	avatar_url?: string | null;
	cover_url?: string | null;
	primary_space?: SpaceRef | null;
	ui?: { nav_collapsed?: boolean; theme?: string } & Record<string, unknown>;
	/** ADR-064a: ISO 3166-1 alpha-2 origin country code (custom field). */
	origin_country?: string | null;
	/** ADR-064a: origin state space id (custom field). */
	origin_state_id?: string | null;
	/** ADR-064a: origin city space id (custom field). */
	origin_city_id?: string | null;
	/** ADR-064a: ISO 3166-1 alpha-2 destination country code (custom field). */
	destination_country?: string | null;
}

/** GET/PATCH /me/privacy — a map of privacy toggles. */
export interface PrivacySettings {
	profile_visibility?: 'public' | 'members' | 'followers';
	dm_privacy?: 'everyone' | 'followers_only' | 'nobody';
	show_online_status?: boolean;
	show_activity?: boolean;
	search_indexable?: boolean;
	/** Section 28 — opt in to show Points balance on public profile. Credits are never exposed. */
	show_points_publicly?: boolean;
	[key: string]: unknown;
}

// ─── Auth (Section 02) ───────────────────────────────────────────────
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md

/** Response from POST /auth/login and POST /auth/register. */
export interface AuthSession {
	totp_required?: undefined;
	access_token: string;
	/** Refresh token extracted from the backend Set-Cookie header; set as
	 * the jc_refresh browser cookie by the SSR proxy so /me/sessions can
	 * identify the current session (JEOS §50). */
	refresh_token?: string;
	user: AuthUser;
}

/** Response from POST /auth/login when the account has TOTP enabled — the
 * backend returns a challenge token instead of an access token; the client
 * must collect a 6-digit code and POST /auth/totp/verify-challenge. */
export interface AuthTotpChallenge {
	totp_required: true;
	challenge_token: string;
	user: AuthUser;
	access_token?: undefined;
}

/** Minimal user shape returned by the auth endpoints (register/login). */
export interface AuthUser {
	id: string;
	email: string;
	username: string;
}

/** Machine-readable error codes from the auth API, mapped to field-level messages. */
export type AuthErrorCode =
	| 'missing_fields'
	| 'invalid_credentials'
	| 'email_exists'
	| 'username_exists'
	| 'weak_password'
	| 'password_breached'
	| 'invalid_body'
	| 'service_unavailable'
	| 'unsupported_provider'
	| 'state_error'
	| 'invalid_state'
	| 'missing_email'
	| 'create_failed'
	| 'token_error'
	| 'missing_provider'
	| 'rate_limited';

/** Onboarding status from GET /onboarding/status. */
export interface OnboardingStatus {
	current_step: number;
	completed: boolean;
	step_data: unknown;
	tour_dismissed: boolean;
	/** Per-surface tour dismissals — { "main": true, "docs": true, … } (§09). */
	tour_dismissals?: Record<string, boolean>;
	/** Per-surface ordered tour step keys from the onboarding.tour_steps
	 *  system setting — { "main": ["primary-nav", …], … } (§09). */
	tour_steps?: Record<string, string[]>;
}

/** GET /onboarding/recommendations item (Section 09). */
export interface OnboardingRecommendation {
	type: 'space' | 'user';
	id: string;
	name: string;
	slug: string;
	avatar_url?: string;
	reason: string;
	score: number;
}

/** GET /recommendations — geo-aware tool/feature/content/space recommendations (ADR-064a §5). */
export interface RecommendationItem {
	slug: string;
	label: string;
	url: string;
	reason: string;
}

export interface RecommendationContentItem {
	id: string;
	title: string;
	url: string;
	content_type: string;
}

export interface RecommendationSpaceItem {
	id: string;
	name: string;
	slug: string;
	level: string;
}

export interface VisitorContextSummary {
	content_country: string;
	content_region: string;
	source: string;
	is_authenticated: boolean;
	user_id: string;
	geoip_country: string;
	origin_country: string;
	current_country: string;
	destination_country: string;
	japa_stage: string;
	journey_stage: string;
	visa_route: string;
	purpose: string;
	travel_date: string;
	roadmap_slugs: string[];
	weakest_dimensions: string[];
	assessment_threshold: string;
	language: string;
	archetype: string;
	confidence: number;
	current_city: string;
	origin_city: string;
	dest_city: string;
	current_geo_space_id: string;
}

export interface RecommendationsResponse {
	tools: RecommendationItem[];
	features: RecommendationItem[];
	content: RecommendationContentItem[];
	spaces: RecommendationSpaceItem[];
	context: VisitorContextSummary;
}

/** GET /onboarding/options — option lists for Step 1 & Step 2 (Rule 16). */
export interface OnboardingOption {
	value: string;
	label_key: string;
}

/** GET /onboarding/options response. */
export interface OnboardingOptions {
	japa_statuses: OnboardingOption[];
	primary_goals: OnboardingOption[];
}

/* ------------------------------------------------------------------ *
 * Forum — Section 08 (forum Q&A / solved)
 * Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
 * The backend returns forum thread/reply bodies as base64-encoded
 * TipTap JSON (not pre-rendered HTML). See $lib/tiptap/serialize.
 * ------------------------------------------------------------------ */

/** A forum discussion thread (GET /forum, GET /forum/{slug}). */
export interface ForumThread {
	id: string;

	content_id: string | null;
	space_id: string;
	author_user_id: string;
	title: string;
	slug: string;
	/** Base64-encoded TipTap JSON document. */
	body: string;
	is_anonymous: boolean;
	solved: boolean;
	solved_at: string | null;
	solved_by_user_id: string | null;
	vote_score: number;
	reply_count: number;
	view_count: number;
	bookmark_count: number;
	edit_window_minutes: number;
	is_edited: boolean;
	created_at: string;
	updated_at: string;
	deleted_at: string | null;
	parent_reply_id: string | null;
	/** Joined author username (null when is_anonymous). */
	author_username: string | null;
	/** Joined author display name (null when is_anonymous). */
	author_display_name: string | null;
	/** Joined author avatar URL (null when is_anonymous or no avatar). */
	author_avatar_url: string | null;
	/** Joined space name (null when no space). */
	space_name: string | null;
	/** Joined space slug (null when no space). */
	space_slug: string | null;
	/** Joined space level: 'country' | 'state' | 'city' | 'custom'. */
	space_level: string | null;
}

/* ------------------------------------------------------------------ *
 * Forum Polls — Section 08 (GET /forum/polls/{id}, POST /forum/polls/{id}/vote)
 * ------------------------------------------------------------------ */

/** A forum tag attached to a thread — GET /forum/{id}/tags.
 * Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Tags) */
export interface ForumTag {
	id: string;

	name: string;
	slug: string;
	description?: string | null;
	rules?: string | null;
	cover_image_id?: string | null;
	follower_count: number;
	moderator_user_id?: string | null;
	created_at: string;
}

/** A forum poll attached to a thread. */
export interface ForumPoll {
	id: string;
	thread_id: string;
	question: string;
	is_multi_select: boolean;
	close_at: string | null;
	created_at: string;
}

/** A single poll option with its vote count (from GetPollResults). */
export interface PollResult {
	option_id: string;
	label: string;
	position: number;
	vote_count: number;
}

/** GET /forum/polls/{id} response — poll + results. */
export interface PollWithResults {
	poll: ForumPoll;
	results: PollResult[];
	/** IDs of options the current user has already voted on (BUG-002). */
	user_votes?: string[];
}

/** A forum reply (GET /forum/{id}/replies, POST /forum/{id}/replies). */
export interface ForumReply {
	id: string;

	thread_id: string;
	parent_reply_id: string | null;
	author_user_id: string;
	/** Base64-encoded TipTap JSON document. */
	body: string;
	vote_score: number;
	view_count: number;
	is_anonymous: boolean;
	is_edited: boolean;
	created_at: string;
	updated_at: string;
	deleted_at: string | null;
	/** Joined author username (null when is_anonymous). */
	author_username: string | null;
	/** Joined author display name (null when is_anonymous). */
	author_display_name: string | null;
	/** Joined author avatar URL (null when is_anonymous or no avatar). */
	author_avatar_url: string | null;
	/** §88: true when the reply is a pending AMA question awaiting host approval. */
	ama_pending?: boolean;
	/** §88: when the pending AMA question was approved by the host. */
	ama_approved_at?: string | null;
}

/** Vote entity type accepted by POST /forum/vote. */
export type ForumVoteEntity = 'thread' | 'reply';

/** POST /forum/vote response row. */
export interface ForumVote {
	entity_type: ForumVoteEntity;
	entity_id: string;
	value: 1 | -1;
}

/* ------------------------------------------------------------------ *
 * Unified comment engine — Section 49
 * Ref: japaconnect-engineering/sections/49-unified-comments.md
 * Comments attach to any content type via content_id. Nested replies
 * support up to depth 3 (parent_id links them). The body is a plain
 * string (not base64 TipTap like forum replies).
 * ------------------------------------------------------------------ */

/** GET /content/{id}/comments and GET /content/{id}/comments?parent_id=. */
export interface Comment {
	id: string;

	content_id: string;
	author_user_id: string | null;
	parent_id: string | null;
	/** Base64-encoded TipTap JSON document (Section 49 — jsonb body). */
	body: string;
	/** Nesting depth (0 = top-level, max 3). */
	depth: number;
	vote_score: number;
	/** Denormalized count of value=1 votes (Section 49 — daily.dev-style "N upvotes" label). */
	upvote_count: number;
	/** Denormalized count of value=-1 votes. */
	downvote_count: number;
	/** The authenticated viewer's current vote on this comment (1, -1, or 0). */
	user_vote: 1 | -1 | 0;
	is_pinned: boolean;
	is_edited: boolean;
	edited_at: string | null;
	created_at: string;
	updated_at: string;
	deleted_at: string | null;
	/** Anonymous comment — author identity redacted by backend (Section 49). */
	is_anonymous: boolean;
	/** Joined author username (empty for anonymous comments). */
	username: string;
	/** Joined author display name (empty string when absent). */
	display_name: string;
	/** Joined author avatar URL (empty string when absent). */
	avatar_url: string;
	/** Number of published, non-deleted direct replies (Section 49). */
	reply_count: number;
}

/* ------------------------------------------------------------------ *
 * Public profiles — Sections 07, 31, 50
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 *      docs/superpowers/specs/2026-09-03-unified-user-tiers-and-professional-profile-removal-design.md
 * ------------------------------------------------------------------ */

/** GET /users/{username} — lean public profile header. */
export interface PublicUser {
	id: string;
	username: string;
	display_name: string;
	avatar_url?: string | null;
	cover_url?: string | null;
	bio?: string | null;
	/** Unified user tier (regular / pro / business). */
	profile_type: 'regular' | 'pro' | 'business';
	follower_count: number;
	following_count: number;
	created_at: string;
	/** Section 28 — Points balance, only present when the user has opted in via show_points_publicly. Credits are never exposed. */
	points?: number;
	/** §106 video — true when this profile is an imported video channel. */
	is_synthetic?: boolean;
	/** §106 video — origin platform for synthetic channels (e.g. 'youtube'). */
	synthetic_source?: string | null;
	/** §106 video — channel/profile location metadata. */
	location?: string | null;
	/** §106 video — channel banner resolved from user_profiles.banner_media_id. */
	banner_url?: string | null;
}

/** GET /users/{id}/followers — mapped row for the profile followers tab. */
export type ProfileFollower = PublicUser & { is_following?: boolean; entity_type: 'user' };

/** GET /users/{id}/following — mapped row for the profile following tab. */
export interface ProfileFollowing {
	id: string;
	username: string;
	display_name: string;
	avatar_url: string | null;
	is_following?: boolean;
	entity_type?: string;
	/** Follow-row timestamp — used for newest/oldest sorting on the tab. */
	created_at?: string;
}

/** GET /users/{id}/content and /organizations/{id}/content — PostCard row.
 *  Carries the same fields as a feed item so profile/org tabs can render
 *  through FeedCard with the `daily` style. Ref: 2026-09-10 daily feed design. */
export interface ProfilePost {
	id: string;
	author_id: string;
	author_name?: string | null;
	author_username?: string | null;
	author_avatar_url?: string | null;
	organization_id: string | null;
	organization?: FeedOrganization | null;
	space_id: string | null;
	space_name?: string | null;
	space_slug?: string | null;
	space_avatar_url?: string | null;
	content_type: string;
	title: string;
	slug: string;
	body_html: string;
	excerpt?: string;
	header_image_id: string | null;
	featured_image_id: string | null;
	cover_image_url?: string | null;
	read_time_minutes: number;
	view_count: number;
	comment_count: number;
	share_count: number;
	reaction_count?: number;
	bookmark_count?: number;
	is_pinned: boolean;
	tags?: string[];
	journey_stage?: string | null;
	visa_route?: string | null;
	source_country?: string | null;
	target_country?: string | null;
	guide_category?: string | null;
	video_source_id?: string | null;
	/** GET /users/{id}/content emits the linked video's length as
	 *  video_duration_secs (PostCard). duration_secs is the legacy alias. */
	video_duration_secs?: number | null;
	duration_secs?: number | null;
	published_at: string;
	created_at: string;
}

/** GET /users/{id}/likes — profile Likes tab (published+public content the user reacted to). */
export interface ProfileLike {
	id: string;
	title: string;
	slug: string;
	content_type: string;
	author_id: string;
	space_id: string | null;
	created_at: string;
	view_count: number;
	comment_count: number;
	author_name: string;
	author_avatar_url: string;
	excerpt: string;
	reacted_at: string;
}

/** GET /users/{id}/replies — profile Replies tab. */
export interface ProfileReply {
	id: string;
	content_id: string;
	parent_id: string | null;
	body: string;
	vote_score: number;
	created_at: string;
	content_title: string;
	content_slug: string;
}

/** GET /users/{id}/spaces — profile Spaces tab (public+active memberships). */
export interface ProfileSpace {
	id: string;
	slug: string;
	name: string;
	role?: string;
}

/** GET /users/{id}/certificates — earned certificate badges (social proof). */
export interface ProfileCertificate {
	id: string;
	title?: string | null;
	track_slug?: string | null;
	certificate_number?: string;
	issued_at?: string;
}

/** GET /users/{id}/analytics — public profile Analytics tab. */
export interface ProfileAnalytics {
	total_views: number;
	total_shares: number;
	total_posts: number;
	total_likes: number;
	total_followers: number;
	total_following: number;
}

/** GET /me/analytics/followers — daily follower/following growth time-series (ADR-059). */
export interface FollowerGrowthItem {
	day: string;
	new_followers: number;
	new_following: number;
}

/** GET /me/analytics/content — per-content analytics for the top-posts BarChart (ADR-059). */
export interface TopContentItem {
	id: string;
	title: string;
	content_type: string;
	status: string;
	view_count: number;
	comment_count: number;
	share_count: number;
	created_at: string;
}

/** GET /users/{id}/media — profile Media tab. */
export interface ProfileMedia {
	id: string;
	url: string;
	filename?: string;
	mime_type?: string;
	alt_text?: string | null;
	created_at?: string;
}

/** GET /organizations/{slug} — lean public org header. */
export interface PublicOrganization {
	id: string;
	name: string;
	slug: string;
	tagline?: string | null;
	description?: string | null;
	org_type: string;
	logo_media_id?: string | null;
	header_media_id?: string | null;
	website_url?: string | null;
	country_code?: string | null;
	verified: boolean;
	verification_type?: string | null;
	follower_count: number;
	member_count: number;
	created_at: string;
	owner?: OrgOwner | null;
}

/** Org owner public profile, embedded in PublicOrganization for the "Org
 * Leader" sidebar widget. Shown regardless of the owner's account standing. */
export interface OrgOwner {
	id: string;
	username: string;
	display_name?: string | null;
	avatar_url?: string | null;
}

/** GET /organizations/meta — valid option lists for org dropdowns (Rule 16).
 * Used by org list, new, and dashboard pages to populate selects from the API
 * instead of hardcoding option arrays. */
export interface OrgMeta {
	org_types: OrgMetaOption[];
	member_roles: OrgMetaOption[];
	invitation_roles: OrgMetaOption[];
	claim_statuses: OrgMetaOption[];
}

export interface OrgMetaOption {
	value: string;
	label: string;
}

/** GET /me/claim-ownership row — one ownership claim filed by the current
 * user, joined with org name/slug for display (Section 07). */
export interface MyOwnershipClaim {
	id: string;
	organization_id: string;
	claimant_user_id: string;
	evidence_url?: string | null;
	notes?: string | null;
	status: 'pending' | 'approved' | 'rejected' | 'withdrawn' | string;
	resolution_note?: string | null;
	form_submission_id?: string | null;
	org_name: string;
	org_slug: string;
	created_at: string;
	reviewed_at?: string | null;
}

/** GET /organizations/{id} (authed member) — full gen.Organization row for the
 * org admin dashboard. Includes owner_user_id, status, and timestamps. */
export interface OrganizationAdmin {
	id: string;
	owner_user_id: string;
	name: string;
	slug: string;
	tagline?: string | null;
	description?: string | null;
	logo_media_id?: string | null;
	header_media_id?: string | null;
	website_url?: string | null;
	country_code?: string | null;
	org_type: string;
	verified: boolean;
	verification_type?: string | null;
	follower_count: number;
	member_count: number;
	status: string;
	created_at: string;
	updated_at: string;
}

/** GET /organizations/{id}/members — active members, lean DTO. */
export interface OrgMember {
	user: {
		id: string;
		username: string;
		display_name: string;
		avatar_url?: string | null;
	};
	role: string;
	joined_at: string;
}

/** GET /organizations/{id}/invitations — pending invitations. */
export interface OrgInvitation {
	id: string;
	email: string;
	role: string;
	status?: string;
	created_at?: string;
}

/** Lean org row for the /create composer's "post on behalf of" selector.
 *  Sourced from GET /me/organizations (the caller's memberships). */
export interface ComposerOrganization {
	id: string;
	name: string;
	slug: string;
	logo_media_id?: string | null;
	member_role: string;
}

/** GET /organizations/{id}/media item — the org profile Media tab. Mirrors the
 *  media MediaResponse shape (id, url, filename, mime_type, alt_text, created_at). */
export interface OrgMedia {
	id: string;
	url: string;
	filename: string;
	mime_type: string;
	file_size?: number;
	alt_text?: string | null;
	status?: string;
	created_at?: string;
}

/** GET /me/affiliate-earnings — EarningsSummary (total clicks/conversions/
 *  earned, breakdown by product type, recent events). Powers the earnings
 *  card in the settings Affiliate section. */
export interface AffiliateEarnings {
	total_clicks: number;
	total_conversions: number;
	total_earned: string;
	by_product: Array<{
		product_type: string;
		clicks: number;
		conversions: number;
		earned: string;
	}>;
	recent_events: Array<{
		id: string;
		event_type: string;
		commission_amount?: string;
		created_at: string;
		link_code: string;
		product_type: string;
	}>;
}

/** GET /me/payments — payment history for the billing Payments section. */
export interface Payment {
	id: string;
	provider: string;
	provider_ref?: string;
	purpose_type?: string;
	purpose_id?: string;
	amount: string;
	currency: string;
	status: string;
	metadata?: Record<string, unknown> | null;
	subscription_id?: string;
	discount_code_id?: string;
	discount_amount?: string;
	checkout_url?: string;
	created_at: string;
	updated_at?: string;
}

/** GET /me/api-keys — API key returned to the owner (no secret). */
export interface ApiKey {
	id: string;
	name: string;
	key_prefix?: string;
	scopes?: string[];
	rate_limit_tier?: string;
	status?: string;
	notes?: string;
	created_at?: string;
	updated_at?: string;
}

/** GET /me/api-keys/{id}/ip-allowlist — single IP allowlist entry. */
export interface ApiKeyIPAllowlist {
	id: string;
	api_key_id: string;
	cidr: string;
	description?: string;
	created_at?: string;
}

/** GET /me/analytics — aggregate analytics for the dashboard. */
export interface MeAnalytics {
	total_content: number;
	draft_content: number;
	published_content: number;
	total_bookmarks: number;
	total_followers: number;
	total_following: number;
	total_views: number;
	total_reactions: number;
	total_comments: number;
	total_points_earned: number;
	total_followers_gained: number;
	daily_impressions: { day: string; views: number }[];
	readers_by_country: { country_code: string; readers: number }[];
	top_content: TopContentItem[];
	content_created: { total: number; draft_count: number; published_count: number };
}

/** GET /me/wallet — current points, credits, and lifetime points balance (Section 28). */
export interface WalletSummary {
	points_balance: number;
	credits_balance: number;
	lifetime_points: number;
}

/** GET /analytics/dashboard — user/platform dashboard (Section 40). */
export interface UserDashboard {
	top_content: {
		content_id: string;
		content_title: string;
		total_views: number;
		total_comments: number;
		total_reactions: number;
		total_shares: number;
		total_unique_viewers: number;
	}[];
	user_engagement: {
		day: string;
		posts_created: number;
		reactions_given: number;
		comments_made: number;
		views: number;
	}[];
	content_over_time: {
		day: string;
		content_count: number;
	}[];
	traffic_sources: {
		referrer: string;
		visits: number;
	}[];
	ad_performance: {
		campaign_id: string;
		campaign_name: string;
		status: string;
		impressions: number;
		clicks: number;
		spent: number;
		ctr: number;
	}[];
}

/* ------------------------------------------------------------------ *
 * Spaces — Section 22 (GET /spaces, GET /spaces/{slug})
 * The backend returns the raw gen.Space row. Universities are spaces
 * with a university space_type; /universities returns the same shape.
 * ------------------------------------------------------------------ */

/** GET /space-types — space type definition row. A university space is one
 * whose type slug is 'university' (Section 39) — there is no is_university
 * column; use the slug. */
export interface SpaceType {
	id: string;

	name: string;
	slug: string;
	description: string | null;
	icon: string | null;
	field_group_id: string | null;
	is_system: boolean;
	category?: string;
	created_at: string;
	updated_at: string;
}

/** GET /spaces/{slug} `space` field — full gen.Space row. */
export interface Space {
	id: string;

	parent_id: string | null;
	space_type_id: string;
	name: string;
	slug: string;
	description: string | null;
	wiki_content: string | null;
	logo_media_id: string | null;
	header_image_id: string | null;
	geo_code: string | null;
	level: string;
	member_count: number;
	post_count: number;
	follower_count: number;
	visibility: string;
	posting_perm: string;
	replying_perm: string;
	commenting_perm: string;
	joining_perm: string;
	status: string;
	created_at: string;
	updated_at: string;
	keywords?: string[];
	/** Resolved imgproxy-signed logo URL (GET /spaces list & detail). Empty/null when no logo is set. */
	logo_url?: string | null;
	/** University short code / acronym (initials custom field, Section 39). */
	initials?: string | null;
	/** Viewer-scoped (list responses): the caller is an approved member. */
	is_member?: boolean;
	/** Viewer-scoped: the caller has a pending join request. */
	is_pending?: boolean;
	/** Viewer-scoped: the caller follows this space. */
	is_following?: boolean;
}

/** GET /spaces/{slug} response — `{ space, rules, keywords, logo_url }` bundle. */
export interface SpaceDetail {
	space: Space;
	rules: SpaceRule[];
	keywords?: string[];
	logo_url?: string;
	header_url?: string;
	/** Viewer-scoped: true when the caller is the space owner (role='owner'). */
	is_owner?: boolean;
	/** Viewer-scoped: true when the caller is an assigned space moderator. */
	is_moderator?: boolean;
	/** Viewer-scoped: true when the caller is an approved member. */
	is_member?: boolean;
	/** Viewer-scoped: true when the caller has a pending join request. */
	is_pending?: boolean;
	/** Viewer-scoped: true when the caller follows the space. */
	is_following?: boolean;
	/** §58 — the space's resolved country currency (own code when the space
	 *  is a country, else its country ancestor's); display fallback for
	 *  currency/price_range fields without an explicit {currency_code}. */
	country_currency_code?: string;
}

/** GET /spaces/{slug}/moderators item — ListSpaceModeratorsRow. */
export interface SpaceModerator {
	id: string;

	space_id: string;
	user_id: string;
	assigned_by: 'site_admin' | 'space_owner';
	assigned_by_user_id: string;
	created_at: string;
	user_username: string;
	user_display_name: string | null;
}

/** GET /spaces/{slug}/rules item — gen.SpaceRule row. */
export interface SpaceRule {
	id: string;
	space_id: string;
	title: string;
	body: string;
	position: number;
	created_at: string;
	updated_at: string;
}

/** GET /spaces/{id}/reviews item — published review with author name. */
export interface SpaceReview {
	id: string;
	rating: number;
	title: string;
	body: string;
	author_name: string;
	created_at: string;
}

/** GET /spaces/{slug}/members item — SpaceMemberRow. */
export interface SpaceMember {
	user_id: string;
	username: string;
	display_name: string | null;
	avatar_url: string | null;
	joined_at: string;
}

/** GET /space-permission-presets item — gen.SpacePermissionPreset row. */
export interface SpacePermissionPreset {
	id: string;

	name: string;
	visibility: string;
	posting_perm: string;
	replying_perm: string;
	commenting_perm: string;
	joining_perm: string;
	is_system: boolean;
	created_at: string;
	updated_at: string;
}

/** GET /spaces/{slug}/pinned item — pinned post row. */
export interface SpacePinnedPost {
	id: string;
	space_id: string;
	content_id: string;
	pinned_by: string;
	position: number;
	created_at: string;
}

/** GET /admin/spaces/{id}/join-requests item — pending join request row. */
export interface SpaceJoinRequest {
	user_id: string;
	username: string;
	display_name: string | null;
	avatar_url: string | null;
	joined_at: string;
	status: string;
}

/** POST /spaces/{id}/report response — gen.Report row. */
export interface SpaceReport {
	id: string;

	reporter_user_id: string;
	entity_type: string;
	entity_id: string;
	reason: string;
	notes: string | null;
	evidence_urls: string[];
	status: string;
	reviewed_by: string | null;
	resolution_note: string | null;
	resolved_at: string | null;
	created_at: string;
}

/** GET /me/reports item — gen.ContentReport row (§51). The current user's
 *  own submitted content reports with their review status. */
export interface MyReport {
	id: string;

	reporter_user_id: string;
	entity_type: string;
	entity_id: string;
	reason: string;
	notes: string | null;
	evidence_urls: string[];
	status: string;
	reviewed_by: string | null;
	resolution_note: string | null;
	resolved_at: string | null;
	created_at: string;
}

/** GET /spaces/{slug}/content item — ListContentsBySpaceRow (content cards). */
export interface SpaceContentItem {
	id: string;
	author_id: string;
	space_id: string;
	content_type: string;
	title: string;
	slug: string;
	body_html: string;
	status: string;
	visibility: string;
	is_pinned: boolean;
	read_time_minutes: number;
	view_count: number;
	comment_count: number;
	share_count: number;
	published_at: string;
	/** Row creation timestamp — the ?cursor= pagination key for
	 * GET /spaces/{slug}/content (ORDER BY created_at DESC, id DESC). */
	created_at: string;
	/** Author fields joined from users/user_profiles — FeedCard renders
	 * these via toFeedItem. Nullable: LEFT JOINs. */
	author_username?: string | null;
	author_display_name?: string | null;
	author_avatar_url?: string | null;
}

/* ------------------------------------------------------------------ *
 * Notifications — Section 11 (GET /me/notifications)
 * ------------------------------------------------------------------ */

/** GET /me/notifications item — GetNotificationsByRecipientRow. */
export interface Notification {
	id: string;
	notification_type: string;
	actor_type: string | null;
	actor_id: string | null;
	subject_type: string | null;
	subject_id: string | null;
	title: string;
	body: string | null;
	action_url: string | null;
	read_at: string | null;
	created_at: string;
	type_label: string;
}

/**
 * GET /me/notification-preferences item — PreferenceOutput (Section 35).
 * The API returns every enabled notification_type merged with the user's
 * overrides; types the user has never configured fall back to defaults.
 */
export interface NotificationPreference {
	type: string;
	label: string;
	category?: string;
	description: string | null;
	in_app: boolean;
	email: boolean;
	push: boolean;
}

/** GET /me/push-tokens item — web push subscription (Section 35 / Section 50). */
export interface PushSubscription {
	id: string;
	endpoint: string;
}

/** GET /me/devices item — mobile push device (Section 77, ADR-074). */
export interface DeviceInfo {
	id: string;
	device_id: string;
	platform: string;
	app_version: string;
	is_active: boolean;
	last_used_at?: string;
	created_at?: string;
}

/* ------------------------------------------------------------------ *
 * DM — Section 48 (GET /dm/conversations, .../messages)
 * ------------------------------------------------------------------ */

/** GET /dm/conversations item — ConversationInfo. */
export interface DMConversation {
	id: string;
	other_user_id: string;
	other_username: string;
	other_display_name: string;
	other_avatar_url: string;
	last_message_at: string | null;
	last_message_preview: string | null;
	unread_count: number;
	created_at: string;
}

/** GET /dm/conversations/{id}/messages item — MessageInfo. */
export interface DMMessage {
	id: string;
	sender_id: string;
	sender_username: string;
	sender_display_name: string;
	body: string;
	created_at: string;
	read_at: string | null;
}

/** GET /dm/requests item — MessageRequestInfo. */
export interface DMRequest {
	id: string;
	conversation_id?: string;
	requester_id: string;
	requester_username: string;
	requester_display_name: string;
	requester_avatar_url?: string;
	preview_message: string;
	status: string;
	created_at: string;
}

/* ------------------------------------------------------------------ *
 * Universities — Section 39 (GET /universities)
 * Universities are spaces; the /universities endpoint returns gen.Space
 * rows filtered to university space types. Reuses the Space shape.
 * ------------------------------------------------------------------ */
export type University = Space;

/** University detail response (GET /universities/{slug}) — Space + fields + resolved logo URL. */
export interface UniversityDetail extends Space {
	fields?: unknown[];
	/** Display-ready custom field values (show_in_frontend-filtered, definition-
	 * joined, with select/multiselect option values resolved to labels). Rendered
	 * in the About facts grid (Section 39 sidebar: ranking, tuition, programs). */
	display_fields?: UniversityDisplayField[];
	/** imgproxy-signed logo URL resolved from logo_media_id (Section 34 / ADR-039). */
	logo_url?: string | null;
}

/** A display-ready custom field value (GET /universities/{slug} display_fields[]). */
export interface UniversityDisplayField {
	label: string;
	key: string;
	value: string;
	field_type: string;
}

/** GET /universities/{id}/compare response — `{ space, fields }`. */
export interface UniversityCompare {
	space: Space;
	fields: Record<string, unknown>;
}

/**
 * Institutional course offering (Section 39) —
 * GET /universities/{slug}/courses[/{offeringSlug}]. A degree program a
 * university offers, joined with its canonical catalog course and the
 * university space. Distinct from CatalogTrack (Japa University creator
 * marketplace tracks).
 */
export interface CourseOffering {
	id: string;
	course_id: string;
	university_id: string;
	slug: string;
	tuition: number | null;
	currency: string | null;
	duration_years: number | null;
	study_mode: string | null;
	intake_months: string[] | null;
	entry_requirements: string | null;
	accreditation: string | null;
	is_published: boolean;
	created_at: string;
	updated_at: string;
	course: {
		title: string;
		slug: string;
		description: string | null;
		degree_type: string | null;
		department: string | null;
		field_of_study: string | null;
		language: string;
		default_duration_years: number | null;
	};
	university: { name: string; slug: string };
}

/* ------------------------------------------------------------------ *
 * Certificates — Section 17 (GET /certificates/{id})
 * ------------------------------------------------------------------ */

/** GET /certificates/{id} — public learning certificate. */
export interface Certificate {
	id: string;
	user_id: string;
	track_id: string;
	certificate_number: string;
	media_id?: string | null;
	issued_at: string;
	title?: string | null;
	track_slug?: string | null;
	recipient_name?: string;
	recipient_username?: string;
}

/* ------------------------------------------------------------------ *
 * Discover — ADR-058 frontend route consuming /suggestions/* + /organizations
 * ------------------------------------------------------------------ */

/** GET /suggestions/users item — SuggestedUser. */
export interface SuggestedUser {
	id: string;
	username: string;
	display_name: string;
	avatar_url?: string | null;
	bio?: string | null;
	shared_spaces: number;
}

/** GET /suggestions/spaces item — SuggestedSpace. */
export interface SuggestedSpace {
	id: string;
	name: string;
	slug: string;
	member_count: number;
	post_count: number;
	shared_members: number;
}

/** GET /suggestions/tags item — SuggestedTag. */
export interface SuggestedTag {
	id: string;
	name: string;
	slug: string;
	follower_count: number;
}

/** GET /suggestions/content item — SuggestedContent ("You might also like", §53). */
export interface SuggestedContent {
	id: string;
	title: string;
	slug: string;
	content_type: string;
}

/** GET /content/{id}/tags item — ContentTag. Mirrors the ListContentTags
 * SQL row (tags joined via content_tags). Used by the /create composer to
 * restore the tag chips when editing existing content. */
export interface ContentTag {
	id: string;
	name: string;
	slug: string;
	description?: string | null;
	color?: string | null;
	follower_count: number;
}

/** GET /content/{id}/spaces item — a single content-space assignment
 * (Section 14: content_spaces). Used by the /create composer to restore
 * the multi-space selector when editing existing content. */
export interface ContentSpace {
	space_id: string;
	is_primary: boolean;
}

/** GET /organizations (public list) item — PublicOrgListItem. */
export interface PublicOrgListItem {
	id: string;
	name: string;
	slug: string;
	tagline: string | null;
	org_type: string;
	country_code: string | null;
	verified: boolean;
	follower_count: number;
	member_count: number;
	created_at: string;
}

/* ------------------------------------------------------------------ *
 * Trending — Section 10 (GET /trending/{content,tags,spaces,users})
 * Each row joins a trending_snapshots row to its entity. The shared
 * snapshot prefix (id, entity_id, score, window, geo_code, computed_at)
 * is followed by the joined entity's fields (id_2, ...).
 * ------------------------------------------------------------------ */

export type TrendingWindow = '24h' | '7d' | '30d';

/** Common snapshot prefix shared by every trending row. */
export interface TrendingSnapshotBase {
	id: string;
	entity_id: string;
	/** Trending score — higher is hotter. Sorting is server-side (DESC). */
	score: number;
	window: TrendingWindow;
	geo_code: string | null;
	computed_at: string;
}

/** GET /trending/content item — joined content row. */
export interface TrendingContent extends TrendingSnapshotBase {
	id_2: string;
	title: string;
	slug: string;
	content_type: string;
	author_id: string;
	space_id: string;
	view_count: number;
}

/** GET /trending/tags item — joined tag row. */
export interface TrendingTag extends TrendingSnapshotBase {
	id_2: string;
	name: string;
	slug: string;
	follower_count: number;
}

/** GET /trending/spaces item — joined space row. */
export interface TrendingSpace extends TrendingSnapshotBase {
	id_2: string;
	name: string;
	slug: string;
	member_count: number;
	post_count: number;
}

/** GET /trending/users item — joined user/profile row. */
export interface TrendingUser extends TrendingSnapshotBase {
	id_2: string;
	username: string;
	display_name: string | null;
	avatar_url: string | null;
	bio: string | null;
}

/* ------------------------------------------------------------------ *
 * Learning — Section 22 (GET /learn/tracks, /learn/tracks/{slug})
 * ------------------------------------------------------------------ */

/** GET /learn/tracks item — a learning track. */
export interface LearnTrack {
	id: string;

	title: string;
	slug: string;
	description?: string | null;
	difficulty: 'beginner' | 'intermediate' | 'advanced' | string;
	is_published: boolean;
	sort_order: number;
	cover_media_id?: string | null;
	created_at: string;
	updated_at: string;
}

/** GET /learn/tracks/{slug} — track with nested modules. */
export interface LearnTrackDetail {
	track: LearnTrack;
	modules: LearnModule[];
}

/** A module within a learning track. */
export interface LearnModule {
	id: string;

	track_id: string;
	title: string;
	slug: string;
	sort_order: number;
	created_at: string;
	updated_at: string;
}

/** GET /learn/tracks/{trackSlug}/modules/{moduleSlug}/lessons/{lessonSlug} — a single lesson. */
export interface LearnLesson {
	id: string;

	module_id: string;
	module_title?: string | null;
	title: string;
	slug: string;
	lesson_type: string;
	content?: string | null;
	media_id?: string | null;
	duration_minutes?: number | null;
	sort_order: number;
	is_published: boolean;
	created_at: string;
	updated_at: string;
}

/** GET /learn/progress/{trackId} — the user's progress in a track. */
export interface LearnProgress {
	completed: number;
	total: number;
	percent: number;
}

/** Navigation reference for the next/continue lesson in a track. */
export interface LessonNavRef {
	slug: string;
	title: string;
	module_slug: string;
}

/** Catalog difficulty / level value. */
export type TrackLevel = string;

/** GET /me/enrollments item — a track enrollment record.
 *
 * Mirrors the backend learning.Enrollment struct (enriched list from
 * ListEnrollmentsByUser). Fields are used by the dashboard Learning tab,
 * the public profile learning hub, and the right-sidebar progress widgets.
 */
export interface LearnEnrollment {
	id: string;
	user_id: string;
	track_id: string;
	track_slug: string;
	track_title: string;
	thumbnail_url?: string | null;
	level: TrackLevel;
	access_type: string;
	/** 0–100 completion percentage. */
	progress_pct: number;
	/** Count of completed lessons. Omitted by the backend when zero. */
	lessons_completed?: number;
	/** Total number of lessons in the track. Omitted by the backend when zero. */
	lessons_total?: number;
	/** ISO 8601 timestamps. */
	enrolled_at: string;
	completed_at?: string | null;
	last_activity_at?: string | null;
	/** Next up lesson for the continue-learning card. */
	next_lesson?: LessonNavRef | null;
}

/* ------------------------------------------------------------------ *
 * Coupons — Section 21 (GET /coupons, /coupons/{id})
 * ------------------------------------------------------------------ */

/** GET /coupons item — a public active coupon. */
export interface Coupon {
	id: string;

	provider_id: string | null;
	coupon_type: string;
	title: string;
	description?: string | null;
	code: string;
	discount_type?: string | null;
	discount_value?: number | string;
	currency?: string | null;
	terms_url?: string | null;
	referral_url?: string | null;
	is_verified: boolean;
	status: string;
	valid_from?: string | null;
	expires_at?: string | null;
	total_clicks: number;
	total_copies: number;
	created_by?: string | null;
	created_at: string;
	updated_at: string;
	provider_name?: string | null;
	provider_slug?: string | null;
	provider_logo_media_id?: string | null;
}

/* ------------------------------------------------------------------ *
 * JCE Comparisons — /api/v1/jce/comparisons
 * ------------------------------------------------------------------ */

/** JCE comparison detail returned by GET /jce/comparisons/{id|slug}. */
export interface JCEComparison {
	content: ComparisonContent;
	metadata: ComparisonMetadata;
	items: ComparisonItem[];
	dimensions: ComparisonDimension[];
}

/** Backing content row for a JCE comparison. */
export interface ComparisonContent {
	id: string;

	author_id: string;
	title: string;
	slug: string;
	body: string;
	status: string;
	visibility: string;
	space_id: string | null;
	content_type: string;
}

/** JCE comparison metadata (depth, promotion state, smart mode). */
export interface ComparisonMetadata {
	content_id: string;
	depth: string;
	promoted_at?: string | null;
	smart_mode?: string | null;
}

/** GET /jce/comparisons/{id} — comparison content metadata with items and dimensions.
 *  Backwards-compatible flat shape; page server may flatten a JCEComparison.
 */
export interface ComparisonDetail {
	id: string;

	author_id: string;
	title: string;
	slug: string;
	body: string;
	visibility: string;
	status: string;
	space_id: string | null;
	items: ComparisonItem[];
	dimensions: ComparisonDimension[];
	/** §30 matrix cell values indexed as `${dimIdx}:${itemIdx}` for the
	 *  ComparisonBuilder so existing values are restored when editing. */
	matrixValues?: Record<string, string>;
}

/** A comparison item (entity being compared, e.g. a university). */
export interface ComparisonItem {
	id: string;
	content_id: string;

	type_id: string | null;
	title: string;
	description: string | null;
	image_id: string | null;
	geo_space_id: string | null;
	position: number;
	item_ref?: Record<string, unknown> | null;
}

/** A comparison dimension (criterion being evaluated). */
export interface ComparisonDimension {
	id: string;
	content_id: string;

	type_id: string | null;
	label: string;
	dimension_type: string;
	position: number;
}

/** GET /jce/comparisons/{id}/matrix — one item's row across all dimensions. */
export interface ComparisonMatrixItem {
	item: {
		id: string;
		content_id: string;
		type_definition_id: string;
		title: string;
		entity_id?: string | null;
		image_media_id?: string | null;
		geo_space_id?: string | null;
		position: number;
	};
	dimensions: ComparisonMatrixDimensionValue[];
}

/** A single dimension's value for one item in the matrix. */
export interface ComparisonMatrixDimensionValue {
	dimension_id: string;
	label: string;
	type: 'text' | 'number' | 'boolean' | 'rating' | 'currency';
	value?: string | null;
	value_rating?: number | null;
	value_boolean?: boolean | null;
	/** ISO code for currency dimensions — from the source field's
	 *  {currency_code} option (§58). */
	currency_code?: string;
}

/** POST /jce/comparisons — created comparison result. */
export interface ComparisonCreated {
	id: string;
	slug: string;
}

/**
 * GET /jce/comparisons — list row. The backend returns content rows backing
 * comparisons (Section 30); item count / type chip / author display are not
 * joined by the list endpoint, so the card renders what the API provides.
 */
export interface ComparisonListItem {
	id: string;

	author_id: string;
	space_id: string | null;
	content_type: string;
	title: string;
	slug: string;
	body: string;
	status: string;
	visibility: string;
	view_count: number;
	comment_count: number;
	share_count: number;
	reshare_count: number;
	published_at: string | null;
	created_at: string;
}

/** GET /jce/comparison-types — a comparison type with field/item counts. */
export interface ComparisonType {
	id: string;

	name: string;
	slug: string;
	description: string | null;
	icon: string | null;
	field_group_id: string | null;
	item_label: string;
	is_system: boolean;
	created_at: string;
	field_count: number;
	item_count: number;
	/** Whether the requesting user's group may create comparisons of this type (§30). */
	can_create?: boolean;
}

/* ------------------------------------------------------------------ *
 * List (listicle) — §14 list amendment
 * field_values schema for content_type='list'; the composer serializes
 * items into body/body_html while field_values stays the editing source.
 * ------------------------------------------------------------------ */

export type ListMode = 'numbered' | 'countdown' | 'unnumbered';
export type ListEmbedKind = 'content' | 'url' | 'video';

export interface ListEmbedSnapshot {
	title?: string;
	excerpt?: string;
	image_url?: string;
	content_type?: string;
}

export interface ListEmbedResolved {
	title?: string;
	description?: string;
	image_url?: string;
	site_name?: string;
	embed_url?: string;
}

export interface ListItemEmbed {
	kind: ListEmbedKind;
	content_id?: string;
	content_slug?: string;
	snapshot?: ListEmbedSnapshot;
	url?: string;
	resolved?: ListEmbedResolved;
	video_source_id?: string;
}

export interface ListItem {
	heading?: string;
	body_html?: string;
	media_id?: string | null;
	embed?: ListItemEmbed | null;
	source_url?: string;
	credit?: string;
}

export interface ListFieldValues {
	list_mode: ListMode;
	intro_html?: string;
	items: ListItem[];
}

/* ------------------------------------------------------------------ *
 * Page Builder — Section 27 (GET /pages/{slug})
 * ------------------------------------------------------------------ */

/** GET /pages/{slug} — a published Page Builder page. */
export interface PageBuilderPage {
	id: string;
	title: string;
	slug: string;
	description?: string | null;
	template_id?: string | null;
	theme_id?: string | null;
	blocks: LandingBlock[];
	meta?: Record<string, unknown>;
	/** Server-resolved template literal values for this page (e.g. Current_Year). */
	literals?: Record<string, string>;
	status: string;
	published_at?: string | null;
	created_by?: string | null;
	created_at: string;
	updated_at: string;
}

/** A single block in a Page Builder page. */
export interface LandingBlock {
	id?: string;
	type: string;
	props: Record<string, unknown>;
}

/* ------------------------------------------------------------------ *
 * Tickets — Section 38 (GET /me/tickets, GET /me/tickets/{id}, POST /tickets)
 * ------------------------------------------------------------------ */

export type TicketStatus =
	| 'open'
	| 'in_progress'
	| 'waiting_on_user'
	| 'resolved'
	| 'closed'
	| 'spam';

export type TicketPriority = 'low' | 'normal' | 'high' | 'urgent';

/** GET /me/tickets — list row (includes the joined type name/slug). */
export interface TicketListItem {
	id: string;
	type_id: string;
	subject: string;
	description: string;
	status: TicketStatus;
	priority: TicketPriority;
	submitter_email?: string | null;
	sla_breach_at?: string | null;
	resolved_at?: string | null;
	closed_at?: string | null;
	created_at: string;
	updated_at: string;
	type_name: string;
	type_slug: string;
}

/** GET /me/tickets/{id} — full ticket + comments + attachments. */
export interface TicketDetail extends TicketListItem {

	submitter_user_id: string;
	assigned_to?: string | null;
	assigned_group_id?: string | null;
	resolution_note?: string | null;
	field_values?: Record<string, unknown> | null;
}

export interface TicketComment {
	id: string;
	ticket_id: string;
	user_id: string;
	body: string;
	is_internal: boolean;
	created_at: string;
	updated_at: string;
}

export interface TicketAttachment {
	id: string;
	ticket_id: string;
	comment_id?: string | null;
	media_id: string;
	created_at: string;
}

export interface TicketDetailResponse {
	ticket: TicketDetail;
	comments: TicketComment[];
	attachments: TicketAttachment[];
}

/** GET /ticket-types — admin-defined ticket types (public ones are selectable
 * by users when filing a new ticket). */
export interface TicketType {
	id: string;
	name: string;
	slug: string;
	description?: string | null;
	default_priority: TicketPriority;
	sla_hours?: number | null;
	is_public: boolean;
	is_system: boolean;
	created_at: string;
	updated_at: string;
}

/** POST /tickets — create-ticket payload. */
export interface CreateTicketInput {
	type_id?: string;
	subject: string;
	description: string;
	priority?: TicketPriority;
	email?: string;
	field_values?: Record<string, unknown>;
}

/** POST /me/tickets/{id}/comments — reply payload. */
export interface AddTicketCommentInput {
	body: string;
}

/* ------------------------------------------------------------------ *
 * Forms — Section 06 (GET /forms/{slug}/embed, POST /forms/{id}/submissions)
 * ------------------------------------------------------------------ */

/** A single field on an embeddable form (GET /forms/{slug}/embed). */
export interface FormField {
	id: string;
	label: string;
	type: string;
	required: boolean;
	sort_order: number;
	options: string[] | null;
}

/** GET /forms/{slug}/embed — public form definition for embedded rendering. */
export interface FormEmbed {
	id: string;
	slug: string;
	name: string;
	description?: string | null;
	status: string;
	is_multi_step: boolean;
	submit_label: string;
	success_message?: string | null;
	submission_fee_credits: number;
	fields: FormField[];
}

/** POST /forms/{id}/submissions — submission response. */
export interface FormSubmissionResponse {
	id: string;
	status: string;
	checkout_url?: string;
	payment_id?: string;
}

/* ------------------------------------------------------------------ *
 * Reviews — Section 29 (POST /reviews/{entity_type}/{entity_id}, GET /reviews/{entity_type}/{entity_id}[/aggregate])
 * ------------------------------------------------------------------ */

/** A single review row (gen.Review). */
export interface Review {
	id: string;
	entity_type: string;
	entity_id: string;
	reviewer_user_id: string;
	rating: number;
	title?: string | null;
	body?: string | null;
	status: string;
	created_at: string;
	updated_at: string;
}

/** GET /reviews/{entity_type}/{entity_id}/aggregate — rolled-up ratings. */
export interface ReviewAggregate {
	entity_type: string;
	entity_id: string;
	review_count: number;
	average_rating: number;
	updated_at: string;
}

/** POST /reviews/{entity_type}/{entity_id} — create-review payload. */
export interface CreateReviewInput {
	rating: number;
	title?: string;
	body?: string;
}

/* ------------------------------------------------------------------ *
 * University Marketplace — Section 17 (GET /learn/university/catalog)
 * Public marketplace tracks sold by instructors. Prices are in kobo
 * (the smallest currency unit) per the Go backend's integer-money model.
 * ------------------------------------------------------------------ */

/** GET /learn/university/catalog item — a marketplace track. */
export interface CatalogTrack {
	id: string;
	title: string;
	slug: string;
	description?: string | null;
	difficulty?: 'beginner' | 'intermediate' | 'advanced' | string;
	level?: TrackLevel;
	instructor_id?: string | null;
	instructor_name?: string | null;
	price_kobo?: number | null;
	currency?: string | null;
	category?: string | null;
	language?: string | null;
	total_enrolled?: number;
	subscription_eligible?: boolean;
	is_published_to_university?: boolean;
	/** Resolved cover image URL when a track has cover media. */
	thumbnail_url?: string | null;
	avg_rating?: number;
	review_count?: number;
	created_at: string;
}

/** A module within a marketplace track (GET /learn/university/catalog/{slug}). */
export interface CatalogModule {
	id: string;
	title: string;
	slug: string;
	sort_order: number;
	lessons?: CatalogLesson[];
}

/** A lesson within a marketplace track module. */
export interface CatalogLesson {
	id: string;
	title: string;
	slug: string;
	lesson_type?: string | null;
	duration_minutes?: number | null;
	sort_order: number;
}

/** GET /learn/university/catalog/{slug} — track detail with modules/lessons. */
export interface CatalogTrackDetail {
	track: CatalogTrack;
	modules: CatalogModule[];
}

/** GET /learn/university/instructors/{id} — public instructor profile. */
export interface InstructorProfile {
	id: string;
	display_name: string;
	headline?: string | null;
	bio?: string | null;
	avatar_url?: string | null;
	tier?: string | null;
	tracks?: CatalogTrack[];
}

// ── Geo (ADR-037 / ADR-064) ─────────────────────────────────────────

/** GET /api/v1/geo/me — GeoIP context for the current request IP. */
export interface GeoContext {
	country_code: string;
	country_name: string;
	city: string;
	subdivision: string;
	time_zone: string;
	latitude: number;
	longitude: number;
	is_valid: boolean;
}

/** GET /api/v1/geo/detect — narrower projection for locale detection. */
export interface GeoDetect {
	country: string;
	state: string;
	city: string;
}

/** GET /api/v1/geo/countries — a country-level geo space (ADR-005). */
export interface GeoCountry {
	id: string;
	name: string;
	slug: string;
	geo_code: string;
	level: 'country';
	currency_code?: string | null;
	member_count: number;
	post_count: number;
	follower_count: number;
}

/** GET /api/v1/geo/countries/{code}/states — a state/province geo space. */
export interface GeoState {
	id: string;
	name: string;
	slug: string;
	geo_code: string | null;
	level: 'state';
	parent_id: string;
	currency_code?: string | null;
	member_count: number;
}

/** GET /api/v1/geo/states/{id}/cities — a city-level geo space. */
export interface GeoCity {
	id: string;
	name: string;
	slug: string;
	geo_code: string | null;
	level: 'city';
	parent_id: string;
	currency_code?: string | null;
	member_count: number;
}

/** PATCH /api/v1/me/preferences/content-country response. */
export interface ContentCountryPreference {
	status: string;
	content_country: string;
}


// ============================================================
// V2 Feature Types (Phases 33–35, Sections 60–74)
// ============================================================

// --- Section 70: Visa Checklist ---
export interface ChecklistCountryOption {
	id: string;
	name: string;
	slug: string;
	visa_types?: ChecklistVisaTypeOption[];
}

export interface ChecklistVisaTypeOption {
	value: string;
	label: string;
	guide_id: string;
}

export interface ChecklistSelector {
	origins: ChecklistCountryOption[];
	destinations: ChecklistCountryOption[];
}

export interface ChecklistItem {
	item_key: string;
	label: string;
	completed: boolean;
	notes: string;
}

// --- Section 71: Readiness Assessment ---
export interface AssessmentThreshold {
	min: number;
	max: number;
	label: string;
	color: string;
}

export interface AssessmentScoringConfig {
	max_score: number;
	thresholds: AssessmentThreshold[];
	dimension_weights: Record<string, number>;
}

export interface AssessmentFieldScoringConfig {
	weight: number;
	dimension: string;
	/** Internal scoring key; stripped from public form responses. */
	option_scores?: Record<string, number>;
}

export interface AssessmentFieldOption {
	value: string;
	label: string;
}

export interface AssessmentField {
	id: string;
	label: string;
	key: string;
	field_type: string;
	is_required: boolean;
	position: number;
	placeholder: string;
	options: AssessmentFieldOption[];
	config: AssessmentFieldScoringConfig;
}

export interface AssessmentForm {
	id: string;

	name: string;
	slug: string;
	description: string;
	is_multi_step: boolean;
	submit_label: string;
	status: string;
	scoring_config: AssessmentScoringConfig;
	fields: AssessmentField[];
	/** Derived union of field scoring dimensions and curated roadmap library keys (§71). */
	dimensions: string[];
}

export interface AssessmentRoadmapStep {
	title: string;
	description: string;
	content_slug: string;
	dimension: string;
	priority: number;
}

export interface AssessmentResult {
	id: string;
	total_score: number;
	dimension_scores: Record<string, number>;
	threshold_label: string;
	roadmap: AssessmentRoadmapStep[];
	user_id: string;
	submission_id: string;
	created_at: string;
}

// --- Section 72: Cost of Living Calculator ---
export interface ColCity {
	id: string;
	title: string;
	description: string;
	geo_space_name: string;
	geo_space_slug: string;
	is_baseline: boolean;
	last_updated: string;
	position: number;
}

export interface ColCitiesListResponse {
	cities: ColCity[];
	total: number;
	page: number;
	limit: number;
}

export interface ColCityData {
	id: string;
	title: string;
	geo_space_name: string;
	is_baseline: boolean;
	last_updated: string;
	values: Record<string, string>;
}

export interface ColCategoryData {
	key: string;
	label: string;
	values: Record<string, number>;
	ratios: Record<string, number>;
}

export interface ColComparisonMatrix {
	cities: ColCityData[];
	categories: ColCategoryData[];
	baseline_city_id: string;
	overall_index: Record<string, number>;
}

// --- Source attribution (scholarships, jobs, remittance, content, feed) ---
export interface SourceAttribution {
	id: string;
	name: string;
	slug: string;
	logo_url?: string | null;
	source_url: string;
	trust_level: number;
}

// --- Section 73: Scholarship Finder ---
export interface Scholarship {
	guide_id: string;
	id: string;
	title: string;
	slug: string;
	summary: string;
	description: string;
	institution: string;
	country: string;
	level_of_study: string;
	award_amount_usd: number | null;
	is_renewable: boolean | null;
	application_deadline: string;
	application_url: string;
	eligibility: string;
	how_to_apply: string;
	contact_email: string;
	field_values: Record<string, string>;
	// Present in the API payload; optional here since not every consumer
	// renders them.
	intel_record_id?: string;
	country_space_id?: string;
	fields_of_study?: string[];
	source?: SourceAttribution;
}

export interface SavedScholarship {
	id: string;
	scholarship: Scholarship;
	status: string;
	notes: string | null;
	remind_before_days: number;
	last_reminded_at: string | null;
	created_at: string;
	updated_at: string;
}

// --- Section 74: Remittance Tracker ---
export interface RemittanceProvider {
	id: string;
	name: string;
	slug: string;
	website_url?: string;
	data_source: string;
	is_active: boolean;
	display_order: number;
	created_at: string;
}

export interface RemittanceRate {
	id: string;
	provider_id: string;
	provider_name: string;
	provider_slug: string;
	provider_website?: string;
	send_currency: string;
	receive_currency: string;
	rate: number;
	fee_flat?: number;
	fee_pct?: number;
	transfer_time?: string;
	fetched_at: string;
	is_manual: boolean;
	/** §112 — mid-market benchmark row (seeded 'mid-market' provider). Not a
	 *  bookable provider quote — display with a badge, exclude from best-rate. */
	is_benchmark?: boolean;
	source_url?: string;
}

export interface RemittanceBestRate {
	provider_id: string;
	provider_name: string;
	rate: number;
	receive_amount: number;
}

/**
 * GET /remittance/calculate response — ranked provider results plus an
 * optional mid-market benchmark reference (§112, ADR-125). The backend
 * serializes the rows under `results`, not `providers`.
 */
export interface RemittanceCalculation {
	send_amount: number;
	send_currency: string;
	receive_currency: string;
	results: RemittanceProviderCalc[];
	best_provider_id?: string;
	best_receive_amount?: number;
	/** §112 — mid-market benchmark reference; carries no fees and is never
	 *  ranked against the provider results. */
	benchmark?: RemittanceBenchmark | null;
}

/** §112 — mid-market benchmark reference row in the calculator response. */
export interface RemittanceBenchmark {
	provider_id: string;
	provider_name: string;
	rate: number;
	receive_amount: number;
}

export interface RemittanceProviderCalc {
	provider_id: string;
	provider_name: string;
	rate: number;
	receive_amount: number;
	fee_flat: number;
	fee_pct?: number;
	total_fees: number;
	effective_rate?: number;
	transfer_time?: string;
	/** §112 — defensive flag; benchmark rows are normally split into
	 *  `RemittanceCalculation.benchmark` by the backend. */
	is_benchmark?: boolean;
}

export interface RemittanceHistoryPoint {
	day: string;
	max_rate: number;
	min_rate: number;
	avg_rate: number;
}

export interface RemittanceCorridor {
	send_currency: string;
	receive_currency: string;
}

export interface RemittanceAlert {
	id: string;
	send_currency: string;
	receive_currency: string;
	target_rate: number;
	direction: string;
	is_active: boolean;
	last_triggered_at?: string;
	created_at: string;
}

/* --- §112 FX Currency Converter (ADR-125) --- */
// GET /fx/currencies, GET /fx/convert?amount=&from=&to=

/** GET /fx/currencies — picker currency row (matrix-covered codes). */
export interface FxCurrency {
	code: string;
	name: string;
	symbol: string;
}

/** GET /fx/convert?amount=&from=&to= — mid-market conversion result. */
export interface FxConvertResult {
	amount: number;
	from: string;
	to: string;
	/** Quote units per 1 unit of `from`. */
	rate: number;
	converted: number;
	/** When the underlying matrix row was fetched. */
	fetched_at: string;
	/** True when the serving matrix row is older than the freshness window —
	 *  render a staleness warning (§112). */
	stale: boolean;
}

// --- Section 63: Jobs Board ---
/** The entity that published the listing — org (preferred) or author user.
 *  Finder cards use it as the avatar/name/link fallback (§63, migration 000734). */
export interface JobPoster {
	kind: 'organization' | 'user';
	id: string;
	/** Org slug or username — hover card + profile link target. */
	slug: string;
	name: string;
	logo_url?: string;
	verified?: boolean;
	website_url?: string;
}

export interface JobListing {
	id: string;
	title: string;
	slug: string;
	summary: string;
	field_values: Record<string, string>;
	published_at: string;
	organization_id?: string;
	source?: SourceAttribution | null;
	poster?: JobPoster | null;
}

export interface JobDetail {
	id: string;
	title: string;
	slug: string;
	summary: string;
	body: string;
	field_values: Record<string, string>;
	published_at: string;
	organization_id?: string;
	source?: SourceAttribution | null;
	poster?: JobPoster | null;
	/** Authenticated viewer's own application, or null. */
	my_application?: { id: string; status: string; applied_at?: string } | null;
}

export interface JobApplication {
	id: string;
	job_id: string;
	job_title?: string;
	job_slug?: string;
	applicant_id?: string;
	applicant_username?: string;
	applicant_email?: string;
	status: string;
	employer_notes?: string;
	applied_at: string;
	status_updated_at?: string;
	/** Linked form_submissions.field_values (resume_url, cover_letter, …) — employer view only. */
	submission?: Record<string, unknown>;
}

// --- Section 64: Gamification ---
export interface Badge {
	id: string;
	slug: string;
	name: string;
	description: string;
	trigger_type: string;
	trigger_value: number;
	trigger_activity_key: string;
	is_active: boolean;
	is_system: boolean;
	display_order: number;
	created_at: string;
}

export interface EarnedBadge {
	id: string;
	slug: string;
	name: string;
	description: string;
	icon_media_id: string | null;
	awarded_at: string;
}

export interface Challenge {
	id: string;
	slug: string;
	name: string;
	description?: string;
	/** Optional hero image for the challenge card (media id). */
	image_media_id?: string;
	/** Activity key that increments progress (e.g. "content.created"). */
	activity_key?: string;
	/** Number of qualifying actions needed to complete the challenge. */
	target_count?: number;
	starts_at: string;
	/** NULL when the challenge has no end date (ongoing). */
	ends_at?: string | null;
	reward_points: number;
	/** Badge id granted on completion, when the challenge awards one. */
	reward_badge_id?: string;
	is_active: boolean;
	created_at?: string;
}

export interface LeaderboardEntry {
	user_id: string;
	username: string;
	display_name?: string;
	avatar_url?: string | null;
	points: number;
	rank: number;
}

// --- Section 60: Referral System ---
export interface ReferralStats {
	code: string;
	click_count: number;
	total: number;
	credited: number;
	pending: number;
}

export interface ReferralHistoryEntry {
	id: string;
	referral_code: string;
	status: string;
	created_at: string;
}

// --- Section 62: Newsletter ---
export interface NewsletterSubscriber {
	id: string;

	email: string;
	user_id: string | null;
	status: string;
	created_at: string;
	digest_enabled: boolean;
	campaign_enabled: boolean;
	subscribed_at: string;
	unsubscribe_token: string | null;
}

// --- Section 65: Content Import ---
export interface ContentImport {
	id: string;

	user_id: string;
	source_type: string;
	source_url: string | null;
	status: string;
	total_items: number;
	imported_count: number;
	failed_count: number;
	error_log: number[];
	created_at: string;
	completed_at: string | null;
}

export interface ImportSource {
	value: string;
	label: string;
	requires_url: boolean;
}

// --- Section 106: Video Content Import ---
export interface VideoImportJob {
	id: string;

	user_id: string;
	source_type: string;
	source_url: string | null;
	query: string | null;
	target_author_user_id: string | null;
	status: string;
	total_items: number;
	imported_count: number;
	failed_count: number;
	error_log: unknown[];
	created_at: string;
	started_at: string | null;
	completed_at: string | null;
	auto_publish: boolean;
}

// §106 — a channel ownership claim or takedown request filed by a user.
export interface VideoChannelClaim {
	id: string;
	claimant_user_id: string;
	video_author_user_id: string;
	claim_type: 'claim' | 'takedown';
	evidence_url: string | null;
	notes: string | null;
	status: 'pending' | 'approved' | 'rejected' | 'withdrawn';
	reviewed_by: string | null;
	resolution_note: string | null;
	created_at: string;
	resolved_at: string | null;
	video_source_id: string | null;
}

// --- Section 67: Travel Search ---
export interface TravelFlightResult {
	provider: string;
	price: number;
	currency: string;
	departure_time: string;
	arrival_time: string;
	airline: string;
	/** Canonical travel_airlines key (e.g. "iata:BA") resolved at
	 *  persistence — present on inventory-backed results. */
	airline_key?: string;
	flight_number: string;
	stops: number;
	duration: string;
	booking_url: string;
	offer_id?: string;
}

export interface TravelHotelResult {
	provider: string;
	name: string;
	price: number;
	currency: string;
	check_in: string;
	check_out: string;
	star_rating: number;
	address: string;
	image_url: string;
	booking_url: string;
	offer_id?: string;
}

/** GET /travel/hotels — one property row in the hotel directory (ADR-124
 *  extension). Rows accumulate from hotel searches and the provider content
 *  feed sync — last_price is the last-seen offer price, not live pricing. */
export interface TravelHotelDirectoryItem {
	hotel_id?: string;
	name: string;
	city: string;
	star_rating?: number;
	address?: string;
	image_url?: string;
	last_price?: number;
	last_currency?: string;
}

export interface TravelHotelDirectoryResponse {
	results: TravelHotelDirectoryItem[];
	total: number;
	page: number;
	page_size: number;
}

/** GET /travel/airlines — catalog row joined to observed provider coverage
 *  (ADR-124 extension). `providers` holds the display names of configured
 *  integrations whose offers have carried this airline. */
export interface TravelAirlineItem {
	airline_key: string;
	iata_code?: string;
	icao_code?: string;
	name: string;
	country?: string;
	observed: boolean;
	providers: string[];
	last_seen_at?: string;
}

export interface TravelAirlineDirectoryResponse {
	results: TravelAirlineItem[];
	total: number;
	page: number;
	page_size: number;
}

export interface TravelSearchResponse {
	results: TravelFlightResult[] | TravelHotelResult[];
	cache_hit: boolean;
	/** Which result layer served the query (§67, ADR-124): Valkey cache,
	 *  offer inventory, a fresh or stale travel_search_results row, or a
	 *  live provider call. */
	source?: 'cache' | 'inventory' | 'stored' | 'stale' | 'provider';
	search_time_ms: number;
	/** Filtered-set size before pagination (ADR-124 presentation params). */
	total?: number;
	page?: number;
	/** 0 = full set returned (client-side pagination contract). */
	page_size?: number;
}

/* ================================================================== *
 * V2 Community Features (§79–§89, §98) + §102 Mentorship Matching
 *
 * Shapes mirror the Go backend DTOs exactly (verified against the
 * internal/{story,wiki,scamalert,planninggroups,visatimeline,digest,
 * mutualaid,calendar,japastage,ama,achievement,servicedir,mentorship}
 * service.go JSON tags). Optional backend fields use `omitempty` and
 * are marked optional here; nullable pointer/pgtype fields are unioned
 * with null.
 * ================================================================== */

// --- Section 79: Community Story Wall ---
export interface Story {
	id: string;
	author_id?: string;
	title: string;
	slug: string;
	body: string;
	body_html: string;
	status: string;
	visibility: string;
	view_count: number;
	comment_count: number;
	published_at: string;
	created_at: string;
	updated_at: string;
	origin_space_id?: string;
	origin_space_name?: string;
	origin_space_code?: string;
	destination_space_id: string;
	destination_space_name: string;
	destination_space_code?: string;
	relocation_year?: number;
	visa_category?: string;
	profession?: string;
	biggest_challenge?: string;
	most_useful_resource?: string;
	best_advice?: string;
	verified: boolean;
	author_name?: string;
	author_username?: string;
	author_display_name?: string;
	author_avatar?: string;
	reaction_count?: number;
	bookmark_count?: number;
	/** Series membership (shared blog_series model — §14 series nav on the
	 *  story single mirrors the blog single). */
	series_id?: string;
	series_title?: string;
	series_slug?: string;
	series_part?: number;
	series_total?: number;
	/** §34 cover/hero media — rendered on the story single when set. */
	featured_image_url?: string | null;
	header_image_url?: string | null;
}

/** Structured meta subset of a Story (origin/destination/year/visa/highlights). */
export interface StoryMeta {
	origin_space_name: string | null;
	destination_space_name: string | null;
	relocation_year: number | null;
	visa_category: string | null;
	profession: string | null;
	biggest_challenge: string | null;
	most_useful_resource: string | null;
	best_advice: string | null;
	verified: boolean;
}

// --- Section 80: Destination Wiki ---
// §80 Wiki Types: category tokens come from the wiki_type_categories
// template (admin-managed), not a fixed enum — this is a plain string.
export type WikiCategory = string;

export interface WikiPageIndex {
	id: string;
	space_id: string;
	slug: string;
	title: string;
	category: string;
	is_featured: boolean;
	is_locked: boolean;
	updated_at: string;
	space_slug?: string;
	space_name?: string;
	/** §80: binds the page to a child entity class (e.g. 'visa_route') — the
	 *  page's Data panel lists those entities instead of the space's fields. */
	data_entity_type?: string | null;
}

export interface WikiDestination {
	space_id: string;
	slug: string;
	name: string;
	geo_code?: string;
	region?: string;
	/** The space's `level` field — 'custom' for non-geo types (e.g.
	 *  universities report 'custom', not 'university'). */
	level: string;
	page_count: number;
	/** §80 Reader UX: space logo resolved to a signed URL; absent when
	 *  the space has no logo media. */
	logo_url?: string;
	/** §80 Reader UX: space description excerpt for card subtitles. */
	description?: string;
}

/** One row of GET /wiki/types — an enabled wiki type over a space_type. */
export interface WikiTypeItem {
	space_type_id: string;
	space_type_slug: string;
	space_type_name: string;
	enabled: boolean;
	label?: string | null;
	icon?: string | null;
	position: number;
	/** §80 Hierarchical UX: declared parent space_type ids — the landing
	 *  chooser's drill-down levels derive from these edges. */
	parent_type_ids?: string[];
}

/** §80 Hierarchical UX — one node in a page's ancestor chain (breadcrumb)
 *  or a descendant row reference (rollup table). */
export interface WikiSpaceRef {
	/** §80 Reader UX: space UUID — enables compare-preselect links on
	 *  rollup rows; absent when the backend can't resolve it. */
	id?: string;
	slug: string;
	name: string;
	level: string;
	type_slug?: string;
	/** §80 Hierarchical UX: the ancestor's wiki-type display label
	 *  (wiki_type_configs.label, falling back to the space_types name). */
	type_label?: string;
}

/** §80 Hierarchical UX — one sibling page for Prev/Next navigation. */
export interface WikiSibling {
	slug: string;
	title: string;
	position: number;
	current?: boolean;
}

/** §80 Wiki Topic Pages — one segment of a page's ancestor chain (the
 *  category stub, then template ancestors root-first). Drives the in-page
 *  breadcrumb segments and the "Back to {parent}" link. */
export interface WikiPageParent {
	slug: string;
	title: string;
}

/** Aggregated metric values for one rollup field — absent members are
 *  metrics the category's rollup config didn't request. */
export interface WikiRollupMetric {
	min?: number;
	avg?: number;
	max?: number;
}

/** One aggregated field in a category rollup (e.g. a price_range item's
 *  min-of-mins / avg-of-avgs / max-of-maxes across descendant spaces). */
export interface WikiRollupField {
	key: string;
	label: string;
	field_type: string;
	section?: string;
	/** ISO code from the field's {currency_code} option — resolved to a
	 *  name/symbol via the shared currency list. */
	currency_code?: string;
	/** §80 Wiki Topic Pages — the part a price_parts field aggregates;
	 *  row cells project this member of the stored {parts:{...}}. */
	primary_part?: string;
	/** The primary part's declared kind (money|number) — controls whether
	 *  the projected cell currency-formats or renders a bare number. */
	primary_kind?: string;
	metrics: WikiRollupMetric;
	samples: number;
}

/** One contributing descendant space and its values — a row in the
 *  "Cost of Living by City" table on a country/state page. */
export interface WikiRollupRow {
	space: WikiSpaceRef;
	values: Record<string, unknown>;
}

/** Read-time descendant aggregate attached to a rollup-enabled category
 *  page (§80). Never persisted — recomputed per request. */
export interface WikiRollup {
	fields: WikiRollupField[];
	rows: WikiRollupRow[];
	spaces: number;
}

/** §80 directories — one card in a directory-mode listing: the descendant
 *  space's identity plus geo_code and its active wiki page count. */
export interface WikiDirectoryChild {
	slug: string;
	name: string;
	level: string;
	type_slug?: string;
	geo_code?: string;
	page_count: number;
	depth: number;
}

/** §80 directories — read-time child-space listing on a directory-mode
 *  category page. total counts every matching descendant (pre-cap);
 *  children is capped — the full set stays reachable via
 *  GET /wiki/destinations?within={slug}&level={level}. */
export interface WikiDirectory {
	levels: string[];
	children: WikiDirectoryChild[];
	total: number;
}

export interface WikiPage extends WikiPageIndex {
	space_slug: string;
	space_name: string;
	current_content: unknown;
	revision_summary?: string;
	submitted_by?: string;
	/** §80 Reader UX: byline avatar for the approved submitter. */
	submitted_by_avatar_url?: string;
	/** §80 Reader UX: "Was this page helpful?" aggregate + the viewer's
	 *  own vote; absent on lazy pages (no row to vote on). */
	helpful?: WikiHelpful | null;
	/** §80 Wiki Types: the page category's bound field group — schema + this
	 *  space's stored values. The group is the page's schema (always bound);
	 *  a prose-only page has a group with no fields. */
	fields?: import('$lib/api/wiki').WikiEntityFieldGroup | null;
	/** The bound group's id — present even when the group has no fields so
	 *  the entity-fields panel can scope itself (absent = unscoped). */
	field_group_id?: string;
	/** The bound group's slug — propose_field ops target groups by slug. */
	field_group_slug?: string;
	/** Admin-set label from the wiki type's category template. */
	category_label?: string;
	/** §80 Hierarchical UX: the space's ancestor chain (root-first) for the
	 *  breadcrumb — resolved from actual space links, arbitrated by the
	 *  declared type edges. */
	ancestors?: WikiSpaceRef[];
	/** §80 Hierarchical UX: multi-parent links not picked into the
	 *  breadcrumb chain — undeclared junction parents and declared
	 *  runners-up — rendered as "also under X". */
	ancestors_alt?: WikiSpaceRef[];
	/** §80 Wiki Topic Pages: page-level ancestor chain — category stub
	 *  first, then template ancestors root → immediate parent. Empty on
	 *  the category stub page itself and unmanaged-category pages. */
	parents?: WikiPageParent[];
	/** §80 Hierarchical UX: ordered sibling pages in this page's category
	 *  (Prev/Next derives from neighbors; the current page is flagged).
	 *  §80 Wiki Topic Pages: same-parentage — nested pages list sibling
	 *  topic pages under the same parent template. */
	siblings?: WikiSibling[];
	/** §80 Hierarchical UX: read-time descendant aggregate — present only on
	 *  a rollup-enabled category's canonical page. */
	rollup?: WikiRollup;
	/** §80 directories: read-time child-space listing — present only on a
	 *  directory-mode category's canonical page (which is always its lazy
	 *  or materialized stub page). */
	directory?: WikiDirectory;
	/** §58 — the host space's resolved country currency; display fallback
	 *  for currency/price_range fields without an explicit {currency_code}. */
	space_currency_code?: string;
	/** §80 Hierarchical UX: the page template the page was created from —
	 *  echoed so UIs can link sibling template pages. */
	page_template_id?: string;
	/** §80 Hierarchical UX: intra-category ordering. */
	position?: number;
	/** §80 lazy template pages: the category's declared sibling template
	 *  links ({name} substituted by the backend) — discovery targets for
	 *  pages that render without a wiki_pages row. */
	page_templates?: WikiPageTemplateLink[];
	/** §80 lazy template pages: the page rendered straight from its
	 *  template — no wiki_pages row until the first submission. */
	lazy?: boolean;
}

/** §80 Reader UX: aggregated "Was this page helpful?" votes on a page. */
export interface WikiHelpful {
	up: number;
	down: number;
	/** The viewer's current vote — null when unsigned-in or unvoted. */
	my_vote?: 'up' | 'down' | null;
}

/** One page-template discovery link on a wiki page (§80 lazy pages). */
export interface WikiPageTemplateLink {
	slug: string;
	label: string;
	position: number;
	/** §80 Wiki Topic Pages — the template's nesting edge; children group
	 *  under their parent in "More in this section" nav. */
	parent_template_id?: string;
	/** The template row id — parent_template_id references resolve to
	 *  this without a slug join. */
	template_id?: string;
}

export type WikiRevisionStatus = 'approved' | 'rejected' | 'pending';

export interface WikiRevision {
	id: string;
	status: string;
	/** §80: 'content' (prose), 'fields' (field ops), or 'mixed'. */
	kind?: string;
	content?: unknown;
	/** Field ops submitted by a fields/mixed revision (redacted while pending
	 *  for non-reviewers). */
	ops?: unknown;
	summary?: string;
	ai_generated: boolean;
	submitted_by?: string;
	reviewed_by?: string;
	reviewed_at?: string;
	created_at: string;
}

// --- Section 81: Scam Alert Board ---
export type ScamType =
	| 'fraudulent_agent'
	| 'fake_job'
	| 'housing_fraud'
	| 'fake_scholarship'
	| 'whatsapp_telegram'
	| 'other';

export type ScamStatus = 'under_review' | 'confirmed' | 'debunked' | 'duplicate';

export type ScamPlatform =
	| 'whatsapp'
	| 'instagram'
	| 'linkedin'
	| 'email'
	| 'website'
	| 'other';

export interface ScamReport {
	id: string;
	reporter_id?: string;
	scam_type: string;
	destination_space_id?: string;
	destination_space_name?: string;
	destination_space_code?: string;
	origin_space_id?: string;
	origin_space_name?: string;
	origin_space_code?: string;
	origin_community?: string;
	platform_used?: string;
	approximate_date?: string;
	amount_range?: string;
	description: string;
	reported_to_authorities: boolean;
	status: string;
	moderator_note?: string;
	is_pinned: boolean;
	is_subscribed?: boolean;
	pinned_space_id?: string;
	reviewed_by?: string;
	reviewed_at?: string;
	created_at: string;
	updated_at?: string;
	reporter_name?: string;
}

// --- Section 82: Japa Planning Groups ---
export interface PlanningGroup {
	id: string;
	name: string;
	description?: string;
	destination_space_id: string;
	visa_category?: string;
	target_departure_window?: string;
	target_departure_at?: string;
	max_members?: number;
	is_open_join: boolean;
	owner_id: string;
	expires_at?: string;
	created_at: string;
	updated_at?: string;
	member_count: number;
	status: string;
}

export interface PlanningMilestone {
	id: string;
	group_id: string;
	user_id: string;
	title: string;
	completed: boolean;
	completed_at?: string;
	created_at: string;
}

/** GET /planning-groups/:id/members item — a planning group member. */
export interface MemberDTO {
	user_id: string;
	departure_date?: string;
	joined_at: string;
	username?: string;
	avatar_url?: string;
}

export interface PlanningCheckin {
	id: string;
	group_id: string;
	user_id: string;
	note: string;
	created_at: string;
	username?: string;
}

// --- Section 83: Visa Timeline Tracker ---
export type VisaApplicationMethod = 'online' | 'in_person' | 'vfs';

export type VisaMilestoneType =
	| 'applied'
	| 'biometrics'
	| 'medical'
	| 'docs_requested'
	| 'decision'
	| 'visa_issued';

export type VisaTimelineOutcome = 'approved' | 'refused' | 'withdrawn' | 'pending';

export interface VisaTimeline {
	id: string;
	user_id: string;
	origin_space_id?: string;
	destination_space_id: string;
	visa_category: string;
	consulate?: string;
	application_method?: string;
	outcome?: string;
	is_shared: boolean;
	anonymised_at?: string;
	created_at: string;
	updated_at: string;
	milestones?: VisaTimelineMilestone[];
}

export interface VisaTimelineMilestone {
	id: string;
	timeline_id: string;
	milestone_type: string;
	occurred_at: string;
	notes?: string;
	created_at: string;
}

export interface VisaProcessingStats {
	origin_space_id?: string;
	destination_space_id: string;
	visa_category: string;
	consulate?: string;
	period_start: string;
	sample_count: number;
	avg_days?: number;
	median_days?: number;
	p10_days?: number;
	p90_days?: number;
	refreshed_at: string;
}

export interface TrendPoint {
	period_start: string;
	sample_count: number;
	avg_days?: number;
	median_days?: number;
}

// --- Section 84: Community Digest Widget ---
export interface DigestEdition {
	id: string;
	space_id?: string;
	cadence: string;
	status: string;
	scheduled_for?: string;
	published_at?: string;
	email_sent: boolean;
	generated_by?: string;
	reviewed_by?: string;
	created_at: string;
	/** JSONB content blob (present on the detail/latest endpoint). */
	content?: DigestContent | string | null;
}

export interface DigestContent {
	top_threads?: DigestThread[];
	new_members?: DigestMember[];
	events?: DigestEvent[];
	story?: DigestStory | null;
	poll?: DigestPoll | null;
	[key: string]: unknown;
}

export interface DigestThread {
	id?: string;
	title?: string;
	slug?: string;
	url?: string;
}

export interface DigestMember {
	id?: string;
	username?: string;
	name?: string;
	/** Backend emits display_name (§84 — only username + display_name). */
	display_name?: string;
	avatar_url?: string;
}

export interface DigestEvent {
	id?: string;
	title?: string;
	slug?: string;
	url?: string;
	starts_at?: string;
}

export interface DigestStory {
	title?: string;
	slug?: string;
	url?: string;
}

export interface DigestPoll {
	question?: string;
	url?: string;
}

// --- Section 85: Mutual Aid Board ---
export type MutualAidPostType = 'need' | 'offer' | 'skill_swap';

export type MutualAidCategory =
	| 'housing'
	| 'transport'
	| 'documents'
	| 'job_search'
	| 'language'
	| 'social'
	| 'other';

export interface MutualAidPost {
	id: string;
	post_type: string;
	category?: string;
	title: string;
	description?: string;
	destination_space_id: string;
	destination_space_name?: string;
	destination_space_code?: string;
	city_space_id?: string;
	city_space_name?: string;
	city_space_code?: string;
	is_fulfilled: boolean;
	status: string;
	expires_at: string;
	created_at: string;
	updated_at: string;
	author_id?: string;
	author_name?: string;
	author_avatar?: string;
}

// --- Section 86: Community Calendar ---
export type CalendarCategory =
	| 'visa_window'
	| 'embassy_closure'
	| 'application_deadline'
	| 'exam_window'
	| 'immigration_office'
	| 'other';

export interface CalendarEntry {
	id: string;
	title: string;
	description?: string;
	category: string;
	destination_space_id?: string;
	origin_space_id?: string;
	occurs_at: string;
	ends_at?: string;
	is_recurring: boolean;
	recurrence_pattern?: string;
	source_url?: string;
	status?: string;
	submitted_by?: string;
	reviewed_by?: string;
	reviewed_at?: string;
	// ADR-088 universal AI content generation columns.
	ai_generated?: boolean;
	ai_model?: string;
	hitl_reviewed_by?: string;
	hitl_review_notes?: string;
	ai_generated_at?: string;
	is_saved?: boolean;
	created_at: string;
	updated_at?: string;
}

// --- Section 87: Japa Stage Profiles ---
export type JapaStage =
	| 'considering'
	| 'planning'
	| 'preparing'
	| 'applied'
	| 'approved'
	| 'relocated'
	| 'settled'
	| 'traveled'
	| 'returned';

/** ADR-122: journey class — class-scoped terminal stages apply per intent. */
export type JapaIntent =
	| 'relocation'
	| 'study'
	| 'work'
	| 'family'
	| 'visit'
	| 'business';

export const RELOCATION_INTENTS: readonly JapaIntent[] = ['relocation', 'study', 'work', 'family'];
export const TRIP_INTENTS: readonly JapaIntent[] = ['visit', 'business'];

/** GET /japa-stage/stages item — `intents` lists the classes the stage applies to (ADR-122). */
export interface JapaStageOption {
	value: JapaStage;
	label?: string;
	intents?: JapaIntent[];
}

/** GET /me/japa-stage — the caller's own stage profile. */
export interface MyJapaStage {
	user_id: string;
	username?: string;
	display_name?: string;
	avatar_url?: string;
	stage?: string;
	intent?: string;
	destination_space_id?: string;
	destination_space_name?: string;
	destination_space_code?: string;
	is_public: boolean;
	history?: unknown;
	updated_at?: string;
}

/** GET /members/by-stage item — a public member's stage card. */
export interface MemberStage {
	user_id: string;
	username?: string;
	display_name?: string;
	avatar_url?: string;
	stage: string;
	intent?: string;
	destination_space_id?: string;
	destination_space_name?: string;
	destination_space_code?: string;
	updated_at?: string;
}

/** GET /users/:username/japa-stage — a single member's public japa stage. */
export interface PublicJapaStage {
	user_id: string;
	username?: string;
	stage?: string;
	intent?: string;
	destination_space_id?: string;
	destination_space_name?: string;
	destination_space_code?: string;
	updated_at?: string;
}

// --- Section 88: Ask an Expat (AMA) ---
export interface AMAThread {
	id: string;
	title: string;
	slug: string;
	body: string;
	author_user_id?: string;
	author_name?: string;
	author_avatar?: string;
	is_anonymous?: boolean;
	ama_config: unknown;
	created_at: string;
	updated_at: string;
}

// --- Section 89: Achievement Feed ---
export type AchievementMilestoneType =
	| 'visa_approved'
	| 'job_offer'
	| 'university_accepted'
	| 'arrived'
	| 'one_year'
	| 'citizenship'
	| 'other';

export interface Achievement {
	id: string;
	title: string;
	slug: string;
	body: string;
	body_html: string;
	status: string;
	visibility: string;
	view_count: number;
	comment_count: number;
	reaction_count: number;
	published_at: string;
	created_at: string;
	updated_at: string;
	milestone_type?: string;
	origin_space_id?: string;
	origin_space_name?: string;
	origin_space_code?: string;
	destination_space_id: string;
	destination_space_name: string;
	destination_space_code?: string;
	is_anonymous: boolean;
	is_owner?: boolean;
	author_id?: string;
	author_name?: string;
	author_avatar?: string;
	allowed_reactions?: AchievementReaction[];
}

export interface AchievementReaction {
	emoji: string;
	label: string;
}

// --- Section 98: Service Directory ---
export type ServiceCategory =
	| 'immigration_legal'
	| 'relocation'
	| 'finance'
	| 'education'
	| 'health'
	| 'community'
	| 'other';

export interface ServiceListing {
	id: string;
	title: string;
	slug: string;
	body: string;
	body_html: string;
	status: string;
	visibility: string;
	view_count: number;
	comment_count: number;
	published_at: string;
	created_at: string;
	updated_at: string;
	org_id?: string;
	org_name?: string;
	org_slug?: string;
	service_category?: string;
	destination_space_id?: string;
	destination_space_name?: string;
	destination_space_code?: string;
	city_space_id?: string;
	city_space_name?: string;
	city_space_code?: string;
	origin_communities?: string[];
	is_remote: boolean;
	website_url?: string;
	booking_url?: string;
	price_range?: string;
	is_verified: boolean;
	verification_type?: string;
	claim_status?: string;
	claimed_by_org_id?: string;
	claimed_org_name?: string;
	verified_by?: string;
	verified_at?: string;
	author_id?: string;
	author_name?: string;
	author_avatar?: string;
	// §111 package fields — populated when listing_kind='package'.
	listing_kind?: string;
	price_amount?: number | null; // minor units (kobo/cents)
	price_currency?: string;
	price_per?: 'person' | 'package' | 'group' | '';
	purchase_mode?: 'inquiry' | 'external' | 'paid' | '';
	capacity?: number | null;
	available_from?: string;
	available_until?: string;
	items?: PackageItem[];
	departures?: PackageDeparture[];
	platform_fee?: number | null;
	agent_balance_due?: number | null;
}

/** One ordered component line item of a service package (§111). */
export interface PackageItem {
	id: string;
	position: number;
	title: string;
	description?: string;
	component_content_id?: string;
	included: boolean;
	optional: boolean;
}

/** One scheduled departure/intake of a service package (§111). */
export interface PackageDeparture {
	id: string;
	starts_on: string;
	ends_on?: string;
	capacity?: number | null;
	booked_count: number;
	status: 'open' | 'full' | 'cancelled' | string;
}

/** A row of GET /me/service-bookings or /org/:id/service-bookings (§111). */
export interface ServiceBooking {
	id: string;
	listing_meta_id: string;
	listing_id?: string;
	listing_title: string;
	listing_slug: string;
	listing_kind: string;
	departure_id?: string;
	departure_starts_on?: string;
	departure_ends_on?: string;
	booker_user_id?: string;
	booker_username?: string;
	booker_avatar?: string;
	org_id: string;
	org_name?: string;
	org_slug?: string;
	intent: 'inquiry' | 'booking' | string;
	party_size: number;
	preferred_dates?: { start_on: string; end_on: string; flexible: boolean } | null;
	note?: string;
	status: string;
	purchase_mode: string;
	payment_id?: string;
	/** Joined payment row — pending means the fee checkout can be resumed. */
	payment_status?: string;
	payment_provider?: string;
	total_price?: number | null;
	fee_amount?: number | null;
	fee_currency?: string;
	agent_balance_due?: number | null;
	org_response_note?: string;
	dispute_reason?: string;
	expires_at?: string;
	confirmed_at?: string;
	fulfilled_at?: string;
	cancelled_at?: string;
	cancel_reason?: string;
	created_at: string;
}

/** POST /services/:id/bookings response — booking + fee checkout when paid. */
export interface ServiceBookingResult {
	booking: ServiceBooking;
	payment?: { id: string; checkout_url: string } | null;
}

/** One row of GET /me/organizations — org + the caller's membership role. */
export interface MyOrganization {
	id: string;
	name: string;
	slug: string;
	verified: boolean;
	verification_type?: string | null;
	member_role: string;
}

/** A single review embedded in GET /services/:slug (§98 + §29 sidecar). */
export interface ServiceReview {
	id: string;
	author_id: string;
	author_username: string;
	author_display_name?: string | null;
	author_avatar_url?: string | null;
	rating: number;
	title?: string | null;
	body?: string | null;
	created_at: string;
}

/** Review aggregate embedded in GET /services/:slug (§98 + §29 sidecar). */
export interface ServiceReviewAggregate {
	count: number;
	avg: number;
}

// --- Section 102: Mentorship Matching ---
/** GET /mentors item — a mentor's public discovery card. */
export interface Mentor {
	user_id: string;
	username: string;
	display_name: string | null;
	avatar_url: string | null;
	mentor_areas: string[];
	mentor_bio: string | null;
	mentor_since: string | null;
	mentor_capacity: number | null;
	active_mentee_count: number | null;
	available_slots: number;
}

/** GET /me/mentor-profile — the caller's own mentor settings. */
export interface MentorProfile {
	user_id: string;

	is_mentor: boolean | null;
	mentor_capacity: number | null;
	mentor_max_capacity: number;
	active_mentee_count: number | null;
	mentor_areas: string[];
	mentor_bio: string | null;
	mentor_since: string | null;
	display_name: string | null;
	avatar_url: string | null;
}

/** Mentorship connection (outgoing/incoming/admin list item). */
export interface MentorshipConnection {
	id: string;

	mentor_id: string;
	mentee_id: string;
	status: string;
	goals: string | null;
	destination_space_id: string | null;
	responded_at: string | null;
	completed_at: string | null;
	cancelled_at: string | null;
	cancel_reason: string | null;
	created_at: string;
	updated_at: string;
	mentor_username?: string;
	mentee_username?: string;
	dm_conversation_id?: string | null;
	mentor_display_name?: string;
	mentee_display_name?: string;
}

/* ================================================================== *
 * V2 functional tools (§§90–97, §§99–101) — geo-aware tool pattern.
 * Mirrors the apps/tools types so the main-app tool pages share the
 * same backend contract. Ref: ADR-086, ADR-088.
 * ================================================================== */

/** Common AI generation metadata (ADR-088). Present on all admin-managed content. */
export interface AiGenerationMeta {
	ai_generated: boolean;
	ai_model?: string | null;
	ai_prompt_hash?: string | null;
	hitl_reviewed_by?: string | null;
	hitl_reviewed_at?: string | null;
	hitl_review_notes?: string | null;
	is_japabot_indexed: boolean;
}

/** Geo Space reference for the ?dest= destination selector (GET /spaces?level=country). */
export interface GeoSpaceOption {
	id: string;
	name: string;
	slug: string;
	code?: string | null;
}

/* --- §90 Visa Route Comparison --- */
export interface VisaRoute {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	visa_category: string;
	name: string;
	slug: string;
	eligibility_summary?: string | null;
	requirements?: Record<string, unknown> | null;
	typical_fee_usd?: number | null;
	path_to_pr?: string | null;
	renewal_terms?: string | null;
	restrictions?: string | null;
	gov_source_url: string;
	source_url_last_checked?: string | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}
export interface VisaRouteWithAi extends VisaRoute, AiGenerationMeta {}

/* --- §91 Embassy Appointment Alerter --- */
export interface Embassy {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	slug: string;
	name: string;
	booking_url: string;
	poll_interval_seconds: number;
	is_active: boolean;
	last_polled_at?: string | null;
	created_at: string;
	updated_at: string;
}
export interface AppointmentAlert {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	embassy_slug: string;
	visa_category?: string | null;
	date_range_start: string;
	date_range_end: string;
	status: string;
	last_checked_at?: string | null;
	fired_at?: string | null;
	expires_at?: string | null;
	notification_channels: string[];
	created_at: string;
	updated_at: string;
}

/* --- §92 Arrival Checklist --- */
export interface ArrivalChecklistSelectorEntry {
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	visa_category?: string | null;
	guide_count: number;
	guide_id: string;
}
export interface ArrivalChecklistItem {
	id: string;
	guide_id: string;
	phase: string;
	title: string;
	description?: string | null;
	official_link?: string | null;
	origin_country_modifier?: Record<string, unknown> | null;
	sort_order: number;
	is_flagged: boolean;
	created_at: string;
}

export interface UserArrivalChecklist {
	guide_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	visa_category?: string | null;
	applicant_type?: string | null;
	total_items: number;
	completed_items: number;
	first_progress_at?: string;
	last_progress_at?: string;
}

export interface UserArrivalChecklistItem {
	id: string;
	guide_id: string;
	phase: string;
	title: string;
	description?: string | null;
	official_link?: string | null;
	sort_order: number;
	is_flagged: boolean;
	completed: boolean;
	completed_at?: string;
	notes?: string;
}

/* --- §93 Processing Time Tracker (reuses §83 Visa Timeline backend) --- */
export type ProcessingStat = VisaProcessingStats;
export type ProcessingTrendPoint = TrendPoint;

/* --- §94 Professional Credential Checker --- */
export interface CredentialProfession {
	id: string;
	slug: string;
	name: string;
	category?: string | null;
}
export interface CredentialPathway {
	id: string;
	profession_id: string;
	profession_slug: string;
	profession_name: string;
	profession_category?: string | null;
	issuing_space_id: string;
	issuing_space_name?: string | null;
	issuing_space_code?: string | null;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	regulatory_body: string;
	regulatory_body_url?: string | null;
	pathway_type: string;
	timeline_estimate?: string | null;
	cost_estimate_usd?: string | null;
	pathway_description: string;
	official_application_url?: string | null;
	source_url_last_checked?: string | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}
export interface CredentialPathwayWithAi extends CredentialPathway, AiGenerationMeta {}
export interface CredentialCommunityNote {
	id: string;
	pathway_id: string;
	user_id: string;
	user_name?: string | null;
	note: string;
	upvotes: number;
	status: string;
	created_at: string;
}
export interface CredentialPathwayDetail {
	pathway: CredentialPathwayWithAi;
	notes: CredentialCommunityNote[];
}

/* --- §95 CV Formatter --- */
export interface CvStandard {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	recommended_length_pages?: string | null;
	photo_policy?: string | null;
	date_format?: string | null;
	recommended_sections?: string[] | null;
	tone_guidance?: string | null;
	common_pitfalls?: string | null;
	created_at: string;
	updated_at: string;
}
export interface CvFormatSession {
	id: string;
	user_id: string;
	origin_space_id?: string | null;
	destination_space_id: string;
	input_text: string;
	output_text?: string | null;
	output_format: string;
	status: string;
	ai_model?: string | null;
	tokens_used?: number | null;
	created_at: string;
	expires_at: string;
}

/* --- §96 Housing Guide --- */
export interface HousingGuide {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	city_space_id?: string | null;
	city_space_name?: string | null;
	city_space_code?: string | null;
	accommodation_type?: string | null;
	search_platforms?: string[] | Record<string, unknown> | null;
	landlord_requirements?: string | null;
	rent_ranges?: Record<string, unknown> | null;
	scam_categories?: string[] | null;
	tips?: string | null;
	status: string;
	is_active: boolean;
	last_verified_at?: string | null;
	reviewer_name?: string | null;
	created_at: string;
	updated_at: string;
}
export interface HousingGuideWithAi extends HousingGuide, AiGenerationMeta {}

/* --- §97 Attestation Guide --- */
export interface AttestationPathway {
	id: string;
	document_type: string;
	issuing_space_id: string;
	issuing_space_name?: string | null;
	issuing_space_code?: string | null;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	apostille_required: boolean;
	pathway_steps: unknown[] | Record<string, unknown>;
	community_last_verified?: string | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}
export interface AttestationPathwayWithAi extends AttestationPathway, AiGenerationMeta {}
export interface AttestationCommunityUpdate {
	id: string;
	pathway_id: string;
	user_id: string;
	step_index?: number | null;
	fee_estimate?: string | null;
	turnaround_days?: number | null;
	note?: string | null;
	status: string;
	reviewed_by?: string | null;
	created_at: string;
}

/* --- §99 Language Test Hub --- */
export interface LanguageTest {
	id: string;
	slug: string;
	name: string;
	official_url?: string | null;
	validity_months?: number | null;
}
export interface LanguageTestRequirement {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	visa_category?: string | null;
	test_id: string;
	test_slug: string;
	test_name: string;
	minimum_score: string;
	score_breakdown?: Record<string, unknown> | null;
	notes?: string | null;
	source_url?: string | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}
export interface LanguageTestRequirementWithAi extends LanguageTestRequirement, AiGenerationMeta {}
export interface LanguageTestResource {
	id: string;
	test_id: string;
	title: string;
	resource_url: string;
	resource_type?: string | null;
	community_rating?: string | null;
	rating_count: number;
	created_at: string;
}
export interface LanguageTestScoreStory {
	id: string;
	user_id: string;
	test_id: string;
	starting_level?: string | null;
	final_score: string;
	destination_space_id?: string | null;
	destination_space_name?: string | null;
	prep_duration_weeks?: number | null;
	resources_used?: string[] | null;
	advice?: string | null;
	status: string;
	created_at: string;
	author_name?: string | null;
}
export interface LanguageTestHubPage {
	test: LanguageTest;
	resources_summary: LanguageTestResource[];
	stories_summary: LanguageTestScoreStory[];
}

/* --- §100 Tax Calculator --- */
export interface TaxGuide {
	id: string;
	origin_space_id: string;
	origin_space_name?: string | null;
	destination_space_id: string;
	destination_space_name?: string | null;
	employment_status: string;
	residency_trigger?: string | null;
	worldwide_income_taxed?: boolean | null;
	first_year_obligations?: string | null;
	action_checklist?: unknown[] | Record<string, unknown> | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}
export interface TaxGuideWithAi extends TaxGuide, AiGenerationMeta {}
export interface TaxTreaty {
	id: string;
	origin_space_id: string;
	origin_space_name?: string | null;
	destination_space_id: string;
	destination_space_name?: string | null;
	treaty_exists: boolean;
	treaty_summary?: string | null;
	treaty_source_url?: string | null;
	created_at: string;
	updated_at: string;
}

/* --- §101 Diaspora Community Locator --- */
export interface CommunityOrg {
	id: string;
	name: string;
	slug: string;
	org_type: string;
	origin_community: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	city_space_id?: string | null;
	city_space_name?: string | null;
	city_space_code?: string | null;
	address?: string | null;
	latitude?: number | null;
	longitude?: number | null;
	website_url?: string | null;
	meeting_frequency?: string | null;
	contact_info?: string | null;
	japaconnect_space_id?: string | null;
	japaconnect_org_id?: string | null;
	claim_status: string;
	is_verified: boolean;
	is_active: boolean;
	is_community_submitted: boolean;
	status: string;
	submitted_by?: string | null;
	submitter_name?: string | null;
	reviewed_by?: string | null;
	reviewed_at?: string | null;
	created_at: string;
	updated_at: string;
}
export interface CommunityOrgWithAi extends CommunityOrg, AiGenerationMeta {}

/** Org-type allow-list entry from GET /community-locator/org-types (Rule 16). */
export interface OrgTypeOption {
	value: string;
	label: string;
}

/* --- Japa Intelligence System (§7 Japa Radar) --- */

/**
 * v1 radar feed item shape — mirrors the backend `radarItem` struct in
 * internal/intelligence/handlers/radar.go. v1 surfaces promoted scholarships
 * only; the shape is forward-compatible so future domains (jobs, visa alerts,
 * FX updates) union in via the same envelope.
 */
export interface RadarItem {
	id: string;
	type: string;
	title: string;
	description?: string | null;
	country: string;
	university?: string | null;
	degree_level?: string | null;
	deadline?: string | null;
	application_url?: string | null;
	confidence_overall: number;
	trust_level: number;
	freshness_score: number;
	source_url: string;
	created_at?: string | null;
	/** Only populated by the detail endpoint (GET /intel/radar/feed/:id). */
	saved?: boolean | null;
}

/** Per-country scholarship count row from GET /intel/radar/counts. */
export interface RadarCountryCount {
	country: string;
	count: number;
}

/**
 * Response body for GET /intel/radar/counts (unwrapped from the {data: ...}
 * envelope by the API client). Ref: internal/intelligence/handlers/radar.go
 * HandleRadarCounts — {total_new, by_type, by_country}.
 */
export interface RadarCountsResponse {
	total_new: number;
	by_type: Record<string, number>;
	by_country: Record<string, number>;
}

/** Meta for GET /intel/radar/counts. */
export interface RadarCountsMeta {
	total_countries?: number;
	total_scholarships?: number;
}

/** Meta for GET /intel/radar/followed. */
export interface RadarFollowedMeta {
	page_size?: number;
	page_offset?: number;
	count?: number;
	followed_count?: number;
}

/** GET /api/v1/catalogs/:name — a public option catalog (Rule 16). */
export interface CatalogOption {
	value: string;
	label: string;
	default?: boolean;
}

/* ------------------------------------------------------------------ *
 * Guest homepage (ADR-058 §5 Exception #3 default seed)
 * ------------------------------------------------------------------ */

/** A curated tool card surfaced on the guest landing page.
 *  Backed by the public `landing.tools` site setting (Rule 16),
 *  falling back to the platform's default tool registry. */
export interface GuestHomeTool {
	id: string;
	name: string;
	slug?: string;
	description?: string;
	href?: string;
	icon?: string;
	accent?: string;
	badge?: string;
}

/** A featured-opportunity card on the guest landing page.
 *  Populated from the admin-configurable `landing.featured_opportunities`
 *  public setting; falls back to a curated seed set. */
export interface GuestHomeOpportunity {
	id: string;
	type: 'job' | 'scholarship' | 'visa' | 'business' | 'internship' | string;
	title: string;
	organization: string;
	location: string;
	location_code?: string;
	tag: string;
	badge?: string;
	value?: string;
	logo_url?: string | null;
	href: string;
	published_at?: string;
}

/** A success-story card on the guest landing page.
 *  Loaded from GET /api/v1/stories; falls back to seed examples. */
export interface GuestHomeStory {
	id: string;
	slug?: string;
	title: string;
	quote: string;
	author_name: string;
	author_role?: string;
	author_avatar_url?: string | null;
	destination?: string;
}

/** A marketing-stat block on the guest landing page.
 *  Backed by the public `landing.stats` setting; falls back to defaults. */
export interface GuestHomeStat {
	value: string;
	label: string;
	icon: string;
}

/** A category card on the guest landing page.
 *  Backed by the public `landing.categories` setting; falls back to defaults. */
export interface GuestHomeCategory {
	id: string;
	title: string;
	description: string;
	href: string;
	icon: string;
	color: string;
}

/** SSR payload for the default guest homepage. */
export interface GuestHomeData {
	topDestinations: SpaceSuggestion[];
	opportunities: GuestHomeOpportunity[];
	stories: GuestHomeStory[];
	tools: GuestHomeTool[];
	stats: GuestHomeStat[];
	categories: GuestHomeCategory[];
}

/* --- SEO Growth Tools (§§107–110, ADR-118..121) --- */

/** GET /crs/factors — a versioned CRS ruleset + factor point tables. */
export interface CrsRuleset {
	id: string;
	version_label: string;
	effective_date: string;
	status: string;
	source_url?: string;
}

export interface CrsFactor {
	group: string;
	key: string;
	input_descriptor: Record<string, unknown>;
	label: string;
	points_single: number;
	points_with_spouse?: number;
	sort_order: number;
}

export interface CrsFactorsResponse {
	ruleset: CrsRuleset;
	factors: CrsFactor[];
}

/** POST /crs/calculate request. */
export interface CrsCalculateInput {
	marital_status: 'single' | 'married';
	age: number;
	education: string;
	clb_first: { listening: number; reading: number; writing: number; speaking: number };
	clb_second?: { listening: number; reading: number; writing: number; speaking: number };
	canadian_experience_years: number;
	foreign_experience_years: number;
	certificate_of_qualification: boolean;
	spouse?: {
		education: string;
		clb_first: { listening: number; reading: number; writing: number; speaking: number };
		canadian_experience_years: number;
	};
	provincial_nomination: boolean;
	job_offer: boolean;
	sibling_in_canada: boolean;
	french_clb?: { listening: number; reading: number; writing: number; speaking: number };
	canadian_education: string;
	ruleset_version?: string;
}

/** POST /crs/calculate response. */
export interface CrsBreakdownItem {
	group: string;
	key: string;
	label: string;
	points: number;
}

export interface CrsCalculateResult {
	ruleset_version: string;
	score: number;
	breakdown: CrsBreakdownItem[];
	saved: boolean;
}

/** GET /crs/draws + /crs/draws/latest — Express Entry draw history. */
export interface CrsDraw {
	id: string;
	draw_number?: number;
	draw_date: string;
	category: string;
	crs_cutoff: number;
	invitations_issued: number;
	tie_break_rule?: string;
	source_url?: string;
}

/** GET /me/crs-scores — saved score history (auth). */
export interface CrsScoreHistoryItem {
	id: string;
	score: number;
	inputs: Record<string, unknown>;
	breakdown: CrsBreakdownItem[];
	created_at: string;
}

/** GET /pof/requirements — a proof-of-funds requirement row. */
export interface PofRequirement {
	id: string;
	destination_space_id: string;
	destination_space_name?: string;
	destination_space_code?: string;
	visa_type: string;
	program_label: string;
	amount_schedule: PofScheduleBand[] | PofScheduleFormula;
	currency: string;
	funds_must_be_liquid: boolean;
	accepted_proof?: string[];
	notes?: string;
	effective_date: string;
	source_url?: string;
	status: string;
	is_active: boolean;
	ai_generated: boolean;
	is_japabot_indexed: boolean;
}

export interface PofScheduleBand {
	members: number;
	amount: number;
	per_additional?: number;
}

export interface PofScheduleFormula {
	base: number;
	base_members: number;
	per_additional: number;
}

/** GET /pof/calculate response. */
export interface PofCalculateResult {
	requirement_id: string;
	program_label: string;
	household: number;
	amount: number;
	currency: string;
	ngn_equivalent?: number;
	usd_equivalent?: number;
	funds_must_be_liquid: boolean;
	accepted_proof?: string[];
	notes?: string;
	effective_date: string;
	source_url?: string;
}

/** GET /eca/providers — an ECA provider directory entry. */
export interface EcaProvider {
	id: string;
	slug: string;
	name: string;
	website?: string;
	is_active?: boolean;
}

/** GET /eca/guides + /eca/guides/:id — an ECA guide. */
export interface EcaGuide {
	id: string;
	destination_space_id: string;
	provider_id: string;
	provider_slug?: string;
	provider_name?: string;
	purpose: string;
	pathway_steps: EcaPathwayStep[];
	fee_estimate?: string;
	currency?: string;
	timeline_estimate?: string;
	documents_required?: string[];
	accepted_for?: string[];
	official_url?: string;
	source_url_last_checked?: string;
	status: string;
	is_active: boolean;
	ai_generated: boolean;
	is_japabot_indexed: boolean;
}

export interface EcaPathwayStep {
	step: number;
	title: string;
	detail: string;
	duration?: string;
}

/** GET /eca/guides/:id — guide detail + approved community updates. */
export interface EcaGuideDetail {
	guide: EcaGuide;
	updates: EcaCommunityUpdate[];
}

export interface EcaCommunityUpdate {
	id: string;
	guide_id: string;
	user_name?: string;
	step_index?: number;
	fee_estimate?: string;
	turnaround_days?: number;
	note?: string;
	status: string;
	created_at: string;
}

/** GET /eca/compare — side-by-side provider comparison. */
export interface EcaCompareResponse {
	providers: EcaProvider[];
	guides: EcaGuide[];
}

/** GET /sop/templates — a public SOP template. */
export interface SopTemplate {
	id: string;
	destination_space_id?: string;
	purpose: string;
	title: string;
	structure: SopTemplateSection[];
}

export interface SopTemplateSection {
	section: string;
	guidance: string;
	min_words?: number;
}

/** GET /sop/samples + /sop/samples/:id — a published SOP sample. */
export interface SopSample {
	id: string;
	destination_space_name?: string;
	program_level: string;
	field_of_study?: string;
	title: string;
	body: string;
	word_count?: number;
	ai_generated: boolean;
}

/** GET /sop/drafts + /sop/drafts/:id — a user-owned draft. */
export interface SopDraft {
	id: string;
	destination_space_id?: string;
	purpose: string;
	program?: string;
	institution?: string;
	title?: string;
	inputs: Record<string, unknown>;
	generated_text?: string;
	status: 'queued' | 'generated' | 'failed';
	ai_model?: string;
	version: number;
	created_at: string;
	updated_at: string;
}

/** GET /sop/quota — remaining monthly generations (auth). */
export interface SopQuota {
	limit: number;
	used: number;
	remaining: number;
}
