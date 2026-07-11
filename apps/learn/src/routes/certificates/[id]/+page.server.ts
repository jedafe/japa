import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token') ?? undefined;

	try {
		const result = await apiFetch<any>(`/certificates/${params.id}`, {}, token);
		if (!result.ok) {
			return { certificate: null, error: 'Certificate not found' };
		}
		return { certificate: result.data, error: null };
	} catch {
		return { certificate: null, error: 'Failed to load certificate' };
	}
};
