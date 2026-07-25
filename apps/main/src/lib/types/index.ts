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
	status: string;
	visibility: string;
	publish_at?: string | null;
	space_id?: string | null;
}

/** Uploaded media (POST /media/upload → confirm). */
export interface MediaAsset {
	id: string;
	url: string;
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
