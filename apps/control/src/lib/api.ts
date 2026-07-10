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
