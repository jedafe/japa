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
	const { features } = await event.parent();
	if (features?.['story.enabled'] !== true) {
		throw error(404, { code: 'feature_disabled', message: 'Community Stories are not available' });
	}

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
	const isOwnStory = !!currentUserId && story.author_id === currentUserId;

	// Everything downstream of the story fetch is independent — run the whole
	// wave concurrently (permissions, follow state, comments, related,
	// recommended, analytics, series posts).
	const permPromise =
		!isOwner && !isSuperAdmin && event.locals.user
			? apiGet<{ permissions: string[] }>(ctx, '/me/permissions')
					.then((r) => r?.permissions ?? [])
					.catch(() => [] as string[])
			: Promise.resolve([] as string[]);

	// Hydrate the CreatorCard follow button's initial state. Best-effort:
	// a failure degrades to `false` (not following). Skip when the viewer
	// is the author (no self-follow) or the author is unknown.
	const followingPromise =
		authed && story.author_id && !isOwnStory
			? apiGet<{ following: boolean }>(ctx, `/users/${story.author_id}/follow-status`)
					.then((r) => r.following)
					.catch(() => false)
			: Promise.resolve(false);

	// Unified comment engine (Section 49) — top-level comments are SSR-seeded.
	const commentsPromise = apiGetEnvelope<Comment[]>(
		ctx,
		`/content/${story.id}/comments?page_size=50`
	).then(
		(envelope) => ({
			comments: envelope.data ?? [],
			deletedUserIds: envelope.deleted_user_ids ?? null,
			commentsTotal: (envelope.meta as { total?: number } | undefined)?.total ?? null,
			loadError: false
		}),
		() => ({ comments: [] as Comment[], deletedUserIds: null as string[] | null, commentsTotal: null as number | null, loadError: true })
	);

	// Related content (Section 14 — GET /content/{id}/related). Best-effort.
	// NB: the endpoint reads `page_size`, not `limit`.
	const relatedPromise = apiGet<ContentArticle[]>(ctx, `/content/${story.id}/related?page_size=4`)
		.then((r) => r ?? [])
		.catch(() => [] as ContentArticle[]);

	// "Recommended Stories" sidebar card — same-type items ranked by the
	// related endpoint's content_type filter. Best-effort; the card hides
	// itself when the list is empty.
	const recommendedPromise = apiGet<ContentArticle[]>(
		ctx,
		`/content/${story.id}/related?content_type=story&page_size=5`
	)
		.then((r) => r ?? [])
		.catch(() => [] as ContentArticle[]);

	// Owner analytics (Section 40 — GET /content/{id}/analytics is owner or
	// analytics.admin only). Fetch only for the author to avoid guaranteed
	// 403s for other viewers; a failure degrades to no stats card.
	const analyticsPromise = isOwnStory
		? apiGet<ContentAnalytics>(ctx, `/content/${story.id}/analytics`).catch(() => null)
		: Promise.resolve(null);

	// Series parts (§14 — stories share the blog_series model and render the
	// same series card as the blog single). Public endpoint.
	const seriesPromise = story.series_id
		? apiGet<SeriesPost[]>(ctx, `/series/${story.series_id}/posts`)
				.then((r) => r ?? [])
				.catch(() => [] as SeriesPost[])
		: Promise.resolve([] as SeriesPost[]);

	const [perms, initiallyFollowing, commentsResult, relatedContent, recommendedContent, analytics, seriesPosts] =
		await Promise.all([
			permPromise,
			followingPromise,
			commentsPromise,
			relatedPromise,
			recommendedPromise,
			analyticsPromise,
			seriesPromise
		]);

	// Moderation affordances: comment pinning needs comments.pin; the Edit
	// affordance needs story.edit_any/content.edit_any for non-owners — the
	// backend enforces the actual permission on PATCH /stories/{id} and
	// DELETE /stories/{id}; these flags only gate the UI controls.
	const hasCommentsPinPermission = perms.includes('comments.pin');
	const canEditAny = perms.includes('story.edit_any') || perms.includes('content.edit_any');
	const canModerate = isOwner || isSuperAdmin || hasCommentsPinPermission;
	const canEdit = isOwner || isSuperAdmin || canEditAny;
	const { comments, deletedUserIds: commentDeletedUserIds, commentsTotal, loadError: commentsLoadError } =
		commentsResult;

	return {
		story,
		authed,
		currentUserId,
		canEdit,
		canModerate,
		comments,
		commentsLoadError,
		commentsTotal,
		relatedContent,
		recommendedContent,
		analytics,
		seriesPosts,
		deletedUserIds: commentDeletedUserIds,
		initiallyFollowing,
		loadError: false
	};
};
