import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';
import type { QueueStats } from '$lib/api/types';

interface BackendQueueStats {
	message?: string;
	critical?: { size?: number; active?: number; pending?: number; scheduled?: number; completed?: number; failed?: number; paused?: number; latency_seconds?: number };
	default?: { size?: number; active?: number; pending?: number; scheduled?: number; completed?: number; failed?: number; paused?: number; latency_seconds?: number };
	low?: { size?: number; active?: number; pending?: number; scheduled?: number; completed?: number; failed?: number; paused?: number; latency_seconds?: number };
	dlq?: { depth?: number; checked_at?: string; alert_active?: boolean };
}

const EMPTY_STATS: QueueStats = {
	queues: [],
	total_pending: 0,
	total_active: 0,
	total_completed_24h: 0,
	total_failed_24h: 0,
	dlq_count: 0
};

function transformBackendResponse(raw: BackendQueueStats): QueueStats {
	const queueNames = ['critical', 'default', 'low'] as const;

	const queues = queueNames
		.filter((name): name is 'critical' | 'default' | 'low' => !!raw[name])
		.map((name) => ({
			name,
			pending: raw[name]?.pending ?? 0,
			active: raw[name]?.active ?? 0,
			completed_24h: raw[name]?.completed ?? 0,
			failed_24h: raw[name]?.failed ?? 0,
			latency_ms: (raw[name]?.latency_seconds ?? 0) * 1000
		}));

	return {
		queues,
		total_pending: queues.reduce((sum, q) => sum + q.pending, 0),
		total_active: queues.reduce((sum, q) => sum + q.active, 0),
		total_completed_24h: queues.reduce((sum, q) => sum + q.completed_24h, 0),
		total_failed_24h: queues.reduce((sum, q) => sum + q.failed_24h, 0),
		dlq_count: raw.dlq?.depth ?? 0
	};
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const res = await fetch(`${API_BASE}/control/queue/stats`, {
			headers: {
				Authorization: `Bearer ${token}`,
				'Content-Type': 'application/json'
			}
		});

		if (!res.ok) {
			return { queueStats: null, error: `API ${res.status}` };
		}

		const body = await res.json();
		const raw: BackendQueueStats = body?.data ?? body ?? {};

		if (raw.message) {
			return {
				queueStats: EMPTY_STATS,
				error: raw.message
			};
		}

		const queueStats = transformBackendResponse(raw);

		return {
			queueStats,
			error: null
		};
	} catch (e) {
		return {
			queueStats: null,
			error: e instanceof Error ? e.message : 'Network error'
		};
	}
};
