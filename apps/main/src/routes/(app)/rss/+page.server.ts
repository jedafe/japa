import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { Space, TagDetail } from '$lib/types';

/**
 * RSS directory at /rss — public, SSR. Lists every feed surface the
 * platform exposes, grouped by entity: platform-wide feeds, the viewer's
 * own feed (when signed in), space feeds, and tag feeds. Entity lists are
 * API-sourced (Rule 16) — top spaces/tags only, since every space and tag
 * has its own feed and the full catalogs live on their own index pages.
 *
 * Ref: japaconnect-engineering/sections/26-seo-performance.md
 */
const ENTITY_LIMIT = 12;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let spaces: Space[] = [];
	let tags: TagDetail[] = [];
	let loadError = false;

	try {
		const [spacesRes, tagsRes] = await Promise.all([
			apiList<Space>(ctx, `/spaces?page_size=${ENTITY_LIMIT}&sort=members`),
			apiList<TagDetail>(ctx, `/tags?page_size=${ENTITY_LIMIT}`)
		]);
		spaces = spacesRes.items;
		tags = tagsRes.items;
	} catch (err) {
		// The static platform feeds don't depend on these lists — degrade to
		// showing just them rather than failing the whole page.
		if (!(err instanceof ApiError)) throw err;
		loadError = true;
	}

	return {
		spaces,
		tags,
		viewerUsername: event.locals.user?.username ?? null,
		loadError
	};
};
