// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { MutualAidPost } from '$lib/types';

/**
 * Mutual Aid list at /mutual-aid — community help requests and offers inside
 * the app shell. Passes through all query params (dest, city, type, category,
 * sort) to GET /mutual-aid so the backend can filter and sort server-side.
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const city = event.url.searchParams.get('city') ?? '';
	const type = event.url.searchParams.get('type') ?? '';
	const category = event.url.searchParams.get('category') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'latest';
	const page = Number(event.url.searchParams.get('page') ?? '1');

	const filters = { dest, city, type, category, sort, page };

	const params = new URLSearchParams();
	params.set('page_size', String(PAGE_SIZE));
	if (dest) params.set('dest', dest);
	if (city) params.set('city', city);
	if (type) params.set('type', type);
	if (category) params.set('category', category);
	if (sort) params.set('sort', sort);

	try {
		const { items, meta } = await apiList<MutualAidPost>(ctx, `/mutual-aid?${params.toString()}`);
		return { posts: items, meta, authed, pageSize: PAGE_SIZE, filters, loadError: false };
	} catch {
		return {
			posts: [] as MutualAidPost[],
			meta: undefined,
			authed,
			pageSize: PAGE_SIZE,
			filters,
			loadError: true
		};
	}
};
