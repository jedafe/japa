import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { VisaRouteWithAi, GeoSpaceOption } from '$lib/types';

/**
 * Visa Route Comparison — §90, ADR-088.
 * SSR loads the destination country list from /spaces?level=country (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the routes for that destination are fetched server-side
 * so the page renders with content immediately.
 *
 * API: GET /spaces?level=country — destination countries (geo selector).
 *      GET /visa-routes?dest={space_id} — visa routes for a destination.
 *      GET /visa-routes/compare?ids=... — comparison (client-side, on demand).
 *
 * Ref: §90 (Visa Route Comparison), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when visa_routes.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('visa-routes')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, settings)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			routes: [],
			initialDestId: null,
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination list — the geo-aware selector source (Rule 16).
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch {
		destinations = [];
	}

	// If ?dest= is present, fetch routes for that destination so the page
	// renders with content on first paint (SSR). Otherwise the user picks a
	// destination client-side.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId = destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	let routes: VisaRouteWithAi[] = [];
	if (initialDestId) {
		try {
			routes = (await apiList<VisaRouteWithAi>(ctx, `/visa-routes?dest=${initialDestId}`)).items;
		} catch {
			routes = [];
		}
	}

	return {
		enabled: true,
		tool,
		destinations,
		routes,
		initialDestId,
		user: event.locals.user
	};
};
