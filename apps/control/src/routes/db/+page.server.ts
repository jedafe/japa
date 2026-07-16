import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const res = await fetch(`${API_BASE}/control/health/db`, {
			headers: {
				Authorization: `Bearer ${token}`,
				'Content-Type': 'application/json'
			}
		});

		if (!res.ok) {
			return { dbHealth: null, error: `API ${res.status}` };
		}

		const body = await res.json();
		const raw = body?.data ?? body ?? {};
		const status = typeof raw === 'object' && raw !== null ? raw.status : undefined;

		const dbHealth = {
			connected: status === 'ok',
			version: '',
			latency_ms: 0,
			size_bytes: 0,
			table_count: 0,
			index_count: 0,
			avg_query_time_ms: 0,
			tables: [],
			slow_queries: [],
			indexes_unused: [],
			connection_pool: undefined,
			replication: undefined
		};

		return {
			dbHealth,
			error: null
		};
	} catch (e) {
		return {
			dbHealth: null,
			error: e instanceof Error ? e.message : 'Network error'
		};
	}
};
