// Ref: japaconnect-engineering/sections/79-community-story-wall.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { Story, Space } from '$lib/types';

/**
 * Edit-story form at /stories/[slug]/edit — auth-gated, author-only.
 * Loads the story by slug and the country list for the select dropdowns.
 * Only the story author may access this page; non-authors get a 403.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const slug = event.params.slug;

	let story: Story | null = null;
	try {
		story = await apiGet<Story>(ctx, `/stories/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Story not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load story' });
	}

	// Only the author may edit. The backend enforces this too (story.edit_own),
	// but we check here to avoid showing the form to non-authors.
	if (story.author_id && story.author_id !== event.locals.user.id) {
		throw error(403, { code: 'not_owner', message: 'You can only edit your own stories' });
	}

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	return { story, countries, authed: true };
};
