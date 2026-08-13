// Ref: japaconnect-engineering/sections/89-achievement-feed.md
// Edit achievement at /achievements/[slug]/edit — SSR fetches the achievement
// by slug, pre-fills the form, and the client submits PATCH /achievements/{id}
// (by content id). Only the author can edit (backend enforces ownership).
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { Achievement, Space } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(401, { code: 'unauthorized', message: 'Authentication required' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let achievement: Achievement;
	try {
		achievement = await apiGet<Achievement>(ctx, `/achievements/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Achievement not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load achievement' });
	}

	// Gate the UI: only the author sees the edit form. The backend also
	// enforces ownership on PATCH, so this is defense-in-depth. §89 Privacy:
	// author_id is not exposed for anonymous posts; use is_owner instead.
	if (!achievement.is_owner) {
		throw error(403, { code: 'not_owner', message: 'You can only edit your own achievements' });
	}

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	return { achievement, countries };
};
