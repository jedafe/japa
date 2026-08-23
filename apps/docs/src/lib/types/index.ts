/**
 * Shared domain + transport types for the Docs App.
 * Shapes mirror the Go backend's JSON contract (internal/docs/types.go),
 * verified against the live API (ADR-060 — Docs drift on the unified shell).
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
 * Session / user (lean — the docs app only needs identity + groups)
 * ------------------------------------------------------------------ */

export interface SessionUser {
	id: string;
	email: string;
	username: string;
	displayName: string;
	avatar_url?: string | null;
	groups?: string[];
	/** RBAC permission nodes resolved from /me/permissions (DOC-007). */
	permissions?: string[];
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
 * Documentation — Section 44 (internal/docs/types.go)
 * Ref: ADR-060 (Docs drift), sections/44 (Documentation System)
 * ------------------------------------------------------------------ */

/**
 * Canonical audience slugs. The backend's ValidAudiences map (internal/docs/types.go)
 * is the source of truth; 'api' is always public, the rest are group-gated.
 */
export type DocAudience =
	| 'users'
	| 'premium'
	| 'organizations'
	| 'moderators'
	| 'api'
	| 'admin'
	| 'superadmin';

/** Canonical display order (mirrors docs.AudienceOrder). */
export const AUDIENCE_ORDER: DocAudience[] = [
	'users',
	'premium',
	'organizations',
	'moderators',
	'api',
	'admin',
	'superadmin'
];

/** Human-readable audience labels (i18n keys live in en.json/fr.json). */
export const AUDIENCE_LABEL_KEY: Record<DocAudience, string> = {
	users: 'audience.users',
	premium: 'audience.premium',
	organizations: 'audience.organizations',
	moderators: 'audience.moderators',
	api: 'audience.api',
	admin: 'audience.admin',
	superadmin: 'audience.superadmin'
};

/**
 * Set of valid audience slugs (mirrors docs.ValidAudiences in the backend).
 * Used to validate the `audience` query param before forwarding to the API.
 */
export const ValidAudiences: Set<DocAudience> = new Set(AUDIENCE_ORDER);

/** A published documentation page (GET /docs/{audience}/{slug}). */
export interface DocPage {
	id: string;
	title: string;
	slug: string;
	audience: DocAudience;
	/** TipTap JSON document (raw). Rendered to HTML server-side via @tiptap/html. */
	content: unknown;
	ai_generated: boolean;
	review_status: string;
	last_reviewed_by?: string | null;
	last_reviewed_at?: string | null;
	parent_id?: string | null;
	position: number;
	created_at: string;
	updated_at: string;
	/** Breadcrumb chain (audience → parents → current). Ref: §44. */
	breadcrumb?: DocBreadcrumbItem[];
	/** Previous page in the audience's nav order. Ref: §44. */
	prev?: { title: string; slug: string } | null;
	/** Next page in the audience's nav order. Ref: §44. */
	next?: { title: string; slug: string } | null;
	/** Content language code (e.g. 'en', 'fr'). Ref: §18 ROADMAP 08.03. */
	content_lang?: string;
}

/** A single breadcrumb entry. Ref: §44. */
export interface DocBreadcrumbItem {
	title: string;
	slug: string;
	audience: DocAudience;
}

/** Lightweight listing item (no content body). GET /docs/{audience}. */
export interface DocPageSummary {
	id: string;
	title: string;
	slug: string;
	audience: DocAudience;
	review_status: string;
	parent_id?: string | null;
	position: number;
	created_at: string;
}

/** A single nav entry (a published page), nesting children one level deep. */
export interface DocNavItem {
	id: string;
	title: string;
	slug: string;
	audience: DocAudience;
	parent_id?: string | null;
	position: number;
	children?: DocNavItem[];
}

/** Nav tree grouped by audience section. GET /docs/nav returns DocNavTree[]. */
export interface DocNavTree {
	audience: DocAudience;
	items: DocNavItem[];
}

/** A lightweight search hit. GET /docs/search?q=&limit=. */
export interface DocSearchResult {
	id: string;
	title: string;
	slug: string;
	audience: DocAudience;
	excerpt: string;
}

/* ------------------------------------------------------------------ *
 * OpenAPI — GET /docs/openapi.json (served at backend root, not /api/v1)
 * The playground fetches this server-side and passes the spec to the client.
 * ------------------------------------------------------------------ */

/** Minimal OpenAPI 3.0.3 shape — only the fields the playground reads. */
export interface OpenAPISpec {
	openapi: string;
	info: {
		title: string;
		version: string;
		description?: string;
	};
	servers?: { url: string; description?: string }[];
	paths: Record<string, Record<string, OpenAPIOperation>>;
}

export interface OpenAPIOperation {
	summary?: string;
	description?: string;
	operationId?: string;
	tags?: string[];
	parameters?: OpenAPIParameter[];
	requestBody?: OpenAPIRequestBody;
	responses?: Record<string, { description?: string; content?: Record<string, { schema?: unknown }> }>;
	security?: unknown[];
}

export interface OpenAPIParameter {
	name: string;
	in: 'query' | 'header' | 'path' | 'cookie';
	required?: boolean;
	description?: string;
	schema?: unknown;
}

export interface OpenAPIRequestBody {
	required?: boolean;
	description?: string;
	content?: Record<string, { schema?: unknown }>;
}

/* ------------------------------------------------------------------ *
 * Developer API Keys (Section 47) — GET /me/api-keys
 * ------------------------------------------------------------------ */

/** Summary of a user's API key (full key only returned on creation). */
export interface ApiKeySummary {
	id: string;
	app_name: string;
	/** First 12 chars of the key — the full key is only returned on POST. */
	key_prefix: string;
	environment: 'live' | 'test';
	scopes: string[];
	rate_limit_tier: string;
	last_used_at?: string | null;
	status: 'active' | 'revoked';
	created_at: string;
}

/** Full API key response from POST /me/api-keys (includes the full key once). */
export interface ApiKeyCreated extends ApiKeySummary {
	/** The full API key — shown once on creation, never again. */
	key: string;
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
