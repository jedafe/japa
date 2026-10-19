import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { GeoCountry, CatalogOption } from '$lib/types';

/**
 * Tax Calculator — §100, ADR-088.
 * SSR loads the full 250 primary geo space countries from GET /geo/countries (the
 * geo-aware pattern). No initial tax data is fetched — the tool requires both
 * an origin and a destination selection before querying the backend.
 *
 * API: GET /geo/countries — destination + origin countries (the full 250 primary geo space countries).
 *      GET /tax-calculator?dest={space_id}&origin={space_id}&employment={status}
 *          — tax guides for an origin→destination pair.
 *      GET /tax-calculator/treaty?dest={space_id}&origin={space_id}
 *          — double taxation treaty info.
 *
 * Ref: §100 (Tax Calculator), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['tax_calculator.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Tax Calculator is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination list — used for BOTH origin and destination selectors
	// (Rule 16 — the country list comes from the backend API, never hardcoded).
	let destinations: GeoCountry[] = [];
	let disabled = false;
	const [destinationsRes, employmentOptions] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch((e) => {
				if (e instanceof ApiError && (e.status === 404 || e.code.endsWith('_disabled'))) {
					disabled = true;
				}
				return [] as GeoCountry[];
			}),
		apiGet<CatalogOption[]>(ctx, '/catalogs/tax_calculator.employment_status').catch((e) => {
			// Non-fatal: fall back to an empty list; the UI can show a plain text input.
			console.error('failed to load tax calculator employment catalog:', e);
			return [] as CatalogOption[];
		})
	]);
	destinations = destinationsRes;

	return { destinations, disabled, authed, employmentOptions };
};
