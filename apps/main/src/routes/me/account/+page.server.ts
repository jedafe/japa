import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type Account = {
	id: string;
	email: string;
	name: string;
	username: string;
	created_at: string;
	avatar_url?: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<Account>('/me/account', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
