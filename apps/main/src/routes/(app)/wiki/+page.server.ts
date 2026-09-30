// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// SSR the landing page directory so search engines see the destination list
// without executing JavaScript.
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { WikiDestination, WikiTypeItem } from '$lib/types';
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

	return { destinations, total, page: pageNo, limit, sort, types, chain };
};
