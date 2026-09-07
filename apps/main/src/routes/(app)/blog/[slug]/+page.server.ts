import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import type { ContentAnalytics, ContentArticle, SeriesPost } from '$lib/types/content';
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
	const { features } = await event.parent();
	if (!features?.['feature.blog']) {
		throw error(404, { code: 'feature_disabled', message: 'Blog is not available' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	// Custom invalidation key so client-side mutations (submit-for-review)
	// can re-run this load without depending on the exact backend fetch URL.
	event.depends(`blog:${slug}`);

	let article: ContentArticle;
	let jsonLd: unknown = null;
	let deletedUserIds: string[] | null = null;
	try {
		const envelope = await apiGetEnvelope<ContentArticle>(ctx, `/blog/${slug}`);
		article = envelope.data;
		jsonLd = envelope.json_ld ?? null;
		deletedUserIds = envelope.deleted_user_ids ?? null;
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Post not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load post' });
	}

	const currentUserId = event.locals.user?.id ?? null;
	const authed = !!event.locals.user;
	const userGroups = event.locals.user?.groups ?? [];

	// Edit affordance: the viewer may edit if they are the content owner
	// with `content.edit_own`, a super-admin, or an editor with
	// `content.edit_any`. The backend enforces the actual permission on
	// PATCH /content/{id} and PATCH /admin/content/{id}; this flag only
	// gates the UI Edit button. Ref: §03 RBAC — the legacy `content.edit`
	// node was renamed to `content.edit_own`; `content.edit_any` is the
	// admin/editor non-owner edit node.
	const isOwner = authed && !!article.author_id && currentUserId === article.author_id;
	const isSuperAdmin = userGroups.some((g) => g === 'super_admin' || g === 'Super Admin');

	let hasContentEditOwn = false;
	let hasContentEditAny = false;
	let hasCommentsPinPermission = false;
	if (!isOwner && !isSuperAdmin && event.locals.user) {
		try {
			const permRes = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			const perms = permRes?.permissions ?? [];
			hasContentEditOwn = perms.includes('content.edit_own');
			hasContentEditAny = perms.includes('content.edit_any');
			hasCommentsPinPermission = perms.includes('comments.pin');
		} catch {
			hasContentEditOwn = false;
			hasContentEditAny = false;
			hasCommentsPinPermission = false;
		}
	}
	const canEdit = isSuperAdmin || isOwner || hasContentEditOwn || hasContentEditAny;

	// Pin affordance: content author or moderator (comments.pin). The backend
	// enforces the actual permission; this flag only gates the UI control.
	// BUG-004: include moderator permission check, not just content author.
	const canModerate = isOwner || isSuperAdmin || hasCommentsPinPermission;

	// Hydrate the CreatorCard follow button's initial state. Best-effort:
	// a failure degrades to `false` (not following). Skip when the viewer
	// is the author (no self-follow), the author is unknown, or the viewer
	// is a guest (guests can't follow).
	const isOwnArticle = !!currentUserId && article.author_id === currentUserId;
	const initiallyFollowing =
		authed && article.author_id && !isOwnArticle
			? apiGet<{ following: boolean }>(ctx, `/users/${article.author_id}/follow-status`)
					.then((r) => r.following)
					.catch(() => false)
			: Promise.resolve(false);

	let comments: Comment[] = [];
	let commentsLoadError = false;
	let commentDeletedUserIds: string[] | null = null;
	try {
		const envelope = await apiGetEnvelope<Comment[]>(ctx, `/content/${article.id}/comments?page_size=50`);
		comments = envelope.data ?? [];
		commentDeletedUserIds = envelope.deleted_user_ids ?? null;
	} catch {
		commentsLoadError = true;
	}

	// Merge deleted user IDs from the article body and comments so the
	// frontend can render @[deleted user] for all erased mentions (ADR-029).
	const allDeletedUserIds = new Set<string>();
	if (deletedUserIds) for (const id of deletedUserIds) allDeletedUserIds.add(id);
	if (commentDeletedUserIds) for (const id of commentDeletedUserIds) allDeletedUserIds.add(id);
	const mergedDeletedUserIds = allDeletedUserIds.size > 0 ? Array.from(allDeletedUserIds) : null;

	// Sidebar widgets (Section 59, ADR-066) come from the parent app layout's
	// data — (app)/+layout.server.ts already resolves /sidebars/resolve for
	// this exact pathname, so a second call here would be a duplicate request.
	// `data.rightSidebarWidgets` flows through SvelteKit's data merging.

	// Series parts (Section 34 / Phase 06.10). Public series detail now shows
	// every published part ordered by position, mirroring dev.to's series card.
	let seriesPosts: SeriesPost[] = [];
	if (article.series_id) {
		try {
			seriesPosts = await apiGet<SeriesPost[]>(ctx, `/blog/series/${article.series_id}/posts`);
			seriesPosts = seriesPosts ?? [];
		} catch {
			// series posts endpoint not available — keep empty
		}
	}

	// If the current post is a draft and not returned by the public list, splice
	// it in at its known position so the author still sees it in the series card.
	if (article.series_id && article.series_part != null && !seriesPosts.some((p) => p.slug === article.slug)) {
		const idx = Math.min(article.series_part, seriesPosts.length);
		seriesPosts.splice(idx, 0, {
			content_id: article.id,
			position: article.series_part,
			title: article.title,
			slug: article.slug,
			status: article.status,
			created_at: article.created_at
		});
	}

	// Related content (Section 14 — GET /content/{id}/related). Best-effort:
	// a failure degrades to an empty list.
	let relatedContent: ContentArticle[] = [];
	try {
		const relRes = await apiGet<ContentArticle[]>(ctx, `/content/${article.id}/related?limit=4`);
		relatedContent = relRes ?? [];
	} catch {
		// related endpoint not available — keep empty
	}

	// Owner analytics (Section 40 — GET /content/{id}/analytics is owner or
	// analytics.admin only). Fetch only for the author to avoid guaranteed
	// 403s for other viewers; a failure degrades to no stats card.
	let analytics: ContentAnalytics | null = null;
	if (isOwnArticle) {
		try {
			analytics = await apiGet<ContentAnalytics>(ctx, `/content/${article.id}/analytics`);
		} catch {
			analytics = null;
		}
	}

	// JSON-LD structured data is now resolved server-side by the backend
	// (Section 14 / Section 26 — admin-managed Article template interpolated
	// with the post's field values) and returned in the response envelope's
	// `json_ld` sidecar. No separate template fetch is needed.

	const isFollowing = await initiallyFollowing;

	return {
		article,
		authed,
		currentUserId,
		canModerate,
		canEdit,
		comments,
		commentsLoadError,
		seriesPosts,
		relatedContent,
		analytics,
		jsonLd,
		deletedUserIds: mergedDeletedUserIds,
		initiallyFollowing: isFollowing
	};
};
