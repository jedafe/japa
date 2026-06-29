import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import type { Space, PaginatedResponse } from '$lib/types';

export const load: PageServerLoad = async ({ url, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const params = new URLSearchParams();
	params.set('space_type', 'university');

	const country = url.searchParams.get('country');
	if (country) params.set('country', country);

	const sort = url.searchParams.get('sort') || 'world_ranking';
	params.set('sort', sort);

	const page = url.searchParams.get('page') || '1';
	params.set('page', page);

	const result = await apiFetch<PaginatedResponse<Space>>(`/spaces?${params.toString()}`, { headers });

	return {
		universities: result.ok && result.data ? result.data.data : [],
		filters: { country, sort, page }
	};
};
