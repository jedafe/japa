import { error } from '@sveltejs/kit';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import type { JCEComparisonDetail, MatrixItem, ResolvedItem } from '$lib/types/jce';
import type { Comment } from '$lib/types';
import type { ContentAnalytics, ContentArticle } from '$lib/types/content';
import type { PageServerLoad } from './$types';

function resolvedItems(cmp: JCEComparisonDetail): ResolvedItem[] {
	return (
		cmp.items?.map((it) => ({
			type: (it.item_ref?.type as string) ?? '',
			id: (it.item_ref?.id as string) ?? (it.id as string) ?? '',
			title: it.title,
			description: it.description,
			position: it.position,
			metadata: (it.item_ref as Record<string, unknown>) ?? undefined
		})) ?? []
	);
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;
	const currentUserId = event.locals.user?.id ?? null;

	// Single batched request — the /page endpoint returns detail + matrix so
	// the backend doesn't fetch the same items/dimensions rows twice.
	let comparison: JCEComparisonDetail | null = null;
	let matrix: MatrixItem[] = [];
	try {
		const page = await apiGet<{ comparison: JCEComparisonDetail; matrix: MatrixItem[] }>(
			ctx,
			`/jce/comparisons/${slug}/page`
		);
		comparison = page?.comparison ?? null;
		matrix = page?.matrix ?? [];
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			if (err.code === 'feature_disabled') {
				throw error(503, { code: 'feature_disabled', message: 'jce.error.feature_disabled' });
			}
			error(404, { code: 'not_found', message: 'jce.error.not_found' });
		}
		throw err;
	}

	if (!comparison) {
		error(404, { code: 'not_found', message: 'jce.error.not_found' });
	}

	const isOwner = !!event.locals.user?.id && comparison.content.author_id === event.locals.user.id;
	const isOwnComparison = isOwner;
	const contentId = comparison.content.id;

	// Second wave — everything downstream of the detail fetch is independent
	// of each other (comments, owner analytics, follow status, related items),
	// so they run concurrently instead of stacking round-trips.
	const commentsPromise = apiGetEnvelope<Comment[]>(
		ctx,
		`/content/${contentId}/comments?page_size=50`
	).then(
		(envelope) => ({
			comments: envelope.data ?? [],
			deletedUserIds: envelope.deleted_user_ids ?? null,
			loadError: false
		}),
		() => ({ comments: [] as Comment[], deletedUserIds: null as string[] | null, loadError: true })
	);

	// Owner analytics (Section 40 — GET /content/{id}/analytics is owner or
	// analytics.admin only). Fetch only for the author to avoid guaranteed
	// 403s for other viewers; a failure degrades to no stats card.
	const analyticsPromise = isOwner
		? apiGet<ContentAnalytics>(ctx, `/content/${contentId}/analytics`).catch(() => null)
		: Promise.resolve(null);

	// Hydrate the CreatorCard follow button's initial state. Best-effort:
	// a failure degrades to `false` (not following). Skip when the viewer
	// is the author (no self-follow) or the author is unknown.
	const followingPromise =
		authed && comparison.content.author_id && !isOwnComparison
			? apiGet<{ following: boolean }>(ctx, `/users/${comparison.content.author_id}/follow-status`)
					.then((r) => r.following)
					.catch(() => false)
			: Promise.resolve(false);

	// "Recommended Comparisons" sidebar card — same-type items ranked by the
	// related endpoint's content_type filter (Section 14). Best-effort; the
	// card hides itself when the list is empty.
	const recommendedPromise = apiGet<ContentArticle[]>(
		ctx,
		`/content/${contentId}/related?content_type=comparison&page_size=5`
	)
		.then((r) => r ?? [])
		.catch(() => [] as ContentArticle[]);

	const [commentsResult, analytics, isFollowing, recommendedContent] = await Promise.all([
		commentsPromise,
		analyticsPromise,
		followingPromise,
		recommendedPromise
	]);

	const { comments, deletedUserIds: commentDeletedUserIds, loadError: commentsLoadError } =
		commentsResult;

	// §30 edit affordance: the author or a comparison moderator can jump
	// straight to the unified composer for this comparison.
	const canEdit =
		isOwner ||
		!!event.locals.user?.permissions?.includes('comparison.edit_any');

	// Moderation affordances: comment pinning needs comments.pin; the Edit
	// affordance is already handled by canEdit. Backend enforces actual permissions.
	const isSuperAdmin = (event.locals.user?.groups ?? []).some(
		(g) => g === 'super_admin' || g === 'Super Admin'
	);
	const canModerate =
		isOwner ||
		isSuperAdmin ||
		!!event.locals.user?.permissions?.includes('comments.pin');

	return {
		comparison,
		matrix,
		resolvedItems: resolvedItems(comparison),
		analytics,
		canEdit,
		authed,
		currentUserId,
		canModerate,
		comments,
		commentsLoadError,
		recommendedContent,
		deletedUserIds: commentDeletedUserIds,
		initiallyFollowing: isFollowing
	};
};
