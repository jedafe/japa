import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { GeoCountry, GeoSpaceOption, PofRequirement } from '$lib/types';

/**
 * Proof of Funds Calculator at /tools/pof-calculator (§108, ADR-119).
 * SSR loads the destination list and published requirements when ?dest= is
 * present. The calculation runs client-side via GET /pof/calculate.
 *
 * API: GET /geo/countries — destination selector.
 *      GET /pof/requirements?dest= — published requirements (public).
 *      GET /pof/visa-types?dest= — distinct visa types (public).
 *      GET /pof/calculate — requirement resolution + NGN/USD conversion (public).
 *
 * Ref: japaconnect-engineering/sections/108-pof-calculator.md, ADR-042 (flag → 404).
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['pof_calculator.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Proof of Funds Calculator is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	let destinations: GeoSpaceOption[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		destinations = items.map((c) => ({ id: c.id, name: c.name, slug: c.slug, code: c.geo_code ?? null }));
	} catch {
		destinations = [];
	}

	let requirements: PofRequirement[] = [];
	let visaTypes: string[] = [];
	const destParam = event.url.searchParams.get('dest');
	if (destParam) {
		try {
			requirements = (await apiList<PofRequirement>(ctx, `/pof/requirements?dest=${destParam}`)).items;
		} catch {
			requirements = [];
		}
		try {
			visaTypes = (await apiList<string>(ctx, `/pof/visa-types?dest=${destParam}`)).items;
		} catch {
			visaTypes = [];
		}
	}

	return { destinations, requirements, visaTypes, authed };
};
