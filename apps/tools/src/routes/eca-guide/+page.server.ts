import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, EcaProvider, EcaGuide } from '$lib/types';

/**
 * ECA Evaluation Guide — §109, ADR-120.
 * SSR loads the destination list and providers. Guides are fetched
 * client-side once a destination (and optional purpose/provider) is
 * selected — with SSR pre-fetch when ?dest= is present for deep links.
 *
 * API: GET /geo/countries — destination selector.
 *      GET /eca/providers?dest= — provider directory (public).
 *      GET /eca/guides?dest=&purpose=&provider= — guide list (public).
 *      GET /eca/guides/:id — guide detail + community updates (public).
 *      GET /eca/compare?dest=&provider_ids= — comparison (public).
 *      POST /eca/guides/:id/updates — community update (auth).
 *
 * Ref: §109 (ECA Guide), ROADMAP-TOOLS 06.03, ADR-042 (flag → 404).
 */
export const load: PageServerLoad = async (event) => {
	const tool = getTool('eca-guide')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			providers: [],
			guides: [],
			initialDestId: null,
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = await loadCountries(ctx);
	} catch {
		destinations = [];
	}

	// Optional SSR pre-fetch of guides when ?dest= is present. The id is
	// validated against the loaded destination list so the selector
	// reflects the active filter on deep links.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	// Provider directory + dropdown options — destination-scoped when a
	// deep link pre-selects a destination (GET /eca/providers?dest=).
	let providers: EcaProvider[] = [];
	try {
		const path = initialDestId ? `/eca/providers?dest=${initialDestId}` : '/eca/providers';
		providers = (await apiList<EcaProvider>(ctx, path)).items;
	} catch {
		providers = [];
	}

	let guides: EcaGuide[] = [];
	if (initialDestId) {
		try {
			guides = (await apiList<EcaGuide>(ctx, `/eca/guides?dest=${initialDestId}`)).items;
		} catch {
			guides = [];
		}
	}

	return {
		enabled: true,
		tool,
		destinations,
		providers,
		guides,
		initialDestId,
		user: event.locals.user
	};
};
