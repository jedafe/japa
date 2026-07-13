import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type Analytics = {
	views: number;
	clicks: number;
	engagements: number;
	period: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<Analytics>('/me/analytics', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
