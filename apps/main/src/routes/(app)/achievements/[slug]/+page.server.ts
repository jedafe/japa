// Ref: japaconnect-engineering/sections/89-achievement-feed.md
// Achievement detail at /achievements/[slug] — SSR fetches GET /achievements/{slug}.
// Returns the achievement plus an authed flag and canEdit (from the backend's
// is_owner field) so the detail page can show Edit/Delete affordances for the
// author (backend enforces ownership).
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { Achievement } from '$lib/types';
import type { ContentArticle } from '$lib/types/content';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;

	let achievement: Achievement;
	let canEdit = false;
	try {
		achievement = await apiGet<Achievement>(ctx, `/achievements/${slug}`);
		// §89 Privacy: the backend does NOT expose author_id for anonymous
		// achievements. Instead it returns is_owner (true when the viewer is
		// the author). The viewer can edit/delete their own posts regardless
		// of the anonymous flag — the backend enforces ownership on
		// PATCH/DELETE. This flag only gates the UI. Ref: §89 privacy.
		canEdit = authed && !!achievement.is_owner;
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Achievement not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load achievement' });
	}

	// "Recommended Achievements" card — same-type items ranked by the
	// related endpoint's content_type filter. Best-effort; the card hides
	// itself when the list is empty. NB: the endpoint reads `page_size`,
	// not `limit`.
	let recommendedContent: ContentArticle[] = [];
	try {
		recommendedContent =
			(await apiGet<ContentArticle[]>(
				ctx,
				`/content/${achievement.id}/related?content_type=achievement&page_size=5`
			)) ?? [];
	} catch {
		recommendedContent = [];
	}

	return { achievement, authed, canEdit, recommendedContent, loadError: false };
};
