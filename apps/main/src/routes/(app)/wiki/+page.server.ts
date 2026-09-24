// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// SSR the landing page directory so search engines see the destination list
// without executing JavaScript.
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { WikiDestination } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	// Default landing view: active destination countries, plus universities
	// that already carry wiki pages (§80 amendment — University Spaces).
	let destinations: WikiDestination[] = [];
	let universityWikis: WikiDestination[] = [];
	try {
		const [countries, unis] = await Promise.all([
			apiList<WikiDestination>(ctx, '/wiki/destinations?level=country&has_wiki=false'),
			apiList<WikiDestination>(ctx, '/wiki/destinations?level=university&has_wiki=true')
		]);
		destinations = countries.items;
		universityWikis = unis.items;
	} catch {
		/* degrade to empty list */
	}

	return { destinations, universityWikis };
};
