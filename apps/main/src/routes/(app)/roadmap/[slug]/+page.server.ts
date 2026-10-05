import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import type { Comment, RoadmapBoard, RoadmapBoardItem, RoadmapItemDetail } from '$lib/types';

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

	// SSR-seed the comment thread from the shadow contents row (§113).
	let comments: Comment[] = [];
	let commentsLoadError = false;
	let commentDeletedUserIds: string[] | null = null;
	if (item.content_id) {
		try {
			const envelope = await apiGetEnvelope<Comment[]>(
				ctx,
				`/content/${item.content_id}/comments?page_size=50`
			);
			comments = envelope.data ?? [];
			commentDeletedUserIds = envelope.deleted_user_ids ?? null;
		} catch {
			commentsLoadError = true;
		}
	}

	// Merge deleted user IDs from the item and comments so the frontend can
	// render @[deleted user] for all erased mentions (ADR-029).
	const allDeletedUserIds = new Set<string>();
	if (deletedUserIds) for (const id of deletedUserIds) allDeletedUserIds.add(id);
	if (commentDeletedUserIds) for (const id of commentDeletedUserIds) allDeletedUserIds.add(id);
	const mergedDeletedUserIds = allDeletedUserIds.size > 0 ? Array.from(allDeletedUserIds) : null;

	// Related items — same category via the board endpoint (Rule 16: no
	// dedicated related-items route exists; reuse /roadmap/board?category=).
	// Best-effort; a failure degrades to an empty list.
	let related: RoadmapBoardItem[] = [];
	if (item.category_slug) {
		try {
			const board = await apiGet<RoadmapBoard>(
				ctx,
				`/roadmap/board?category=${encodeURIComponent(item.category_slug)}`
			);
			related = board.columns
				.flatMap((c) => c.items)
				.filter((i) => i.id !== item.id)
				.slice(0, 3);
		} catch {
			related = [];
		}
	}

	return {
		item,
		authed,
		currentUserId,
		canModerate,
		comments,
		commentsLoadError,
		deletedUserIds: mergedDeletedUserIds,
		related
	};
};
