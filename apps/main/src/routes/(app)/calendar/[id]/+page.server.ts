// Ref: japaconnect-engineering/sections/86-community-calendar.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { CalendarEntry } from '$lib/types';

/**
 * Calendar entry detail at /calendar/{id} — SSR. Shows the full entry with
 * description, category, dates, destination/origin, source_url, recurrence,
 * and a status badge. On 404, throws a SvelteKit 404.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['calendar.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Community Calendar is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const id = event.params.id;
	let entry: CalendarEntry;
	try {
		entry = await apiGet<CalendarEntry>(ctx, `/calendar/${id}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Calendar entry not found' });
		}
		return { entry: null, authed, loadError: true, canEdit: false, categories: [] };
	}

	// The author (or anyone holding calendar.edit_any) may edit — mirrors the
	// service-layer rule on PATCH /calendar/:id.
	const user = event.locals.user;
	const canEdit =
		authed &&
		(entry.submitted_by === user?.id ||
			!!user?.permissions?.includes('calendar.edit_any'));

	let categories: { value: string; label: string }[] = [];
	try {
		categories = await apiGet<{ value: string; label: string }[]>(ctx, '/calendar/categories');
	} catch {
		categories = [];
	}

	return { entry, authed, loadError: false, canEdit, categories };
};
