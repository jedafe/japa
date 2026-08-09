// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { AMAThread } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const dest = event.url.searchParams.get('dest') ?? '';
	const profession = event.url.searchParams.get('profession') ?? '';

	const params = new URLSearchParams({ page_size: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (profession) params.set('profession', profession);

	try {
		const { items } = await apiList<AMAThread>(ctx, `/forum/ama?${params.toString()}`);
		return {
			sessions: items,
			authed,
			pageSize: PAGE_SIZE,
			loadError: false,
			filters: { dest, profession }
		};
	} catch {
		return {
			sessions: [] as AMAThread[],
			authed,
			pageSize: PAGE_SIZE,
			loadError: true,
			filters: { dest, profession }
		};
	}
};
