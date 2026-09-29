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
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let destinations: WikiDestination[] = [];
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
			const rootDest = await apiList<WikiDestination>(
				ctx,
				`/wiki/destinations?level=${encodeURIComponent(rootSlug)}&has_wiki=false`
			);
			destinations = rootDest?.items ?? [];
		}
	} catch {
		/* degrade to empty list */
	}

	return { destinations, types, chain };
};
