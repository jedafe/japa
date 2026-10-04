import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { CommunityOrgWithAi, GeoCountry, OrgTypeOption } from '$lib/types';

/**
 * Diaspora Community Locator — §101, ADR-088.
 * SSR loads the full 250 primary geo space countries from GET /geo/countries (the
 * geo-aware pattern — destination is the primary data axis). When a ?dest=
 * param is present, the community orgs for that destination are fetched
 * server-side so the page renders with content immediately.
 *
 * List filters (?type=, ?origin=, ?city=) and ?page= are forwarded to the
 * backend so the public list is fully server-filtered and paginated.
 *
 * The feature flag (community_locator.enabled) is read from the root
 * layout's features map (data.features). Disabled tools are hidden from the
 * /tools hub and direct navigation to this route returns a 404.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /community-locator/org-types — org-type allow-list (Rule 16).
 *      GET /community-locator?dest={space_id}&type=&origin=&city=&page=&limit= — community orgs.
 *      GET /community-locator/{slug} — single org (client-side, on demand).
 *      POST /community-locator — submit a new org (auth, client-side).
 *      POST /community-locator/{id}/claim — claim an org (auth, client-side).
 *
 * Ref: §101 (Diaspora Community Locator), ROADMAP-TOOLS 02.06, ADR-088.
 * Route: /tools/community-locator
 */

const ORG_PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Check the feature flag via the root layout's features map.
	// The /spaces endpoint is not gated by community_locator, so checking
	// its 404 would never set disabled=true. Read the flag directly instead.
	const { features } = await event.parent();
	if (!features?.['community_locator.enabled']) {
		throw error(404, {
			code: 'feature_disabled',
			message: 'Diaspora Community Locator is not available'
		});
	}

	// Destination list — the geo-aware selector source (Rule 16).
	let destinations: GeoCountry[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
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
	// page renders with content on first paint (SSR). Filters (?type=,
	// ?origin=, ?city=) and ?page= are forwarded to the backend.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	const initialType = event.url.searchParams.get('type') ?? '';
	const initialOrigin = event.url.searchParams.get('origin') ?? '';
	const initialCity = event.url.searchParams.get('city') ?? '';
	const pageParam = Number.parseInt(event.url.searchParams.get('page') ?? '1', 10);
	const initialPage = Number.isFinite(pageParam) && pageParam > 0 ? pageParam : 1;

	let orgs: CommunityOrgWithAi[] = [];
	let orgsTotal = 0;
	if (initialDestId) {
		try {
			const qs = new URLSearchParams({
				dest: initialDestId,
				limit: String(ORG_PAGE_SIZE),
				page: String(initialPage)
			});
			if (initialType) qs.set('type', initialType);
			if (initialOrigin) qs.set('origin', initialOrigin);
			if (initialCity) qs.set('city', initialCity);
			const res = await apiList<CommunityOrgWithAi>(ctx, `/community-locator?${qs}`);
			orgs = res.items;
			orgsTotal = res.meta?.total ?? res.items.length;
		} catch {
			orgs = [];
		}
	}

	return {
		destinations,
		orgTypes,
		orgs,
		orgsTotal,
		initialDestId,
		initialType,
		initialOrigin,
		initialCity,
		initialPage,
		disabled,
		authed
	};
};
