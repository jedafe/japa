import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { GeoCountry, GeoSpaceOption, EcaProvider, EcaGuide } from '$lib/types';

/**
 * ECA Evaluation Guide at /tools/eca-guide (§109, ADR-120).
 * SSR loads the destination list and providers. Guides are fetched
 * client-side once a destination is selected — with SSR pre-fetch when
 * ?dest= is present for deep links.
 *
 * API: GET /geo/countries — destination selector.
 *      GET /eca/providers — provider directory (public).
 *      GET /eca/guides?dest=&purpose=&provider= — guide list (public).
 *      GET /eca/guides/:id — guide detail + community updates (public).
 *      POST /eca/guides/:id/updates — community update (auth).
 *
 * Ref: japaconnect-engineering/sections/109-eca-guide.md, ADR-042 (flag → 404).
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['eca_guide.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'ECA Evaluation Guide is not available' });
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

	// Optional SSR pre-fetch of guides when ?dest= is present — improves
	// first paint for deep links. The id is validated against the loaded
	// destination list so the selector reflects the active filter.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	// Provider directory + dropdown options — destination-scoped when a
	// deep link pre-selects a destination (GET /eca/providers?dest=).
	const [providers, guides] = await Promise.all([
		apiList<EcaProvider>(
			ctx,
			initialDestId ? `/eca/providers?dest=${initialDestId}` : '/eca/providers'
		)
			.then((r) => r.items)
			.catch(() => [] as EcaProvider[]),
		initialDestId
			? apiList<EcaGuide>(ctx, `/eca/guides?dest=${initialDestId}`)
					.then((r) => r.items)
					.catch(() => [] as EcaGuide[])
			: Promise.resolve([] as EcaGuide[])
	]);

	return { destinations, providers, guides, initialDestId, authed };
};
