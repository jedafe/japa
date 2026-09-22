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

	let providers: EcaProvider[] = [];
	try {
		providers = (await apiList<EcaProvider>(ctx, '/eca/providers')).items;
	} catch {
		providers = [];
	}

	// Optional SSR pre-fetch of guides when ?dest= is present.
	let guides: EcaGuide[] = [];
	const destParam = event.url.searchParams.get('dest');
	if (destParam) {
		try {
			guides = (await apiList<EcaGuide>(ctx, `/eca/guides?dest=${destParam}`)).items;
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
		user: event.locals.user
	};
};
