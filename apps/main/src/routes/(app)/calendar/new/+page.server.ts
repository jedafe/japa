// Ref: japaconnect-engineering/sections/86-community-calendar.md
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { GeoCountry } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
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
		const resp = await apiGet<{ data: { value: string; label: string }[] }>(ctx, '/calendar/categories');
		categories = resp.data ?? [];
	} catch {
		categories = [];
	}

	return { countries, categories };
};
