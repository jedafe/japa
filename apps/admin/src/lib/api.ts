import { apiFetch } from './api/client';
import { PUBLIC_JC_API_URL } from '$env/static/public';

type FetchOptions = {
	method?: string;
	body?: unknown;
	signal?: AbortSignal;
};

type ApiResult<T> = { ok: true; data: T } | { ok: false; error: string };

export function setToken(token: string | null): void {
	// Deprecated no-op: JWT auth state is no longer managed client-side.
}

export async function api<T>(path: string, opts: FetchOptions = {}): Promise<ApiResult<T>> {
	const headers: Record<string, string> = {};

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
	createIpBlock: (body: { cidr: string; block_type: string; reason?: string; expires_at?: string }) =>
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
	createRule: (body: { activity_key: string; point_value: number; is_enabled: boolean; description?: string }) =>
		api('/admin/wallet/rules', { method: 'POST', body }),
	updateRule: (id: string, body: { activity_key?: string; point_value?: number; is_enabled?: boolean; description?: string }) =>
		api(`/admin/wallet/rules/${id}`, { method: 'PATCH', body }),

	getSettings: () => api<unknown>('/admin/wallet/settings'),
	updateSettings: (body: { points_to_credits_rate: number; daily_earning_cap: number; enable_earning: boolean }) =>
		api('/admin/wallet/settings', { method: 'PUT', body }),

	getUserBalance: (userId: string) => api<unknown>(`/admin/wallet/user/${userId}/balance`),
	adjustBalance: (body: { user_id: string; amount: number; reason: string; type: 'points' | 'credits' }) =>
		api('/admin/wallet/adjust', { method: 'POST', body }),
};

export const spaceTypesApi = {
	list: (params?: string) => api<unknown>(`/admin/space-types${params ? `?${params}` : ''}`),
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
	updateType: (id: string, body: { name?: string; slug?: string; description?: string }) =>
		api<ProfileType>(`/profiles/types/${id}`, { method: 'PATCH', body }),
	deleteType: (id: string) =>
		api<{ deleted: boolean }>(`/profiles/types/${id}`, { method: 'DELETE' }),
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
	list: (params?: string) =>
		api<{ users: UserDetail[]; total: number; page: number; total_pages: number }>(`/admin/users${params ? `?${params}` : ''}`),
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
	// Group membership is managed via the RBAC user-group routes; the backend
	// PATCH /admin/users/{id}/group endpoint only ADDS one membership by
	// group_id and cannot remove, so a set-style "change groups" must diff
	// against these two endpoints.
	addGroupMember: (groupId: string, userId: string) =>
		api(`/admin/user-groups/${groupId}/members`, { method: 'POST', body: { user_id: userId } }),
	removeGroupMember: (groupId: string, userId: string) =>
		api(`/admin/user-groups/${groupId}/members/${userId}`, { method: 'DELETE' }),
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
	list: (params?: string) => api<unknown>(`/admin/spaces${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/spaces/${id}`),
	create: (body: { name: string; space_type: string; owner_id?: string }) =>
		api('/admin/spaces', { method: 'POST', body }),
	update: (id: string, body: { name?: string; space_type?: string; status?: string }) =>
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
	create: (body: { name: string; slug: string }) =>
		api('/admin/tags', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string }) =>
		api(`/admin/tags/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/tags/${id}`, { method: 'DELETE' }),
	addModerator: (id: string, body: { user_id: string }) =>
		api(`/admin/tags/${id}/moderators`, { method: 'POST', body }),
	getModerators: (id: string) =>
		api<unknown>(`/admin/tags/${id}/moderators`),
};

export const organisationsAdminApi = {
	list: (params?: string) => api<unknown>(`/organizations${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/organizations/${id}`),
	create: (body: Record<string, unknown>) => api<unknown>('/organizations', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) => api<unknown>(`/organizations/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/organizations/${id}`, { method: 'DELETE' }),
	verify: (id: string, body: { verified: boolean }) =>
		api(`/admin/organizations/${id}/verify`, { method: 'POST', body }),
	unverify: (id: string) =>
		api(`/admin/organizations/${id}/verify`, { method: 'DELETE' }),
	claim: (id: string, body: { note?: string }) =>
		api(`/admin/organizations/${id}/claim`, { method: 'POST', body }),
};

export const reactionsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/reaction-types${params ? `?${params}` : ''}`),
	create: (body: { key: string; emoji: string; label: string; enabled: boolean }) =>
		api('/admin/reaction-types', { method: 'POST', body }),
	update: (key: string, body: { emoji?: string; label?: string; enabled?: boolean }) =>
		api(`/admin/reaction-types/${key}`, { method: 'PATCH', body }),
	delete: (key: string) =>
		api(`/admin/reaction-types/${key}`, { method: 'DELETE' }),
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

// --- Unconsumed route methods (490 route initiative) ---

export const adsAdminApi = {
	listCampaigns: (params?: string) => api<unknown>(`/admin/ads/campaigns${params ? `?${params}` : ''}`),
	getCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}`),
	getCampaignStats: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/stats`),
	listActiveBoosts: (params?: string) => api<unknown>(`/admin/ads/boosts/active${params ? `?${params}` : ''}`),
	createCampaign: (body: Record<string, unknown>) => api<unknown>('/admin/ads/campaigns', { method: 'POST', body }),
	updateCampaign: (id: string, body: Record<string, unknown>) => api<unknown>(`/admin/ads/campaigns/${id}`, { method: 'PATCH', body }),
	activateCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/activate`, { method: 'POST', body: {} }),
	endCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/end`, { method: 'POST', body: {} }),
	pauseCampaign: (id: string) => api<unknown>(`/admin/ads/campaigns/${id}/pause`, { method: 'POST', body: {} }),
	endBoost: (id: string) => api<unknown>(`/admin/ads/boosts/${id}/end`, { method: 'POST', body: {} }),
};

export const aiAdminApi = {
	listTools: (params?: string) => api<unknown>(`/admin/ai/tools${params ? `?${params}` : ''}`),
	getTool: (id: string) => api<unknown>(`/admin/ai/tools/${id}`),
	getToolsUsage: () => api<unknown>('/admin/ai/tools/usage'),
	getAiUsage: () => api<unknown>('/admin/ai/usage'),
	updateTool: (id: string, body: Record<string, unknown>) => api<unknown>(`/admin/ai/tools/${id}`, { method: 'PUT', body }),
	getUserAiContext: (userId: string) => api<unknown>(`/admin/ai/context/${userId}`),
	deleteAiContextEntry: (userId: string, entryId: string) =>
		api<{ deleted: boolean }>(`/admin/ai/context/${userId}/${entryId}`, { method: 'DELETE' }),
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
};

export const blogAdminApi = {
	getReviewQueue: (params?: string) => api<unknown>(`/admin/blog/review-queue${params ? `?${params}` : ''}`),
	approvePost: (id: string) => api<unknown>(`/admin/blog/review-queue/${id}/approve`, { method: 'POST', body: {} }),
	rejectPost: (id: string, body: { reason?: string }) =>
		api<unknown>(`/admin/blog/review-queue/${id}/reject`, { method: 'POST', body }),
};

export const contentAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/content${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/content/${id}`),
	remove: (id: string, body?: { reason?: string }) => api<unknown>(`/admin/content/${id}/remove`, { method: 'POST', body: body ?? {} }),
	restore: (id: string) => api<unknown>(`/admin/content/${id}/restore`, { method: 'POST', body: {} }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/content/${id}`, { method: 'PATCH', body }),
};

export const mediaAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/media${params ? `?${params}` : ''}`),
	listQuarantined: (params?: string) => api<unknown>(`/admin/media/quarantined${params ? `?${params}` : ''}`),
	approve: (id: string) => api<unknown>(`/admin/media/${id}/approve`, { method: 'POST', body: {} }),
	reject: (id: string) => api<unknown>(`/admin/media/${id}/reject`, { method: 'POST', body: {} }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/media/${id}`, { method: 'DELETE' }),
};

export const moderationAdminApi = {
	getQueue: (params?: string) => api<unknown>(`/admin/moderation-queue${params ? `?${params}` : ''}`),
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
	addComment: (id: string, body: { body: string }) =>
		api<unknown>(`/admin/tickets/${id}/comments`, { method: 'POST', body }),
	addNote: (id: string, body: { body: string }) =>
		api<unknown>(`/admin/tickets/${id}/notes`, { method: 'POST', body }),
	resolve: (id: string, body: { resolution: string }) =>
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
	list: (params?: string) => api<unknown>(`/admin/field-groups${params ? `?${params}` : ''}`),
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

export const comparisonsAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/comparison-types${params ? `?${params}` : ''}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/comparison-types', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/comparison-types/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/comparison-types/${id}`, { method: 'DELETE' }),
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
	updateMatch: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/mentorship/matches/${id}`, { method: 'PATCH', body }),
};

export const pageBuilderAdminApiExtended = {
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

export const pageTemplatesAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/page-templates${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/page-templates/${id}`),
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

export const widgetsAdminApiExtended = {
	list: (params?: string) => api<unknown>(`/admin/widgets${params ? `?${params}` : ''}`),
	create: (body: Record<string, unknown>) => api<unknown>('/admin/widgets', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api<unknown>(`/admin/widgets/${id}`, { method: 'PATCH', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/admin/widgets/${id}`, { method: 'DELETE' }),
};

export const walletAdminApiExtended = {
	award: (body: { user_id: string; amount: number; reason: string; type: string }) =>
		api<unknown>('/admin/wallet/award', { method: 'POST', body }),
};

export const pageBuilderSaveAsTemplateApi = {
	saveAsTemplate: (pageId: string, body: { name: string }) =>
		api<unknown>(`/admin/pages/${pageId}/save-as-template`, { method: 'POST', body }),
};

export const seoAdminApi = {
	validate: (id: string) =>
		api<{ issues: string[] }>(`/admin/seo/validate/${id}`),
};

export const fieldPermissionsApi = {
	set: (fieldId: string, body: { user_group_id: string; can_view: boolean; can_edit: boolean }) =>
		api<unknown>(`/custom-fields/${fieldId}/permissions`, { method: 'PUT', body }),
};

export const affiliateLinksAdminApi = {
	list: (params?: string) =>
		api<unknown>(`/admin/affiliate-links${params ? `?${params}` : ''}`),
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
	reorderItems: (id: string, body: { item_ids: string[] }) =>
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
	bulkDelete: (body: { ids: string[] }) =>
		api<unknown>('/admin/redirects/bulk', { method: 'POST', body }),
};

export const gdprAdminApiExtendedV2 = {
	listConsents: (params?: string) => api<unknown>(`/admin/consents${params ? `?${params}` : ''}`),
	deleteConsent: (id: string) => api<unknown>(`/admin/consents/${id}`, { method: 'DELETE' }),
};

export const aiModelsAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/ai/models${params ? `?${params}` : ''}`),
};

export const spacesModeratorsAdminApi = {
	add: (id: string, body: { user_id: string }) =>
		api<unknown>(`/admin/spaces/${id}/moderators`, { method: 'POST', body }),
	remove: (id: string, moderatorId: string) =>
		api<{ removed: boolean }>(`/admin/spaces/${id}/moderators/${moderatorId}`, { method: 'DELETE' }),
};

export const tagsModeratorsAdminApi = {
	add: (id: string, body: { user_id: string }) =>
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
