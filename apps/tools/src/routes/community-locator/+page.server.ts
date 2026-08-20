import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { CommunityOrgWithAi, GeoSpaceOption, OrgTypeOption } from '$lib/types';

/**
 * Diaspora Community Locator — §101, ADR-088.
 * SSR loads the destination country list from GET /geo/countries (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the community orgs for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /community-locator/org-types — org-type allow-list (Rule 16).
 *      GET /community-locator?dest={space_id} — community orgs for a dest.
 *      GET /community-locator/{slug} — single org (client-side, on demand).
 *      POST /community-locator — submit a new org (auth, client-side).
 *      POST /community-locator/{id}/claim — claim an org (auth, client-side).
 *
 * Ref: §101 (Diaspora Community Locator), ROADMAP-TOOLS 02.06, ADR-088.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when community_locator.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('community-locator')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			orgTypes: [],
			orgs: [],
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

	// BUG-005: org-type allow-list from the backend (Rule 16).
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

	return {
		enabled: true,
		tool,
		destinations,
		orgTypes,
		orgs,
		initialDestId,
		user: event.locals.user
	};
};
