import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { TagDetail } from '$lib/types';

/**
 * Tags index at /tags — public, SSR, indexable. Lists every tenant tag with
 * follower counts; each card links to the tag hub at /tags/[slug].
 *
 * Ref: japaconnect-engineering/sections/08-tags.md
 *      ADR-058 (renders inside the app shell; not a documented tab surface)
 */
const PAGE_SIZE = 48;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();

	let tags: TagDetail[] = [];
	let loadError = false;
	try {
		const path = q
			? `/tags?q=${encodeURIComponent(q)}&page_size=${PAGE_SIZE}`
			: `/tags?page_size=${PAGE_SIZE}`;
		tags = (await apiList<TagDetail>(ctx, path)).items;
	} catch {
		loadError = true;
	}

	return { tags, q, pageSize: PAGE_SIZE, loadError };
};
