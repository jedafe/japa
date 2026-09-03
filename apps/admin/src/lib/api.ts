import { apiFetch as _apiFetch } from './api/client';
import { getAccessToken } from './api/token';
import { buildApiUrl } from '@japaconnect/api-url';
import { PUBLIC_JC_API_URL } from '$env/static/public';

export { _apiFetch as apiFetch };

type FetchOptions = {
	method?: string;
	body?: unknown;
	signal?: AbortSignal;
};

type ApiResult<T> = { ok: true; data: T } | { ok: false; error: string };

// fetchRaw performs a direct fetch that bypasses the JSON envelope parsing in
// apiFetch. Used for non-JSON responses (e.g. CSV exports from
// /admin/analytics/export). Auth credentials and the bearer token are attached
// the same way apiFetch attaches them; CSRF is not needed for GET requests.
async function fetchRaw(path: string): Promise<ApiResult<string>> {
	const url = buildApiUrl(path, PUBLIC_JC_API_URL);
	const accessToken = getAccessToken();
	const authHeader: Record<string, string> = accessToken
		? { Authorization: `Bearer ${accessToken}` }
		: {};
	try {
		const res = await fetch(url, {
			method: 'GET',
			credentials: 'include',
			headers: authHeader,
		});
		if (!res.ok) {
			const text = await res.text();
			let msg = res.statusText;
			try {
				const body = JSON.parse(text);
				if (body?.error?.message) msg = body.error.message;
			} catch {
				if (text) msg = text;
			}
			return { ok: false, error: msg };
		}
		return { ok: true, data: await res.text() };
	} catch (err) {
		return { ok: false, error: err instanceof Error ? err.message : 'Network error' };
	}
}

export function setToken(token: string | null): void {
	// Deprecated no-op: JWT auth state is no longer managed client-side.
}

export async function api<T>(path: string, opts: FetchOptions = {}): Promise<ApiResult<T>> {
	const headers: Record<string, string> = {};

	const isFormData = typeof FormData !== 'undefined' && opts.body instanceof FormData;
	const result = await _apiFetch<T>(path, {
		method: opts.method ?? 'GET',
		headers,
		body: opts.body ? (isFormData ? (opts.body as BodyInit) : JSON.stringify(opts.body)) : undefined,
		signal: opts.signal,
	});

	if (result.error) {
		return { ok: false, error: result.error.message };
	}
	return { ok: true, data: result.data as T };
}

// apiRaw returns the FULL response body (not just body.data), so callers can
// access top-level pagination metadata like { total, total_pages } that some
// backends put alongside `data` instead of nesting it inside `data`.
export async function apiRaw<T = any>(path: string, opts: FetchOptions = {}): Promise<ApiResult<T>> {
	const headers: Record<string, string> = {};

	const isFormData = typeof FormData !== 'undefined' && opts.body instanceof FormData;
	const result = await _apiFetch<any>(path, {
		method: opts.method ?? 'GET',
		headers,
		body: opts.body ? (isFormData ? (opts.body as BodyInit) : JSON.stringify(opts.body)) : undefined,
		signal: opts.signal,
	});

	if (result.error) {
		return { ok: false, error: result.error.message };
	}
	// `body` is the full JSON response body; `data` is just body.data.
	return { ok: true, data: result.body as T };
}

export const spamApi = {
	getIpBlocks: (params?: string) => apiRaw<{ data: unknown[]; total: number; total_pages: number }>(`/admin/spam/ip-blocks${params ? `?${params}` : ''}`),
	createIpBlock: (body: { cidr: string; block_type: string; reason?: string; expires_at?: string }) =>
		api('/admin/spam/ip-blocks', { method: 'POST', body }),
	updateIpBlock: (id: string, body: { cidr?: string; block_type?: string; reason?: string; expires_at?: string }) =>
		api(`/admin/spam/ip-blocks/${id}`, { method: 'PATCH', body }),
	deleteIpBlock: (id: string) =>
		api(`/admin/spam/ip-blocks/${id}`, { method: 'DELETE' }),

	getCensoredWords: (params?: string) => apiRaw<{ data: unknown[]; total: number; total_pages: number }>(`/admin/spam/censored-words${params ? `?${params}` : ''}`),
	createCensoredWord: (body: { pattern: string; action: string; replacement?: string }) =>
		api('/admin/spam/censored-words', { method: 'POST', body }),
	updateCensoredWord: (id: string, body: { pattern?: string; action?: string; replacement?: string }) =>
		api(`/admin/spam/censored-words/${id}`, { method: 'PATCH', body }),
	deleteCensoredWord: (id: string) =>
		api(`/admin/spam/censored-words/${id}`, { method: 'DELETE' }),

	getUserBlocks: (params?: string) => apiRaw<{ data: unknown[]; total: number; total_pages: number }>(`/admin/spam/user-blocks${params ? `?${params}` : ''}`),
	deleteUserBlock: (blockerId: string, blockedId: string) =>
		api(`/admin/spam/user-blocks/${blockerId}/${blockedId}`, { method: 'DELETE' }),
};

export const ticketTypesApi = {
	list: (params?: string) => api<unknown>(`/admin/ticket-types${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string; description?: string; default_priority: string; is_public: boolean }) =>
		api('/admin/ticket-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; description?: string; default_priority?: string; is_public?: boolean }) =>
		api(`/admin/ticket-types/${id}`, { method: 'PUT', body }),
	delete: (id: string) =>
		api(`/admin/ticket-types/${id}`, { method: 'DELETE' }),
};

export interface PlanFeature {
	text: string;
	included: boolean;
}

export interface SubscriptionPlan {
	id: string;
	tenant_id: string;
	slug: string;
	name: string;
	description: string;
	features: PlanFeature[];
	monthly_amount: number;
	yearly_amount: number;
	currency: string;
	highlighted: boolean;
	sort_order: number;
	status: string;
	tier: 'regular' | 'pro' | 'business';
	created_at: string;
	updated_at: string;
}

export const plansApi = {
	list: () => api<SubscriptionPlan[]>('/admin/plans'),
	create: (body: {
		slug: string; name: string; description: string;
		features: PlanFeature[]; monthly_amount: number; yearly_amount: number;
		currency: string; highlighted: boolean; sort_order: number; status: string;
		tier: 'regular' | 'pro' | 'business';
	}) => api('/admin/plans', { method: 'POST', body }),
	update: (id: string, body: {
		slug?: string; name?: string; description?: string;
		features?: PlanFeature[]; monthly_amount?: number; yearly_amount?: number;
		currency?: string; highlighted?: boolean; sort_order?: number; status?: string;
		tier?: 'regular' | 'pro' | 'business';
	}) => api(`/admin/plans/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api(`/admin/plans/${id}`, { method: 'DELETE' }),
	// Plan ↔ Track management (migration 000273)
	listTracks: (planId: string) => api<{ track_id: string }[]>(`/admin/plans/${planId}/tracks`),
	setTracks: (planId: string, trackIds: string[]) => api(`/admin/plans/${planId}/tracks`, { method: 'PUT', body: { track_ids: trackIds } }),
	addTrack: (planId: string, trackId: string) => api(`/admin/plans/${planId}/tracks/${trackId}`, { method: 'POST', body: {} }),
	removeTrack: (planId: string, trackId: string) => api(`/admin/plans/${planId}/tracks/${trackId}`, { method: 'DELETE' }),
};

export interface AdminPage {
	id: string;
	title: string;
	slug: string;
	page_type: string;
	status: string;
	updated_at: string;
	blocks?: unknown;
}

// pagesApi wraps the /admin/pages endpoints (page builder). List supports
// server-side filtering by page_type/slug/status/q.
export const pagesApi = {
	list: (params?: string) =>
		api<AdminPage[]>(`/admin/pages${params ? `?${params}` : ''}`),
	get: (id: string) => api<AdminPage>(`/admin/pages/${id}`),
	create: (body: {
		title: string;
		slug: string;
		page_type?: string;
		blocks?: unknown;
		theme_id?: string;
	}) => api<AdminPage>('/admin/pages', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<AdminPage>(`/admin/pages/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api(`/admin/pages/${id}`, { method: 'DELETE' }),
	publish: (id: string) => api<AdminPage>(`/admin/pages/${id}/publish`, { method: 'POST', body: {} }),
	archive: (id: string) => api<AdminPage>(`/admin/pages/${id}/archive`, { method: 'POST', body: {} }),
	duplicate: (id: string) => api<AdminPage>(`/admin/pages/${id}/duplicate`, { method: 'POST', body: {} }),
};

export const walletApi = {
	getRules: (params?: string) => api<unknown>(`/admin/wallet/rules${params ? `?${params}` : ''}`),
	createRule: (body: { activity_key: string; point_value: number; is_enabled: boolean; one_time: boolean; description?: string }) =>
		api('/admin/wallet/rules', { method: 'POST', body }),
	updateRule: (id: string, body: { activity_key?: string; point_value?: number; is_enabled?: boolean; one_time?: boolean; description?: string }) =>
		api(`/admin/wallet/rules/${id}`, { method: 'PATCH', body }),
	toggleRule: (id: string) =>
		api(`/admin/wallet/rules/${id}/toggle`, { method: 'PATCH' }),
	deleteRule: (id: string) =>
		api(`/admin/wallet/rules/${id}`, { method: 'DELETE' }),

	getSettings: () => api<unknown>('/admin/wallet/settings'),
	updateSettings: (body: { points_to_credits_rate: number; daily_earning_cap: number; enable_earning: boolean }) =>
		api('/admin/wallet/settings', { method: 'PUT', body }),

	getUserBalance: (userId: string) => api<unknown>(`/admin/wallet/user/${userId}/balance`),
	adjustBalance: (body: { user_id: string; amount: number; reason: string; ledger_type: 'points' | 'credits' }) =>
		api('/admin/wallet/adjust', { method: 'POST', body }),

	// Credit Currency System (Section 58, ADR-065)
	listCreditPacks: () => api<unknown>('/admin/wallet/credit-packs'),
	createCreditPack: (body: { name: string; description?: string; credit_amount: number; bonus_credits: number; min_user_level?: string; position: number }) =>
		api('/admin/wallet/credit-packs', { method: 'POST', body }),
	updateCreditPack: (id: string, body: { name?: string; description?: string; credit_amount?: number; bonus_credits?: number; min_user_level?: string; is_active?: boolean; position?: number }) =>
		api(`/admin/wallet/credit-packs/${id}`, { method: 'PATCH', body }),
	deleteCreditPack: (id: string) =>
		api(`/admin/wallet/credit-packs/${id}`, { method: 'DELETE' }),
	listCreditPurchases: (params?: string) =>
		api<unknown>(`/admin/wallet/purchases${params ? `?${params}` : ''}`),
	listWithdrawals: (params?: string) =>
		api<{ data: any[]; count: number }>(`/admin/wallet/withdrawals${params ? `?${params}` : ''}`),
	processWithdrawal: (id: string, body?: { action?: string; admin_note?: string }) =>
		api(`/admin/wallet/withdrawals/${id}/process`, { method: 'POST', body }),

	// Currencies — for currency symbol lookup (Rule 16, BUG-022)
	getCurrencies: () => api<{ code: string; symbol: string; name?: string }[]>('/wallet/currencies'),
};

export const spaceTypesApi = {
	list: (params?: string) => api<unknown>(`/admin/space-types${params ? `?${params}` : ''}`),
	categories: () => api<{ value: string; selectable: boolean }[]>('/admin/space-type-categories'),
	create: (body: { name: string; slug: string; description?: string; category: string; icon?: string }) =>
		api('/admin/space-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; description?: string; category?: string; icon?: string }) =>
		api(`/admin/space-types/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/space-types/${id}`, { method: 'DELETE' }),
};

export const impersonationApi = {
	start: (userId: string) =>
		api<{
			session_key: string;
			expires_at: string;
			target: { id: string; username: string; email: string };
		}>('/admin/impersonate', { method: 'POST', body: { user_id: userId } }),
	end: () =>
		api<{ status: string; target_id: string }>('/admin/impersonate', { method: 'DELETE' }),
	status: () =>
		api<{
			active: boolean;
			target_user_id: string | null;
			expires_at: string | null;
			target: { id: string; username: string; email: string } | null;
		}>('/admin/impersonate/status'),
};

export const docsApi = {
	list: (params?: string) => api<unknown>(`/admin/docs/pages${params ? `?${params}` : ''}`),
	create: (body: { title: string; slug: string; audience: string; content: unknown; review_status?: string }) =>
		api('/admin/docs/pages', { method: 'POST', body }),
	update: (id: string, body: { title?: string; slug?: string; audience?: string; content?: unknown; review_status?: string }) =>
		api(`/admin/docs/pages/${id}`, { method: 'PATCH', body }),
	approve: (id: string) =>
		api(`/admin/docs/pages/${id}/approve`, { method: 'POST' }),
	publish: (id: string) =>
		api(`/admin/docs/pages/${id}/publish`, { method: 'POST' }),
	delete: (id: string) =>
		api(`/admin/docs/pages/${id}`, { method: 'DELETE' }),
	audiences: () => api<string[]>('/docs/audiences'),
	statuses: () => api<string[]>('/docs/statuses'),
	generate: (body: {
		category: 'docs';
		prompt: string;
		context?: { title: string; slug?: string; audience?: string; related_features?: string[] };
	}) => api<{ task_id: string; task_type: string }>('/admin/docs/ai-generate', { method: 'POST', body })
};

export type AdNetwork = {
	id: string;
	tenant_id: string;
	provider: 'adsense' | 'ezoic' | 'media_net' | 'custom';
	name: string;
	client_id?: string;
	api_key?: string;
	slot_mapping?: Record<string, string>;
	config?: Record<string, unknown>;
	is_active: boolean;
	priority: number;
	created_at: string;
	updated_at: string;
};

export const adSlotTypesApi = {
	list: (params?: string) => api<unknown>(`/admin/ads/slot-types${params ? `?${params}` : ''}`),
	create: (body: { name: string; description?: string; is_active?: boolean; max_display_per_page?: number }) =>
		api('/admin/ads/slot-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; description?: string; is_active?: boolean; max_display_per_page?: number }) =>
		api(`/admin/ads/slot-types/${id}`, { method: 'PUT', body }),
	delete: (id: string) =>
		api(`/admin/ads/slot-types/${id}`, { method: 'DELETE' }),
};

export const adNetworksApi = {
	list: (params?: string) => api<{ data: AdNetwork[] }>(`/admin/ads/networks${params ? `?${params}` : ''}`),
	get: (id: string) => api<{ data: AdNetwork }>(`/admin/ads/networks/${id}`),
	create: (body: Record<string, unknown>) => api<{ data: AdNetwork }>('/admin/ads/networks', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) => api<{ data: AdNetwork }>(`/admin/ads/networks/${id}`, { method: 'PUT', body }),
	delete: (id: string) => api<{ data: unknown }>(`/admin/ads/networks/${id}`, { method: 'DELETE' }),
	// Ref: japaconnect-engineering/sections/25-ad-placement.md (Ad Networks)
	// Rule 16 — providers list fetched from backend, not hardcoded.
	listProviders: () => api<{ data: string[] }>('/admin/ads/networks/providers'),
};

export const adBoostsApi = {
	list: (params?: string) => api<unknown>(`/admin/ads/boosts${params ? `?${params}` : ''}`),
	create: (body: { organization_id: string; space_id: string; boost_type: string; start_date: string; end_date: string }) =>
		api('/admin/ads/boosts', { method: 'POST', body }),
	update: (id: string, body: { organization_id?: string; space_id?: string; boost_type?: string; start_date?: string; end_date?: string }) =>
		api(`/admin/ads/boosts/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/ads/boosts/${id}`, { method: 'DELETE' }),
};

export interface ComparisonType {
	id: string;
	name: string;
	slug: string;
	description: string;
	icon?: string;
	field_group_id: string;
	item_label: string;
	is_system: boolean;
	config?: Record<string, unknown>;
	created_at: string;
	field_count?: number;
	item_count?: number;
}

export interface ComparisonTypeDimension {
	id: string;
	name: string;
	description: string;
	field_type: string;
}

export interface ComparisonTypeDetail extends ComparisonType {
	dimensions: ComparisonTypeDimension[];
	item_count: number;
}

// Per-UserGroup access row — matches the backend comparison_type_access shape
// (Section 30). user_group_id is null for AI-agent-only access.
export interface ComparisonTypeAccessRow {
	type_id: string;
	user_group_id: string | null;
	can_create: boolean;
	can_fill: boolean;
	ai_can_fill: boolean;
	group_name: string | null;
}

export interface CustomFieldGroupOption {
	id: string;
	name: string;
	slug: string;
}

// Comparison content entry — the actual comparison articles (Section 30).
export interface ComparisonEntry {
	id: string;
	title: string;
	slug: string;
	body: string;
	visibility: string;
	status: string;
	depth?: string;
	promoted_from_id?: string | null;
	created_at: string;
	updated_at: string;
}

export const comparisonAdminApi = {
	listTypes: (params?: string) =>
		api<ComparisonType[]>(`/jce/comparison-types${params ? `?${params}` : ''}`),
	// Section 30 exposes GET /jce/comparison-types/{slug} (by slug, not id).
	// Returns the enriched detail with dimensions + item_count.
	getType: (slug: string) => api<ComparisonTypeDetail>(`/jce/comparison-types/${slug}`),
	createType: (body: {
		name: string;
		slug: string;
		description?: string;
		icon?: string;
		field_group_id: string;
		item_label?: string;
		config?: Record<string, unknown>;
	}) => api<ComparisonType>('/jce/comparison-types', { method: 'POST', body }),
	updateType: (
		id: string,
		body: {
			name?: string;
			slug?: string;
			description?: string;
			icon?: string;
			field_group_id?: string;
			item_label?: string;
			config?: Record<string, unknown>;
		}
	) => api<ComparisonType>(`/jce/comparison-types/${id}`, { method: 'PATCH', body }),
	deleteType: (id: string) =>
		api<{ deleted: boolean }>(`/jce/comparison-types/${id}`, { method: 'DELETE' }),
	getTypeAccess: (id: string) =>
		api<ComparisonTypeAccessRow[]>(`/jce/comparison-types/${id}/access`),
	setTypeAccess: (
		id: string,
		body: { user_group_id: string; can_create: boolean; can_fill: boolean; ai_can_fill: boolean }
	) => api<{ updated: boolean }>(`/jce/comparison-types/${id}/access`, { method: 'PUT', body }),
	listFieldGroups: () => api<CustomFieldGroupOption[]>('/custom-field-groups'),
	// Dimension types from GET /jce/comparisons/dimension-types (Rule 16).
	getDimensionTypes: () => api<{ value: string; label: string }[]>('/jce/comparisons/dimension-types'),
	// Visibility options from GET /content/visibility-options (Rule 16).
	getVisibilityOptions: () => api<{ value: string; label: string }[]>('/content/visibility-options'),
	// Status options from GET /content/status-options (Rule 16).
	getStatusOptions: () => api<{ value: string; label: string }[]>('/content/status-options'),
	// Comparison content CRUD (Section 30 — actual comparison articles).
	listComparisons: (params?: string) =>
		api<ComparisonEntry[]>(`/jce/comparisons${params ? `?${params}` : ''}`),
	listComparisonsAdmin: (params?: string) =>
		api<ComparisonEntry[]>(`/jce/comparisons/admin${params ? `?${params}` : ''}`),
	createComparison: (body: {
		title: string;
		slug?: string;
		body?: string;
		visibility?: string;
		status?: string;
		type_id?: string;
		space_id?: string;
		items?: Array<{ name: string; slug?: string }>;
		dimensions?: string[];
		depth?: 'quick' | 'detailed';
	}) => api<ComparisonEntry>('/jce/comparisons', { method: 'POST', body: { ...body, depth: body.depth ?? 'detailed' } }),
	updateComparison: (
		id: string,
		body: { title?: string; slug?: string; body?: string; visibility?: string; status?: string }
	) => api<ComparisonEntry>(`/jce/comparisons/${id}`, { method: 'PATCH', body }),
	deleteComparison: (id: string) =>
		api<unknown>(`/jce/comparisons/${id}`, { method: 'DELETE' }),
};

export interface ComparisonProfileAdmin {
	id: string;
	title: string;
	source_space_id?: string | null;
	target_space_ids: string[];
	goals: { key: string; weight: number }[];
	preferences: Record<string, number>;
	demographics: Record<string, any>;
	is_default: boolean;
	is_template: boolean;
}

import type { ScoreFactorAdmin } from '$lib/types/jce';
export type { ScoreFactorAdmin };

export const jceAdminApi = {
	listProfiles: () => api<ComparisonProfileAdmin[]>('/jce/profiles?all=true'),
	createProfile: (body: Record<string, any>) => api<ComparisonProfileAdmin>('/jce/profiles', { method: 'POST', body }),
	updateProfile: (id: string, body: Record<string, any>) => api<ComparisonProfileAdmin>(`/jce/profiles/${id}`, { method: 'PATCH', body }),
	deleteProfile: (id: string) => api<unknown>(`/jce/profiles/${id}`, { method: 'DELETE' }),
	listScoreFactors: (typeID: string) => api<ScoreFactorAdmin[]>(`/jce/comparison-types/${typeID}/score-factors`),
	createScoreFactor: (typeID: string, body: Record<string, any>) => api<ScoreFactorAdmin>(`/jce/comparison-types/${typeID}/score-factors`, { method: 'POST', body }),
	updateScoreFactor: (id: string, body: Record<string, any>) => api<ScoreFactorAdmin>(`/jce/score-factors/${id}`, { method: 'PATCH', body }),
	deleteScoreFactor: (id: string) => api<unknown>(`/jce/score-factors/${id}`, { method: 'DELETE' }),
	getProfileStats: () => api<{ source_space_id: string; profile_count: number }[]>('/jce/profiles/stats?all=true'),
};

export interface UserDetail {
	id: string;
	username: string;
	email: string;
	groups: string[];
	status: string;
	profile_type: 'regular' | 'pro' | 'business';
	avatar_url?: string;
	created_at: string;
	last_login_at?: string;
}

// Moderation config option (value/label pair) — BUG-019/BUG-020/BUG-021
export interface ModerationConfigOption {
	value: string;
	label: string;
}

export interface ModerationConfig {
	statuses: ModerationConfigOption[];
	profile_types: ModerationConfigOption[];
	violation_types: ModerationConfigOption[];
	restriction_operations: ModerationConfigOption[];
	restriction_durations: ModerationConfigOption[];
}

export interface UserActivity {
	type: string;
	content: string;
	created_at: string;
}

export interface AiContextEntry {
	id: string;
	entry_type: string;
	content: string;
	created_at: string;
}

export interface UserSession {
	id: string;
	ip_address: string;
	device: string;
	created_at: string;
	expires_at: string;
}

export interface UserGroup {
	id: string;
	name: string;
	slug: string;
	description: string;
	is_system?: boolean;
	member_count: number;
	permissions: string[];
}

export interface UserRole {
	id: string;
	name: string;
	permissions: string[];
	user_count: number;
}

export const permissionsApi = {
	list: () => api<string[]>('/admin/permissions'),
};

export const userManagementApi = {
	list: (params?: string) =>
		api<{ items: UserDetail[]; total: number; page: number; total_pages: number }>(`/admin/users${params ? `?${params}` : ''}`),
	getUser: (userId: string) => api<UserDetail>(`/admin/users/${userId}`),
	getUserActivity: (userId: string, params?: string) =>
		api<{ activities: UserActivity[]; total: number }>(`/admin/users/${userId}/activity${params ? `?${params}` : ''}`),

	getAiContext: (userId: string) =>
		api<AiContextEntry[]>(`/admin/users/${userId}/ai-context`),
	deleteAiContextEntry: (userId: string, entryId: string) =>
		api<{ deleted: boolean }>(`/admin/users/${userId}/ai-context/${entryId}`, { method: 'DELETE' }),

	getUserSessions: (userId: string) =>
		api<UserSession[]>(`/admin/users/${userId}/sessions`),

	issueWarning: (userId: string, body: { reason: string }) =>
		api(`/admin/users/${userId}/warn`, { method: 'POST', body }),
	banUser: (userId: string) =>
		api(`/admin/users/${userId}/ban`, { method: 'POST', body: {} }),
	unbanUser: (userId: string) =>
		api(`/admin/users/${userId}/ban`, { method: 'DELETE' }),
	suspendUser: (userId: string, body: { status: string; reason?: string }) =>
		api(`/admin/users/${userId}/status`, { method: 'PATCH', body }),
	updateProfileType: (userId: string, body: { profile_type: string }) =>
		api(`/admin/users/${userId}/profile-type`, { method: 'PATCH', body }),
	// Granular ban system
	restrictUser: (userId: string, body: { operations: string[]; reason: string; duration: string }) =>
		api(`/admin/users/${userId}/restrict`, { method: 'POST', body }),
	liftRestriction: (userId: string, operation: string) =>
		api(`/admin/users/${userId}/restrict/${operation}`, { method: 'DELETE' }),
	liftAllRestrictions: (userId: string) =>
		api(`/admin/users/${userId}/restrict`, { method: 'DELETE' }),
	listRestrictions: (userId: string) =>
		api<{ data: Array<{ id: string; operation: string; reason: string | null; expires_at: string | null; created_at: string }> }>(`/admin/users/${userId}/restrictions`),
	suspendUserFull: (userId: string, body: { reason: string }) =>
		api(`/admin/users/${userId}/suspend`, { method: 'POST', body }),
	warnUserTyped: (userId: string, body: { reason: string; violation_type: string }) =>
		api<{ user_id: string; violation_type: string; warning_count: number; auto_restricted: boolean }>(`/admin/users/${userId}/warn-typed`, { method: 'POST', body }),
	// Group membership is managed via the RBAC user-group routes; the backend
	// PATCH /admin/users/{id}/group endpoint only ADDS one membership by
	// group_id and cannot remove, so a set-style "change groups" must diff
	// against these two endpoints.
	addGroupMember: (groupId: string, userId: string) =>
		api(`/admin/user-groups/${groupId}/members`, { method: 'POST', body: { user_id: userId } }),
	removeGroupMember: (groupId: string, userId: string) =>
		api(`/admin/user-groups/${groupId}/members/${userId}`, { method: 'DELETE' }),
	createUser: (body: { email: string; username: string; password: string; display_name?: string }) =>
		api<{ id: string; email: string; username: string }>('/admin/users', { method: 'POST', body }),
	deleteUser: (userId: string) =>
		api(`/admin/users/${userId}`, { method: 'DELETE' }),
	hardDeleteUser: (userId: string) =>
		api(`/admin/users/${userId}/hard`, { method: 'DELETE' }),

	listGroups: (params?: string) =>
		api<{ groups: UserGroup[]; total: number }>(`/admin/user-groups${params ? `?${params}` : ''}`),
	createGroup: (body: { name: string; description: string; permissions: string[] }) =>
		api<UserGroup>('/admin/user-groups', { method: 'POST', body }),
	updateGroup: (id: string, body: { name?: string; description?: string; permissions?: string[] }) =>
		api<UserGroup>(`/admin/user-groups/${id}`, { method: 'PATCH', body }),
	deleteGroup: (id: string) =>
		api<{ deleted: boolean }>(`/admin/user-groups/${id}`, { method: 'DELETE' }),

	listRoles: (params?: string) =>
		api<{ roles: UserRole[]; total: number; page: number; total_pages: number }>(`/admin/roles${params ? `?${params}` : ''}`),
	createRole: (body: { name: string; permissions: string[] }) =>
		api<UserRole>('/admin/roles', { method: 'POST', body }),
	updateRole: (id: string, body: { name?: string; permissions?: string[] }) =>
		api<UserRole>(`/admin/roles/${id}`, { method: 'PATCH', body }),
	deleteRole: (id: string) =>
		api<{ deleted: boolean }>(`/admin/roles/${id}`, { method: 'DELETE' }),

	listCustomFieldEntityTypes: () =>
		api<Array<{ value: string; label: string }>>('/custom-field-entity-types'),
	// Moderation config (profile types, violation types, restriction
	// operations + durations) — BUG-019/BUG-020/BUG-021
	getModerationConfig: () =>
		api<ModerationConfig>('/admin/users/config'),
};

export interface HitlReview {
	id: string;
	content: string;
	content_type: string;
	source: string;
	status: string;
	reviewer_id?: string;
	reviewer_name?: string;
	review_notes?: string;
	created_at: string;
	reviewed_at?: string;
}

export interface HitlVerifyResult {
	claim: string;
	answer: string;
	citations: string[];
	provider: string;
}

export interface GenerateResult {
	generated_text: string;
	model: string;
	tokens_used: number;
}

export const aiApi = {
	generate: (data: {
		prompt: string;
		category: string;
		model?: string;
		voice_id?: string;
		humanize?: boolean;
		humanize_level?: 'low' | 'medium' | 'high';
	}) => api<GenerateResult>('/ai/generate', { method: 'POST', body: data }),

	createHitlReview: (data: { content: string; content_type: string; source: string }) =>
		api<{ id: string; status: string }>('/ai/hitl', { method: 'POST', body: data }),

	listPendingHitlReviews: (params?: { limit?: number; offset?: number }) => {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		const q = p.toString();
		return api<{ reviews: HitlReview[]; total: number }>(
			`/ai/hitl/pending${q ? '?' + q : ''}`
		);
	},

	approveHitlReview: (id: string) =>
		api<{ approved: boolean }>(`/ai/hitl/${id}/approve`, { method: 'POST' }),

	rejectHitlReview: (id: string, reason: string) =>
		api<{ rejected: boolean }>(`/ai/hitl/${id}/reject`, { method: 'POST', body: { reason } }),

	verifyHitlClaim: (data: { claim: string; context?: string }) =>
		api<HitlVerifyResult>('/ai/hitl/verify', { method: 'POST', body: data }),

	listCategories: () =>
		api<Array<{ value: string; label: string; description: string }>>('/ai/categories'),

	listModels: () =>
		api<Array<{ id: string; label: string }>>('/admin/ai/models'),
};

export interface JsonLdTemplate {
	id: string;
	tenant_id: string;
	content_type: string;
	template: Record<string, unknown> | null;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}

export const jsonLdApi = {
	listTemplates: () => api<JsonLdTemplate[]>('/content/jsonld-templates'),
	upsertTemplate: (input: { content_type: string; template: Record<string, unknown>; is_active: boolean }) =>
		api<JsonLdTemplate>('/content/jsonld-templates', {
			method: 'PUT',
			body: input
		}),
	deleteTemplate: (id: string) =>
		api<{ deleted: boolean }>(`/content/jsonld-templates/${id}`, {
			method: 'DELETE'
		}),
};

export interface BlockType {
	type: string;
	name: string;
	description: string;
	icon: string;
	default_props: Record<string, unknown>;
	category: string;
}

export const pageBuilderApi = {
	getBlockTypes: () => api<{ block_types: BlockType[] }>('/page-builder/block-types'),
};

export const spacesAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/spaces${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/spaces/${id}`),
	getModerators: (id: string) => api<unknown>(`/admin/spaces/${id}/moderators`),
	create: (body: { name: string; slug: string; space_type_id: string; level: string; description?: string; parent_id?: string; preset_name?: string }) =>
		api('/admin/spaces', { method: 'POST', body }),
	update: (id: string, body: { name?: string; visibility?: string; status?: string }) =>
		api(`/admin/spaces/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/spaces/${id}`, { method: 'DELETE' }),
	suspend: (id: string, body: { status: string; reason?: string }) =>
		api(`/admin/spaces/${id}/status`, { method: 'PATCH', body }),
	assignModerator: (id: string, body: { username: string }) =>
		api(`/admin/spaces/${id}/moderators`, { method: 'POST', body }),
};

export const tagsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/tags${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string; description?: string; rules?: string; cover_media_id?: string; color?: string; is_system?: boolean; logo_media_id?: string; field_values?: Record<string, unknown> }) =>
		api('/admin/tags', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; description?: string; rules?: string; cover_media_id?: string; color?: string; logo_media_id?: string; is_system?: boolean; field_values?: Record<string, unknown> }) =>
		api(`/admin/tags/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/tags/${id}`, { method: 'DELETE' }),
	addModerator: (id: string, body: { user_id: string }) =>
		api(`/admin/tags/${id}/moderators`, { method: 'POST', body }),
	getModerators: (id: string) =>
		api<unknown>(`/admin/tags/${id}/moderators`),
};

export interface PersonaVoice {
	id: string;
	tenant_id: string;
	name: string;
	slug: string;
	description: string | null;
	voice_prompt: string;
	is_system: boolean;
	created_by: string;
	created_at: string;
	updated_at: string;
}

export const personaVoicesAdminApi = {
	list: (params?: string) =>
		apiRaw<{ data: PersonaVoice[]; meta: { total: number; page_size: number; page_offset: number } }>(
			`/admin/persona-voices${params ? `?${params}` : ''}`
		),
	get: (id: string) => api<PersonaVoice>(`/admin/persona-voices/${id}`),
	create: (body: { name: string; slug: string; description?: string; voice_prompt: string; is_system?: boolean }) =>
		api<PersonaVoice>('/admin/persona-voices', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; description?: string; voice_prompt?: string; is_system?: boolean }) =>
		api<PersonaVoice>(`/admin/persona-voices/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<unknown>(`/admin/persona-voices/${id}`, { method: 'DELETE' }),
};

export const organisationsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/organizations${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/organizations/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/organizations', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) => api<unknown>(`/admin/organizations/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/organizations/${id}`, { method: 'DELETE' }),
	listMembers: (id: string) => api<unknown>(`/admin/organizations/${id}/members`),
	listInvitations: (id: string) => api<unknown>(`/admin/organizations/${id}/invitations`),
	verify: (id: string, body: { verified: boolean }) =>
		api(`/admin/organizations/${id}/verify`, { method: 'POST', body }),
	unverify: (id: string) =>
		api(`/admin/organizations/${id}/verify`, { method: 'DELETE' }),
	claim: (id: string, body: { action: 'approve' | 'reject'; note?: string }) =>
		api(`/admin/organizations/${id}/claim`, { method: 'POST', body }),
	pendingClaims: (params?: string) =>
		api<{
			id: string;
			organization_id: string;
			claimant_user_id: string;
			claimant_username: string;
			claimant_email: string;
			org_name: string;
			org_slug: string;
			status: string;
			evidence_url?: string | null;
			notes?: string | null;
			form_submission_id?: string | null;
			resolution_note?: string | null;
			reviewed_by_username?: string | null;
			reviewed_at?: string | null;
			created_at: string;
		}[]>(`/admin/organizations/pending-claims${params ? `?${params}` : ''}`),
	claimSubmission: (claimId: string) =>
		api<{
			id: string;
			form_id: string;
			field_values: Record<string, unknown> | null;
			status: string;
			created_at: string;
			claim_id: string;
		}>(`/admin/organizations/claims/${claimId}/submission`),
	// BUG-022/023: Org metadata endpoint for API-driven option lists (Rule 16).
	// org_types are read from the custom_fields table (key='org_type').
	// Admins manage org type options via the Custom Fields admin page
	// (organization_fields group → org_type field → options editor).
	meta: () =>
		api<{
			org_types: { value: string; label: string }[];
			member_roles: { value: string; label: string }[];
			invitation_roles: { value: string; label: string }[];
			claim_statuses: { value: string; label: string }[];
		}>('/organizations/meta'),
};

export const reactionsAdminApi = {
	list: (params?: string) => api<unknown>(`/reaction-types${params ? `?${params}` : ''}`),
	create: (body: { key: string; emoji: string; label: string; enabled: boolean }) =>
		api('/reaction-types', { method: 'POST', body }),
	update: (key: string, body: { emoji?: string; label?: string; enabled?: boolean }) =>
		api(`/reaction-types/${key}`, { method: 'PATCH', body }),
	delete: (key: string) =>
		api(`/reaction-types/${key}`, { method: 'DELETE' }),
};

export const universitiesAdminApi = {
	list: (params?: string) => apiRaw(`/admin/universities${params ? `?${params}` : ''}`),
	listPublic: (params?: string) => api<unknown>(`/universities${params ? `?${params}` : ''}`),
	getBySlug: (slug: string) => api<unknown>(`/universities/${encodeURIComponent(slug)}`),
	create: (body: { name: string; slug: string; initials?: string; country?: string; logo_url?: string; verified: boolean }) =>
		api('/admin/universities', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; initials?: string; country?: string; logo_url?: string; verified?: boolean }) =>
		api(`/admin/universities/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/universities/${id}`, { method: 'DELETE' }),
	verify: (id: string) =>
		api(`/admin/universities/${id}/verify`, { method: 'POST' }),
	unverify: (id: string) =>
		api(`/admin/universities/${id}/verify`, { method: 'DELETE' }),
	bulkImport: (body: FormData) =>
		api('/admin/universities/bulk-import', { method: 'POST', body }),
	updateStatus: (id: string, body: { status: string }) =>
		api(`/admin/universities/${id}/status`, { method: 'PATCH', body }),
};

// --- Japa University Creator Economy (ADR-063) ---

export const universityAdminApi = {
	listInstructors: (params?: string) =>
		api<unknown>(`/admin/university/instructors${params ? `?${params}` : ''}`),
	getInstructor: (id: string) =>
		api<unknown>(`/admin/university/instructors/${id}`),
	approveInstructor: (id: string, body: { tier?: string; commission_pct_override?: number }) =>
		api<unknown>(`/admin/university/instructors/${id}/approve`, { method: 'POST', body }),
	rejectInstructor: (id: string) =>
		api<unknown>(`/admin/university/instructors/${id}/reject`, { method: 'POST', body: {} }),
	suspendInstructor: (id: string) =>
		api<unknown>(`/admin/university/instructors/${id}/suspend`, { method: 'POST', body: {} }),
	reinstateInstructor: (id: string) =>
		api<unknown>(`/admin/university/instructors/${id}/reinstate`, { method: 'POST', body: {} }),
	listPendingTracks: (params?: string) =>
		apiRaw<{ data: { id: string; title: string; slug: string; description?: string | null; difficulty: string; instructor_id?: string | null; instructor_name: string; price_kobo?: number | null; currency: string; category?: string | null; language: string; geo_space_id?: string | null; total_enrolled: number; is_published: boolean; is_published_to_university: boolean; created_at: string; updated_at: string }[]; meta: { total: number; limit: number; offset: number } }>(`/admin/university/tracks/pending${params ? `?${params}` : ''}`),
	listAllTracks: (params?: string) =>
		apiRaw<{ data: { id: string; title: string; slug: string; description?: string | null; difficulty: string; instructor_id?: string | null; instructor_name: string; price_kobo?: number | null; currency: string; category?: string | null; language: string; geo_space_id?: string | null; total_enrolled: number; is_published: boolean; is_published_to_university: boolean; created_at: string; updated_at: string }[]; meta: { total: number; limit: number; offset: number } }>(`/admin/university/tracks${params ? `?${params}` : ''}`),
	getTrack: (id: string) =>
		api<unknown>(`/admin/university/tracks/${id}`),
	approveTrack: (id: string) =>
		api<unknown>(`/admin/university/tracks/${id}/approve`, { method: 'POST', body: {} }),
	rejectTrack: (id: string, body: { reason?: string }) =>
		api<unknown>(`/admin/university/tracks/${id}/reject`, { method: 'POST', body }),
	listPayouts: (params?: string) =>
		api<unknown>(`/admin/university/payouts${params ? `?${params}` : ''}`),
	listRevenueSplits: (params?: string) =>
		api<unknown>(`/admin/university/revenue-splits${params ? `?${params}` : ''}`),
	getPayout: (id: string) =>
		api<unknown>(`/admin/university/payouts/${id}`),
	processPayout: (id: string) =>
		api<unknown>(`/admin/university/payouts/${id}/process`, { method: 'POST', body: {} }),
	markPayoutFailed: (id: string, body: { reason?: string }) =>
		api<unknown>(`/admin/university/payouts/${id}/fail`, { method: 'POST', body }),
	deleteInstructor: (id: string) =>
		api<unknown>(`/admin/university/instructors/${id}`, { method: 'DELETE' }),
	updateInstructor: (id: string, body: { tier?: string; commission_pct_override?: number }) =>
		api<unknown>(`/admin/university/instructors/${id}`, { method: 'PATCH', body }),
	featureTrack: (id: string, featured: boolean) =>
		api<unknown>(`/admin/university/tracks/${id}/feature`, { method: 'POST', body: { featured } }),
	// Track categories (ADR-063 §9.1)
	listCategories: (params?: string) =>
		apiRaw<{ data: { id: string; slug: string; name: string; sort_order: number; is_active: boolean; created_at: string; updated_at: string }[]; meta: { total: number; page: number; limit: number } }>(`/admin/university/categories${params ? `?${params}` : ''}`),
	createCategory: (body: { slug: string; name: string; sort_order?: number; is_active?: boolean }) =>
		api<unknown>(`/admin/university/categories`, { method: 'POST', body }),
	getCategory: (id: string) =>
		api<unknown>(`/admin/university/categories/${id}`),
	updateCategory: (id: string, body: { slug?: string; name?: string; sort_order?: number; is_active?: boolean }) =>
		api<unknown>(`/admin/university/categories/${id}`, { method: 'PATCH', body }),
	deleteCategory: (id: string) =>
		api<unknown>(`/admin/university/categories/${id}`, { method: 'DELETE' }),
};

// --- Geo IP (public hierarchy + admin lookup) ---

export interface GeoCountry {
	id: string;
	name: string;
	slug: string;
	level: string;
	geo_code: string;
	description?: string;
	member_count?: number;
	post_count?: number;
	follower_count?: number;
}

export const geoApi = {
	me: () => api<unknown>('/geo/me'),
	detect: () => api<unknown>('/geo/detect'),
	lookup: (ip: string) => api<unknown>(`/geo/lookup?ip=${encodeURIComponent(ip)}`),
	listCountries: () => api<GeoCountry[]>('/geo/countries'),
	listStates: (countryCode: string) => api<unknown>(`/geo/countries/${countryCode}/states`),
	listCities: (stateId: string) => api<unknown>(`/geo/states/${stateId}/cities`),
};

// --- Unconsumed route methods (490 route initiative) ---

export const adsAdminApi = {
	listCampaigns: (params?: string) => api<unknown>(`/admin/ads/campaigns${params ? `?${params}` : ''}`),
	getCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}`),
	getCampaignStats: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/stats`),
	listActiveBoosts: (params?: string) => api<unknown>(`/admin/ads/boosts/active${params ? `?${params}` : ''}`),
	createCampaign: (body: Record<string, unknown>) => api<unknown>('/admin/ads/campaigns', { method: 'POST', body }),
	updateCampaign: (id: string, body: Record<string, unknown>) => api<unknown>(`/admin/ads/campaigns/${id}`, { method: 'PATCH', body }),
	activateCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/activate`, { method: 'POST', body: {} }),
	rejectCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/reject`, { method: 'POST', body: {} }),
	endCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/end`, { method: 'POST', body: {} }),
	pauseCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/pause`, { method: 'POST', body: {} }),
	endBoost: (id: string) => api<unknown>(`/admin/ads/boosts/${id}/end`, { method: 'POST', body: {} }),
};

export interface AiTool {
	id: string;
	name: string;
	description: string;
	category: string;
	config?: Record<string, unknown>;
	is_enabled: boolean;
	required_permission?: string;
	created_at: string;
	updated_at: string;
}

export interface AiUsageSummary {
	total_cost_usd: number;
	tokens_used: number;
	by_task_type: Array<{ task_type: string; tokens_used: number; total_cost_usd: number; calls: number }>;
	by_provider?: Array<{ provider: string; tokens_used: number; total_cost_usd: number; calls: number }>;
}

export interface AiToolUsageStats {
	tool_id: string;
	tool_name: string;
	invocations: number;
	tokens_used: number;
	total_cost_usd: number;
}

export const aiAdminApi = {
	listTools: (params?: string) => api<AiTool[]>(`/admin/ai/tools${params ? `?${params}` : ''}`),
	getTool: (id: string) => api<AiTool>(`/admin/ai/tools/${id}`),
	getToolsUsage: () => api<AiToolUsageStats[]>('/admin/ai/tools/usage'),
	getAiUsage: () => api<AiUsageSummary>('/admin/ai/usage'),
	updateTool: (id: string, body: Record<string, unknown>) => api<AiTool>(`/admin/ai/tools/${id}`, { method: 'PUT', body }),
	deleteTool: (id: string) => api<{ deleted: boolean }>(`/admin/ai/tools/${id}`, { method: 'DELETE' }),
	getUserAiContext: (userId: string) => api<unknown>(`/admin/users/${userId}/ai-context`),
	deleteAiContextEntry: (userId: string, entryId: string) =>
		api<{ deleted: boolean }>(`/admin/users/${userId}/ai-context/${entryId}`, { method: 'DELETE' }),
};

export const analyticsAdminApi = {
	getOverview: (params?: string) => api<unknown>(`/admin/analytics/overview${params ? `?${params}` : ''}`),
	getUsers: (params?: string) => api<unknown>(`/admin/analytics/users${params ? `?${params}` : ''}`),
	getSpaces: (params?: string) => api<unknown>(`/admin/analytics/spaces${params ? `?${params}` : ''}`),
	getContent: (params?: string) => api<unknown>(`/admin/analytics/content${params ? `?${params}` : ''}`),
	getEvents: (params?: string) => api<unknown>(`/admin/analytics/events${params ? `?${params}` : ''}`),
	getRevenue: (params?: string) => api<unknown>(`/admin/analytics/revenue${params ? `?${params}` : ''}`),
	getSearch: (params?: string) => api<unknown>(`/admin/analytics/search${params ? `?${params}` : ''}`),
	getSecurity: (params?: string) => api<unknown>(`/admin/analytics/security${params ? `?${params}` : ''}`),
	flushCache: (body: Record<string, unknown>) => api<unknown>('/admin/analytics/cache/flush', { method: 'POST', body }),
	refresh: (body: Record<string, unknown>) => api<unknown>('/admin/analytics/refresh', { method: 'POST', body }),
	runSecurityScan: (body: Record<string, unknown>) => api<unknown>('/admin/analytics/security/scan', { method: 'POST', body }),
	resolveSecurityItem: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/analytics/security/${id}/resolve`, { method: 'POST', body }),
	// Section 66 — Advanced Analytics Dashboard (ADR-084)
	getTopContent: (params?: string) => api<unknown>(`/admin/analytics/top-content${params ? `?${params}` : ''}`),
	getRevenueStreams: (params?: string) => api<unknown>(`/admin/analytics/revenue-streams${params ? `?${params}` : ''}`),
	getAdPerformance: (params?: string) => api<unknown>(`/admin/analytics/ads${params ? `?${params}` : ''}`),
	getNewsletterEngagement: (params?: string) => api<unknown>(`/admin/analytics/newsletter${params ? `?${params}` : ''}`),
	getRetention: (params?: string) => api<unknown>(`/admin/analytics/retention${params ? `?${params}` : ''}`),
	exportCsv: (metric: string, period?: string) => {
		const params = [metric && `metric=${metric}`, period && `period=${period}`].filter(Boolean).join('&');
		return fetchRaw(`/admin/analytics/export${params ? `?${params}` : ''}`);
	},
};

export const blogAdminApi = {
	getReviewQueue: (params?: string) => api<unknown>(`/admin/blog/review-queue${params ? `?${params}` : ''}`),
	approvePost: (id: string) => api<unknown>(`/admin/blog/review-queue/${id}/approve`, { method: 'POST', body: {} }),
	rejectPost: (id: string, body: { reason?: string }) =>
		api<unknown>(`/admin/blog/review-queue/${id}/reject`, { method: 'POST', body }),
	createPost: (body: { title: string; slug?: string; body?: string; body_html?: string; status?: string; visibility?: string }) =>
		api<unknown>('/admin/blog', { method: 'POST', body }),
	updatePost: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/blog/${id}`, { method: 'PATCH', body }),
	publishPost: (id: string) =>
		api<unknown>(`/content/${id}/publish`, { method: 'POST', body: {} }),
	unpublishPost: (id: string) =>
		api<unknown>(`/admin/blog/${id}`, { method: 'PATCH', body: { status: 'draft' } }),
	restorePost: (id: string) =>
		api<unknown>(`/admin/blog/${id}`, { method: 'PATCH', body: { status: 'draft' } }),
	deletePost: (id: string) =>
		api<unknown>(`/admin/blog/${id}`, { method: 'DELETE' }),
};

export const contentAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/content${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/content/${id}`),
	remove: (id: string, body?: { reason?: string }) => api<unknown>(`/admin/content/${id}/remove`, { method: 'POST', body: body ?? {} }),
	restore: (id: string) => api<unknown>(`/admin/content/${id}/restore`, { method: 'POST', body: {} }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/content/${id}`, { method: 'PATCH', body }),
	feature: (id: string) => api<unknown>(`/blog/${id}/feature`, { method: 'POST', body: {} }),
	unfeature: (id: string) => api<unknown>(`/blog/featured/${id}`, { method: 'DELETE' }),
	archive: (id: string) => api<unknown>(`/content/${id}/archive`, { method: 'POST', body: {} }),
	listContentTypes: () => api<{ value: string; label: string }[]>('/content-types'),
	// BUG-007/BUG-008: status & visibility options from the backend (Rule 16).
	getStatusOptions: () => api<{ value: string; label: string }[]>('/content/status-options'),
	getVisibilityOptions: () => api<{ value: string; label: string }[]>('/content/visibility-options'),
};

export const mediaAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/media${params ? `?${params}` : ''}`),
	listQuarantined: (params?: string) => api<unknown>(`/admin/media/quarantined${params ? `?${params}` : ''}`),
	approve: (id: string) => api<unknown>(`/admin/media/${id}/approve`, { method: 'POST', body: {} }),
	reject: (id: string) => api<unknown>(`/admin/media/${id}/reject`, { method: 'POST', body: {} }),
	updateStatus: (id: string, status: string) => api<unknown>(`/admin/media/${id}/status`, { method: 'POST', body: { status } }),
	update: (id: string, body: { alt_text?: string; caption?: string; width?: number; height?: number; user_group_id?: string }) =>
		api<unknown>(`/admin/media/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/media/${id}`, { method: 'DELETE' }),
};

export interface StockProviderAdmin {
	provider_name: string;
	display_name: string;
	enabled: boolean;
	has_api_key: boolean;
	updated_at?: string;
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
	providers: { name: string; display_name: string }[];
}

export const stockAdminApi = {
	listProviders: () => api<{ providers: StockProviderAdmin[] }>('/admin/stock/providers'),
	patchProvider: (name: string, body: { enabled?: boolean; api_key?: string }) =>
		api<{ status: string }>(`/admin/stock/providers/${name}`, { method: 'PATCH', body }),
	/** User-facing stock endpoints (same backend, admin has wildcard perms). */
	listAvailableProviders: () => api<StockProvidersResponse>('/stock/providers'),
	search: (params: { q: string; provider?: string; page?: number; per_page?: number }) => {
		const sp = new URLSearchParams();
		sp.set('q', params.q);
		if (params.provider) sp.set('provider', params.provider);
		if (params.page) sp.set('page', String(params.page));
		if (params.per_page) sp.set('per_page', String(params.per_page));
		return api<StockSearchResult>(`/stock/search?${sp.toString()}`);
	},
	import: (provider: string, id: string) =>
		api<StockImportResult>('/stock/import', { method: 'POST', body: { provider, id } }),
};

export interface ModerationOptions {
	statuses: { value: string; label: string }[];
	content_types: { value: string; label: string }[];
}

export const moderationAdminApi = {
	getQueue: (params?: string) => api<unknown>(`/admin/moderation-queue${params ? `?${params}` : ''}`),
	// Rule 16: moderation filter options loaded from the backend.
	getOptions: () => api<ModerationOptions>('/admin/moderation/options'),
	getAlerts: (params?: string) => api<unknown>(`/admin/moderation/alerts${params ? `?${params}` : ''}`),
	updateItem: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/moderation-queue/${id}`, { method: 'PATCH', body }),
	assignItem: (id: string, body: { assignee_id: string }) =>
		api<unknown>(`/admin/moderation-queue/${id}/assign`, { method: 'PATCH', body }),
	bulkAction: (body: Record<string, unknown>) => api<unknown>('/admin/moderation-queue/bulk', { method: 'POST', body }),
	appealItem: (id: string, body: { reason: string }) =>
		api<unknown>(`/admin/moderation-queue/${id}/appeal`, { method: 'POST', body }),
	acknowledgeAlert: (id: string) => api<unknown>(`/admin/moderation/alerts/${id}/acknowledge`, { method: 'POST', body: {} }),
	resolveAlert: (id: string, body: { resolution: string }) =>
		api<unknown>(`/admin/moderation/alerts/${id}/resolve`, { method: 'POST', body }),
	listReports: (params?: string) => api<unknown>(`/admin/reports${params ? `?${params}` : ''}`),
	getReport: (id: string) => api<unknown>(`/admin/reports/${id}`),
	updateReport: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/reports/${id}`, { method: 'PATCH', body }),
	deleteReport: (id: string) => api<{ deleted: boolean }>(`/admin/reports/${id}`, { method: 'DELETE' }),
	listAppeals: (params?: string) => api<unknown>(`/admin/appeals${params ? `?${params}` : ''}`),
	getAppeal: (id: string) => api<unknown>(`/admin/appeals/${id}`),
	updateAppeal: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/appeals/${id}`, { method: 'PATCH', body }),
};

export const newsletterAdminApi = {
	listCampaigns: (params?: string) => api<unknown>(`/admin/newsletter/campaigns${params ? `?${params}` : ''}`),
	getCampaign: (id: string) => api<unknown>(`/admin/newsletter/campaigns/${id}`),
	listSubscribers: (params?: string) => api<unknown>(`/admin/newsletter/subscribers${params ? `?${params}` : ''}`),
	getSubscriber: (id: string) => api<unknown>(`/admin/newsletter/subscribers/${id}`),
	createCampaign: (body: Record<string, unknown>) => api<unknown>('/admin/newsletter/campaigns', { method: 'POST', body }),
	updateCampaign: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/newsletter/campaigns/${id}`, { method: 'PATCH', body }),
	sendCampaign: (id: string) => api<unknown>(`/admin/newsletter/campaigns/${id}/send`, { method: 'POST', body: {} }),
	deleteSubscriber: (id: string) => api<{ deleted: boolean }>(`/admin/newsletter/subscribers/${id}`, { method: 'DELETE' }),
};

export const ticketsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/tickets${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/tickets/${id}`),
	getEvents: (id: string) => api<unknown>(`/admin/tickets/${id}/events`),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/tickets/${id}`, { method: 'PATCH', body }),
	addComment: (id: string, body: { body: string; is_internal?: boolean }) =>
		api<unknown>(`/admin/tickets/${id}/comments`, { method: 'POST', body }),
	addNote: (id: string, body: { body: string }) =>
		api<unknown>(`/admin/tickets/${id}/notes`, { method: 'POST', body }),
	resolve: (id: string, body: { resolution_note: string }) =>
		api<unknown>(`/admin/tickets/${id}/resolve`, { method: 'POST', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/tickets/${id}`, { method: 'DELETE' }),
};

export const usersAdminApiExtended = {
	getBans: (id: string) => api<unknown>(`/admin/users/${id}/bans`),
	getWarnings: (id: string) => api<unknown>(`/admin/users/${id}/warnings`),
	deleteWarning: (id: string, warningId: string) =>
		api<unknown>(`/admin/users/${id}/warnings/${warningId}`, { method: 'DELETE' }),
	warn: (id: string, body: { reason: string }) =>
		api<unknown>(`/admin/users/${id}/warn`, { method: 'POST', body }),
	ban: (id: string, body: { reason?: string; duration_days?: number }) =>
		api<unknown>(`/admin/users/${id}/ban`, { method: 'POST', body }),
	updateGroup: (id: string, body: { group_id: string }) =>
		api<unknown>(`/admin/users/${id}/group`, { method: 'PATCH', body }),
};

export const settingsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/settings${params ? `?${params}` : ''}`),
	updateCategory: (category: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/settings/${category}`, { method: 'PUT', body }),
};

export const spacesAdminApiExtended = {
	getJoinRequests: (id: string) => api<unknown>(`/admin/spaces/${id}/join-requests`),
	updateJoinRequest: (id: string, requestId: string, body: { status: string }) =>
		api<unknown>(`/admin/spaces/${id}/join-requests/${requestId}`, { method: 'PATCH', body }),
};

export const fieldGroupsAdminApiExtended = {
	list: (params?: string) => apiRaw<{ data: unknown[]; meta?: { page: number; page_size: number; total: number; total_pages: number } }>(`/admin/field-groups${params ? `?${params}` : ''}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/field-groups', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/field-groups/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/field-groups/${id}`, { method: 'DELETE' }),
	addField: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/field-groups/${id}/fields`, { method: 'POST', body }),
	updateField: (id: string, fid: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/field-groups/${id}/fields/${fid}`, { method: 'PATCH', body }),
	deleteField: (id: string, fid: string) =>
		api<{ deleted: boolean }>(`/admin/field-groups/${id}/fields/${fid}`, { method: 'DELETE' }),
	reorderFields: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/field-groups/${id}/fields/reorder`, { method: 'POST', body }),
};

export interface CustomFieldEntityType {
	id: string;
	value: string;
	label: string;
	is_system: boolean;
	sort_order: number;
}

export const entityTypesAdminApi = {
	list: (params?: string) => apiRaw<{ data: CustomFieldEntityType[]; meta?: { page: number; page_size: number; total: number; total_pages: number } }>(`/admin/custom-field-entity-types${params ? `?${params}` : ''}`),
	create: (body: { value: string; label: string; sort_order?: number }) =>
		api<CustomFieldEntityType>('/admin/custom-field-entity-types', { method: 'POST', body }),
	update: (id: string, body: { label: string; sort_order?: number }) =>
		api<CustomFieldEntityType>(`/admin/custom-field-entity-types/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api<{ deleted: boolean }>(`/admin/custom-field-entity-types/${id}`, { method: 'DELETE' }),
};

// Guide Categories (Section 14 — Guide Content Type System)
// Each category creates a custom_field_entity_types row + a field group +
// assignment in one transaction. After creation, admins manage fields via
// the existing custom field group editor at /custom-fields/{groupId}.
export interface GuideCategory {
	Slug: string;
	Label: string;
	is_system: boolean;
	group_id?: string;
	group_slug?: string;
}

export const guideCategoriesAdminApi = {
	list: () => api<GuideCategory[]>('/admin/guide-categories'),
	create: (body: { slug: string; label: string }) =>
		api<GuideCategory>('/admin/guide-categories', { method: 'POST', body }),
	delete: (slug: string) =>
		api<{ status: string }>(`/admin/guide-categories/${slug}`, { method: 'DELETE' }),
};

export const formsAdminApi = {
	list: (params?: string) => api<unknown>(`/forms${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/forms/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/forms', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/forms/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/forms/${id}`, { method: 'DELETE' }),
	ensureFieldGroup: (id: string) =>
		api<{ field_group_id?: string }>(`/forms/${id}/ensure-field-group`, { method: 'POST' }),
	listSubmissions: (id: string, params?: string) =>
		api<unknown>(`/forms/${id}/submissions${params ? `?${params}` : ''}`),
	getSubmission: (id: string, subId: string) =>
		api<unknown>(`/admin/forms/${id}/submissions/${subId}`),
	updateSubmission: (id: string, subId: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/forms/${id}/submissions/${subId}`, { method: 'PATCH', body }),
	deleteSubmission: (id: string, subId: string) =>
		api<{ deleted: boolean }>(`/admin/forms/${id}/submissions/${subId}`, { method: 'DELETE' }),
	// Trigger CRUD (gracefully handles 404 if backend not yet wired)
	listTriggers: (formId: string) =>
		api<unknown>(`/forms/${formId}/triggers`),
	createTrigger: (formId: string, body: Record<string, unknown>) =>
		api<unknown>(`/forms/${formId}/triggers`, { method: 'POST', body }),
	updateTrigger: (formId: string, triggerId: string, body: Record<string, unknown>) =>
		api<unknown>(`/forms/${formId}/triggers/${triggerId}`, { method: 'PATCH', body }),
	deleteTrigger: (formId: string, triggerId: string) =>
		api<{ deleted: boolean }>(`/forms/${formId}/triggers/${triggerId}`, { method: 'DELETE' }),
	reorderTriggers: (formId: string, body: { trigger_ids: string[] }) =>
		api<unknown>(`/forms/${formId}/triggers/reorder`, { method: 'POST', body }),
	// Step CRUD (multi-step forms, Section 06)
	listSteps: (formId: string) => api<unknown>(`/forms/${formId}/steps`),
	createStep: (formId: string, body: { title: string; position: number }) =>
		api<unknown>(`/forms/${formId}/steps`, { method: 'POST', body }),
	updateStep: (formId: string, stepId: string, body: { title: string; position: number }) =>
		api<unknown>(`/forms/${formId}/steps/${stepId}`, { method: 'PATCH', body }),
	deleteStep: (formId: string, stepId: string) =>
		api<{ deleted: boolean }>(`/forms/${formId}/steps/${stepId}`, { method: 'DELETE' }),
	reorderSteps: (formId: string, body: { step_ids: string[] }) =>
		api<unknown>(`/forms/${formId}/steps/reorder`, { method: 'POST', body }),
};

export const onboardingAdminApi = {
	listTemplates: () => api<unknown[]>('/admin/onboarding/templates'),
	getTemplate: (id: string) => api<unknown>(`/admin/onboarding/templates/${id}`),
	createTemplate: (body: Record<string, unknown>) => api<unknown>('/admin/onboarding/templates', { method: 'POST', body }),
	updateTemplate: (id: string, body: Record<string, unknown>) => api<unknown>(`/admin/onboarding/templates/${id}`, { method: 'PATCH', body }),
	deleteTemplate: (id: string) => api<unknown>(`/admin/onboarding/templates/${id}`, { method: 'DELETE' }),
};

export const webhooksAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/webhooks${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/webhooks/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/webhooks', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/webhooks/${id}`, { method: 'PATCH', body }),
	replace: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/webhooks/${id}`, { method: 'PUT', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/webhooks/${id}`, { method: 'DELETE' }),
	test: (id: string) => api<unknown>(`/admin/webhooks/${id}/test`, { method: 'POST', body: {} }),
	verify: (id: string) => api<unknown>(`/admin/webhooks/${id}/verify`, { method: 'POST', body: {} }),
	listDeliveries: (id: string) => api<unknown>(`/admin/webhooks/${id}/deliveries`),
	retryDelivery: (id: string, deliveryId: string) =>
		api<unknown>(`/admin/webhooks/${id}/deliveries/${deliveryId}/retry`, { method: 'POST', body: {} }),
};

export const botsAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/bots${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/bots/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/bots', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) => api<unknown>(`/admin/bots/${id}`, { method: 'PUT', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/bots/${id}`, { method: 'DELETE' }),
};

export const broadcastsAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/broadcasts${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/broadcasts/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/broadcasts', { method: 'POST', body }),
	send: (id: string) => api<unknown>(`/admin/broadcasts/${id}/send`, { method: 'POST', body: {} }),
};

export const couponsAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/coupons${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/coupons/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/coupons', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/coupons/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/coupons/${id}`, { method: 'DELETE' }),
};

export const discountCodesAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/discount-codes${params ? `?${params}` : ''}`),
	getUsage: (id: string) => api<unknown>(`/admin/discount-codes/${id}/usage`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/discount-codes', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/discount-codes/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/discount-codes/${id}`, { method: 'DELETE' }),
};

export const docsAdminApiExtended = {
	getByAudience: (audience: string) => api<unknown>(`/admin/docs/${audience}`),
	getPage: (id: string) => api<unknown>(`/admin/docs/pages/${id}`),
	updatePage: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/docs/pages/${id}`, { method: 'PATCH', body }),
	deletePage: (id: string) => api<{ deleted: boolean }>(`/admin/docs/pages/${id}`, { method: 'DELETE' }),
	aiGenerate: (body: Record<string, unknown>) => api<unknown>('/admin/docs/ai-generate', { method: 'POST', body }),
	approvePage: (id: string) => api<unknown>(`/admin/docs/pages/${id}/approve`, { method: 'POST', body: {} }),
	publishPage: (id: string) => api<unknown>(`/admin/docs/pages/${id}/publish`, { method: 'POST', body: {} }),
};

export const gdprAdminApiExtended = {
	listDeletionRequests: (params?: string) =>
		api<unknown>(`/admin/gdpr/deletion-requests${params ? `?${params}` : ''}`),
	listExports: (params?: string) => api<unknown>(`/admin/gdpr/exports${params ? `?${params}` : ''}`),
	executeDeletion: (id: string) =>
		api<unknown>(`/admin/gdpr/deletion-requests/${id}/execute`, { method: 'POST', body: {} }),
};

export const i18nAdminApiExtended = {
	listLanguages: () => api<unknown>('/admin/i18n/languages'),
	listStrings: (params?: string) => api<unknown>(`/admin/i18n/strings${params ? `?${params}` : ''}`),
	createString: (body: Record<string, unknown>) => api<unknown>('/admin/i18n/strings', { method: 'POST', body }),
	batchCreate: (body: Record<string, unknown>) => api<unknown>('/admin/i18n/strings/batch', { method: 'POST', body }),
	aiTranslateBatch: (body: Record<string, unknown>) =>
		api<unknown>('/admin/i18n/strings/ai-translate-batch', { method: 'POST', body }),
	translateString: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/i18n/strings/${id}/translate`, { method: 'POST', body }),
};

export const learnAdminApiExtended = {
	listTracks: (params?: string) => api<unknown>(`/admin/learn/tracks${params ? `?${params}` : ''}`),
	getTrackEnrollments: (id: string) => api<unknown>(`/admin/learn/tracks/${id}/enrollments`),
	createTrack: (body: Record<string, unknown>) => api<unknown>('/admin/learn/tracks', { method: 'POST', body }),
	updateTrack: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/learn/tracks/${id}`, { method: 'PATCH', body }),
	publishTrack: (id: string) =>
		api<unknown>(`/admin/learn/tracks/${id}/publish`, { method: 'POST', body: {} }),
	deleteTrack: (id: string) => api<{ deleted: boolean }>(`/admin/learn/tracks/${id}`, { method: 'DELETE' }),
	addModule: (trackId: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/learn/tracks/${trackId}/modules`, { method: 'POST', body }),
	updateModule: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/learn/modules/${id}`, { method: 'PATCH', body }),
	deleteModule: (id: string) => api<{ deleted: boolean }>(`/admin/learn/modules/${id}`, { method: 'DELETE' }),
	addLesson: (moduleId: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/learn/modules/${moduleId}/lessons`, { method: 'POST', body }),
	updateLesson: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/learn/lessons/${id}`, { method: 'PATCH', body }),
	deleteLesson: (id: string) => api<{ deleted: boolean }>(`/admin/learn/lessons/${id}`, { method: 'DELETE' }),
};

export const mentorshipAdminApiExtended = {
	listPrograms: (params?: string) =>
		api<unknown>(`/admin/mentorship/programs${params ? `?${params}` : ''}`),
	getProgram: (id: string) => api<unknown>(`/admin/mentorship/programs/${id}`),
	listMatches: (params?: string) =>
		api<unknown>(`/admin/mentorship/matches${params ? `?${params}` : ''}`),
	createProgram: (body: Record<string, unknown>) =>
		api<unknown>('/admin/mentorship/programs', { method: 'POST', body }),
	updateProgram: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/mentorship/programs/${id}`, { method: 'PATCH', body }),
	deleteProgram: (id: string) =>
		api<unknown>(`/admin/mentorship/programs/${id}`, { method: 'DELETE' }),
	updateMatch: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/mentorship/matches/${id}`, { method: 'PATCH', body }),
	// §102 Mentorship Matching (ADR-110) — connections management.
	listConnections: (params?: string) =>
		api<unknown>(`/admin/mentorship/connections${params ? `?${params}` : ''}`),
	updateConnection: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/mentorship/connections/${id}`, { method: 'PATCH', body }),
};

export const pageBuilderAdminApiExtended = {
	getPageTypes: () => api<{ value: string; label: string }[]>('/admin/pagebuilder/page-types'),
	listBlocks: (params?: string) => api<unknown>(`/admin/pagebuilder/blocks${params ? `?${params}` : ''}`),
	getBlock: (id: string) => api<unknown>(`/admin/pagebuilder/blocks/${id}`),
	listThemes: (params?: string) => api<unknown>(`/admin/pagebuilder/themes${params ? `?${params}` : ''}`),
	getTheme: (id: string) => api<unknown>(`/admin/pagebuilder/themes/${id}`),
	createBlock: (body: Record<string, unknown>) =>
		api<unknown>('/admin/pagebuilder/blocks', { method: 'POST', body }),
	updateBlock: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/pagebuilder/blocks/${id}`, { method: 'PATCH', body }),
	deleteBlock: (id: string) => api<{ deleted: boolean }>(`/admin/pagebuilder/blocks/${id}`, { method: 'DELETE' }),
	createTheme: (body: Record<string, unknown>) =>
		api<unknown>('/admin/pagebuilder/themes', { method: 'POST', body }),
	updateTheme: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/pagebuilder/themes/${id}`, { method: 'PATCH', body }),
	setDefaultTheme: (id: string) =>
		api<unknown>(`/admin/pagebuilder/themes/${id}/default`, { method: 'POST', body: {} }),
	deleteTheme: (id: string) => api<{ deleted: boolean }>(`/admin/pagebuilder/themes/${id}`, { method: 'DELETE' }),
};

// Sidebar Management System (Section 59, ADR-066).
// Replaces the old pageBuilderAdminApiExtended sidebar methods.
// Sidebar widgets use a dedicated widget type registry (search, theme_toggle,
// trending_tags, etc.) — NOT pagebuilder block types. Ref: Section 59, ADR-066.
export interface SidebarOptions {
	apps: { value: string; label: string }[];
	positions: { value: string; label: string }[];
	visibilities: { value: string; label: string }[];
	statuses: { value: string; label: string }[];
}

export const sidebarsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/sidebars${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/sidebars/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/sidebars', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/sidebars/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/sidebars/${id}`, { method: 'DELETE' }),
	addWidget: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/sidebars/${id}/widgets`, { method: 'POST', body }),
	updateWidget: (id: string, widgetId: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/sidebars/${id}/widgets/${widgetId}`, { method: 'PATCH', body }),
	deleteWidget: (id: string, widgetId: string) =>
		api<{ deleted: boolean }>(`/admin/sidebars/${id}/widgets/${widgetId}`, { method: 'DELETE' }),
	reorderWidgets: (id: string, body: { items: { id: string; sort_order: number }[] }) =>
		api<unknown>(`/admin/sidebars/${id}/widgets/reorder`, { method: 'POST', body }),
	// Rule 16: sidebar app/position/visibility/status options loaded from the backend.
	getOptions: () => api<SidebarOptions>('/admin/sidebars/options'),
	// Public: widget type registry (no auth required)
	widgetTypes: () => api<{ widget_types: unknown[] }>('/sidebars/widget-types'),
};

export const pageTemplatesAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/page-templates${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/page-templates/${id}`),
	// Rule 16: page_type category catalog is shared with the page builder.
	getPageTypes: () => api<{ value: string; label: string }[]>('/admin/pagebuilder/page-types'),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/page-templates', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/page-templates/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/page-templates/${id}`, { method: 'DELETE' }),
};

export const referralAdminApiExtended = {
	getProgram: (params?: string) =>
		api<unknown>(`/admin/referral/program${params ? `?${params}` : ''}`),
	listReferrals: (params?: string) =>
		api<unknown>(`/admin/referral/referrals${params ? `?${params}` : ''}`),
	getReferral: (id: string) => api<unknown>(`/admin/referral/referrals/${id}`),
	updateProgram: (body: Record<string, unknown>) =>
		api<unknown>('/admin/referral/program', { method: 'PUT', body }),
};

export const reportsAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/reports${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/reports/${id}`),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/reports/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api<unknown>(`/admin/reports/${id}`, { method: 'DELETE' }),
};

export const reviewsAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/reviews${params ? `?${params}` : ''}`),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/reviews/${id}`, { method: 'PATCH', body }),
};

export const rolesAdminApiExtended = {
	permissions: (id: string) => api<unknown>(`/admin/roles/${id}/permissions`),
	updatePermissions: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/roles/${id}/permissions`, { method: 'PATCH', body }),
	addPermission: (id: string, body: { permission: string }) =>
		api<unknown>(`/admin/roles/${id}/permissions`, { method: 'POST', body }),
};

export const searchAdminApiExtended = {
	getAnalytics: (params?: string) =>
		api<unknown>(`/admin/search/analytics${params ? `?${params}` : ''}`),
};

export interface WidgetConfigField {
	key: string;
	label: string;
	type: 'text' | 'number' | 'checkbox' | 'select';
	options?: CatalogOption[];
	default?: unknown;
	help?: string;
}

export interface CardTypeOption {
	value: string;
	label: string;
	icon: string;
	desc: string;
}

export interface WidgetConfigOptions {
	widget_types: CatalogOption[];
	config_schemas: Record<string, WidgetConfigField[]>;
	card_types: CardTypeOption[];
	content_widget_types: string[];
}

export const widgetsAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/widgets${params ? `?${params}` : ''}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/widgets', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/widgets/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/widgets/${id}`, { method: 'DELETE' }),
	// Rule 16: widget type list and per-type config schemas from the backend.
	getConfigOptions: () => api<WidgetConfigOptions>('/admin/pagebuilder/widget-config-options'),
};

export const pageBuilderSaveAsTemplateApi = {
	saveAsTemplate: (pageId: string, body: { name: string }) =>
		api<unknown>(`/admin/pages/${pageId}/save-as-template`, { method: 'POST', body }),
};

export interface SeoValidationResult {
	id: string;
	type: string;
	title: string;
	slug: string;
	meta_title: string;
	meta_description: string;
	og_title: string;
	og_description: string;
	og_image_url: string;
	canonical_url: string;
	json_ld_type: string;
	robots_meta: string;
	issues: string[];
	score: number;
}

export const seoAdminApi = {
	validate: (id: string) =>
		api<SeoValidationResult>(`/admin/seo/validate/${id}`),
};

// Content search row returned by GET /content?q= (authed admin search across
// title, slug, and body; includes drafts). Used by the SEO validator's
// search-and-paste-URL input to resolve a content item to validate.
export interface ContentSearchItem {
	id: string;
	title: string;
	slug: string;
	content_type: string;
	status: string;
	visibility: string;
	updated_at: string;
}

export const contentSearchApi = {
	search: (q: string, pageSize = 10) =>
		api<ContentSearchItem[]>(`/content?q=${encodeURIComponent(q)}&page_size=${pageSize}`),
};

export const fieldPermissionsApi = {
	set: (fieldId: string, body: { user_group_id: string; can_view: boolean; can_edit: boolean }) =>
		api<unknown>(`/custom-fields/${fieldId}/permissions`, { method: 'PUT', body }),
};

export const affiliateLinksAdminApi = {
	list: (params?: string) =>
		api<unknown>(`/admin/affiliate-links${params ? `?${params}` : ''}`),
};

export const affiliateProgramAdminApi = {
	stats: () =>
		api<unknown>('/admin/affiliate-program'),
	members: (params?: string) =>
		api<unknown>(`/admin/affiliate-program/members${params ? `?${params}` : ''}`),
	updateMemberStatus: (userId: string, body: { status: string; notes?: string }) =>
		api<unknown>(`/admin/affiliate-program/members/${userId}`, { method: 'PATCH', body }),
	commissionRates: () =>
		api<unknown>('/admin/affiliate-program/commission-rates'),
	updateCommissionRates: (body: { pro_monthly?: number; pro_annual?: number; course?: number }) =>
		api<unknown>('/admin/affiliate-program/commission-rates', { method: 'PATCH', body }),
};

export const affiliateEnhancementsAdminApi = {
	scanResults: (params?: string) =>
		api<unknown>(`/admin/affiliate/scan-results${params ? `?${params}` : ''}`),
	reviewScanResult: (id: string, body: { action: string; notes?: string }) =>
		api<unknown>(`/admin/affiliate/scan-results/${id}`, { method: 'PATCH', body }),
	commissions: (params?: string) =>
		api<unknown>(`/admin/affiliate/commissions${params ? `?${params}` : ''}`),
	recordCommission: (body: { provider_id: string; content_id: string; external_commission: number; event_description?: string }) =>
		api<unknown>('/admin/affiliate/commissions', { method: 'POST', body }),
	updateAuthorCommission: (providerId: string, body: { author_commission_pct: number }) =>
		api<unknown>(`/admin/affiliate/providers/${providerId}/author-commission`, { method: 'PATCH', body }),
};

export const affiliateProvidersAdminApi = {
	list: (params?: string) =>
		api<unknown>(`/admin/affiliate-providers${params ? `?${params}` : ''}`),
	get: (id: string) =>
		api<unknown>(`/admin/affiliate-providers/${id}`),
	create: (body: Record<string, unknown>) =>
		api<unknown>('/admin/affiliate-providers', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/affiliate-providers/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api<{ deleted: boolean }>(`/admin/affiliate-providers/${id}`, { method: 'DELETE' }),
};

export const auditLogsAdminApi = {
	list: (params?: string) =>
		api<unknown>(`/admin/audit-logs${params ? `?${params}` : ''}`),
};

export const menusAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/menus${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/menus/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/menus', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/menus/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/menus/${id}`, { method: 'DELETE' }),
	addItem: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/menus/${id}/items`, { method: 'POST', body }),
	updateItem: (id: string, itemId: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/menus/${id}/items/${itemId}`, { method: 'PATCH', body }),
	deleteItem: (id: string, itemId: string) =>
		api<{ deleted: boolean }>(`/admin/menus/${id}/items/${itemId}`, { method: 'DELETE' }),
	reorderItems: (id: string, body: { items: { id: string; sort_order: number }[] }) =>
		api<unknown>(`/admin/menus/${id}/items/reorder`, { method: 'POST', body }),
};

export const emailTemplatesAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/email-templates${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/email-templates/${id}`),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/email-templates/${id}`, { method: 'PATCH', body }),
	reset: (id: string) => api<unknown>(`/admin/email-templates/${id}/reset`, { method: 'POST', body: {} }),
	test: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/email-templates/${id}/test`, { method: 'POST', body }),
};

export const redirectsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/redirects${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/redirects/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/redirects', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/redirects/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/redirects/${id}`, { method: 'DELETE' }),
	bulkDelete: (body: { action: "activate" | "deactivate" | "delete"; ids: string[] }) =>
		api<unknown>('/admin/redirects/bulk', { method: 'POST', body }),
};

export const gdprAdminApiExtendedV2 = {
	listConsents: (params?: string) => api<unknown>(`/admin/consents${params ? `?${params}` : ''}`),
	deleteConsent: (id: string) => api<unknown>(`/admin/consents/${id}`, { method: 'DELETE' }),
};

export interface AiModel {
	id: string;
	label: string;
}

export const aiModelsAdminApi = {
	list: (params?: string) => api<AiModel[]>(`/admin/ai/models${params ? `?${params}` : ''}`),
};

export const spacesModeratorsAdminApi = {
	add: (id: string, body: { user_id?: string; username?: string }) =>
		api<unknown>(`/admin/spaces/${id}/moderators`, { method: 'POST', body }),
	remove: (id: string, moderatorId: string) =>
		api<{ removed: boolean }>(`/admin/spaces/${id}/moderators/${moderatorId}`, { method: 'DELETE' }),
};

export const tagsModeratorsAdminApi = {
	add: (id: string, body: { user_id?: string; username?: string }) =>
		api<unknown>(`/admin/tags/${id}/moderators`, { method: 'POST', body }),
	remove: (id: string, userId: string) =>
		api<{ removed: boolean }>(`/admin/tags/${id}/moderators/${userId}`, { method: 'DELETE' }),
};

export const profilesAdminApiV2 = {
	unverify: (id: string) =>
		api<unknown>(`/admin/profiles/${id}/verify`, { method: 'DELETE' }),
};

export const usersWarningsAdminApi = {
	list: (id: string) => api<unknown>(`/admin/users/${id}/warnings`),
};

export interface PermissionPreset {
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

export const permissionPresetsApi = {
	list: () => api<PermissionPreset[]>('/space-permission-presets'),
	update: (name: string, body: { visibility: string; posting_perm: string; replying_perm: string; commenting_perm: string; joining_perm: string }) =>
		api<PermissionPreset>(`/space-permission-presets/${encodeURIComponent(name)}`, { method: 'PUT', body }),
};

export const sharePlatformsAdminApi = {
	list: (params?: string) => api<unknown>(`/share-platforms${params ? `?${params}` : ''}`),
	create: (body: { key: string; label: string; icon: string; url_template: string; color: string; enabled: boolean; position: number }) =>
		api('/share-platforms', { method: 'POST', body }),
	update: (key: string, body: { label: string; icon: string; url_template: string; color: string; enabled: boolean; position: number }) =>
		api(`/share-platforms/${key}`, { method: 'PATCH', body }),
	delete: (key: string) =>
		api(`/share-platforms/${key}`, { method: 'DELETE' }),
};

// ── V2 Community Features (§§79–89, §98) — admin HITL review queues ──────
// Ref: ADR-088 (universal AI content generation + mandatory HITL review).

export const storyAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/stories${params ? `?${params}` : ''}`),
	verify: (id: string) => api<unknown>(`/admin/stories/${id}/verify`, { method: 'POST', body: {} }),
	unverify: (id: string) => api<unknown>(`/admin/stories/${id}/unverify`, { method: 'POST', body: {} }),
	delete: (id: string) => api<unknown>(`/admin/stories/${id}`, { method: 'DELETE' }),
};

export const wikiAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/wiki${params ? `?${params}` : ''}`),
	approveRevision: (pageId: string, revId: string) =>
		api<unknown>(`/admin/wiki/${pageId}/revisions/${revId}/approve`, { method: 'POST', body: {} }),
	rejectRevision: (pageId: string, revId: string, body?: { reason?: string }) =>
		api<unknown>(`/admin/wiki/${pageId}/revisions/${revId}/reject`, { method: 'POST', body: body ?? {} }),
	update: (pageId: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/wiki/${pageId}`, { method: 'PATCH', body }),
	generate: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/wiki/generate`, { method: 'POST', body }),
};

export const scamAlertAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/scam-alerts${params ? `?${params}` : ''}`),
	listRaw: (params?: string) =>
		apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(
			`/admin/scam-alerts${params ? `?${params}` : ''}`
		),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/scam-alerts/${id}`, { method: 'PATCH', body }),
	pin: (id: string) => api<unknown>(`/admin/scam-alerts/${id}/pin`, { method: 'POST', body: {} }),
	unpin: (id: string) => api<unknown>(`/admin/scam-alerts/${id}/pin`, { method: 'DELETE' }),
};

export const planningGroupAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/planning-groups${params ? `?${params}` : ''}`),
	extend: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/planning-groups/${id}/extend`, { method: 'POST', body }),
};

export const mutualAidAdminApi = {
	// BUG-007: use apiRaw so the full response body (including `meta` with
	// total/total_pages) is available for pagination. The backend emits
	// { data: [...], meta: { page, per_page, total, total_pages } } per the
	// geo-aware tool pattern admin list envelope.
	list: (params?: string) =>
		apiRaw<{ data: unknown[]; meta: { page?: number; per_page?: number; total?: number; total_pages?: number } }>(`/admin/mutual-aid${params ? `?${params}` : ''}`),
	updateStatus: (id: string, body: { status: string }) =>
		api<unknown>(`/admin/mutual-aid/${id}/status`, { method: 'PATCH', body }),
};

export const calendarAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/calendar${params ? `?${params}` : ''}`),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/calendar/${id}`, { method: 'PATCH', body }),
	// BUG-008: soft-delete (calendar.manage, admin+).
	delete: (id: string) => api<unknown>(`/admin/calendar/${id}`, { method: 'DELETE' }),
};

export const achievementAdminApi = {
	list: (params?: string) => apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(`/admin/achievements${params ? `?${params}` : ''}`),
	setStatus: (id: string, status: string) =>
		api<unknown>(`/admin/achievements/${id}/status`, { method: 'PATCH', body: { status } }),
	delete: (id: string) => api<unknown>(`/admin/achievements/${id}`, { method: 'DELETE' }),
};

export const serviceDirectoryAdminApi = {
	list: (params?: string) =>
		apiRaw<{ data: unknown[]; meta: { page: number; limit: number; total: number } }>(
			`/admin/services${params ? `?${params}` : ''}`
		),
	verify: (id: string) => api<unknown>(`/admin/services/${id}/verify`, { method: 'PATCH', body: {} }),
	unverify: (id: string) => api<unknown>(`/admin/services/${id}/unverify`, { method: 'PATCH', body: {} }),
	resolveClaim: (serviceId: string, claimId: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/services/${serviceId}/claim/${claimId}`, { method: 'PATCH', body }),
};

// ── V2 Functional Tools (§§90–97, §§99–101) — admin HITL review queues ───

export const appointmentAlertAdminApi = {
	listEmbassies: (params?: string) =>
		apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(
			`/admin/embassies${params ? `?${params}` : ''}`
		),
	createEmbassy: (body: Record<string, unknown>) => api<unknown>(`/admin/embassies`, { method: 'POST', body }),
	updateEmbassy: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/embassies/${id}`, { method: 'PATCH', body }),
	pollEmbassy: (id: string) => api<unknown>(`/admin/embassies/${id}/poll`, { method: 'POST', body: {} }),
};

// ── Original tools (non-v2) — admin monitoring/management pages ──────────

export const visaChecklistAdminApi = {
	listProgress: (params?: string) => api<unknown>(`/admin/checklist/progress${params ? `?${params}` : ''}`),
	completionStats: () => api<unknown>(`/admin/checklist/completion-stats`),
};

export const readinessAdminApi = {
	getForm: () => api<unknown>(`/admin/assessment/form`),
	updateForm: (body: Record<string, unknown>) => api<unknown>(`/admin/assessment/form`, { method: 'PATCH', body }),
	listResults: (params?: string) => apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(`/admin/assessment/results${params ? `?${params}` : ''}`),
	stats: () => api<unknown>(`/admin/assessment/stats`),
};

export interface ColAdminStatsItem {
	city_id: string;
	city_name: string;
	is_baseline: boolean;
	last_updated: string;
	is_stale: boolean;
	total_dimensions: number;
	filled_dimensions: number;
	completeness_pct: number;
}

export interface ColAdminStatsResponse {
	cities: ColAdminStatsItem[];
	total: number;
	page: number;
	limit: number;
}

export const costOfLivingAdminApi = {
	stats: (params?: string) =>
		api<ColAdminStatsResponse>(`/admin/calculator/cost-of-living/stats${params ? `?${params}` : ''}`),
};

export const scholarshipAdminApi = {
	stats: () => api<unknown>(`/admin/scholarships/stats`),
	deadlines: (params?: string) => api<unknown>(`/admin/scholarships/deadlines${params ? `?${params}` : ''}`),
	reminderLog: (params?: string) => api<unknown>(`/admin/scholarships/reminder-log${params ? `?${params}` : ''}`),
	list: (params?: string) => apiRaw<{ data: unknown[]; meta: { count: number; page_size: number; page_offset: number } }>(`/admin/scholarships${params ? `?${params}` : ''}`),
	create: (body: Record<string, unknown>) => api<unknown>(`/admin/scholarships`, { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) => api<unknown>(`/admin/scholarships/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<unknown>(`/admin/scholarships/${id}`, { method: 'DELETE' }),
};

export const jobsAdminApi = {
	list: (params?: string) => apiRaw<{ data: unknown[]; meta: { page: number; limit: number; total: number }; status_distribution: Record<string, number> }>(`/admin/jobs${params ? `?${params}` : ''}`),
	update: (id: string, body: Record<string, unknown>) => api<unknown>(`/admin/jobs/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<unknown>(`/admin/jobs/${id}`, { method: 'DELETE' }),
	toggleFeature: (id: string) => api<unknown>(`/admin/jobs/${id}/feature`, { method: 'PATCH', body: {} }),
};

export const intelReviewApi = {
	queue: (params?: string) => apiRaw<{ data: unknown[] }>(`/intel/review/queue${params ? `?${params}` : ''}`),
	approve: (id: string) => api<unknown>(`/intel/review/${id}/approve`, { method: 'POST', body: {} }),
	reject: (id: string) => api<unknown>(`/intel/review/${id}/reject`, { method: 'POST', body: {} }),
	edit: (id: string, body: Record<string, unknown>) => api<unknown>(`/intel/review/${id}/edit`, { method: 'PATCH', body }),
	rawDocument: (id: string) => api<unknown>(`/intel/raw-documents/${id}`),
};

export const processingTrackerAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/visa-timelines${params ? `?${params}` : ''}`),
	delete: (id: string) => api<unknown>(`/admin/visa-timelines/${id}`, { method: 'DELETE' }),
};

export const cvFormatterAdminApi = {
	listStandards: (params?: string) => api<unknown>(`/admin/cv-formatter/standards${params ? `?${params}` : ''}`),
	listStandardsRaw: (params?: string) =>
		apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number; total_pages: number } }>(
			`/admin/cv-formatter/standards${params ? `?${params}` : ''}`
		),
	upsertStandard: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/cv-formatter/standards`, { method: 'POST', body }),
	listDestinations: () => api<GeoCountry[]>('/geo/countries'),
};

export const visaRoutesAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/visa-routes${params ? `?${params}` : ''}`),
	listRaw: (params?: string) =>
		apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(
			`/admin/visa-routes${params ? `?${params}` : ''}`
		),
	create: (body: Record<string, unknown>) => api<unknown>(`/admin/visa-routes`, { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/visa-routes/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<unknown>(`/admin/visa-routes/${id}`, { method: 'DELETE' }),
	approve: (id: string) => api<unknown>(`/admin/visa-routes/${id}/approve`, { method: 'PATCH', body: {} }),
	generate: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/visa-routes/generate`, { method: 'POST', body }),
};

export const arrivalChecklistAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/arrival-checklists${params ? `?${params}` : ''}`),
	listRaw: (params?: string) =>
		apiRaw<{ data: unknown[]; meta: { page: number; per_page: number; total: number; total_pages: number } }>(
			`/admin/arrival-checklists${params ? `?${params}` : ''}`
		),
	create: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/arrival-checklists`, { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/arrival-checklists/${id}`, { method: 'PATCH', body }),
	approve: (id: string) =>
		api<unknown>(`/admin/arrival-checklists/${id}/approve`, { method: 'PATCH', body: {} }),
	generate: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/arrival-checklists/generate`, { method: 'POST', body }),
};

export const credentialCheckerAdminApi = {
	list: (params?: string) =>
		apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(
			`/admin/credentials${params ? `?${params}` : ''}`
		),
	create: (body: Record<string, unknown>) => api<unknown>(`/admin/credentials`, { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/credentials/${id}`, { method: 'PATCH', body }),
	approve: (id: string) =>
		api<unknown>(`/admin/credentials/${id}/approve`, { method: 'PATCH', body: {} }),
	generate: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/credentials/generate`, { method: 'POST', body }),
};

export const housingGuideAdminApi = {
	list: (params?: string) =>
		apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(
			`/admin/housing-guides${params ? `?${params}` : ''}`
		),
	create: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/housing-guides`, { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/housing-guides/${id}`, { method: 'PATCH', body }),
	approve: (id: string) =>
		api<unknown>(`/admin/housing-guides/${id}/approve`, { method: 'PATCH', body: {} }),
	reject: (id: string, body: { reason?: string } = {}) =>
		api<unknown>(`/admin/housing-guides/${id}/reject`, { method: 'PATCH', body }),
	generate: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/housing-guides/generate`, { method: 'POST', body }),
};

export const attestationGuideAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/attestation${params ? `?${params}` : ''}`),
	listUpdates: (params?: string) =>
		api<unknown>(`/admin/attestation/updates${params ? `?${params}` : ''}`),
	create: (body: Record<string, unknown>) => api<unknown>(`/admin/attestation`, { method: 'POST', body }),
	approve: (id: string) =>
		api<unknown>(`/admin/attestation/${id}/approve`, { method: 'PATCH', body: {} }),
	generate: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/attestation/generate`, { method: 'POST', body }),
	reviewUpdate: (id: string, approve: boolean) =>
		api<unknown>(`/admin/attestation/updates/${id}`, { method: 'PATCH', body: { approve } }),
};

export const languageTestHubAdminApi = {
	list: (params?: string) =>
		apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(
			`/admin/language-tests/requirements${params ? `?${params}` : ''}`
		),
	create: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/language-tests/requirements`, { method: 'POST', body }),
	approve: (id: string) =>
		api<unknown>(`/admin/language-tests/requirements/${id}/approve`, { method: 'PATCH', body: {} }),
	generate: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/language-tests/requirements/generate`, { method: 'POST', body }),
	// Geo-aware selector sources (ADR-088 geo-aware-tool-pattern).
	listDestinations: () => api<GeoCountry[]>('/geo/countries'),
	listTests: () => api<unknown>(`/language-tests`),
};

export const taxCalculatorAdminApi = {
	list: (params?: string) =>
		apiRaw<{ data: { guides: unknown[]; treaties: unknown[] }; meta: { page: number; per_page: number; total: number; total_pages: number } }>(
			`/admin/tax-calculator${params ? `?${params}` : ''}`
		),
	createGuide: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/tax-calculator/guides`, { method: 'POST', body }),
	approveGuide: (id: string) =>
		api<unknown>(`/admin/tax-calculator/guides/${id}/approve`, { method: 'PATCH', body: {} }),
	generateGuide: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/tax-calculator/guides/generate`, { method: 'POST', body }),
};

export const communityLocatorAdminApi = {
	list: (params?: string) =>
		apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(
			`/admin/community-locator${params ? `?${params}` : ''}`
		),
	approve: (id: string) =>
		api<unknown>(`/admin/community-locator/${id}/approve`, { method: 'PATCH', body: {} }),
	verify: (id: string) =>
		api<unknown>(`/admin/community-locator/${id}/verify`, { method: 'PATCH', body: {} }),
	resolveClaim: (id: string, claimId: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/community-locator/${id}/claim/${claimId}`, { method: 'PATCH', body }),
	generate: (body: Record<string, unknown>) =>
		api<unknown>(`/admin/community-locator/generate`, { method: 'POST', body }),
	archive: (id: string) =>
		api<unknown>(`/admin/community-locator/${id}/archive`, { method: 'PATCH', body: {} }),
	delete: (id: string) =>
		api<unknown>(`/admin/community-locator/${id}`, { method: 'DELETE' }),
};

export const digestAdminApi = {
	list: (params?: string) => apiRaw<{ data: unknown[]; pagination: { page: number; limit: number; total: number } }>(`/admin/digest${params ? `?${params}` : ''}`),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/digest/${id}`, { method: 'PATCH', body }),
	publish: (id: string) => api<unknown>(`/admin/digest/${id}/publish`, { method: 'POST', body: {} }),
	generate: () =>
		api<unknown>(`/admin/digest/generate`, { method: 'POST', body: {} }),
};

// Section 67 — Travel Search providers (ADR-085). No DELETE route in the spec;
// deactivation is via PATCH is_active. api_config is write-only (never returned).
export interface TravelProvider {
	id: string;
	name: string;
	slug: string;
	provider_type: 'flight' | 'hotel' | 'both';
	affiliate_config?: Record<string, string> | null;
	is_active: boolean;
	created_at: string;
}

export const travelAdminApi = {
	list: (params?: string) => api<TravelProvider[]>(`/admin/travel/providers${params ? `?${params}` : ''}`),
	create: (body: {
		name: string;
		slug: string;
		provider_type: string;
		api_config?: unknown;
		affiliate_config?: unknown;
		is_active: boolean;
	}) => api<TravelProvider>('/admin/travel/providers', { method: 'POST', body }),
	update: (id: string, body: {
		name?: string;
		provider_type?: string;
		api_config?: unknown;
		affiliate_config?: unknown;
		is_active?: boolean;
	}) => api<TravelProvider>(`/admin/travel/providers/${id}`, { method: 'PATCH', body }),
};

export interface CatalogOption {
	value: string;
	label: string;
}

// Section 104 — AI Content Campaigns admin API.
export interface Campaign {
	id: string;
	tenant_id: string;
	name: string;
	prompt: string;
	status: string;
	content_type?: string;
	requested_count: number;
	persona_pool?: string[];
	auto_provision_personas: boolean;
	auto_assign: boolean;
	target_space_id?: string;
	target_tags?: string[];
	provisioned_personas: number;
	created_by: string;
	created_at: string;
	updated_at: string;
}

export interface CampaignItem {
	id: string;
	campaign_id: string;
	persona_id: string;
	persona_name?: string;
	content_type: string;
	title?: string;
	topic_prompt: string;
	status: string;
	studio_job_id?: string;
	content_id?: string;
	review_id?: string;
	target_space_id?: string;
	target_tags?: string[];
	created_at: string;
	updated_at: string;
}

export interface CampaignCreateInput {
	prompt: string;
	name?: string;
	content_type?: string;
	count: number;
	persona_ids?: string[];
	auto_provision_personas?: boolean;
	auto_assign?: boolean;
	target_space_id?: string;
	target_tags?: string[];
	run_immediately?: boolean;
}

export const campaignsAdminApi = {
	list: (params?: string) =>
		apiRaw<{ data: Campaign[]; meta: { total: number; page_size: number; page_offset: number } }>(`/admin/campaigns${params ? `?${params}` : ''}`),
	get: (id: string) =>
		api<{ campaign: Campaign; items: CampaignItem[] }>(`/admin/campaigns/${id}`),
	create: (body: CampaignCreateInput) =>
		api<Campaign>('/admin/campaigns', { method: 'POST', body }),
	run: (id: string) =>
		api<{ batch: unknown; jobs: unknown[] }>(`/admin/campaigns/${id}/run`, { method: 'POST' }),
	retryItem: (id: string) =>
		apiRaw<{ status: string }>(`/admin/campaigns/items/${id}/retry`, { method: 'POST' }),
	cancel: (id: string) =>
		apiRaw<{ status: string }>(`/admin/campaigns/${id}/cancel`, { method: 'POST' }),
};

export const adminCatalogApi = {
	get: (name: string) => api<CatalogOption[]>(`/admin/catalogs/${encodeURIComponent(name)}`),
};
