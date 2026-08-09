// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Mentor } from '$lib/types';

/**
 * Mentor discovery at /mentors — lists mentors with optional destination and
 * areas filters and a sort (match | newest | capacity). Passes through all
 * query params to GET /mentors so the backend filters/sorts server-side.
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const areas = event.url.searchParams.get('areas') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'match';
	const page = Number(event.url.searchParams.get('page') ?? '1');

	const filters = { dest, areas, sort, page };

	const params = new URLSearchParams();
	params.set('page_size', String(PAGE_SIZE));
	if (dest) params.set('dest', dest);
	if (areas) params.set('areas', areas);
	if (sort) params.set('sort', sort);

	try {
		const { items, meta } = await apiList<Mentor>(ctx, `/mentors?${params.toString()}`);
		return { mentors: items, meta, authed, pageSize: PAGE_SIZE, filters, loadError: false };
	} catch {
		return {
			mentors: [] as Mentor[],
			meta: undefined,
			authed,
			pageSize: PAGE_SIZE,
			filters,
			loadError: true
		};
	}
};
