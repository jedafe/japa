import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetEnvelope, ApiError } from '$lib/server/api';
import type { PostDetail } from '$lib/types/content';
import type { Comment } from '$lib/types';
import { contentHref } from '$lib/utils/contentLink';

/**
 * Social post wrapper — GET /api/v1/post/{content_type}/{slug}.
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md
 *      docs/superpowers/specs/2026-09-10-daily-feed-style-and-social-post-page-design.md
 *
 * This route fetches the disambiguated public content row for the daily feed
 * card and the canonical single-type content. Comments are loaded from the
 * existing unified comment endpoint so the social wrapper stays thin.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const contentType = event.params.content_type;
	const slug = event.params.slug;
	if (!contentType || !slug) {
		throw error(404, { code: 'not_found', message: 'Post not found' });
	}

	let post: PostDetail;
	let jsonLd: unknown = null;
	let deletedUserIds: string[] | null = null;
	try {
		const envelope = await apiGetEnvelope<PostDetail>(
			ctx,
			`/post/${encodeURIComponent(contentType)}/${encodeURIComponent(slug)}`
		);
		post = envelope.data;
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
	const isSuperAdmin = userGroups.some((g) => g === 'super_admin' || g === 'Super Admin');
	const isOwner = authed && post.author_id === currentUserId;
	const canModerate = isSuperAdmin || isOwner;

	// Hydrate the unified comment thread (Section 49) server-side.
	// A failure degrades to an empty thread so the post still renders.
	let comments: Comment[] = [];
	let commentsLoadError = false;
	let commentDeletedUserIds: string[] | null = null;
	try {
		const envelope = await apiGetEnvelope<Comment[]>(ctx, `/content/${post.id}/comments?page_size=50`);
		comments = envelope.data ?? [];
		commentDeletedUserIds = envelope.deleted_user_ids ?? null;
	} catch {
		commentsLoadError = true;
	}

	const allDeletedUserIds = new Set<string>();
	if (deletedUserIds) for (const id of deletedUserIds) allDeletedUserIds.add(id);
	if (commentDeletedUserIds) for (const id of commentDeletedUserIds) allDeletedUserIds.add(id);
	const mergedDeletedUserIds = allDeletedUserIds.size > 0 ? Array.from(allDeletedUserIds) : null;

	return {
		post,
		canonicalHref: contentHref(post),
		authed,
		currentUserId,
		canModerate,
		comments,
		commentsLoadError,
		jsonLd,
		deletedUserIds: mergedDeletedUserIds
	};
};
