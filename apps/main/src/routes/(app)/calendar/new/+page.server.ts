// Ref: japaconnect-engineering/sections/86-community-calendar.md
import type { PageServerLoad } from './$types';
import { error, redirect } from '@sveltejs/kit';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { GeoCountry } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}

	const { features } = await event.parent();
	if (!features['calendar.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Community Calendar is not available' });
	}

	const ctx = apiContext(event);

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	// BUG-009: fetch categories from the API (calendar.categories system_setting).
	let categories: { value: string; label: string }[] = [];
	try {
		categories = await apiGet<{ value: string; label: string }[]>(ctx, '/calendar/categories');
	} catch {
		categories = [];
	}

	return { countries, categories };
};
