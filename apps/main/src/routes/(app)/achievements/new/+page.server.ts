// Ref: japaconnect-engineering/sections/89-achievement-feed.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';
import type { GeoCountry } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/achievements/new');
	}

	const ctx = apiContext(event);

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	return { countries };
};
