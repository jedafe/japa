// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { PlanningGroup, GeoCountry } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['planning_group.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Planning Groups are not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const dest = event.url.searchParams.get('dest') ?? '';
	const visa = event.url.searchParams.get('visa') ?? '';

	const params = new URLSearchParams({ page_size: String(PAGE_SIZE) });
	if (dest) params.set('dest', dest);
	if (visa) params.set('visa', visa);

	let loadError = false;
	const [countries, groups] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		apiList<PlanningGroup>(ctx, `/planning-groups?${params.toString()}`)
			.then((r) => r.items)
			.catch(() => {
				loadError = true;
				return [] as PlanningGroup[];
			})
	]);

	return {
		groups,
		countries,
		authed,
		pageSize: PAGE_SIZE,
		loadError,
		filters: { dest, visa }
	};
};
