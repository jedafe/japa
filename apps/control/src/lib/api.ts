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

		const data = await res.json();
		return { ok: true, data };
	} catch (e) {
		return { ok: false, error: e instanceof Error ? e.message : 'Network error' };
	}
}

// ── Queue Monitoring Types ──

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

// ── Database Health Types ──

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
