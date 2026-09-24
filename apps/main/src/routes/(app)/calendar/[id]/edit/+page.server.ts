// Ref: japaconnect-engineering/sections/35-calendar.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { CalendarEntry, GeoCountry } from '$lib/types';

/**
 * Edit-calendar form at /calendar/[id]/edit — auth-gated, author-or-edit_any.
 * Loads the entry by id and the country/category lists for the form.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const { features } = await event.parent();
	if (!features['calendar.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Community calendar is not available' });
	}

	const ctx = apiContext(event);
	const id = event.params.id;

	let entry: CalendarEntry;
	try {
		entry = await apiGet<CalendarEntry>(ctx, `/calendar/${id}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Calendar entry not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load calendar entry' });
	}

	const canEdit =
		entry.submitted_by === event.locals.user.id ||
		event.locals.user.permissions?.includes('calendar.edit_own') ||
		event.locals.user.permissions?.includes('calendar.edit_any');
	if (!canEdit) {
		throw error(403, { code: 'not_owner', message: 'You can only edit your own calendar entries' });
	}

	const [countriesRes, categoriesRes] = await Promise.allSettled([
		apiList<GeoCountry>(ctx, '/geo/countries'),
		apiGet<{ value: string; label: string }[]>(ctx, '/calendar/categories')
	]);

	const countries = countriesRes.status === 'fulfilled' ? countriesRes.value.items : [];
	let categories: { value: string; label: string }[] = [];
	if (categoriesRes.status === 'fulfilled' && categoriesRes.value) {
		categories = categoriesRes.value;
	}

	return { entry, countries, categories, user: event.locals.user, features };
};
