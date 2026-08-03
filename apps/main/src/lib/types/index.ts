/**
 * Shared domain + transport types for the Main App.
 * Shapes mirror the Go backend's JSON contract (verified against the live API).
 */

export type ThemeName = 'light' | 'dark';

/** Supported UI locales. Mirrors the i18n store's Locale type. */
export type Locale = 'en' | 'fr';

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
	primary_space?: SpaceRef | null;
	/** UI preferences persisted via PATCH /me/account, e.g. nav_collapsed. */
	ui?: { nav_collapsed?: boolean } & Record<string, unknown>;
	/** Geo-aware feed personalization country (ADR-064). Empty = use GeoIP default. */
	content_country_preference?: string;
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
	created_at: string;
	title?: string | null;
	slug?: string | null;
}

/** GET /me/bookmark-folders — a user's bookmark folder. */
export interface BookmarkFolder {
	id: string;
	name: string;
	bookmark_count?: number;
	created_at: string;
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
}

/** GET/PATCH /me/privacy — a map of privacy toggles (nested). */
export interface PrivacySettings {
	profile_visibility?: 'public' | 'members' | 'followers';
	show_activity?: boolean;
	searchable?: boolean;
	[key: string]: unknown;
}

// ─── Auth (Section 02) ───────────────────────────────────────────────
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md

/** Response from POST /auth/login and POST /auth/register. */
export interface AuthSession {
	totp_required?: undefined;
	access_token: string;
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
	/** Plain-text comment body (not TipTap JSON). */
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
	/** imgproxy-signed logo URL resolved from logo_media_id (Section 34 / ADR-039). */
	logo_url?: string | null;
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
	fields: FormField[];
}

/** POST /forms/{id}/submissions — submission response. */
export interface FormSubmissionResponse {
	id: string;
	status: string;
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

