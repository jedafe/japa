// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// SSR the landing page directory so search engines see the destination list
// without executing JavaScript.
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { WikiDestination, WikiTypeItem } from '$lib/types';

/** Geo chain types are covered by the continent→country→state→city chooser;
 *  every other enabled wiki type gets its own browse section (§80: "new wiki
 *  types appear in navigation with zero code"). */
const GEO_WIKI_TYPES = new Set(['country', 'state', 'city']);

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	// Default landing view: active destination countries, plus every enabled
	// non-geo wiki type's spaces that already carry wiki pages.
	let destinations: WikiDestination[] = [];
	let typeWikis: Record<string, WikiDestination[]> = {};
	let extraTypes: WikiTypeItem[] = [];
	try {
		const [countries, types] = await Promise.all([
			apiList<WikiDestination>(ctx, '/wiki/destinations?level=country&has_wiki=false'),
			apiList<WikiTypeItem>(ctx, '/wiki/types')
		]);
		destinations = countries.items;
		extraTypes = (types.items ?? [])
			.filter((t) => t.enabled !== false && !GEO_WIKI_TYPES.has(t.space_type_slug))
			.sort((a, b) => a.position - b.position);
		const perType = await Promise.all(
			extraTypes.map((t) =>
				apiList<WikiDestination>(
					ctx,
					`/wiki/destinations?level=${encodeURIComponent(t.space_type_slug)}&has_wiki=true`
				).catch(() => null)
			)
		);
		for (let i = 0; i < extraTypes.length; i++) {
			typeWikis[extraTypes[i].space_type_slug] = perType[i]?.items ?? [];
		}
	} catch {
		/* degrade to empty list */
	}

	return { destinations, extraTypes, typeWikis };
};
