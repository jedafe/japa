import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type Certificate = {
	id: string;
	track_title: string;
	issued_at: string;
	download_url: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<Certificate[]>('/me/certificates', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
