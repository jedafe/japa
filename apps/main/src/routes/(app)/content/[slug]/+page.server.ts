import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ContentArticle } from '$lib/types/content';
import type { Comment } from '$lib/types';

/**
 * Content single page (CPTs other than blog posts) — GET /api/v1/content/{slug}.
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 *      japaconnect-engineering/sections/49-unified-comments.md
 *      ADR-058 Exception #1 (dev.to reading layout — same as blog singles)
 * Auth required by the backend; guests are redirected to login. Rendered with
 * the ReactionRail in the left margin, CreatorCard in the right sidebar, and
 * the unified comment engine below the body.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	if (!event.locals.user) {
		throw redirect(302, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}

	const slug = event.params.slug;

	let article: ContentArticle;
	try {
		article = await apiGet<ContentArticle>(ctx, `/content/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Content not found' });
		}
		// A 401/403 for an already-authenticated user is NOT a session issue —
		// it's a backend/permission problem. Redirecting to login would loop
		// (the user is already logged in). Surface it as a 403 instead.
		if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
			throw error(403, { code: 'forbidden', message: 'You do not have access to this content.' });
		}
		throw error(502, { code: 'upstream_error', message: 'Failed to load content' });
	}

	const currentUserId = event.locals.user?.id ?? null;
	// Pin affordance: content author or moderator. The backend enforces the
	// actual permission; this flag only gates the UI control.
	const canModerate = !!article.author_id && currentUserId === article.author_id;

	let comments: Comment[] = [];
	let commentsLoadError = false;
	try {
		const { items } = await apiList<Comment>(ctx, `/content/${article.id}/comments?page_size=50`);
		comments = items;
	} catch {
		commentsLoadError = true;
	}

	return { article, authed: true, currentUserId, canModerate, comments, commentsLoadError };
};
