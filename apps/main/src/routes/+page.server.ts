import type { PageServerLoad } from './$types';
import { API_BASE_URL } from '$lib/server/api';

export const load: PageServerLoad = async () => {
	let healthy = false;
	try {
		const res = await fetch(`${API_BASE_URL.replace('/api/v1', '')}/health`);
		if (res.ok) {
			const body = (await res.json()) as { status: string };
			healthy = body.status === 'ok';
		}
	} catch {
		healthy = false;
	}

	return {
		healthy,
		flags: {}
	};
};
