// Ref: japaconnect-engineering/sections/89-achievement-feed.md
// Achievement Feed list at /achievements — opt-in feed of member relocation
// milestones and wins. SSR fetches GET /achievements?dest=&type=&sort= and
// passes through the active query params so the filter bar reflects state.
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Achievement } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const type = event.url.searchParams.get('type') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'latest';

	const filters = { dest, type, sort };

	try {
		const params = new URLSearchParams();
		params.set('page_size', String(PAGE_SIZE));
		if (dest) params.set('dest', dest);
		if (type) params.set('type', type);
		params.set('sort', sort);
		const { items } = await apiList<Achievement>(ctx, `/achievements?${params.toString()}`);
		return { achievements: items, authed, pageSize: PAGE_SIZE, loadError: false, filters };
	} catch {
		return {
			achievements: [] as Achievement[],
			authed,
			pageSize: PAGE_SIZE,
			loadError: true,
			filters
		};
	}
};
