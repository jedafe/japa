// Ref: japaconnect-engineering/sections/79-community-story-wall.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { Story } from '$lib/types';

/**
 * Single community story at /stories/[slug] — SSR detail. Fetches
 * GET /stories/{slug}. On 404, throws a SvelteKit 404; other errors
 * degrade to loadError: true so the page can render a fallback.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;

	try {
		const story = await apiGet<Story>(ctx, `/stories/${slug}`);
		return { story, authed, currentUserId: event.locals.user?.id ?? null, loadError: false };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Story not found' });
		}
		return { story: null, authed, currentUserId: event.locals.user?.id ?? null, loadError: true };
	}
};
