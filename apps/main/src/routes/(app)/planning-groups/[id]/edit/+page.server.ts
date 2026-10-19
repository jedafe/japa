// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { PlanningGroup, GeoCountry } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const id = event.params.id;

	const countriesPromise = apiList<GeoCountry>(ctx, '/geo/countries')
		.then((r) => r.items)
		.catch(() => [] as GeoCountry[]);

	let group: PlanningGroup | null = null;
	try {
		group = await apiGet<PlanningGroup>(ctx, `/planning-groups/${id}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Planning group not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load planning group' });
	}

	// Only the owner may edit.
	if (group.owner_id !== event.locals.user!.id) {
		throw error(403, { code: 'not_owner', message: 'Only the group owner can edit this group' });
	}

	const countries = await countriesPromise;

	return { group, countries };
};
