// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// SSR the landing page directory so search engines see the destination list
// without executing JavaScript.
import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGetEnvelope, ApiError } from '#lib/server/api.js';
import { resolveGeoContext } from '#lib/server/published-page.js';
import {
	normalizeLegacyBlocks,
	resolveGeoCopy,
	getGeoTokens,
	type Block
} from '@japaconnect/landing-blocks';
import type { WikiDestination, WikiTypeItem, PageBuilderPage } from '#lib/types/index.js';
import { wikiDestinationChain } from '#lib/utils/wikiTypeChain.js';

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
	// §wiki-seo — ?region={name} filters the country gallery to one UN M49
	// region; the region hubs remain reachable as pages via breadcrumbs and
	// their own /wiki/{region} index, but the chips filter in place.
	const region = event.url.searchParams.get('region') ?? '';
	// §wiki-seo guest surface — logged-out visitors only see destinations
	// with real prose (has_content); members keep the full stub-bearing
	// list so the gallery doubles as the contribution map.
	const hasContent = !event.locals.user;

	let destinations: WikiDestination[] = [];
	let hubDestinations: WikiDestination[] = [];
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

	// Geo context is needed whenever the builder page carries blocks — fire it
	// alongside instead of paying a GeoIP round-trip after the envelope lands.
	const geoPromise = resolveGeoContext(event);

	try {
		const typesRes = await apiList<WikiTypeItem>(ctx, '/wiki/types');
		types = (typesRes.items ?? [])
			.filter((t) => t.enabled !== false)
			.sort((a, b) => a.position - b.position);
		chain = wikiDestinationChain(types);

		const rootSlug = chain[0]?.space_type_slug ?? '';
		// The flag gallery is inherently country-level: when the chain roots
		// at a hub type (region), that level renders as the browse row above
		// the gallery instead of replacing it.
		const gallerySlug =
			chain.find((t) => t.space_type_slug === 'country')?.space_type_slug ??
			chain[1]?.space_type_slug ??
			rootSlug;
		if (gallerySlug) {
			const usp = new URLSearchParams({
				level: gallerySlug,
				has_wiki: 'false',
				sort,
				page: String(pageNo)
			});
			if (hasContent) usp.set('has_content', 'true');
			if (region) usp.set('region', region);
			const rootDest = await apiList<WikiDestination>(ctx, `/wiki/destinations?${usp.toString()}`);
			destinations = rootDest?.items ?? [];
			total = rootDest?.meta?.total ?? destinations.length;
			limit = rootDest?.meta?.limit ?? 0;
		}
		if (rootSlug && rootSlug !== gallerySlug) {
			const hubUsp = new URLSearchParams({
				level: rootSlug,
				has_wiki: 'false',
				limit: '50',
				sort: 'name'
			});
			const hubRes = await apiList<WikiDestination>(ctx, `/wiki/destinations?${hubUsp.toString()}`);
			hubDestinations = hubRes?.items ?? [];
		}
	} catch {
		/* degrade to empty list */
	}

	let builderBlocks: Block[] | null = null;
	let contentCountry: string | undefined;
	const builderEnvelope = await builderPagePromise;
	const rawBlocks = builderEnvelope?.data?.blocks;
	if (Array.isArray(rawBlocks) && rawBlocks.length > 0) {
		const { geoCountry, geoCountryName } = await geoPromise;
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

	return {
		destinations,
		hubDestinations,
		region,
		total,
		page: pageNo,
		limit,
		sort,
		types,
		chain,
		builderBlocks,
		contentCountry
	};
};
