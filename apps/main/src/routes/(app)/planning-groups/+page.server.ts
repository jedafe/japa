// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { PlanningGroup } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const dest = event.url.searchParams.get('dest') ?? '';
	const visa = event.url.searchParams.get('visa') ?? '';

	let groups: PlanningGroup[] = [];
	let loadError = false;
	try {
		const params = new URLSearchParams({ page_size: String(PAGE_SIZE) });
		if (dest) params.set('dest', dest);
		if (visa) params.set('visa', visa);
		groups = (await apiList<PlanningGroup>(ctx, `/planning-groups?${params.toString()}`)).items;
	} catch {
		loadError = true;
	}

	return {
		groups,
		authed,
		pageSize: PAGE_SIZE,
		loadError,
		filters: { dest, visa }
	};
};
