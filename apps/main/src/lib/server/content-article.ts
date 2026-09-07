import { error } from '@sveltejs/kit';
import type { ServerLoadEvent } from '@sveltejs/kit';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import type { ContentArticle, ContentAnalytics, SeriesPost } from '$lib/types/content';
import type { Comment } from '$lib/types';

/**
 * Shared loader for the generic content single page — used by /content/[slug]
 * (fallback for types without a dedicated route) and /guide/[slug].
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 *      japaconnect-engineering/sections/49-unified-comments.md
 *      ADR-058 Exception #1 (dev.to reading layout — same as blog singles)
 */
export async function loadContentArticle(event: ServerLoadEvent) {
	const ctx = apiContext(event);

	// Published content is publicly readable (Section 14 — public content
	// SSR). Guests are no longer redirected to login; the backend returns
	// 404 for non-published content when unauthenticated.
	const slug = event.params.slug!;

	let article: ContentArticle;
	let jsonLd: unknown = null;
	let deletedUserIds: string[] | null = null;
	try {
		const envelope = await apiGetEnvelope<ContentArticle>(ctx, `/content/${slug}`);
		article = envelope.data;
		jsonLd = envelope.json_ld ?? null;
		deletedUserIds = envelope.deleted_user_ids ?? null;
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
	const userGroups = event.locals.user?.groups ?? [];

	// Edit affordance: the viewer may edit if they are the content owner,
	// a super-admin, or hold an RBAC edit node. §03 RBAC: the seeded/enforced
	// nodes are `content.edit_own` / `content.edit_any` — `content.edit` is
	// not a seeded node, so checking it hid the Edit action from every
	// moderator holding content.edit_any. The backend enforces the actual
	// permission on PATCH /content/{id}; this flag only gates the UI button.
	const isOwner = !!article.author_id && currentUserId === article.author_id;
	const isSuperAdmin = userGroups.some((g) => g === 'super_admin' || g === 'Super Admin');

	let hasContentEditPermission = false;
	let hasCommentsPinPermission = false;
	if (!isOwner && !isSuperAdmin && event.locals.user) {
		try {
			const permRes = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			const perms = permRes?.permissions ?? [];
			hasContentEditPermission =
				perms.includes('content.edit_own') || perms.includes('content.edit_any');
			hasCommentsPinPermission = perms.includes('comments.pin');
		} catch {
			hasContentEditPermission = false;
			hasCommentsPinPermission = false;
		}
	}
	const canEdit = isOwner || isSuperAdmin || hasContentEditPermission;

	// Pin affordance: content author or moderator (comments.pin). The backend
	// enforces the actual permission; this flag only gates the UI control.
	// BUG-004: include moderator permission check, not just content author.
	const canModerate = isOwner || isSuperAdmin || hasCommentsPinPermission;

	// Hydrate the CreatorCard follow button's initial state. Best-effort:
	// a failure degrades to `false` (not following). Skip when the viewer
	// is the author (no self-follow) or the author is unknown.
	const isOwnArticle = !!currentUserId && article.author_id === currentUserId;
	const initiallyFollowing =
		article.author_id && !isOwnArticle
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

	// Sidebar widgets (Section 59, ADR-066). The content reading layout renders
	// these below the CreatorCard in the right <aside>. The (app) layout already
	// resolves /sidebars/resolve for this exact pathname — reuse its result
	// instead of issuing a second identical request.
	const parentData = (await event.parent()) as { rightSidebarWidgets?: unknown[] };
	const rightSidebarWidgets = parentData.rightSidebarWidgets ?? [];

	// Related content (Section 14 — GET /content/{id}/related). Best-effort.
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

	// Series parts (Phase 06.10) — the public posts list powers the same
	// dev.to-style series card the blog single renders (§34).
	let seriesPosts: SeriesPost[] = [];
	if (article.series_id) {
		try {
			seriesPosts =
				(await apiGet<SeriesPost[]>(ctx, `/blog/series/${article.series_id}/posts`)) ?? [];
		} catch {
			// series posts endpoint not available — keep empty
		}
	}

	// JSON-LD structured data is resolved server-side by the backend
	// (Section 14 / Section 26 — admin-managed template matching the
	// content's content_type, interpolated with field values) and returned
	// in the response envelope's `json_ld` sidecar.

	const isFollowing = await initiallyFollowing;

	return {
		article,
		authed: !!event.locals.user,
		currentUserId,
		canModerate,
		canEdit,
		comments,
		commentsLoadError,
		rightSidebarWidgets,
		relatedContent,
		analytics,
		seriesPosts,
		jsonLd,
		deletedUserIds: mergedDeletedUserIds,
		initiallyFollowing: isFollowing
	};
}
