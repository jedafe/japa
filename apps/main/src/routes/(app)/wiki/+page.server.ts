// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// SSR the landing page directory so search engines see the destination list
// without executing JavaScript.
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { WikiDestination } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	// Default landing view: active destination countries.
	let destinations: WikiDestination[] = [];
	try {
		const res = await apiList<WikiDestination>(ctx, '/wiki/destinations?level=country&has_wiki=false');
		destinations = res.items;
	} catch {
		/* degrade to empty list */
	}

	return { destinations };
};
