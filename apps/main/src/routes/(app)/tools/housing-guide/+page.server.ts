import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '#lib/server/api.js';
import type { HousingGuideWithAi, GeoCountry } from '#lib/types/index.js';

/**
 * Housing Guide — §96, ADR-088.
 * SSR loads the full 250 primary geo space countries from GET /geo/countries (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the housing guides for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /housing-guide?dest={space_id} — housing guides for a destination.
 *      GET /housing-guide/{id} — single housing guide detail.
 *
 * Route: /tools/housing-guide
 * Ref: §96 (Housing Guide), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['housing_guide.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Housing Guide is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination list — the geo-aware selector source (Rule 16).
	// A 404 or housing_guide_disabled code means the tool is turned off.
	let destinations: GeoCountry[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'housing_guide_disabled')) {
			disabled = true;
		}
		destinations = [];
	}

	// ?dest= takes a space id; ?dest_geo= takes a geo code — the editorial
	// guide cross-link convention (§97 pattern: guide field_values store
	// geo codes, the tool landing resolves them to space ids). ?type=
	// forwards the accommodation_type filter.
	const destParam = event.url.searchParams.get('dest');
	const destGeoParam = event.url.searchParams.get('dest_geo');
	const typeParam = event.url.searchParams.get('type');
	const initialDestId = destParam && destinations.some((d) => d.id === destParam)
		? destParam
		: (destGeoParam
			? (destinations.find(
					(d) => d.geo_code?.toUpperCase() === destGeoParam.trim().toUpperCase()
				)?.id ?? null)
			: null);
	const initialType = typeParam?.trim().toLowerCase() || null;

	let guides: HousingGuideWithAi[] = [];
	if (initialDestId) {
		try {
			const qs = new URLSearchParams({ dest: initialDestId });
			if (initialType) qs.set('type', initialType);
			guides = (await apiList<HousingGuideWithAi>(ctx, `/housing-guide?${qs}`)).items;
		} catch {
			guides = [];
		}
	}

	return {
		destinations,
		guides,
		initialDestId,
		initialType,
		disabled,
		authed
	};
};
