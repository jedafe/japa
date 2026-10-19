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

	const destParam = event.url.searchParams.get('dest') ?? '';
	const visaTypeParam = event.url.searchParams.get('visa_type') ?? '';
	const enc = encodeURIComponent(destParam);

	const [destinations, requirements, visaTypes] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then(({ items }) =>
				items.map((c) => ({ id: c.id, name: c.name, slug: c.slug, code: c.geo_code ?? null }))
			)
			.catch(() => [] as GeoSpaceOption[]),
		destParam
			? apiList<PofRequirement>(ctx, `/pof/requirements?dest=${enc}`)
					.then((r) => r.items)
					.catch(() => [] as PofRequirement[])
			: Promise.resolve([] as PofRequirement[]),
		destParam
			? apiList<string>(ctx, `/pof/visa-types?dest=${enc}`)
					.then((r) => r.items)
					.catch(() => [] as string[])
			: Promise.resolve([] as string[])
	]);

	// initialDestId/initialVisaType restore the shareable-URL selection so
	// the hydrated page matches what the link author saw.
	return {
		destinations,
		requirements,
		visaTypes,
		initialDestId: destParam,
		initialVisaType: visaTypes.includes(visaTypeParam) ? visaTypeParam : ''
	};
};
