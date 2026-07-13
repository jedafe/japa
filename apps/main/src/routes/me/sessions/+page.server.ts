import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type Session = {
	id: string;
	ip_address: string;
	user_agent: string;
	created_at: string;
	last_active_at: string;
	is_current: boolean;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<Session[]>('/me/sessions', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
