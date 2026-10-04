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
			initialDestId: '',
			initialVisaType: '',
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
	const destParam = event.url.searchParams.get('dest') ?? '';
	const visaTypeParam = event.url.searchParams.get('visa_type') ?? '';
	if (destParam) {
		const enc = encodeURIComponent(destParam);
		try {
			requirements = (
				await apiList<PofRequirement>(ctx, `/pof/requirements?dest=${enc}`)
			).items;
		} catch {
			requirements = [];
		}
		try {
			visaTypes = (await apiList<string>(ctx, `/pof/visa-types?dest=${enc}`)).items;
		} catch {
			visaTypes = [];
		}
	}

	// initialDestId/initialVisaType restore the shareable-URL selection so
	// the hydrated page matches what the link author saw.
	return {
		enabled: true,
		tool,
		destinations,
		requirements,
		visaTypes,
		initialDestId: destParam,
		initialVisaType: visaTypes.includes(visaTypeParam) ? visaTypeParam : '',
		user: event.locals.user
	};
};
