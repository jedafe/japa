import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { HousingGuideWithAi, GeoSpaceOption } from '$lib/types';

/**
 * Housing Guide — §96, ADR-088.
 * SSR loads the destination country list from GET /geo/countries (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the housing guides for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /housing-guide?dest={space_id} — housing guides for a destination.
 *      GET /housing-guide/{id} — single housing guide detail.
 *
 * Ref: §96 (Housing Guide), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when housing_guide.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('housing-guide')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			guides: [],
			initialDestId: null,
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
		enabled: true,
		tool,
		destinations,
		guides,
		initialDestId,
		user: event.locals.user
	};
};
