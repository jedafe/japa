import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { HousingGuideWithAi, GeoSpaceOption } from '$lib/types';

/**
 * Housing Guide — §96, ADR-088.
 * SSR loads the destination country list from /spaces?level=country (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the housing guides for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * API: GET /spaces?level=country — destination countries (geo selector).
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
	let destinations: GeoSpaceOption[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'housing_guide_disabled')) {
			disabled = true;
		}
		destinations = [];
	}

	// If ?dest= is present, fetch housing guides for that destination so the
	// page renders with content on first paint (SSR). Otherwise the user picks
	// a destination client-side.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	let guides: HousingGuideWithAi[] = [];
	if (initialDestId) {
		try {
			guides = (
				await apiList<HousingGuideWithAi>(ctx, `/housing-guide?dest=${initialDestId}`)
			).items;
		} catch {
			guides = [];
		}
	}

	return {
		destinations,
		guides,
		initialDestId,
		disabled,
		authed
	};
};
