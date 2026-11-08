// Ref: japaconnect-engineering/sections/80-destination-wiki.md
import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '#lib/server/api.js';
import type { WikiPageIndex, SpaceDetail, WikiSpaceRef } from '#lib/types/index.js';

type WikiApiCategory = {
	slug: string;
	label_key: string;
	label: string;
	/** §80 lazy template pages — declared sibling links ({name}
	 *  substituted server-side); present only on ?space= requests. */
	page_templates?: { slug: string; label: string; position: number }[];
	/** Stub page slug for /wiki/{space}/{page_slug} links. */
	page_slug?: string;
	/** §80 directories — the stub page is the child-space listing. */
	directory?: boolean;
	/** §80 directories — true when the space has zero matching
	 *  descendants; the tab/card is hidden. */
	empty?: boolean;
	/** §80 category icon — admin-configured lucide name. */
	icon?: string;
	/** The category's bound field group — the index scopes its fields
	 *  panel to the overview category's group. */
	field_group_id?: string;
	field_group_slug?: string;
};
/** §80 Hierarchical UX — the index envelope gains ancestors[] alongside
 *  the page list for the space-level breadcrumb, and ancestors_alt[]
 *  carrying the space's other ("also under") parent chains for
 *  multi-parent nodes. */
type WikiIndexEnvelope = {
	data: WikiPageIndex[] | null;
	/** §43 — false when every page in the index is a stub (no approved
	 *  prose revision); the index page emits noindex,follow. */
	indexable?: boolean;
	ancestors?: WikiSpaceRef[];
	ancestors_alt?: WikiSpaceRef[];
	/** Entity-bound wiki children keyed by type slug (JCE ↔ wiki mapping) —
	 *  a country's visa routes/banks/embassies, a university's courses. */
	entity_children?: Record<
		string,
		{ id: string; slug: string; name: string; discriminator?: string; indexable?: boolean }[]
	>;
	/** §80 attractions — the space's `kind` select value for the hero
	 *  chip (e.g. "waterfall", "museum"). */
	kind?: string;
};

/**
 * Wiki index for a space at /wiki/{space_slug} — lists all wiki pages for the
 * space, grouped by category on the client. The backend returns the page
 * summaries (no content) via GET /wiki/{space_slug}.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const spaceSlug = event.params.space_slug;
	const authed = !!event.locals.user;
	const userPermissions = event.locals.user?.permissions ?? [];
	// Redirects from /spaces/[slug] for wiki-only spaces carry ?from_space=1
	// so the index can explain why there is no community feed here.
	const fromSpace = event.url.searchParams.get('from_space') === '1';

	try {
		// All three calls depend only on space_slug — one wave. The index
		// fetch stays the 404 gate; detail and categories fail soft.
		const idxPromise = apiGetEnvelope<WikiPageIndex[]>(
			ctx,
			`/wiki/${encodeURIComponent(spaceSlug)}`
		);
		const spaceDetailPromise = apiGet<SpaceDetail>(
			ctx,
			`/spaces/${encodeURIComponent(spaceSlug)}`
		).catch(() => null);
		// Scoped category set + scope flag — university spaces get the
		// university categories and a university search input (§80 amendment).
		const catPromise = apiGetEnvelope<WikiApiCategory[]>(
			ctx,
			`/wiki/categories?space=${encodeURIComponent(spaceSlug)}`
		).catch(() => null);

		const idx = await idxPromise;
		const indexEnvelope = idx as unknown as WikiIndexEnvelope;
		const items = indexEnvelope.data ?? [];
		const spaceDetail = await spaceDetailPromise;
		const catRes = await catPromise;
		// §43 crawl gate — an all-stub index is thin content: keep it out
		// of the index but let crawlers follow its links so non-stub
		// children and sibling destinations stay discoverable.
		if (indexEnvelope.indexable === false) {
			event.setHeaders({ 'x-robots-tag': 'noindex, follow' });
		}
		// §wiki-seo guest surface — an index with no indexable prose page
		// AND no live directory shows a "coming soon" empty state to guests
		// rather than erroring; members still see the contribution hub.
		const apiCats = catRes?.data ?? [];
		const hasLiveDirectory = apiCats.some((c) => c.directory && !c.empty);
		const guestEmpty = !authed && indexEnvelope.indexable === false && !hasLiveDirectory;
		// §16 explore_map — state/city spaces carry a *local* geo_code ("AD"),
		// not a full ISO-3166-2 ("NG-AD"). Resolve ancestor geo_codes so the
		// wiki map can pin the admin-1 unit, highlight the parent country, and
		// fit the right zoom level (state → country bounds, city → state bounds).
		let countryGeoCode: string | null = null;
		let stateGeoCode: string | null = null;
		if (spaceDetail?.space) {
			const level = spaceDetail.space.level;
			const resolveAncestor = async (lvl: string) => {
				const anc = indexEnvelope.ancestors?.find((a) => a.level === lvl);
				if (!anc) return null;
				const parent = await apiGet<SpaceDetail>(
					ctx,
					`/spaces/${encodeURIComponent(anc.slug)}`
				).catch(() => null);
				return (parent?.space?.geo_code ?? '').toUpperCase() || null;
			};
			if (level === 'country') {
				countryGeoCode = (spaceDetail.space.geo_code ?? '').toUpperCase() || null;
			} else if (level === 'state') {
				stateGeoCode = (spaceDetail.space.geo_code ?? '').toUpperCase() || null;
				countryGeoCode = await resolveAncestor('country');
			} else {
				[countryGeoCode, stateGeoCode] = await Promise.all([
					resolveAncestor('country'),
					resolveAncestor('state')
				]);
			}
		}
		// Guests get the public surface only: stub page links and
		// non-indexable entity children are stripped server-side so no
		// rendered link can dead-end at the 404 gates.
		const visibleItems = authed ? items : items.filter((p) => p.indexable !== false);
		const entityChildren = indexEnvelope.entity_children ?? {};
		const visibleEntityChildren = authed
			? entityChildren
			: Object.fromEntries(
					Object.entries(entityChildren)
						.map(([k, list]) => [k, list.filter((c) => c.indexable !== false)] as const)
						.filter(([, list]) => list.length > 0)
				);
		return {
			spaceSlug,
			index: visibleItems,
			indexable: indexEnvelope.indexable ?? true,
			ancestors: indexEnvelope.ancestors ?? [],
			ancestorsAlt: indexEnvelope.ancestors_alt ?? [],
			entityChildren: visibleEntityChildren,
			spaceKind: indexEnvelope.kind ?? '',
			authed,
			isGuest: !authed,
			guestEmpty,
			userPermissions,
			loadError: false,
			fromSpace,
			spaceDetail,
			countryGeoCode,
			stateGeoCode,
			apiCategories: apiCats,
			wikiScope:
				catRes?.scope === 'university' || catRes?.scope === 'attraction'
					? catRes.scope
					: 'geo'
		};
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			error(404, { code: 'wiki_not_found', message: 'Wiki not found for this destination' });
		}
		event.setHeaders({ 'x-robots-tag': 'noindex, nofollow' });
		return {
			spaceSlug,
			index: [] as WikiPageIndex[],
			ancestors: [] as WikiSpaceRef[],
			ancestorsAlt: [] as WikiSpaceRef[],
			entityChildren: {},
			spaceKind: '',
			authed,
			userPermissions,
			loadError: true,
			fromSpace,
			spaceDetail: null,
			countryGeoCode: null,
			stateGeoCode: null,
			apiCategories: [] as WikiApiCategory[],
			wikiScope: 'geo' as const
		};
	}
};
