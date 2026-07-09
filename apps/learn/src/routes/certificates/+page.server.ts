import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import type { Certificate } from '$lib/api';

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('learn_token') ?? cookies.get('jc_token');

	try {
		const result = await apiFetch<Certificate[]>('/me/certificates', {}, token);
		return { certificates: result.ok ? result.data ?? [] : [] };
	} catch {
		return { certificates: [] };
	}
};
