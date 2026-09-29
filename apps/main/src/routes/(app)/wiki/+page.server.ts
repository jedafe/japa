// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// SSR the landing page directory so search engines see the destination list
// without executing JavaScript.
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { WikiDestination, WikiTypeItem } from '$lib/types';
import { wikiDrillChain } from '$lib/utils/wikiTypeChain';

/**
 * §80 Hierarchical UX: the drill-down chain (country → state → city today)
 * derives from the declared wiki_type_parents edges on GET /wiki/types, not
 * a hardcoded type set — enabling a new child type extends the chooser
 * without code. Enabled types outside the chain keep their own search +
 * browse sections.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let destinations: WikiDestination[] = [];
	let typeWikis: Record<string, WikiDestination[]> = {};
	let types: WikiTypeItem[] = [];
	let chain: WikiTypeItem[] = [];
	let extraTypes: WikiTypeItem[] = [];
	try {
		const typesRes = await apiList<WikiTypeItem>(ctx, '/wiki/types');
		types = (typesRes.items ?? [])
			.filter((t) => t.enabled !== false)
			.sort((a, b) => a.position - b.position);
		chain = wikiDrillChain(types);
		const chainIds = new Set(chain.map((c) => c.space_type_id));
		extraTypes = types.filter((t) => !chainIds.has(t.space_type_id));

		const rootSlug = chain[0]?.space_type_slug ?? '';
		const [rootDest, ...perType] = await Promise.all([
			rootSlug
				? apiList<WikiDestination>(
						ctx,
						`/wiki/destinations?level=${encodeURIComponent(rootSlug)}&has_wiki=false`
					)
				: Promise.resolve(null),
			...extraTypes.map((t) =>
				apiList<WikiDestination>(
					ctx,
					`/wiki/destinations?level=${encodeURIComponent(t.space_type_slug)}&has_wiki=true`
				).catch(() => null)
			)
		]);
		destinations = rootDest?.items ?? [];
		for (let i = 0; i < extraTypes.length; i++) {
			typeWikis[extraTypes[i].space_type_slug] = perType[i]?.items ?? [];
		}
	} catch {
		/* degrade to empty list */
	}

	return { destinations, types, chain, extraTypes, typeWikis };
};
