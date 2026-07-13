import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type TotpStatus = {
	enabled: boolean;
	qr_code?: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<TotpStatus>('/me/totp', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
