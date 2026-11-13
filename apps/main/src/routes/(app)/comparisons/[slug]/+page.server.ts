import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '#lib/server/api.js';
import type { JCEComparisonDetail, JCEFieldPoolGroup, MatrixItem, ResolvedItem } from '#lib/types/jce.js';
import type { ContentArticle } from '#lib/types/content.js';
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
	let indexable = true;
	try {
		const page = await apiGet<{
			comparison: JCEComparisonDetail;
			matrix: MatrixItem[];
			indexable?: boolean;
		}>(ctx, `/jce/comparisons/${slug}/page`);
		comparison = page?.comparison ?? null;
		matrix = page?.matrix ?? [];
		indexable = page?.indexable ?? true;
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			if (err.code === 'feature_disabled') {
				throw error(404, 'Comparisons are not available', { code: 'feature_disabled' });
			}
			error(404, 'Comparison not found', { code: 'not_found' });
		}
		throw err;
	}

	if (!comparison) {
		error(404, 'Comparison not found', { code: 'not_found' });
	}

	const isOwner = !!event.locals.user?.id && comparison.content.author_id === event.locals.user.id;
	const isOwnComparison = isOwner;
	const contentId = comparison.content.id;

	// Second wave — everything downstream of the detail fetch is independent
	// of each other (owner analytics, follow status, related items), so they
	// run concurrently instead of stacking round-trips. The comment engine
	// no longer mounts here — EntityDiscussions previews 5 comments
	// client-side and the full engine lives at /thread/{slug}.

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

	// Field pool for matrix grouping (§30 Sub-spec 6): dimensions carry
	// source_field_id → each resolves to a pool group so the matrix can
	// render one table per selected group. Best-effort — failure degrades
	// to the single-table layout.
	const typeId =
		comparison.items.find((i) => i.type_id)?.type_id ??
		comparison.dimensions.find((d) => d.type_id)?.type_id ??
		null;
	const fieldPoolPromise = typeId
		? apiGet<{ groups: JCEFieldPoolGroup[] }>(ctx, `/jce/comparison-types/${typeId}/fields`)
				.then((r) => r?.groups ?? [])
				.catch(() => [] as JCEFieldPoolGroup[])
		: Promise.resolve([] as JCEFieldPoolGroup[]);

	const [isFollowing, recommendedContent, fieldPoolGroups] = await Promise.all([
		followingPromise,
		recommendedPromise,
		fieldPoolPromise
	]);

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
		indexable,
		matrix,
		fieldPoolGroups,
		resolvedItems: resolvedItems(comparison),
		canEdit,
		authed,
		currentUserId,
		canModerate,
		recommendedContent,
		initiallyFollowing: isFollowing
	};
};
