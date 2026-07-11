import { apiFetch } from './client';
import type {
	User,
	HealthStatus,
	ServiceStatus,
	QueueInfo,
	QueueStats,
	DlqEntry,
	CacheStats,
	SystemSetting,
	FeatureFlag,
	Tenant,
	SecurityScan,
	SecurityFinding,
	IpBlock,
	ApiKey,
	AiBudget,
	AuditLogEntry,
	DbHealth,
	Deployment,
	PaginatedResponse,
} from './types';

export type {
	User,
	HealthStatus,
	ServiceStatus,
	QueueInfo,
	QueueStats,
	DlqEntry,
	CacheStats,
	SystemSetting,
	FeatureFlag,
	Tenant,
	SecurityScan,
	SecurityFinding,
	IpBlock,
	ApiKey,
	AiBudget,
	AuditLogEntry,
	DbHealth,
	Deployment,
	PaginatedResponse,
};

export interface ApiOptions {
	method?: string;
	body?: any;
}

export async function api<T = any>(path: string, options?: ApiOptions): Promise<{ ok: boolean; data: T; error?: string }> {
	const result = await apiFetch<T>(path, {
		method: options?.method ?? 'GET',
		body: options?.body ? JSON.stringify(options.body) : undefined,
	});

	if (result.error) {
		return { ok: false, data: null as T, error: result.error.message };
	}

	return { ok: true, data: result.data as T };
}
