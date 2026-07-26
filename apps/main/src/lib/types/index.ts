/**
 * Shared domain + transport types for the Main App.
 * Shapes mirror the Go backend's JSON contract (verified against the live API).
 */

export type ThemeName = 'light' | 'dark';

/* ------------------------------------------------------------------ *
 * Transport envelope
 * ------------------------------------------------------------------ */

/** Success envelope: every endpoint wraps its payload in `{ data }`. */
export interface ApiOk<T> {
	data: T;
	meta?: ListMeta;
}

/** Error envelope: `{ error: { code, message } }`. */
export interface ApiErr {
	error: { code: string; message: string; status?: number };
}

export interface ListMeta {
	total?: number;
	page?: number;
	limit?: number;
	next_cursor?: string | null;
}

/* ------------------------------------------------------------------ *
 * Session / user
 * ------------------------------------------------------------------ */

export interface SessionUser {
	id: string;
	email: string;
	username: string;
	displayName: string;
	avatar_url?: string | null;
	groups?: string[];
	primary_space?: SpaceRef | null;
	/** UI preferences persisted via PATCH /me/account, e.g. nav_collapsed. */
	ui?: { nav_collapsed?: boolean } & Record<string, unknown>;
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

export type MenuPlacement = 'sidebar' | 'bottom_bar';
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
	color?: string | null;
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

export type Visibility = 'public' | 'unlisted' | 'private';

/** GET /content-types item. */
export interface ContentTypeMeta {
	value: string;
	label: string;
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
	entity_type: string;
	fields: CustomFieldDef[];
}

/** GET /blog/series item. */
export interface BlogSeries {
	id: string;
	slug: string;
	title: string;
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
	status: string;
	visibility: string;
	publish_at?: string | null;
	space_id?: string | null;
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
	displayName: string;
	bio?: string | null;
	avatar_url?: string | null;
	cover_url?: string | null;
	primary_space?: SpaceRef | null;
	ui?: { nav_collapsed?: boolean; theme?: string } & Record<string, unknown>;
}

/** GET/PATCH /me/privacy — a map of privacy toggles (nested). */
export interface PrivacySettings {
	profile_visibility?: 'public' | 'members' | 'private';
	show_activity?: boolean;
	searchable?: boolean;
	[key: string]: unknown;
}

// ─── Auth (Section 02) ───────────────────────────────────────────────
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md

/** Response from POST /auth/login and POST /auth/register. */
export interface AuthSession {
	access_token: string;
	user: AuthUser;
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

/** GET /me/analytics — aggregate analytics for the dashboard. */
export interface MeAnalytics {
	total_content: number;
	draft_content: number;
	published_content: number;
	total_bookmarks: number;
}

/* ------------------------------------------------------------------ *
 * Spaces — Section 22 (GET /spaces, GET /spaces/{slug})
 * The backend returns the raw gen.Space row. Universities are spaces
 * with a university space_type; /universities returns the same shape.
 * ------------------------------------------------------------------ */

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
}

/** GET /spaces/{slug} response — `{ space, rules }` bundle. */
export interface SpaceDetail {
	space: Space;
	rules: SpaceRule[];
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

