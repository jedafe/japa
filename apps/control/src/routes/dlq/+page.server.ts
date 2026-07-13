import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const res = await fetch(`${API_BASE}/control/queue/dlq`, {
			headers: {
				Authorization: `Bearer ${token}`,
				'Content-Type': 'application/json'
			}
		});

		if (!res.ok) {
			return { entries: null, error: `API ${res.status}` };
		}

		const body = await res.json();
		const data = body?.data ?? body ?? {};

		return {
			entries: data ?? null,
			error: null
		};
	} catch (e) {
		return {
			entries: null,
			error: e instanceof Error ? e.message : 'Network error'
		};
	}
};
