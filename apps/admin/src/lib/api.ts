import { apiFetch } from './api/client';
import { PUBLIC_JC_API_URL } from '$env/static/public';

type FetchOptions = {
	method?: string;
	body?: unknown;
	signal?: AbortSignal;
};

type ApiResult<T> = { ok: true; data: T } | { ok: false; error: string };

let jwtToken: string | null = null;

export function setToken(token: string | null) {
	jwtToken = token;
}

export async function api<T>(path: string, opts: FetchOptions = {}): Promise<ApiResult<T>> {
	const headers: Record<string, string> = {};
	if (jwtToken) {
		headers['Authorization'] = `Bearer ${jwtToken}`;
	}

	const result = await apiFetch<T>(path, {
		method: opts.method ?? 'GET',
		headers,
		body: opts.body ? JSON.stringify(opts.body) : undefined,
		signal: opts.signal,
	});

	if (result.error) {
		return { ok: false, error: result.error.message };
	}
	return { ok: true, data: result.data as T };
}

export const spamApi = {
	getIpBlocks: (params?: string) => api<unknown>(`/admin/spam/ip-blocks${params ? `?${params}` : ''}`),
	createIpBlock: (body: { ip_cidr: string; reason: string; expires_at?: string }) =>
		api('/admin/spam/ip-blocks', { method: 'POST', body }),
	deleteIpBlock: (id: string) =>
		api(`/admin/spam/ip-blocks/${id}`, { method: 'DELETE' }),

	getCensoredWords: (params?: string) => api<unknown>(`/admin/spam/censored-words${params ? `?${params}` : ''}`),
	createCensoredWord: (body: { pattern: string; action: string; replacement?: string }) =>
		api('/admin/spam/censored-words', { method: 'POST', body }),
	deleteCensoredWord: (id: string) =>
		api(`/admin/spam/censored-words/${id}`, { method: 'DELETE' }),

	getUserBlocks: (params?: string) => api<unknown>(`/admin/spam/user-blocks${params ? `?${params}` : ''}`),
	deleteUserBlock: (blockerId: string, blockedId: string) =>
		api(`/admin/spam/user-blocks/${blockerId}/${blockedId}`, { method: 'DELETE' }),
};

export const ticketTypesApi = {
	list: (params?: string) => api<unknown>(`/admin/ticket-types${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string; description?: string; default_priority: string; is_active: boolean }) =>
		api('/admin/ticket-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; description?: string; default_priority?: string; is_active?: boolean }) =>
		api(`/admin/ticket-types/${id}`, { method: 'PUT', body }),
	delete: (id: string) =>
		api(`/admin/ticket-types/${id}`, { method: 'DELETE' }),
};

export const walletApi = {
	getRules: (params?: string) => api<unknown>(`/admin/wallet/rules${params ? `?${params}` : ''}`),
	createRule: (body: { event_type: string; points_value: number; description?: string; is_active: boolean }) =>
		api('/admin/wallet/rules', { method: 'POST', body }),
	updateRule: (id: string, body: { event_type?: string; points_value?: number; description?: string; is_active?: boolean }) =>
		api(`/admin/wallet/rules/${id}`, { method: 'PATCH', body }),

	getSettings: () => api<unknown>('/admin/wallet/settings'),
	updateSettings: (body: { points_to_credit_ratio: number; daily_earning_cap: number; enable_earning: boolean }) =>
		api('/admin/wallet/settings', { method: 'PUT', body }),

	getUserBalance: (userId: string) => api<unknown>(`/admin/wallet/user/${userId}/balance`),
	adjustBalance: (body: { user_id: string; amount: number; reason: string; type: 'points' | 'credits' }) =>
		api('/admin/wallet/adjust', { method: 'POST', body }),
};

export const spaceTypesApi = {
	list: (params?: string) => api<unknown>(`/space-types${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string; description?: string; is_university: boolean; icon?: string }) =>
		api('/admin/space-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; description?: string; is_university?: boolean; icon?: string }) =>
		api(`/admin/space-types/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/space-types/${id}`, { method: 'DELETE' }),
};

export const impersonationApi = {
	start: (userId: string) =>
		api<{ token: string }>('/admin/impersonate', { method: 'POST', body: { user_id: userId } }),
	end: () =>
		api('/admin/impersonate', { method: 'DELETE' }),
	status: () =>
		api<{ impersonating: boolean; username?: string; user_id?: string }>('/admin/impersonate/status'),
};

export const docsApi = {
	list: (params?: string) => api<unknown>(`/admin/docs/pages${params ? `?${params}` : ''}`),
	create: (body: { title: string; slug: string; audience: string; body: string }) =>
		api('/admin/docs', { method: 'POST', body }),
	update: (id: string, body: { title?: string; slug?: string; audience?: string; body?: string; status?: string }) =>
		api(`/admin/docs/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/docs/${id}`, { method: 'DELETE' }),
};

export const adSlotTypesApi = {
	list: (params?: string) => api<unknown>(`/admin/ads/slot-types${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string; width: number; height: number; description?: string }) =>
		api('/admin/ads/slot-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; width?: number; height?: number; description?: string }) =>
		api(`/admin/ads/slot-types/${id}`, { method: 'PUT', body }),
	delete: (id: string) =>
		api(`/admin/ads/slot-types/${id}`, { method: 'DELETE' }),
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
	is_public: boolean;
	created_at: string;
}

export interface ComparisonTypeDetail extends ComparisonType {
	dimensions: Array<{ id: string; name: string; description: string }>;
	item_count: number;
}

export interface ComparisonTypeAccess {
	allowed_groups: string[];
	allowed_roles: string[];
}

export const comparisonAdminApi = {
	listTypes: () => api<ComparisonType[]>('/comparisons/types'),
	getType: (id: string) => api<ComparisonTypeDetail>(`/comparisons/types/${id}`),
	createType: (body: { name: string; slug: string; description: string; is_public: boolean }) =>
		api<ComparisonType>('/comparisons/types', { method: 'POST', body }),
	updateType: (id: string, body: { name?: string; description?: string; is_public?: boolean }) =>
		api<ComparisonType>(`/comparisons/types/${id}`, { method: 'PATCH', body }),
	deleteType: (id: string) =>
		api<{ deleted: boolean }>(`/comparisons/types/${id}`, { method: 'DELETE' }),
	getTypeAccess: (id: string) =>
		api<ComparisonTypeAccess>(`/comparisons/types/${id}/access`),
	setTypeAccess: (id: string, body: { allowed_groups: string[]; allowed_roles: string[] }) =>
		api<{ updated: boolean }>(`/comparisons/types/${id}/access`, { method: 'PUT', body }),
};

export interface ProfileType {
	id: string;
	name: string;
	slug: string;
	description: string;
	created_at: string;
}

export interface ProfileTypeDetail extends ProfileType {
	profile_count: number;
	fields: Array<{ name: string; field_type: string; required: boolean }>;
}

export interface ProfileMember {
	user_id: string;
	username: string;
	display_name: string;
	role: string;
	joined_at: string;
}

export interface ProfileInvitation {
	id: string;
	email: string;
	role: string;
	status: string;
	created_at: string;
}

export const professionalAdminApi = {
	listTypes: () => api<ProfileType[]>('/profiles/types'),
	getType: (id: string) => api<ProfileTypeDetail>(`/profiles/types/${id}`),
	createType: (body: { name: string; slug: string; description: string }) =>
		api<ProfileType>('/profiles/types', { method: 'POST', body }),
	listMembers: (profileId: string) =>
		api<ProfileMember[]>(`/profiles/${profileId}/members`),
	addMember: (profileId: string, body: { user_id: string; role: string }) =>
		api<ProfileMember>(`/profiles/${profileId}/members`, { method: 'POST', body }),
	removeMember: (profileId: string, userId: string) =>
		api<{ removed: boolean }>(`/profiles/${profileId}/members/${userId}`, { method: 'DELETE' }),
	updateMember: (profileId: string, userId: string, body: { role: string }) =>
		api<ProfileMember>(`/profiles/${profileId}/members/${userId}`, { method: 'PATCH', body }),
	listInvitations: (profileId: string) =>
		api<ProfileInvitation[]>(`/profiles/${profileId}/invitations`),
	sendInvitation: (profileId: string, body: { email: string; role: string }) =>
		api<ProfileInvitation>(`/profiles/${profileId}/invitations`, { method: 'POST', body }),
};

export interface UserDetail {
	id: string;
	username: string;
	email: string;
	groups: string[];
	status: string;
	avatar_url?: string;
	created_at: string;
	last_login_at?: string;
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
	started_at: string;
}

export interface UserGroup {
	id: string;
	name: string;
	description: string;
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
	getUser: (userId: string) => api<UserDetail>(`/users/${userId}`),
	getUserActivity: (userId: string, params?: string) =>
		api<{ activities: UserActivity[]; total: number }>(`/admin/users/${userId}/activity${params ? `?${params}` : ''}`),

	getAiContext: (userId: string) =>
		api<AiContextEntry[]>(`/admin/ai/context/${userId}`),
	deleteAiContextEntry: (userId: string, entryId: string) =>
		api<{ deleted: boolean }>(`/admin/ai/context/${userId}/${entryId}`, { method: 'DELETE' }),

	getUserSessions: (userId: string) =>
		api<UserSession[]>(`/admin/users/${userId}/sessions`),

	issueWarning: (userId: string, body: { reason: string }) =>
		api(`/admin/users/${userId}/warn`, { method: 'POST', body }),
	banUser: (userId: string) =>
		api(`/admin/users/${userId}/ban`, { method: 'POST' }),
	unbanUser: (userId: string) =>
		api(`/admin/users/${userId}/ban`, { method: 'DELETE' }),
	suspendUser: (userId: string, body: { status: string; reason?: string }) =>
		api(`/admin/users/${userId}/status`, { method: 'PATCH', body }),
	changeGroup: (userId: string, body: { group: string }) =>
		api(`/admin/users/${userId}/group`, { method: 'PATCH', body }),
	deleteUser: (userId: string) =>
		api(`/admin/users/${userId}`, { method: 'DELETE' }),

	listGroups: (params?: string) =>
		api<{ groups: UserGroup[]; total: number }>(`/admin/user-groups${params ? `?${params}` : ''}`),
	createGroup: (body: { name: string; description: string; permissions: string[] }) =>
		api<UserGroup>('/admin/user-groups', { method: 'POST', body }),
	updateGroup: (id: string, body: { name?: string; description?: string; permissions?: string[] }) =>
		api<UserGroup>(`/admin/user-groups/${id}`, { method: 'PATCH', body }),
	deleteGroup: (id: string) =>
		api<{ deleted: boolean }>(`/admin/user-groups/${id}`, { method: 'DELETE' }),

	listRoles: (params?: string) =>
		api<{ roles: UserRole[]; total: number }>(`/admin/roles${params ? `?${params}` : ''}`),
	createRole: (body: { name: string; permissions: string[] }) =>
		api<UserRole>('/admin/roles', { method: 'POST', body }),
	updateRole: (id: string, body: { name?: string; permissions?: string[] }) =>
		api<UserRole>(`/admin/roles/${id}`, { method: 'PATCH', body }),
	deleteRole: (id: string) =>
		api<{ deleted: boolean }>(`/admin/roles/${id}`, { method: 'DELETE' }),

	listCustomFieldEntityTypes: () =>
		api<Array<{ value: string; label: string }>>('/custom-field-entity-types'),
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

export interface GenerateResult {
	generated_text: string;
	model: string;
	tokens_used: number;
}

export const aiApi = {
	generate: (data: { prompt: string; category: string; model?: string }) =>
		api<GenerateResult>('/ai/generate', { method: 'POST', body: data }),

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

	listCategories: () =>
		api<Array<{ value: string; label: string; description: string }>>('/ai/categories'),

	listModels: () =>
		api<Array<{ id: string; label: string }>>('/admin/ai/models'),
};

export const jsonLdApi = {
	getTemplates: () => api<Record<string, unknown>>('/content/jsonld-templates'),
	updateTemplates: (templates: Record<string, unknown>) =>
		api<{ updated: boolean }>('/content/jsonld-templates', {
			method: 'PUT',
			body: templates
		}),
};

export interface BlockType {
	name: string;
	description: string;
	category: string;
	is_enabled: boolean;
}

export const pageBuilderApi = {
	getBlockTypes: () => api<BlockType[]>('/page-builder/block-types'),
};

export const spacesAdminApi = {
	list: (params?: string) => api<unknown>(`/spaces${params ? `?${params}` : ''}`),
	suspend: (id: string, body: { status: string; reason?: string }) =>
		api(`/admin/spaces/${id}/status`, { method: 'PATCH', body }),
	assignModerator: (id: string, body: { username: string }) =>
		api(`/admin/spaces/${id}/moderators`, { method: 'POST', body }),
};

export const tagsAdminApi = {
	list: (params?: string) => api<unknown>(`/tags${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string }) =>
		api('/admin/tags', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string }) =>
		api(`/admin/tags/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/tags/${id}`, { method: 'DELETE' }),
};

export const organisationsAdminApi = {
	list: (params?: string) => api<unknown>(`/organizations${params ? `?${params}` : ''}`),
	verify: (id: string, body: { verified: boolean }) =>
		api(`/admin/organizations/${id}/verify`, { method: 'POST', body }),
	claim: (id: string, body: { note?: string }) =>
		api(`/admin/organizations/${id}/claim`, { method: 'POST', body }),
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
	list: (params?: string) => api<unknown>(`/universities${params ? `?${params}` : ''}`),
	create: (body: { name: string; domain: string; verified: boolean }) =>
		api('/admin/universities', { method: 'POST', body }),
	update: (id: string, body: { name?: string; domain?: string; verified?: boolean }) =>
		api(`/admin/universities/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/universities/${id}`, { method: 'DELETE' }),
};
