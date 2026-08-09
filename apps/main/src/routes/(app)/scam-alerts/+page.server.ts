// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ScamReport } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const dest = event.url.searchParams.get('dest') ?? '';
	const type = event.url.searchParams.get('type') ?? '';

	const params = new URLSearchParams({ page_size: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (type) params.set('type', type);

	try {
		const { items } = await apiList<ScamReport>(ctx, `/scam-alerts?${params.toString()}`);
		return { alerts: items, pageSize: PAGE_SIZE, loadError: false, filters: { dest, type } };
	} catch {
		return {
			alerts: [] as ScamReport[],
			pageSize: PAGE_SIZE,
			loadError: true,
			filters: { dest, type }
		};
	}
};
