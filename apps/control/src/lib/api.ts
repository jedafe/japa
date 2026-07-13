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

export interface QueueInfo {
	name: string;
	pending: number;
	active: number;
	completed_24h: number;
	failed_24h: number;
	latency_ms: number;
}

export interface QueueStats {
	queues: QueueInfo[];
	total_pending: number;
	total_active: number;
	total_completed_24h: number;
	total_failed_24h: number;
}

export interface SlowQuery {
	query: string;
	duration_ms: number;
	timestamp: string;
}

export interface TableStat {
	name: string;
	row_count: number;
	size_bytes: number;
	index_count: number;
	last_vacuum: string | null;
}

export interface ConnectionPool {
	active: number;
	idle: number;
	waiting: number;
	max: number;
}

export interface ReplicationStatus {
	lag_seconds: number;
	state: string;
}

export interface DbHealth {
	connected: boolean;
	version: string;
	latency_ms: number;
	size_bytes: number;
	table_count: number;
	index_count: number;
	tables: TableStat[];
	slow_queries: SlowQuery[];
	avg_query_time_ms: number;
	connection_pool: ConnectionPool;
	replication: ReplicationStatus | null;
}

export interface Tenant {
	id: string;
	name: string;
	slug: string;
	domain: string | null;
	status: string;
	created_at: string;
}

export interface ApiKey {
	id: string;
	name: string;
	key_prefix: string;
	status: string;
	created_at: string;
}

export interface FeatureFlag {
	name: string;
	enabled: boolean;
	description: string;
}

export interface SystemSetting {
	key: string;
	value: unknown;
	updated_at: string;
}

export interface DeployInfo {
	id: string;
	status: string;
	version: string;
	deployed_at: string;
}

export const tenantsApi = {
	list: () => api<Tenant[]>('/control/tenants'),
	get: (id: string) => api<Tenant>(`/control/tenants/${id}`),
	create: (body: { name: string; slug: string; domain?: string }) =>
		api<Tenant>('/control/tenants', { method: 'POST', body }),
	update: (id: string, body: { name?: string; domain?: string }) =>
		api<Tenant>(`/control/tenants/${id}`, { method: 'PATCH', body }),
	suspend: (id: string) => api<Tenant>(`/control/tenants/${id}/suspend`, { method: 'POST', body: {} }),
	unsuspend: (id: string) => api<Tenant>(`/control/tenants/${id}/unsuspend`, { method: 'POST', body: {} }),
	delete: (id: string) => api<{ deleted: boolean }>(`/control/tenants/${id}`, { method: 'DELETE' }),
};

export const apiKeysApi = {
	list: () => api<ApiKey[]>('/control/api-keys'),
	get: (id: string) => api<ApiKey>(`/control/api-keys/${id}`),
	create: (body: { name: string; scopes?: string[] }) =>
		api<ApiKey>('/control/api-keys', { method: 'POST', body }),
	update: (id: string, body: { name?: string; scopes?: string[] }) =>
		api<ApiKey>(`/control/api-keys/${id}`, { method: 'PATCH', body }),
	suspend: (id: string, body: { reason?: string }) =>
		api<ApiKey>(`/control/api-keys/${id}/suspend`, { method: 'POST', body }),
	delete: (id: string) => api<{ deleted: boolean }>(`/control/api-keys/${id}`, { method: 'DELETE' }),
	getAnalytics: () => api<unknown>('/control/api-keys/analytics'),
};

export const cacheApi = {
	getStats: () => api<unknown>('/control/cache/stats'),
	flush: (body: { keys?: string[] }) =>
		api<unknown>('/control/cache/flush', { method: 'POST', body }),
};

export const securityApi = {
	getStatus: () => api<unknown>('/control/security'),
	listIpBlocks: () => api<unknown>('/control/security/ip-blocks'),
	createIpBlock: (body: { cidr: string; reason?: string }) =>
		api<unknown>('/control/security/ip-blocks', { method: 'POST', body }),
	deleteIpBlock: (id: string) =>
		api<{ deleted: boolean }>(`/control/security/ip-blocks/${id}`, { method: 'DELETE' }),
	resolveAlert: (id: string, body: { resolution: string }) =>
		api<unknown>(`/control/security/${id}/resolve`, { method: 'POST', body }),
};

export const featureFlagsApi = {
	list: () => api<unknown>('/control/feature-flags'),
	update: (flagName: string, body: { enabled: boolean }) =>
		api<unknown>(`/control/feature-flags/${flagName}`, { method: 'PATCH', body }),
};

export const systemSettingsApi = {
	list: () => api<unknown>('/control/system-settings'),
	get: (key: string) => api<unknown>(`/control/system-settings/${key}`),
	update: (body: Record<string, unknown>) =>
		api<unknown>('/control/system-settings', { method: 'PATCH', body }),
	updateKey: (key: string, body: { value: unknown }) =>
		api<unknown>(`/control/system-settings/${key}`, { method: 'PATCH', body }),
	resetKey: (key: string) =>
		api<unknown>(`/control/system-settings/${key}/reset`, { method: 'POST', body: {} }),
};

export const queueApi = {
	getStats: () => api<QueueStats>('/control/queue/stats'),
	getDlq: () => api<unknown>('/control/queue/dlq'),
	retryDlqItem: (taskId: string) =>
		api<unknown>(`/control/queue/dlq/${taskId}/retry`, { method: 'POST', body: {} }),
	deleteDlqItem: (taskId: string) =>
		api<{ deleted: boolean }>(`/control/queue/dlq/${taskId}`, { method: 'DELETE' }),
	discardAllDlq: () =>
		api<{ discarded: number }>('/control/queue/dlq/discard-all', { method: 'POST', body: {} }),
};

export const cronApi = {
	list: () => api<unknown>('/control/cron/jobs'),
	trigger: (id: string) =>
		api<unknown>(`/control/cron/jobs/${id}/trigger`, { method: 'POST', body: {} }),
};

export const healthApi = {
	get: () => api<unknown>('/control/health'),
	getDb: () => api<DbHealth>('/control/health/db'),
	vacuumDb: () =>
		api<unknown>('/control/health/db/vacuum', { method: 'POST', body: {} }),
};

export const aiBudgetApi = {
	get: () => api<unknown>('/control/ai-budget'),
	update: (body: { monthly_limit: number; alert_threshold: number }) =>
		api<unknown>('/control/ai-budget', { method: 'PATCH', body }),
};

export const auditApi = {
	list: (params?: string) => api<unknown>(`/control/audit-logs${params ? `?${params}` : ''}`),
};

export const deploymentsApi = {
	list: () => api<DeployInfo[]>('/control/deployments'),
};
