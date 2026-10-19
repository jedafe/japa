import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { GeoCountry, CvStandard, CatalogOption } from '$lib/types';

/**
 * CV Formatter — §95.
 * SSR loads the full 250 primary geo space countries and all CV standards. The standards
 * list is used to filter the destination dropdown to only show destinations
 * that have a standard available.
 *
 * API: GET /geo/countries — destination selector options (the full 250 primary geo space countries).
 *      GET /cv-formatter/standards — all CV standards (public).
 *      GET /cv-formatter/standards/{dest} — single standard for a destination.
 *      POST /cv-formatter/format — AI-assisted formatting (auth, 202 Accepted).
 *      GET /cv-formatter/sessions/{id} — poll session status (auth, owner).
 *      GET /cv-formatter/sessions/{id}/download — binary download (auth, owner).
 *
 * Ref: §95 (CV Formatter), ROADMAP-Tools 02.06, ADR-088.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['cv_formatter.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'CV Formatter is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let disabled = false;
	let destinations: GeoCountry[] = [];
	let standards: CvStandard[] = [];

	// All CV standards — primary call; a 404 with cv_formatter_disabled (or any
	// 404) means the tool is turned off. Also used to filter the destination
	// dropdown to only show destinations that have a standard available, and to
	// display the standard inline when a destination is selected.
	const [standardsRes, formatOptions] = await Promise.all([
		apiList<CvStandard>(ctx, '/cv-formatter/standards')
			.then((r) => r.items)
			.catch((e) => {
				if (e instanceof ApiError && (e.code === 'cv_formatter_disabled' || e.status === 404)) {
					disabled = true;
				}
				// Other errors: leave standards empty; page shows an empty state.
				return [] as CvStandard[];
			}),
		apiGet<CatalogOption[]>(ctx, '/catalogs/cv_formatter.format').catch((e) => {
			console.error('failed to load CV formatter format catalog:', e);
			return [] as CatalogOption[];
		})
	]);
	standards = standardsRes;

	// Destination country list — the primary data axis (ADR-088). Rule 16.
	if (!disabled) {
		try {
			destinations = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
		} catch {
			destinations = [];
		}
	}

	return { destinations, standards, disabled, authed, formatOptions };
};
