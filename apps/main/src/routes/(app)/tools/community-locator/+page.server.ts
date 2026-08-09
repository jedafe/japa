import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { CommunityOrgWithAi, GeoSpaceOption } from '$lib/types';

/**
 * Diaspora Community Locator — §101, ADR-088.
 * SSR loads the destination country list from /spaces?level=country (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the community orgs for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * API: GET /spaces?level=country — destination countries (geo selector).
 *      GET /community-locator?dest={space_id} — community orgs for a dest.
 *      GET /community-locator/{slug} — single org (client-side, on demand).
 *      POST /community-locator — submit a new org (auth, client-side).
 *      POST /community-locator/{id}/claim — claim an org (auth, client-side).
 *
 * Ref: §101 (Diaspora Community Locator), ROADMAP-TOOLS 02.06, ADR-088.
 * Route: /tools/community-locator
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination list — the geo-aware selector source (Rule 16).
	let destinations: GeoSpaceOption[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'community_locator_disabled')) {
			disabled = true;
		}
		// Other errors: leave destinations empty; page shows an empty state.
	}

	// If ?dest= is present, fetch community orgs for that destination so the
	// page renders with content on first paint (SSR). Otherwise the user picks
	// a destination client-side.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	let orgs: CommunityOrgWithAi[] = [];
	if (initialDestId) {
		try {
			orgs = (await apiList<CommunityOrgWithAi>(ctx, `/community-locator?dest=${initialDestId}`))
				.items;
		} catch {
			orgs = [];
		}
	}

	return { destinations, orgs, initialDestId, disabled, authed };
};
