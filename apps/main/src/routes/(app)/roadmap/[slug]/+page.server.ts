import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import type {
	Comment,
	FeatureFlagDetail,
	RoadmapBoard,
	RoadmapBoardItem,
	RoadmapItemDetail
} from '$lib/types';

/**
 * Roadmap item detail — GET /api/v1/roadmap/items/{slug} (public).
 * Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
 *      japaconnect-engineering/sections/49-unified-comments.md
 *
 * Comments ride the §49 engine via the item's shadow contents row
 * (content_id). Top-level comments are SSR-seeded exactly like the blog
 * single; a failed comment fetch degrades to an empty thread — the item
 * must still render.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['roadmap.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Roadmap is not available' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	// Custom invalidation key so client-side mutations (votes, moderator
	// status changes) can re-run this load without the exact fetch URL.
	event.depends(`roadmap:${slug}`);

	let item: RoadmapItemDetail;
	let deletedUserIds: string[] | null = null;
	try {
		const envelope = await apiGetEnvelope<RoadmapItemDetail>(
			ctx,
			`/roadmap/items/${encodeURIComponent(slug)}`
		);
		item = envelope.data;
		deletedUserIds = envelope.deleted_user_ids ?? null;
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Roadmap item not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load roadmap item' });
	}

	const currentUserId = event.locals.user?.id ?? null;
	const authed = !!event.locals.user;
	// Inline-edit affordance: the viewer carries roadmap.moderate. The backend
	// enforces the node on PATCH /admin/roadmap/items/{id}(/status); this flag
	// only gates the UI control.
	const canModerate = !!event.locals.user?.permissions?.includes('roadmap.moderate');

	// Flag detail, comment thread, and related board are all keyed off the
	// resolved item but independent of each other — one wave.
	let commentsLoadError = false;
	const [flagDetail, commentsRes, related] = await Promise.all([
		// Flag-linked items get an enriched rollout badge driven by the flag's
		// live phase/percentage (§113 + unified-rollout spec §5.3). The
		// single-key details endpoint is public; a fetch failure degrades to
		// the generic badge — the item must still render.
		item.feature_flag_key
			? apiGet<FeatureFlagDetail>(
					ctx,
					'/feature-flags/details/' + encodeURIComponent(item.feature_flag_key)
				).catch(() => null)
			: Promise.resolve(null),
		// SSR-seed the comment thread from the shadow contents row (§113).
		item.content_id
			? apiGetEnvelope<Comment[]>(ctx, `/content/${item.content_id}/comments?page_size=50`).catch(
					() => {
						commentsLoadError = true;
						return null;
					}
				)
			: Promise.resolve(null),
		// Related items — same category via the board endpoint (Rule 16: no
		// dedicated related-items route exists; reuse /roadmap/board?category=).
		// Best-effort; a failure degrades to an empty list.
		item.category_slug
			? apiGet<RoadmapBoard>(
					ctx,
					`/roadmap/board?category=${encodeURIComponent(item.category_slug)}`
				)
					.then(
						(board) =>
							board.columns
								.flatMap((c) => c.items)
								.filter((i) => i.id !== item.id)
								.slice(0, 3) as RoadmapBoardItem[]
					)
					.catch(() => [] as RoadmapBoardItem[])
			: Promise.resolve([] as RoadmapBoardItem[])
	]);
	const comments = commentsRes?.data ?? [];
	const commentDeletedUserIds = commentsRes?.deleted_user_ids ?? null;

	// Merge deleted user IDs from the item and comments so the frontend can
	// render @[deleted user] for all erased mentions (ADR-029).
	const allDeletedUserIds = new Set<string>();
	if (deletedUserIds) for (const id of deletedUserIds) allDeletedUserIds.add(id);
	if (commentDeletedUserIds) for (const id of commentDeletedUserIds) allDeletedUserIds.add(id);
	const mergedDeletedUserIds = allDeletedUserIds.size > 0 ? Array.from(allDeletedUserIds) : null;

	return {
		item,
		flagDetail,
		authed,
		currentUserId,
		canModerate,
		comments,
		commentsLoadError,
		deletedUserIds: mergedDeletedUserIds,
		related
	};
};
