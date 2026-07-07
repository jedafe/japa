const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080/api/v1';

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
	const headers: Record<string, string> = {
		'Content-Type': 'application/json'
	};
	if (jwtToken) {
		headers['Authorization'] = `Bearer ${jwtToken}`;
	}

	try {
		const res = await fetch(`${API_BASE}${path}`, {
			method: opts.method ?? 'GET',
			headers,
			body: opts.body ? JSON.stringify(opts.body) : undefined,
			signal: opts.signal
		});

		if (!res.ok) {
			const text = await res.text().catch(() => '');
			return { ok: false, error: `API ${res.status}: ${text || res.statusText}` };
		}

		const raw = await res.json();
		// Unwrap the backend's { data: ... } envelope so callers get the payload directly.
		const data = raw && typeof raw === 'object' && 'data' in raw ? raw.data : raw;
		return { ok: true, data };
	} catch (e) {
		return { ok: false, error: e instanceof Error ? e.message : 'Network error' };
	}
}

// ─── Spam Management ───────────────────────────────────────────────

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
	deleteUserBlock: (id: string) =>
		api(`/admin/spam/user-blocks/${id}`, { method: 'DELETE' }),
};

// ─── Ticket Types ──────────────────────────────────────────────────

export const ticketTypesApi = {
	list: (params?: string) => api<unknown>(`/admin/ticket-types${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string; description?: string; default_priority: string; is_active: boolean }) =>
		api('/admin/ticket-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; description?: string; default_priority?: string; is_active?: boolean }) =>
		api(`/admin/ticket-types/${id}`, { method: 'PUT', body }),
	delete: (id: string) =>
		api(`/admin/ticket-types/${id}`, { method: 'DELETE' }),
};

// ─── Wallet Management ─────────────────────────────────────────────

export const walletApi = {
	getRules: (params?: string) => api<unknown>(`/wallet/rules${params ? `?${params}` : ''}`),
	createRule: (body: { event_type: string; points_value: number; description?: string; is_active: boolean }) =>
		api('/wallet/rules', { method: 'POST', body }),
	updateRule: (id: string, body: { event_type?: string; points_value?: number; description?: string; is_active?: boolean }) =>
		api(`/wallet/rules/${id}`, { method: 'PATCH', body }),

	getSettings: () => api<unknown>('/wallet/settings'),
	updateSettings: (body: { points_to_credit_ratio: number; daily_earning_cap: number; enable_earning: boolean }) =>
		api('/wallet/settings', { method: 'PUT', body }),

	getUserBalance: (userId: string) => api<unknown>(`/wallet/admin/user/${userId}/balance`),
	adjustBalance: (body: { user_id: string; amount: number; reason: string; type: 'points' | 'credits' }) =>
		api('/wallet/admin/adjust', { method: 'POST', body }),
};

// ─── Space Types ───────────────────────────────────────────────────

export const spaceTypesApi = {
	list: (params?: string) => api<unknown>(`/space-types${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string; description?: string; is_university: boolean; icon?: string }) =>
		api('/space-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; description?: string; is_university?: boolean; icon?: string }) =>
		api(`/space-types/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/space-types/${id}`, { method: 'DELETE' }),
};

// ─── User Impersonation ────────────────────────────────────────────

export const impersonationApi = {
	start: (userId: string) =>
		api<{ token: string }>('/admin/impersonate', { method: 'POST', body: { user_id: userId } }),
	end: () =>
		api('/admin/impersonate', { method: 'DELETE' }),
	status: () =>
		api<{ impersonating: boolean; username?: string; user_id?: string }>('/admin/impersonate/status'),
};

// ─── Documentation Management ──────────────────────────────────────

export const docsApi = {
	list: (params?: string) => api<unknown>(`/admin/docs${params ? `?${params}` : ''}`),
	create: (body: { title: string; slug: string; audience: string; body: string }) =>
		api('/admin/docs', { method: 'POST', body }),
	update: (id: string, body: { title?: string; slug?: string; audience?: string; body?: string; status?: string }) =>
		api(`/admin/docs/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/docs/${id}`, { method: 'DELETE' }),
};

// ─── Ad Slot Types ─────────────────────────────────────────────────

export const adSlotTypesApi = {
	list: (params?: string) => api<unknown>(`/admin/advertising/slot-types${params ? `?${params}` : ''}`),
	create: (body: { name: string; slug: string; width: number; height: number; description?: string }) =>
		api('/admin/advertising/slot-types', { method: 'POST', body }),
	update: (id: string, body: { name?: string; slug?: string; width?: number; height?: number; description?: string }) =>
		api(`/admin/advertising/slot-types/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/advertising/slot-types/${id}`, { method: 'DELETE' }),
};

// ─── Ad Boosts ─────────────────────────────────────────────────────

export const adBoostsApi = {
	list: (params?: string) => api<unknown>(`/admin/advertising/boosts${params ? `?${params}` : ''}`),
	create: (body: { organization_id: string; space_id: string; boost_type: string; start_date: string; end_date: string }) =>
		api('/admin/advertising/boosts', { method: 'POST', body }),
	update: (id: string, body: { organization_id?: string; space_id?: string; boost_type?: string; start_date?: string; end_date?: string }) =>
		api(`/admin/advertising/boosts/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/advertising/boosts/${id}`, { method: 'DELETE' }),
};

// ─── Comparison Types (Admin) ──────────────────────────────────────

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
	listTypes: () => api<ComparisonType[]>('/comparison/types'),
	getType: (id: string) => api<ComparisonTypeDetail>(`/comparison/types/${id}`),
	createType: (body: { name: string; slug: string; description: string; is_public: boolean }) =>
		api<ComparisonType>('/comparison/types', { method: 'POST', body }),
	updateType: (id: string, body: { name?: string; description?: string; is_public?: boolean }) =>
		api<ComparisonType>(`/comparison/types/${id}`, { method: 'PATCH', body }),
	deleteType: (id: string) =>
		api<{ deleted: boolean }>(`/comparison/types/${id}`, { method: 'DELETE' }),
	getTypeAccess: (id: string) =>
		api<ComparisonTypeAccess>(`/comparison/types/${id}/access`),
	setTypeAccess: (id: string, body: { allowed_groups: string[]; allowed_roles: string[] }) =>
		api<{ updated: boolean }>(`/comparison/types/${id}/access`, { method: 'PUT', body }),
};

// ─── Professional Profile Types & Members (Admin) ──────────────────

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
	listTypes: () => api<ProfileType[]>('/professional-profiles/types'),
	getType: (id: string) => api<ProfileTypeDetail>(`/professional-profiles/types/${id}`),
	createType: (body: { name: string; slug: string; description: string }) =>
		api<ProfileType>('/professional-profiles/types', { method: 'POST', body }),
	listMembers: (profileId: string) =>
		api<ProfileMember[]>(`/professional-profiles/${profileId}/members`),
	addMember: (profileId: string, body: { user_id: string; role: string }) =>
		api<ProfileMember>(`/professional-profiles/${profileId}/members`, { method: 'POST', body }),
	removeMember: (profileId: string, userId: string) =>
		api<{ removed: boolean }>(`/professional-profiles/${profileId}/members/${userId}`, { method: 'DELETE' }),
	updateMember: (profileId: string, userId: string, body: { role: string }) =>
		api<ProfileMember>(`/professional-profiles/${profileId}/members/${userId}`, { method: 'PATCH', body }),
	listInvitations: (profileId: string) =>
		api<ProfileInvitation[]>(`/professional-profiles/${profileId}/invitations`),
	sendInvitation: (profileId: string, body: { email: string; role: string }) =>
		api<ProfileInvitation>(`/professional-profiles/${profileId}/invitations`, { method: 'POST', body }),
};

// ─── AI Core ───────────────────────────────────────────────────────

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

	listModels: () =>
		api<Array<{ id: string; label: string }>>('/admin/ai/models'),
};

// ─── JSON-LD Templates ─────────────────────────────────────────────

export const jsonLdApi = {
	getTemplates: () => api<Record<string, unknown>>('/content/jsonld-templates'),
	updateTemplates: (templates: Record<string, unknown>) =>
		api<{ updated: boolean }>('/content/jsonld-templates', {
			method: 'PUT',
			body: templates
		}),
};

// ─── Page Builder Block Types ──────────────────────────────────────

export interface BlockType {
	name: string;
	description: string;
	category: string;
	is_enabled: boolean;
}

export const pageBuilderApi = {
	getBlockTypes: () => api<BlockType[]>('/page-builder/block-types'),
};
