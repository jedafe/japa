import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCatalog } from '$lib/server/catalog';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { Embassy, GeoSpaceOption, CatalogOption } from '$lib/types';

/**
 * Embassy Appointment Alerter — §91, ADR-099.
 * SSR loads the destination country list from GET /geo/countries (the
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
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when appointment_alert.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('embassy-alerter')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			embassies: [],
			initialDestId: null,
			channelOptions: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination list — the geo-aware selector source (Rule 16).
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = await loadCountries(ctx);
	} catch {
		destinations = [];
	}

	// If ?dest= is present, fetch embassies for that destination (SSR).
	const destParam = event.url.searchParams.get('dest');
	const initialDestId = destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

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

	// Notification channel options from the public catalog (Rule 16).
	// The catalog lists all canonical channels; UI gating for premium SMS
	// happens client-side via the user capabilities check.
	let channelOptions: CatalogOption[] = [];
	try {
		channelOptions = await loadCatalog(ctx, 'appointment_alert.notification_channels');
	} catch {
		channelOptions = [];
	}

	return {
		enabled: true,
		tool,
		destinations,
		embassies,
		initialDestId,
		channelOptions,
		user: event.locals.user
	};
};
