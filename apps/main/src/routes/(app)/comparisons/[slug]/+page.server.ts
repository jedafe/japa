import { error } from '@sveltejs/kit';
import { apiContext, apiGet, apiGetEnvelope, ApiError } from '$lib/server/api';
import type { JCEComparisonDetail, MatrixItem, ResolvedItem } from '$lib/types/jce';
import type { Comment } from '$lib/types';
import type { ContentAnalytics } from '$lib/types/content';
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

	let comparison: JCEComparisonDetail | null = null;
	let matrix: MatrixItem[] = [];

	try {
		comparison = await apiGet<JCEComparisonDetail>(ctx, `/jce/comparisons/${slug}`);
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

	try {
		matrix = await apiGet<MatrixItem[]>(ctx, `/jce/comparisons/${slug}/matrix`);
	} catch {
		matrix = [];
	}

	// Unified comment engine (Section 49) — top-level comments are SSR-seeded.
	let comments: Comment[] = [];
	let commentsLoadError = false;
	let commentDeletedUserIds: string[] | null = null;
	try {
		const envelope = await apiGetEnvelope<Comment[]>(ctx, `/content/${comparison.content.id}/comments?page_size=50`);
		comments = envelope.data ?? [];
		commentDeletedUserIds = envelope.deleted_user_ids ?? null;
	} catch {
		commentsLoadError = true;
	}

	// Owner analytics (Section 40 — GET /content/{id}/analytics is owner or
	// analytics.admin only). Fetch only for the author to avoid guaranteed
	// 403s for other viewers; a failure degrades to no stats card.
	let analytics: ContentAnalytics | null = null;
	if (event.locals.user?.id && comparison.content.author_id === event.locals.user.id) {
		try {
			analytics = await apiGet<ContentAnalytics>(ctx, `/content/${comparison.content.id}/analytics`);
		} catch {
			analytics = null;
		}
	}

	// §30 edit affordance: the author or a comparison moderator can jump
	// straight to the unified composer for this comparison.
	const isOwner = !!event.locals.user?.id && comparison.content.author_id === event.locals.user.id;
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
		deletedUserIds: commentDeletedUserIds
	};
};
