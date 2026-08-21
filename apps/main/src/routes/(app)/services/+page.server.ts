// Ref: japaconnect-engineering/sections/98-service-directory.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { ServiceListing, ServiceCategory, GeoCountry } from '$lib/types';

/**
 * Service Directory list at /services — geo-indexed directory of
 * diaspora-relevant businesses and service providers. SSR-fetches
 * GET /services?dest=&category=&verified=&sort=, passing through all
 * query params so the backend filters and sorts server-side.
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['service_directory.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Service Directory is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const category = event.url.searchParams.get('category') ?? '';
	const verified = event.url.searchParams.get('verified') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'latest';
	const page = Number(event.url.searchParams.get('page') ?? '1');

	const filters = { dest, category, verified, sort, page };

	const params = new URLSearchParams();
	params.set('limit', String(PAGE_SIZE));
	if (page > 1) params.set('page', String(page));
	if (dest) params.set('dest', dest);
	if (category) params.set('category', category);
	if (verified) params.set('verified', verified);
	if (sort) params.set('sort', sort);

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	let serviceCategories: ServiceCategory[] = [];
	try {
		const cat = await apiGet<{ value: string; label: string }[]>(ctx, '/services/categories');
		serviceCategories = (cat ?? []).map((o) => o.value as ServiceCategory);
	} catch {
		serviceCategories = [];
	}

	try {
		const { items, meta } = await apiList<ServiceListing>(ctx, `/services?${params.toString()}`);
		return { services: items, meta, authed, pageSize: PAGE_SIZE, filters, countries, serviceCategories, loadError: false };
	} catch {
		return {
			services: [] as ServiceListing[],
			meta: undefined,
			authed,
			pageSize: PAGE_SIZE,
			filters,
			countries,
			serviceCategories,
			loadError: true
		};
	}
};
