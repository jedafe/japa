// Ref: japaconnect-engineering/sections/89-achievement-feed.md
// Achievement Feed list at /achievements — opt-in feed of member relocation
// milestones and wins. SSR fetches GET /achievements?dest=&milestone=&sort= and
// passes through the active query params so the filter bar reflects state.
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Achievement, Space } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const milestone = event.url.searchParams.get('milestone') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'latest';

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	const filters = { dest, milestone, sort };

	try {
		const params = new URLSearchParams();
		params.set('page_size', String(PAGE_SIZE));
		if (dest) params.set('dest', dest);
		if (milestone) params.set('milestone', milestone);
		params.set('sort', sort);
		const { items } = await apiList<Achievement>(ctx, `/achievements?${params.toString()}`);
		return { achievements: items, authed, pageSize: PAGE_SIZE, loadError: false, filters, countries };
	} catch {
		return {
			achievements: [] as Achievement[],
			authed,
			pageSize: PAGE_SIZE,
			loadError: true,
			filters,
			countries
		};
	}
};
