import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import type { Space } from '$lib/types';

interface GeoSpace {
	id: string;
	name: string;
	slug: string;
	geo_code?: string;
	level: string;
}

interface SpaceType {
	id: string;
	name: string;
	slug: string;
}

const PAGE_SIZE = 20;

export const load: PageServerLoad = async ({ url, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const country = url.searchParams.get('country');
	const sort = url.searchParams.get('sort') || 'world_ranking';
	const page = Math.max(1, Number(url.searchParams.get('page') || '1') || 1);

	// The backend's /spaces filter is `type=<space_type UUID>`, so resolve the
	// "university" space type first. Country/sort/page are not supported
	// server-side; they are applied below.
	const [typesResult, countriesResult] = await Promise.allSettled([
		apiFetch<SpaceType[]>('/space-types', { headers }),
		apiFetch<GeoSpace[]>('/geo/countries')
	]);

	const universityType =
		typesResult.status === 'fulfilled' && typesResult.value.ok && typesResult.value.data
			? typesResult.value.data.find((t) => t.slug === 'university')
			: undefined;

	let universities: Space[] = [];
	if (universityType) {
		const spacesResult = await apiFetch<Space[]>(`/spaces?type=${universityType.id}`, { headers });
		if (spacesResult.ok && spacesResult.data) {
			universities = spacesResult.data;
		}
	}

	if (country) {
		universities = universities.filter((u) => u.country === country);
	}
	universities = [...universities].sort((a, b) => {
		if (sort === 'name') return a.name.localeCompare(b.name);
		// world_ranking: unranked entries sink to the bottom
		return (a.world_ranking ?? Number.MAX_SAFE_INTEGER) - (b.world_ranking ?? Number.MAX_SAFE_INTEGER);
	});
	const total = universities.length;
	universities = universities.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

	const countries =
		countriesResult.status === 'fulfilled' && countriesResult.value.ok && countriesResult.value.data
			? countriesResult.value.data.map((c) => ({ value: c.name, label: c.name }))
			: [];

	return {
		universities,
		countries,
		total,
		filters: { country, sort, page: String(page) }
	};
};
