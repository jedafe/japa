import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { VisaRouteWithAi, GeoCountry } from '$lib/types';

/**
 * Visa Route Comparison — §90, ADR-088.
 * SSR loads the full 250 primary geo space countries from GET /geo/countries (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the routes for that destination are fetched server-side
 * so the page renders with content immediately.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /visa-routes?dest={space_id} — visa routes for a destination.
 *      GET /visa-routes/compare?ids=... — comparison (client-side, on demand).
 *
 * Route: /tools/visa-routes
 * Ref: §90 (Visa Route Comparison), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['visa_routes.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Visa Route Comparison is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination list — the geo-aware selector source (Rule 16).
	// A 404 or visa_routes_disabled code means the tool is turned off.
	let destinations: GeoCountry[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'visa_routes_disabled')) {
			disabled = true;
		}
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
		destinations,
		routes,
		initialDestId,
		disabled,
		authed
	};
};
