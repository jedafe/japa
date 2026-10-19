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

	const [countries, categories] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		// BUG-009: fetch categories from the API (calendar.categories system_setting).
		apiGet<{ value: string; label: string }[]>(ctx, '/calendar/categories').catch(
			() => [] as { value: string; label: string }[]
		)
	]);

	return { countries, categories };
};
