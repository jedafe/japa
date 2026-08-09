// Ref: japaconnect-engineering/sections/79-community-story-wall.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Story } from '$lib/types';

/**
 * Community Story Wall at /stories — SSR list of relocation stories.
 * Fetches GET /stories?sort=top&page_size=20, passing through the sort
 * (top|latest) and filter query params (origin, dest, visa, profession).
 * On error, degrades to an empty list with loadError: true.
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const sort = event.url.searchParams.get('sort') ?? 'top';
	const origin = event.url.searchParams.get('origin') ?? '';
	const dest = event.url.searchParams.get('dest') ?? '';
	const visa = event.url.searchParams.get('visa') ?? '';
	const profession = event.url.searchParams.get('profession') ?? '';

	const params = new URLSearchParams();
	params.set('sort', sort === 'latest' ? 'latest' : 'top');
	params.set('page_size', String(PAGE_SIZE));
	if (origin) params.set('origin', origin);
	if (dest) params.set('dest', dest);
	if (visa) params.set('visa', visa);
	if (profession) params.set('profession', profession);

	try {
		const { items } = await apiList<Story>(ctx, `/stories?${params.toString()}`);
		return {
			stories: items,
			authed,
			pageSize: PAGE_SIZE,
			loadError: false,
			sort: sort === 'latest' ? 'latest' : 'top',
			filters: { origin, dest, visa, profession }
		};
	} catch {
		return {
			stories: [] as Story[],
			authed,
			pageSize: PAGE_SIZE,
			loadError: true,
			sort: sort === 'latest' ? 'latest' : 'top',
			filters: { origin, dest, visa, profession }
		};
	}
};
