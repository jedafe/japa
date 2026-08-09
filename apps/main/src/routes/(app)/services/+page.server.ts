// Ref: japaconnect-engineering/sections/98-service-directory.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ServiceListing } from '$lib/types';

/**
 * Service Directory list at /services — geo-indexed directory of
 * diaspora-relevant businesses and service providers. SSR-fetches
 * GET /services?dest=&city=&category=&origin=&verified=&sort=, passing
 * through all query params so the backend filters and sorts server-side.
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const city = event.url.searchParams.get('city') ?? '';
	const category = event.url.searchParams.get('category') ?? '';
	const origin = event.url.searchParams.get('origin') ?? '';
	const verified = event.url.searchParams.get('verified') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'latest';
	const page = Number(event.url.searchParams.get('page') ?? '1');

	const filters = { dest, city, category, origin, verified, sort, page };

	const params = new URLSearchParams();
	params.set('page_size', String(PAGE_SIZE));
	if (dest) params.set('dest', dest);
	if (city) params.set('city', city);
	if (category) params.set('category', category);
	if (origin) params.set('origin', origin);
	if (verified) params.set('verified', verified);
	if (sort) params.set('sort', sort);

	try {
		const { items, meta } = await apiList<ServiceListing>(ctx, `/services?${params.toString()}`);
		return { services: items, meta, authed, pageSize: PAGE_SIZE, filters, loadError: false };
	} catch {
		return {
			services: [] as ServiceListing[],
			meta: undefined,
			authed,
			pageSize: PAGE_SIZE,
			filters,
			loadError: true
		};
	}
};
