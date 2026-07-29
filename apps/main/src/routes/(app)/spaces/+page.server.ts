import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Space } from '$lib/types';

/**
 * Spaces index at /spaces — auth-gated (GET /spaces requires a session),
 * SSR. Lists every tenant space the caller can see, with member/post counts.
 * Each card links to the space hub at /spaces/[slug].
 *
 * Ref: japaconnect-engineering/sections/22-spaces.md
 *      ADR-058 (renders inside the app shell)
 */
const PAGE_SIZE = 48;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();

	let spaces: Space[] = [];
	let loadError = false;
	try {
		const path = q
			? `/spaces?q=${encodeURIComponent(q)}&page_size=${PAGE_SIZE}`
			: `/spaces?page_size=${PAGE_SIZE}`;
		spaces = (await apiList<Space>(ctx, path)).items;
	} catch {
		loadError = true;
	}

	return { spaces, q, pageSize: PAGE_SIZE, loadError };
};
