// Ref: japaconnect-engineering/sections/79-community-story-wall.md
//      japaconnect-engineering/sections/49-unified-comments.md
//      ADR-058 Exception #1 (dev.to reading layout — CPT singles)
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import type { Comment, Story } from '$lib/types';
import type { ContentAnalytics, ContentArticle, SeriesPost } from '$lib/types/content';

/**
 * Single community story at /stories/[slug] — SSR detail rendered in the
 * ADR-058 Exception #1 reading layout (ReactionRail in the left margin,
 * CreatorCard in the right sidebar, unified comment engine below the body).
 *
 * Fetches GET /stories/{slug}. On 404, throws a SvelteKit 404; other errors
 * degrade to loadError: true so the page can render a fallback instead of
 * erroring out.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;
	const currentUserId = event.locals.user?.id ?? null;

	let story: Story;
	try {
		const envelope = await apiGetEnvelope<Story>(ctx, `/stories/${slug}`);
		story = envelope.data;
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Story not found' });
		}
		return {
			story: null,
			authed,
			currentUserId,
			canEdit: false,
			canModerate: false,
			comments: [] as Comment[],
			commentsLoadError: false,
			relatedContent: [] as ContentArticle[],
			recommendedContent: [] as ContentArticle[],
			deletedUserIds: null,
			initiallyFollowing: false,
			loadError: true
		};
	}

	const userGroups = event.locals.user?.groups ?? [];
	const isOwner = !!story.author_id && currentUserId === story.author_id;
	const isSuperAdmin = userGroups.some((g) => g === 'super_admin' || g === 'Super Admin');

	// Moderation affordances: comment pinning needs comments.pin; the Edit
	// affordance needs story.edit_any/content.edit_any for non-owners — the
	// backend enforces the actual permission on PATCH /stories/{id} and
	// DELETE /stories/{id}; these flags only gate the UI controls.
	let hasCommentsPinPermission = false;
	let canEditAny = false;
	if (!isOwner && !isSuperAdmin && event.locals.user) {
		try {
			const permRes = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			const perms = permRes?.permissions ?? [];
			hasCommentsPinPermission = perms.includes('comments.pin');
			canEditAny = perms.includes('story.edit_any') || perms.includes('content.edit_any');
		} catch {
			hasCommentsPinPermission = false;
			canEditAny = false;
		}
	}
	const canModerate = isOwner || isSuperAdmin || hasCommentsPinPermission;
	const canEdit = isOwner || isSuperAdmin || canEditAny;

	// Hydrate the CreatorCard follow button's initial state. Best-effort:
	// a failure degrades to `false` (not following). Skip when the viewer
	// is the author (no self-follow) or the author is unknown.
	const isOwnStory = !!currentUserId && story.author_id === currentUserId;
	const initiallyFollowing =
		story.author_id && !isOwnStory
			? apiGet<{ following: boolean }>(ctx, `/users/${story.author_id}/follow-status`)
					.then((r) => r.following)
					.catch(() => false)
			: Promise.resolve(false);

	// Unified comment engine (Section 49) — top-level comments are SSR-seeded.
	let comments: Comment[] = [];
	let commentsLoadError = false;
	let commentDeletedUserIds: string[] | null = null;
	try {
		const envelope = await apiGetEnvelope<Comment[]>(ctx, `/content/${story.id}/comments?page_size=50`);
		comments = envelope.data ?? [];
		commentDeletedUserIds = envelope.deleted_user_ids ?? null;
	} catch {
		commentsLoadError = true;
	}

	// Related content (Section 14 — GET /content/{id}/related). Best-effort.
	// NB: the endpoint reads `page_size`, not `limit`.
	let relatedContent: ContentArticle[] = [];
	try {
		relatedContent = (await apiGet<ContentArticle[]>(ctx, `/content/${story.id}/related?page_size=4`)) ?? [];
	} catch {
		relatedContent = [];
	}

	// "Recommended Stories" sidebar card — same-type items ranked by the
	// related endpoint's content_type filter. Best-effort; the card hides
	// itself when the list is empty.
	let recommendedContent: ContentArticle[] = [];
	try {
		recommendedContent =
			(await apiGet<ContentArticle[]>(
				ctx,
				`/content/${story.id}/related?content_type=story&page_size=5`
			)) ?? [];
	} catch {
		recommendedContent = [];
	}

	// Owner analytics (Section 40 — GET /content/{id}/analytics is owner or
	// analytics.admin only). Fetch only for the author to avoid guaranteed
	// 403s for other viewers; a failure degrades to no stats card.
	let analytics: ContentAnalytics | null = null;
	if (isOwnStory) {
		try {
			analytics = await apiGet<ContentAnalytics>(ctx, `/content/${story.id}/analytics`);
		} catch {
			analytics = null;
		}
	}

	// Series parts (§14 — stories share the blog_series model and render the
	// same series card as the blog single). Public endpoint.
	let seriesPosts: SeriesPost[] = [];
	if (story.series_id) {
		try {
			seriesPosts =
				(await apiGet<SeriesPost[]>(ctx, `/blog/series/${story.series_id}/posts`)) ?? [];
		} catch {
			seriesPosts = [];
		}
	}

	return {
		story,
		authed,
		currentUserId,
		canEdit,
		canModerate,
		comments,
		commentsLoadError,
		relatedContent,
		recommendedContent,
		analytics,
		seriesPosts,
		deletedUserIds: commentDeletedUserIds,
		initiallyFollowing: await initiallyFollowing,
		loadError: false
	};
};
