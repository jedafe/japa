import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ContentArticle } from '$lib/types/content';
import type { Comment } from '$lib/types';

/**
 * Blog single post — GET /api/v1/blog/{slug} (public, tenant-scoped).
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 *      japaconnect-engineering/sections/49-unified-comments.md
 * Rendered as a dev.to reading layout (ADR-058 Exception #1):
 * left nav collapsed, ReactionRail in left margin, CreatorCard in right sidebar.
 *
 * Top-level comments are fetched server-side (GET /content/{id}/comments returns
 * only parent_id IS NULL rows); replies are lazy-loaded client-side via
 * ?parent_id=. A failed comment fetch degrades to an empty thread — the article
 * must still render.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	let article: ContentArticle;
	try {
		article = await apiGet<ContentArticle>(ctx, `/blog/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Post not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load post' });
	}

	const currentUserId = event.locals.user?.id ?? null;
	const authed = !!event.locals.user;
	// Pin affordance: content author or moderator. The backend enforces the
	// actual permission; this flag only gates the UI control.
	const canModerate = authed && !!article.author_id && currentUserId === article.author_id;

	let comments: Comment[] = [];
	let commentsLoadError = false;
	try {
		const { items } = await apiList<Comment>(ctx, `/content/${article.id}/comments?page_size=50`);
		comments = items;
	} catch {
		commentsLoadError = true;
	}

	return { article, authed, currentUserId, canModerate, comments, commentsLoadError };
};
