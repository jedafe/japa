import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ params }) => {
	const result = await apiFetch<any>(`/pages/${params.id}`);
	return {
		page: result.ok ? result.data : null
	};
};
