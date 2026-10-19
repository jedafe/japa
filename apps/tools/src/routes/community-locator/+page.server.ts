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
 * List filters (?type=, ?origin=, ?city=) and ?page= are forwarded to the
 * backend so the public list is fully server-filtered and paginated.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /community-locator/org-types — org-type allow-list (Rule 16).
 *      GET /community-locator?dest={space_id}&type=&origin=&city=&page=&limit= — community orgs.
 *      GET /community-locator/{slug} — single org (client-side, on demand).
 *      POST /community-locator — submit a new org (auth, client-side).
 *      POST /community-locator/{id}/claim — claim an org (auth, client-side).
 *
 * Ref: §101 (Diaspora Community Locator), ROADMAP-TOOLS 02.06, ADR-088.
 */

const ORG_PAGE_SIZE = 20;

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
			orgsTotal: 0,
			initialDestId: null,
			initialType: '',
			initialOrigin: '',
			initialCity: '',
			initialPage: 1,
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination list (the geo-aware selector source, Rule 16) and the
	// BUG-005 org-type allow-list — independent catalogs, one wave.
	const [destinations, orgTypes] = await Promise.all([
		loadCountries(ctx).catch(() => [] as GeoSpaceOption[]),
		apiList<OrgTypeOption>(ctx, '/community-locator/org-types')
			.then((r) => r.items)
			.catch(() => [] as OrgTypeOption[])
	]);

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
		enabled: true,
		tool,
		destinations,
		orgTypes,
		orgs,
		orgsTotal,
		initialDestId,
		initialType,
		initialOrigin,
		initialCity,
		initialPage,
		user: event.locals.user
	};
};
