// Ref: japaconnect-engineering/sections/89-achievement-feed.md
// Achievement detail at /achievements/[slug] — SSR fetches GET /achievements/{slug}.
// Returns the achievement plus an authed flag and canEdit (from the backend's
// is_owner field) so the detail page can show Edit/Delete affordances for the
// author (backend enforces ownership).
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import type { Achievement, Comment } from '$lib/types';
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

	// SSR-seed the comment thread (§49) — achievement posts accept comments
	// like any other content row; the stat was previously shown with no way
	// to read or write them (E-011). Best-effort: the page still renders if
	// the comments fetch fails; CommentThread shows its own error state.
	let comments: Comment[] = [];
	let commentsTotal: number | null = null;
	let commentsLoadError = false;
	let deletedUserIds: string[] = [];
	try {
		const envelope = await apiGetEnvelope<Comment[]>(
			ctx,
			`/content/${achievement.id}/comments?page_size=50`
		);
		comments = envelope.data ?? [];
		commentsTotal = (envelope.meta as { total?: number } | undefined)?.total ?? null;
		deletedUserIds =
			(envelope as { deleted_user_ids?: string[] }).deleted_user_ids ?? [];
	} catch {
		commentsLoadError = true;
	}

	return {
		achievement,
		authed,
		canEdit,
		recommendedContent,
		comments,
		commentsTotal,
		commentsLoadError,
		deletedUserIds,
		currentUserId: event.locals.user?.id ?? null,
		loadError: false
	};
};
