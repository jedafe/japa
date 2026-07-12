import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import type { Space, PaginatedResponse } from '$lib/types';

interface GeoSpace {
	id: string;
	name: string;
	slug: string;
	geo_code?: string;
	level: string;
}

interface GeoResponse {
	data: GeoSpace[];
}

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

	const [universitiesResult, countriesResult] = await Promise.allSettled([
		apiFetch<PaginatedResponse<Space>>(`/spaces?${params.toString()}`, { headers }),
		apiFetch<GeoResponse>('/geo/countries')
	]);

	const universities = universitiesResult.status === 'fulfilled' && universitiesResult.value.ok && universitiesResult.value.data
		? universitiesResult.value.data.data
		: [];

	const countries = countriesResult.status === 'fulfilled' && countriesResult.value.ok && countriesResult.value.data
		? countriesResult.value.data.data.map(c => ({ value: c.name, label: c.name }))
		: [];

	return {
		universities,
		countries,
		filters: { country, sort, page }
	};
};
