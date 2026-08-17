import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { CommunityOrgWithAi, GeoSpaceOption, OrgTypeOption } from '$lib/types';

/**
 * Diaspora Community Locator — §101, ADR-088.
 * SSR loads the destination country list from /spaces?level=country (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the community orgs for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * The feature flag (community_locator.enabled) is checked via the root
 * layout's siteSettings so the page can show a "tool unavailable" state
 * without making API calls to a disabled endpoint (BUG-014).
 *
 * API: GET /spaces?level=country — destination countries (geo selector).
 *      GET /community-locator/org-types — org-type allow-list (Rule 16).
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

	// Check the feature flag via the root layout's siteSettings (BUG-014).
	// The /spaces endpoint is not gated by community_locator, so checking
	// its 404 would never set disabled=true. Read the flag directly instead.
	const { siteSettings } = await event.parent();
	const enabled = siteSettings?.['community_locator.enabled'] === true;
	if (!enabled) {
		return { destinations: [], orgTypes: [], orgs: [], initialDestId: null, disabled: true, authed };
	}

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

	// BUG-005: org-type allow-list from the backend (Rule 16). Falls back
	// to an empty list on error; the page renders an empty select.
	let orgTypes: OrgTypeOption[] = [];
	try {
		orgTypes = (await apiList<OrgTypeOption>(ctx, '/community-locator/org-types')).items;
	} catch {
		orgTypes = [];
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

	return { destinations, orgTypes, orgs, initialDestId, disabled, authed };
};
