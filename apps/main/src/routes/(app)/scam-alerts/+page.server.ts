// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ScamReport, Space } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const dest = event.url.searchParams.get('dest') ?? '';
	const scam_type = event.url.searchParams.get('scam_type') ?? '';

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	const params = new URLSearchParams({ page: '1', limit: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (scam_type) params.set('scam_type', scam_type);

	try {
		const { items } = await apiList<ScamReport>(ctx, `/scam-alerts?${params.toString()}`);
		return { alerts: items, pageSize: PAGE_SIZE, loadError: false, filters: { dest, scam_type }, countries };
	} catch {
		return {
			alerts: [] as ScamReport[],
			pageSize: PAGE_SIZE,
			loadError: true,
			filters: { dest, scam_type },
			countries
		};
	}
};
