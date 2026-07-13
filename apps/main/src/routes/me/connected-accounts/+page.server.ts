import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type ConnectedAccount = {
	provider: string;
	email: string;
	connected_at: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<ConnectedAccount[]>('/me/connected-accounts', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
