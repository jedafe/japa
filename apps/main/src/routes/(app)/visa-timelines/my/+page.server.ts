// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { VisaTimeline, GeoCountry } from '$lib/types';

/**
 * My Visa Timelines at /visa-timelines/my (Section 83).
 * Auth-gated list of the current user's visa application timelines with
 * milestones. The backend returns the timelines with joined milestones.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}
	const ctx = apiContext(event);

	let timelines: VisaTimeline[] = [];
	let loadError = false;
	try {
		timelines = (await apiList<VisaTimeline>(ctx, '/me/visa-timelines')).items;
	} catch {
		loadError = true;
	}

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	return { timelines, loadError, countries };
};
