// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// SSR the landing page directory so search engines see the destination list
// without executing JavaScript.
import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGetEnvelope, ApiError } from '$lib/server/api';
import { resolveGeoContext } from '$lib/server/published-page';
import {
	normalizeLegacyBlocks,
	resolveGeoCopy,
	getGeoTokens,
	type Block
} from '@japaconnect/landing-blocks';
import type { WikiDestination, WikiTypeItem, PageBuilderPage } from '$lib/types';
import { wikiDrillChain } from '$lib/utils/wikiTypeChain';

/**
 * §80 Hierarchical UX + directories: the landing is a root-grid +
 * all-types search. The root grid lists the first root type's spaces
 * (country today) — children live on each parent's directory category
 * pages (/wiki/{country}/states), so the landing needs only the root
 * level and a search that spans every enabled type.
 *
 * §80 Reader UX: ?page= + ?sort= (name|page_count) drive server-side
 * pagination — the page size comes from wiki.destinations_page_limit
 * when no explicit limit is requested.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	const rawPage = parseInt(event.url.searchParams.get('page') ?? '1', 10);
	const pageNo = Number.isFinite(rawPage) && rawPage > 0 ? rawPage : 1;
	const sort = event.url.searchParams.get('sort') === 'page_count' ? 'page_count' : 'name';

	let destinations: WikiDestination[] = [];
	let total = 0;
	let limit = 0;
	let types: WikiTypeItem[] = [];
	let chain: WikiTypeItem[] = [];

	// §16/§80 — optional Page Builder hero: a published page with slug "wiki"
	// renders its blocks above the destination grid (same convention as the
	// "homepage" slug on /). Fetched in parallel; any failure (404 = none,
	// or an upstream error) degrades to the built-in explorer map hero.
	const builderPagePromise = apiGetEnvelope<PageBuilderPage>(ctx, '/pages/wiki')
		.then((envelope) => envelope)
		.catch((err) => {
			if (!(err instanceof ApiError && err.status === 404)) {
				console.error('[WIKI LOAD] /pages/wiki fetch failed:', err);
			}
			return null;
		});

	try {
		const typesRes = await apiList<WikiTypeItem>(ctx, '/wiki/types');
		types = (typesRes.items ?? [])
			.filter((t) => t.enabled !== false)
			.sort((a, b) => a.position - b.position);
		chain = wikiDrillChain(types);

		const rootSlug = chain[0]?.space_type_slug ?? '';
		if (rootSlug) {
			const usp = new URLSearchParams({
				level: rootSlug,
				has_wiki: 'false',
				sort,
				page: String(pageNo)
			});
			const rootDest = await apiList<WikiDestination>(ctx, `/wiki/destinations?${usp.toString()}`);
			destinations = rootDest?.items ?? [];
			total = rootDest?.meta?.total ?? destinations.length;
			limit = rootDest?.meta?.limit ?? 0;
		}
	} catch {
		/* degrade to empty list */
	}

	let builderBlocks: Block[] | null = null;
	let contentCountry: string | undefined;
	const builderEnvelope = await builderPagePromise;
	const rawBlocks = builderEnvelope?.data?.blocks;
	if (Array.isArray(rawBlocks) && rawBlocks.length > 0) {
		const { geoCountry, geoCountryName } = await resolveGeoContext(event);
		const tokens = getGeoTokens(geoCountry, 'en', geoCountryName);
		const literals = builderEnvelope?.literals ?? {};
		const blocks = normalizeLegacyBlocks(rawBlocks) as Block[];
		builderBlocks = resolveGeoCopy(blocks, {
			contentCountry: geoCountry,
			tokens,
			locale: 'en',
			literals
		}) as unknown as Block[];
		contentCountry = geoCountry;
	}

	return { destinations, total, page: pageNo, limit, sort, types, chain, builderBlocks, contentCountry };
};
