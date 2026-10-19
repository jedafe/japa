// Ref: japaconnect-engineering/sections/89-achievement-feed.md
// Achievement Feed list at /achievements — opt-in feed of member relocation
// milestones and wins. SSR fetches GET /achievements?dest=&milestone=&sort= and
// passes through the active query params so the filter bar reflects state.
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Achievement, GeoCountry } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['achievement.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Achievements are not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const milestone = event.url.searchParams.get('milestone') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'latest';

	const filters = { dest, milestone, sort };

	const countriesPromise = apiList<GeoCountry>(ctx, '/geo/countries')
		.then((r) => r.items)
		.catch(() => [] as GeoCountry[]);

	try {
		const params = new URLSearchParams();
		params.set('limit', String(PAGE_SIZE));
		if (dest) params.set('dest', dest);
		if (milestone) params.set('milestone', milestone);
		params.set('sort', sort);
		const [{ items, meta }, countries] = await Promise.all([
			apiList<Achievement>(ctx, `/achievements?${params.toString()}`),
			countriesPromise
		]);
		return { achievements: items, total: meta?.total, authed, pageSize: PAGE_SIZE, loadError: false, filters, countries };
	} catch {
		return {
			achievements: [] as Achievement[],
			total: undefined as number | undefined,
			authed,
			pageSize: PAGE_SIZE,
			loadError: true,
			filters,
			countries: await countriesPromise
		};
	}
};
