import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { Embassy, GeoSpaceOption } from '$lib/types';

/**
 * Embassy Appointment Alerter — §91, ADR-099.
 * SSR loads the destination country list from /spaces?level=country (the
 * geo-aware pattern). When ?dest= is present, embassies for that destination
 * are fetched server-side. User-specific alerts are fetched client-side on
 * mount (not in SSR) to avoid leaking user data across cached responses.
 *
 * API: GET /spaces?level=country — destination countries (geo selector).
 *      GET /embassies?dest={space_id} — embassies for a destination (public).
 *      GET /me/appointment-alerts — user's alerts (auth, client-side only).
 *      POST/PATCH/DELETE /me/appointment-alerts — mutations (client-side).
 *
 * Ref: §91 (Embassy Appointment Alerter), ROADMAP-TOOLS 02.06.
 * Route: /tools/embassy-alerter
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let destinations: GeoSpaceOption[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'appointment_alert_disabled')) {
			disabled = true;
		}
		// Other errors: leave destinations empty; page shows an empty state.
	}

	// If ?dest= is present, fetch embassies for that destination (SSR).
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	let embassies: Embassy[] = [];
	if (initialDestId) {
		try {
			embassies = (await apiList<Embassy>(ctx, `/embassies?dest=${initialDestId}`)).items;
		} catch {
			embassies = [];
		}
	}

	// User alerts are NOT fetched in SSR — they are loaded client-side on
	// mount to avoid leaking user-specific data across cached SSR responses.
	// Ref: jc-tools-audit checklist §4 (SSR/CSR boundary).

	return { destinations, embassies, initialDestId, disabled, authed };
};
