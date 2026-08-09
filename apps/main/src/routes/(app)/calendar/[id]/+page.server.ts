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
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const id = event.params.id;
	try {
		const entry = await apiGet<CalendarEntry>(ctx, `/calendar/${id}`);
		return { entry, authed, loadError: false };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Calendar entry not found' });
		}
		return { entry: null, authed, loadError: true };
	}
};
