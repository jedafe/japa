// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { Mentor, GeoCountry } from '$lib/types';

/**
 * Mentor profile at /mentors/{username} — SSR-fetches GET /mentors/{username}.
 * Feature-flagged via `mentorship.enabled` (§102): 404 when disabled.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['mentorship.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const username = event.params.username;

	let countries: GeoCountry[] = [];
	try {
		countries = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch {
		countries = [];
	}

	try {
		const mentor = await apiGet<Mentor>(ctx, `/mentors/${encodeURIComponent(username)}`);
		return { mentor, authed, countries, loadError: false };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			throw error(404, { code: 'mentor_not_found', message: 'Mentor not found' });
		}
		return { mentor: null, authed, countries, loadError: true };
	}
};
