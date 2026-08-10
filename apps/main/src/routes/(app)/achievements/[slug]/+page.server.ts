// Ref: japaconnect-engineering/sections/89-achievement-feed.md
// Achievement detail at /achievements/[slug] — SSR fetches GET /achievements/{slug}.
// Returns the achievement plus an authed flag and the viewer's id so the detail
// page can show Edit/Delete affordances for the author (backend enforces ownership).
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { Achievement } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;
	const currentUserId = event.locals.user?.id ?? null;

	let achievement: Achievement;
	let canEdit = false;
	try {
		achievement = await apiGet<Achievement>(ctx, `/achievements/${slug}`);
		// The backend exposes author_id on the DTO even for anonymous posts
		// (anonymity only suppresses author_name/author_avatar in the
		// response). The viewer can edit/delete their own posts regardless of
		// the anonymous flag — the backend enforces ownership on PATCH/DELETE.
		// This flag only gates the UI. Ref: §89 privacy.
		canEdit = authed && !!achievement.author_id && achievement.author_id === currentUserId;
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Achievement not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load achievement' });
	}

	return { achievement, authed, currentUserId, canEdit, loadError: false };
};
