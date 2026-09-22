import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { GeoSpaceOption, PofRequirement } from '$lib/types';

/**
 * Proof of Funds Calculator — §108, ADR-119.
 * SSR loads the destination list and the published requirements for the
 * default destination when ?dest= is present. The calculation itself runs
 * client-side via GET /pof/calculate?dest=&visa_type=&members=&dependants=.
 *
 * API: GET /geo/countries — destination selector.
 *      GET /pof/requirements?dest= — published requirements (public).
 *      GET /pof/visa-types?dest= — distinct visa types (public).
 *      GET /pof/calculate — requirement resolution + NGN/USD conversion (public).
 *
 * Ref: §108 (POF Calculator), ROADMAP-TOOLS 06.02, ADR-042 (flag → 404).
 */
export const load: PageServerLoad = async (event) => {
	const tool = getTool('pof-calculator')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			requirements: [],
			visaTypes: [],
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

	// Pre-fetch requirements + visa types when ?dest= is present.
	let requirements: PofRequirement[] = [];
	let visaTypes: string[] = [];
	const destParam = event.url.searchParams.get('dest');
	if (destParam) {
		try {
			requirements = (
				await apiList<PofRequirement>(ctx, `/pof/requirements?dest=${destParam}`)
			).items;
		} catch {
			requirements = [];
		}
		try {
			visaTypes = (await apiList<string>(ctx, `/pof/visa-types?dest=${destParam}`)).items;
		} catch {
			visaTypes = [];
		}
	}

	return {
		enabled: true,
		tool,
		destinations,
		requirements,
		visaTypes,
		user: event.locals.user
	};
};
