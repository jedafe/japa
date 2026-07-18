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
	space_id?: string | null;
	space_name?: string | null;
	excerpt?: string;
	created_at: string;
	view_count?: number;
	comment_count?: number;
	vote_score?: number;
}
