import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { Embassy, GeoCountry, CatalogOption } from '$lib/types';

/**
 * Embassy Appointment Alerter — §91, ADR-099.
 * SSR loads the full 250 primary geo space countries from GET /geo/countries (the
 * geo-aware pattern). When ?dest= is present, embassies for that destination
 * are fetched server-side. User-specific alerts are fetched client-side on
 * mount (not in SSR) to avoid leaking user data across cached responses.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /embassies?dest={space_id} — embassies for a destination (public).
 *      GET /me/appointment-alerts — user's alerts (auth, client-side only).
 *      POST/PATCH/DELETE /me/appointment-alerts — mutations (client-side).
 *
 * Ref: §91 (Embassy Appointment Alerter), ROADMAP-TOOLS 02.06.
 * Route: /tools/embassy-alerter
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['appointment_alert.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Embassy Appointment Alerter is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let destinations: GeoCountry[] = [];
	let disabled = false;
	let loadError: string | null = null;
	try {
		destinations = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'appointment_alert_disabled')) {
			disabled = true;
		} else if (e instanceof ApiError) {
			loadError = e.message;
		} else if (e instanceof Error) {
			loadError = e.message;
		} else {
			loadError = 'Failed to load destinations.';
		}
	}

	// If ?dest= is present, fetch embassies for that destination (SSR).
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	let embassies: Embassy[] = [];
	if (initialDestId) {
		try {
			embassies = (await apiList<Embassy>(ctx, `/embassies?dest=${initialDestId}`)).items;
		} catch (e) {
			embassies = [];
			if (e instanceof ApiError) {
				loadError = loadError ? `${loadError} ${e.message}` : e.message;
			} else if (e instanceof Error) {
				loadError = loadError ? `${loadError} ${e.message}` : e.message;
			} else {
				loadError = loadError ? `${loadError} Failed to load embassies.` : 'Failed to load embassies.';
			}
		}
	}

	// User alerts are NOT fetched in SSR — they are loaded client-side on
	// mount to avoid leaking user-specific data across cached SSR responses.
	// Ref: jc-tools-audit checklist §4 (SSR/CSR boundary).

	// Pass the user's effective permissions so the page can gate premium-only
	// UI (e.g. the SMS channel checkbox) on the appointment_alert.premium
	// permission node (§91, ADR-099, Rule 16).
	const userPermissions = event.locals.user?.permissions ?? [];

	let channelOptions: CatalogOption[] = [];
	try {
		channelOptions = await apiGet<CatalogOption[]>(
			ctx,
			'/catalogs/appointment_alert.notification_channels'
		);
	} catch (e) {
		console.error('failed to load appointment alert notification channel catalog:', e);
	}

	return { destinations, embassies, initialDestId, disabled, authed, userPermissions, channelOptions, loadError };
};
