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
	/** Optional JSON-LD structured data emitted by some GET endpoints (e.g. forum thread detail). */
	json_ld?: unknown;
	/** Optional capability flag emitted by some GET endpoints (e.g. forum can_solve). */
	can_solve?: boolean;
	/** Optional capability flag emitted by forum thread detail (can_edit). */
	can_edit?: boolean;
	/** Optional bookmark state emitted by forum thread detail (is_bookmarked + bookmark_id). */
	is_bookmarked?: boolean;
	bookmark_id?: string;
	/** Optional list of erased user UUIDs emitted by content/forum detail
	 * endpoints for GDPR/ADR-029 mention redaction (Section 15). */
	deleted_user_ids?: string[];
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

export type MenuPlacement = 'sidebar' | 'bottom_bar' | 'dropdown' | 'footer' | 'header';
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
 * Tag detail — GET /tags/{slug} (public, tenant-scoped)
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

export type ContentType =
	| 'blog_post'
	| 'discussion'
	| 'question'
	| 'forum_thread'
	| 'comparison'
	| string;

export interface FeedItem {
	id: string;
	title: string;
	slug: string;
	content_type: ContentType;
	author_id: string;
	author_name?: string;
	author_avatar_url?: string | null;
	space_id?: string | null;
	space_name?: string | null;
	space_slug?: string | null;
	excerpt?: string;
	created_at: string;
	view_count?: number;
	comment_count?: number;
	vote_score?: number;
	velocity_score?: number;
	bookmark_count?: number;
	reading_time_minutes?: number;
	cover_image_url?: string | null;
	tags?: string[];
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
}

/** One custom-field definition (from GET /custom-field-groups/{id}). */
export interface CustomFieldDef {
	id: string;
	key: string;
	label: string;
	field_type: string; // text|textarea|rich_text|number|boolean|date|select|multiselect|rating|url|...
	is_required: boolean;
	options?: string[] | null;
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
	tenant_id: string;
	slug: string;
	title: string;
	description?: string | null;
	cover_image_id?: string | null;
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
}

/** Uploaded media (POST /media/upload → confirm, or GET /me/media). */
export interface MediaAsset {
	id: string;
	url: string;
	filename?: string;
	mime_type?: string;
	file_size?: number;
	alt_text?: string | null;
	status?: string;
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
	avatar_url?: string | null;
	cover_url?: string | null;
	primary_space?: SpaceRef | null;
	ui?: { nav_collapsed?: boolean; theme?: string } & Record<string, unknown>;
	/** ADR-064a: ISO 3166-1 alpha-2 origin country code (custom field). */
	origin_country?: string | null;
	/** ADR-064a: ISO 3166-1 alpha-2 destination country code (custom field). */
	destination_country?: string | null;
}

/** GET/PATCH /me/privacy — a map of privacy toggles (nested). */
export interface PrivacySettings {
	profile_visibility?: 'public' | 'members' | 'followers';
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
	| 'weak_password'
	| 'password_breached'
	| 'invalid_body'
	| 'tenant_unavailable'
	| 'unsupported_provider'
	| 'state_error'
	| 'invalid_state'
	| 'missing_email'
	| 'create_failed'
	| 'token_error';

/** Onboarding status from GET /onboarding/status. */
export interface OnboardingStatus {
	current_step: number;
	completed: boolean;
	step_data: unknown;
	tour_dismissed: boolean;
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
	archetype: string;
	content_country: string;
	origin_country: string;
	current_country: string;
	destination_country: string;
	japa_stage: string;
	current_city: string;
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
	tenant_id: string;
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
	tenant_id: string;
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
}

/** A forum reply (GET /forum/{id}/replies, POST /forum/{id}/replies). */
export interface ForumReply {
	id: string;
	tenant_id: string;
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
	tenant_id: string;
	content_id: string;
	author_user_id: string;
	parent_id: string | null;
	/** Base64-encoded TipTap JSON document (Section 49 — jsonb body). */
	body: string;
	/** Nesting depth (0 = top-level, max 3). */
	depth: number;
	vote_score: number;
	is_pinned: boolean;
	is_edited: boolean;
	edited_at: string | null;
	created_at: string;
	updated_at: string;
	deleted_at: string | null;
	/** Joined author username. */
	username: string;
	/** Joined author display name (empty string when absent). */
	display_name: string;
	/** Joined author avatar URL (empty string when absent). */
	avatar_url: string;
}

/* ------------------------------------------------------------------ *
 * Public profiles — Sections 07, 31, 33, 50
 * Ref: japaconnect-engineering/sections/33-professional-profiles.md
 *      japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 * ------------------------------------------------------------------ */

/** GET /users/{username} — lean public profile header. */
export interface PublicUser {
	id: string;
	username: string;
	display_name: string;
	avatar_url?: string | null;
	cover_url?: string | null;
	bio?: string | null;
	profile_type: string;
	follower_count: number;
	following_count: number;
	created_at: string;
	/** Section 28 — Points balance, only present when the user has opted in via show_points_publicly. Credits are never exposed. */
	points?: number;
}

/** GET /users/{id}/content — PostCard row (profile Posts tab). */
export interface ProfilePost {
	id: string;
	author_id: string;
	organization_id: string | null;
	organization?: { id: string; name: string; slug: string } | null;
	space_id: string | null;
	content_type: string;
	title: string;
	slug: string;
	body_html: string;
	excerpt?: string;
	header_image_id: string | null;
	featured_image_id: string | null;
	read_time_minutes: number;
	view_count: number;
	comment_count: number;
	share_count: number;
	is_pinned: boolean;
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
	title?: string;
	track_slug?: string;
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

/** GET /me/analytics/followers — daily follower growth time-series (ADR-059). */
export interface FollowerGrowthItem {
	day: string;
	new_followers: number;
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

/** GET /me/analytics — aggregate analytics for the dashboard. */
export interface MeAnalytics {
	total_content: number;
	draft_content: number;
	published_content: number;
	total_bookmarks: number;
	total_followers: number;
	total_following: number;
}

/* ------------------------------------------------------------------ *
 * Spaces — Section 22 (GET /spaces, GET /spaces/{slug})
 * The backend returns the raw gen.Space row. Universities are spaces
 * with a university space_type; /universities returns the same shape.
 * ------------------------------------------------------------------ */

/** GET /space-types — space type definition row. */
export interface SpaceType {
	id: string;
	tenant_id: string;
	name: string;
	slug: string;
	description: string | null;
	icon: string | null;
	field_group_id: string | null;
	is_system: boolean;
	is_university: boolean;
	created_at: string;
	updated_at: string;
}

/** GET /spaces/{slug} `space` field — full gen.Space row. */
export interface Space {
	id: string;
	tenant_id: string;
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
}

/** GET /spaces/{slug}/moderators item — ListSpaceModeratorsRow. */
export interface SpaceModerator {
	id: string;
	tenant_id: string;
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
	tenant_id: string;
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
	tenant_id: string;
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
	tenant_id: string;
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
	description: string | null;
	in_app: boolean;
	email: boolean;
	push: boolean;
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
	tenant_id: string;
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
	tenant_id: string;
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
	tenant_id: string;
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

/** GET /me/enrollments item — a track enrollment record. */
export interface LearnEnrollment {
	id: string;
	track_id: string;
	track_title?: string;
	enrolled_at: string;
	completed_at?: string | null;
	progress_percent?: number;
}

/* ------------------------------------------------------------------ *
 * Coupons — Section 21 (GET /coupons, /coupons/{id})
 * ------------------------------------------------------------------ */

/** GET /coupons item — a public active coupon. */
export interface Coupon {
	id: string;
	tenant_id: string;
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
 * Comparisons — Section 30 (GET /comparisons/{id})
 * ------------------------------------------------------------------ */

/** GET /comparisons/{id} — comparison content metadata with items and dimensions. */
export interface ComparisonDetail {
	id: string;
	tenant_id: string;
	author_id: string;
	title: string;
	slug: string;
	body: string;
	visibility: string;
	status: string;
	space_id: string | null;
	items: ComparisonItem[];
	dimensions: ComparisonDimension[];
}

/** A comparison item (entity being compared, e.g. a university). */
export interface ComparisonItem {
	id: string;
	content_id: string;
	tenant_id: string;
	type_id: string | null;
	title: string;
	description: string | null;
	image_id: string | null;
	geo_space_id: string | null;
	position: number;
}

/** A comparison dimension (criterion being evaluated). */
export interface ComparisonDimension {
	id: string;
	content_id: string;
	tenant_id: string;
	type_id: string | null;
	label: string;
	dimension_type: string;
	position: number;
}

/** GET /comparisons/{id}/matrix — one item's row across all dimensions. */
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
}

/** POST /comparisons — created comparison result. */
export interface ComparisonCreated {
	id: string;
	tenant_id: string;
	author_id: string;
	title: string;
	slug: string;
	visibility: string;
	space_id: string | null;
}

/**
 * GET /comparisons — list row. The backend returns content rows backing
 * comparisons (Section 30); item count / type chip / author display are not
 * joined by the list endpoint, so the card renders what the API provides.
 */
export interface ComparisonListItem {
	id: string;
	tenant_id: string;
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

/** GET /comparisons/types — a comparison type with field/item counts. */
export interface ComparisonType {
	id: string;
	tenant_id: string;
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
}

/* ------------------------------------------------------------------ *
 * Professional Profiles — Section 12 (GET /profiles/{slug})
 * ------------------------------------------------------------------ */

/** GET /profiles/{slug} — a professional profile. */
export interface ProfessionalProfile {
	id: string;
	tenant_id: string;
	owner_user_id: string;
	type_id: string;
	name: string;
	slug: string;
	tagline?: string | null;
	description?: string | null;
	logo_media_id?: string | null;
	header_media_id?: string | null;
	website_url?: string | null;
	verified: boolean;
	verification_type?: string | null;
	follower_count: number;
	status: string;
	field_values?: Record<string, unknown>;
	created_at: string;
}

/** GET /profiles/types — a professional profile type. */
export interface ProfessionalProfileType {
	id: string;
	tenant_id: string;
	name: string;
	slug: string;
	icon?: string | null;
	field_group_id?: string | null;
	is_system: boolean;
	created_at: string;
}

/** GET /profiles/{id}/members — a professional profile member. */
export interface ProfessionalProfileMember {
	profile_id: string;
	user_id: string;
	role: string;
	invited_by?: string | null;
	joined_at: string;
	username: string;
	email: string;
}

/** GET /profiles/{id}/invitations — a professional profile invitation. */
export interface ProfessionalProfileInvitation {
	id: string;
	profile_id: string;
	email: string;
	role: string;
	invited_by?: string | null;
	token: string;
	expires_at: string;
	accepted_at?: string | null;
	created_at: string;
}

/** GET /profiles/{id}/relocations — a professional profile relocation. */
export interface ProfessionalRelocation {
	id: string;
	tenant_id: string;
	profile_id: string;
	country: string;
	city?: string | null;
	status?: string | null;
	year?: number | null;
	created_at: string;
	updated_at: string;
}

/** GET /profiles/{id}/documents — a professional profile document. */
export interface ProfessionalDocument {
	id: string;
	tenant_id: string;
	profile_id: string;
	title: string;
	document_type: string;
	description?: string | null;
	file_url?: string | null;
	verified: boolean;
	created_at: string;
	updated_at: string;
}

/** GET /profiles/{id}/verifiers — a professional profile verifier. */
export interface ProfessionalVerifier {
	id: string;
	tenant_id: string;
	profile_id: string;
	user_id: string;
	role?: string | null;
	status?: string | null;
	created_at: string;
	updated_at: string;
	user_name: string;
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
	tenant_id: string;
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
	difficulty: 'beginner' | 'intermediate' | 'advanced' | string;
	instructor_id: string;
	instructor_name?: string | null;
	price_kobo: number;
	currency?: string | null;
	category?: string | null;
	language?: string | null;
	total_enrolled?: number;
	is_published_to_university?: boolean;
	cover_media_url?: string | null;
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
	member_count: number;
}

/** GET /api/v1/geo/countries/{code}/states — a state/province geo space. */
export interface GeoState {
	id: string;
	name: string;
	slug: string;
	geo_code: string | null;
	level: 'state';
	parent_id: string;
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
	option_scores: Record<string, number>;
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
	tenant_id: string;
	name: string;
	slug: string;
	description: string;
	is_multi_step: boolean;
	submit_label: string;
	status: string;
	scoring_config: AssessmentScoringConfig;
	fields: AssessmentField[];
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
}

export interface SavedScholarship {
	id: string;
	scholarship: Scholarship;
	status: string;
	notes: string;
	remind_before_days: number;
	last_reminded_at: string;
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
	source_url?: string;
}

export interface RemittanceBestRate {
	provider_id: string;
	provider_name: string;
	rate: number;
	receive_amount: number;
}

export interface RemittanceCalculation {
	providers: RemittanceProviderCalc[];
}

export interface RemittanceProviderCalc {
	provider_id: string;
	provider_name: string;
	rate: number;
	receive_amount: number;
	fee_flat: number;
	total_cost: number;
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

// --- Section 63: Jobs Board ---
export interface JobListing {
	id: string;
	title: string;
	slug: string;
	summary: string;
	field_values: Record<string, string>;
	published_at: string;
}

export interface JobDetail {
	id: string;
	title: string;
	slug: string;
	summary: string;
	body: string;
	field_values: Record<string, string>;
	published_at: string;
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
	description: string;
	start_date: string;
	end_date: string;
	reward_points: number;
	is_active: boolean;
}

export interface LeaderboardEntry {
	user_id: string;
	username: string;
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
	tenant_id: string;
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
	tenant_id: string;
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

// --- Section 67: Travel Search ---
export interface TravelFlightResult {
	provider: string;
	price: number;
	currency: string;
	departure_time: string;
	arrival_time: string;
	airline: string;
	flight_number: string;
	stops: number;
	duration: string;
	booking_url: string;
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
}

export interface TravelSearchResponse {
	results: TravelFlightResult[] | TravelHotelResult[];
	cache_hit: boolean;
	search_time_ms: number;
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
	author_avatar?: string;
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
export type WikiCategory =
	| 'overview'
	| 'visa'
	| 'cost_of_living'
	| 'healthcare'
	| 'banking'
	| 'community'
	| 'housing'
	| 'other';

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
}

export interface WikiPage extends WikiPageIndex {
	space_slug: string;
	space_name: string;
	current_content: unknown;
	revision_summary?: string;
	submitted_by?: string;
}

export type WikiRevisionStatus = 'approved' | 'rejected' | 'pending';

export interface WikiRevision {
	id: string;
	status: string;
	content?: unknown;
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
	avatar_url?: string;
}

export interface DigestEvent {
	id?: string;
	title?: string;
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
	ai_generated_at?: string;
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
	| 'settled';

/** GET /me/japa-stage — the caller's own stage profile. */
export interface MyJapaStage {
	user_id: string;
	username?: string;
	display_name?: string;
	avatar_url?: string;
	stage: string;
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
	tenant_id: string;
	is_mentor: boolean | null;
	mentor_capacity: number | null;
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
	tenant_id: string;
	mentor_id: string;
	mentee_id: string;
	status: string;
	goals: string | null;
	destination_space_id: string;
	responded_at: string | null;
	completed_at: string | null;
	cancelled_at: string | null;
	cancel_reason: string | null;
	created_at: string;
	updated_at: string;
	mentor_username?: string;
	mentee_username?: string;
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
