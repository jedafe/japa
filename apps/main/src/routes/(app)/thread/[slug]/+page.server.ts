import { error } from '@sveltejs/kit';
import type { ServerLoadEvent } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, apiList, ApiError } from '#lib/server/api.js';
import { hasPerm } from '#lib/server/auth.js';
import { contentHref } from '#lib/utils/contentLink.js';
import type { ForumThread, ForumReply, ForumTag, PollWithResults, Comment } from '#lib/types/index.js';
import type { ContentArticle } from '#lib/types/content.js';
import type { ThreadEmbed } from '#lib/components/thread/ThreadEmbedView.svelte';

/**
 * Single thread at /thread/[slug] — the one place discussion happens
 * (thread-rename spec). Two kinds dispatch on the forum API result:
 *
 *   kind 'forum' — a §08 forum_thread: votes, replies, polls, solve.
 *     Rendered exactly as the old /forum/{slug} page (ForumThreadView).
 *   kind 'embed' — any other contents row: §49 discussion (possibly a
 *     thread_anchors row for a tool/wiki page, or a linked-discussion
 *     thread about a parent content) or another CPT reached via a thread
 *     URL. Renders ThreadEmbedCard + the ReplyThread conversation engine
 *     (§49 comments — the one reply UI for every discussion).
 *
 * Parallel fetches (forum kind):
 *   - GET /thread/{slug}         → the thread
 *   - GET /thread/{id}/replies   → flat reply list (nested via parent_reply_id)
 *   - GET /thread/{id}/tags      → tags attached to the thread (Section 08)
 *   - GET /thread/polls/{id}     → poll + results (if the thread has a poll)
 *   - GET /content/{id}/related → related posts sidebar (Phase 05.14)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;

	let thread: ForumThread;
	let jsonLd: unknown = null;
	let backendCanSolve = false;
	let canEdit = false;
	let canDelete = false;
	let canModerate = false;
	let isBookmarked = false;
	let bookmarkId = '';
	let deletedUserIds: string[] | null = null;
	let userVotes: Record<string, number> = {};
	let indexable = true;
	let seoIndexable = true;
	try {
		// BUG-013/BUG-014: use the full envelope to capture the backend-provided
		// json_ld (QAPage structured data with acceptedAnswer text) and the
		// can_solve capability flag (which includes moderator scope, not just
		// the author). Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
		const envelope = await apiGetEnvelope<ForumThread>(ctx, `/thread/${slug}`);
		thread = envelope.data;
		indexable = envelope.indexable ?? thread.indexable ?? true;
		seoIndexable = envelope.seo_indexable ?? thread.seo_indexable ?? true;
		jsonLd = envelope.json_ld ?? null;
		backendCanSolve = envelope.can_solve ?? false;
		canEdit = envelope.can_edit ?? false;
		canDelete = envelope.can_delete ?? false;
		canModerate = envelope.can_moderate ?? false;
		isBookmarked = envelope.is_bookmarked ?? false;
		bookmarkId = envelope.bookmark_id ?? '';
		deletedUserIds = envelope.deleted_user_ids ?? null;
		userVotes = envelope.user_votes ?? {};
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			// Not a forum thread — entity threads and §49 discussions resolve
			// through the content API instead.
			return loadEmbedThread(event, slug);
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load discussion' });
	}

	let replies: ForumReply[] = [];
	let replyDeletedUserIds: string[] | null = null;
	try {
		// BUG-001: use the full envelope to capture deleted_user_ids emitted
		// by ListReplies for erased-mention redaction in reply bodies
		// (Section 15 / ADR-029). The thread envelope's deleted_user_ids only
		// covers the thread body — replies may mention different erased users.
		const repliesEnvelope = await apiGetEnvelope<ForumReply[]>(ctx, `/thread/${thread.id}/replies`);
		replies = repliesEnvelope.data ?? [];
		replyDeletedUserIds = repliesEnvelope.deleted_user_ids ?? null;
		userVotes = repliesEnvelope.user_votes ?? {};
	} catch {
		// A failed reply fetch must not blank the thread; render with no replies.
		replies = [];
	}

	// Tags, poll, and related content are independent once the thread is
	// known — fetch concurrently. Each degrades independently on failure.
	const [tags, poll, related] = await Promise.all([
		// Universal tags on the backing content (GET /content/{id}/tags,
		// Section 14). Best-effort — degrades to empty.
		thread.content_id
			? apiList<ForumTag>(ctx, `/content/${thread.content_id}/tags`)
					.then((r) => r.items)
					.catch(() => [] as ForumTag[])
			: Promise.resolve([] as ForumTag[]),
		// The thread's poll (GET /thread/polls/{threadId}) — 404s when none is
		// attached; degrade to null.
		apiGet<PollWithResults>(ctx, `/thread/polls/${thread.id}`).catch(() => null),
		// Related content for the sidebar (GET /content/{content_id}/related)
		// — forum threads have a backing contents row; related content must
		// be looked up by content_id, not the forum thread id.
		thread.content_id
			? apiList<{ id: string; title: string; slug: string; content_type: string }>(
					ctx,
					`/content/${thread.content_id}/related?page_size=5`
				)
					.then((r) => r.items)
					.catch(() => [] as { id: string; title: string; slug: string; content_type: string }[])
			: Promise.resolve([] as { id: string; title: string; slug: string; content_type: string }[])
	]);

	// BUG-014: canSolve now uses the backend-provided can_solve flag which
	// correctly includes moderators and permission-holders, not just the
	// author. Fall back to author check when the backend didn't provide it.
	const currentUserId = event.locals.user?.id ?? null;
	const canSolve = backendCanSolve || (authed && currentUserId === thread.author_user_id);

	// BUG-001: merge erased-user IDs from the thread body and reply bodies so
	// the frontend redacts mentions of erased users in both (Section 15 / ADR-029).
	const mergedDeletedUserIds =
		deletedUserIds || replyDeletedUserIds
			? Array.from(new Set([...(deletedUserIds ?? []), ...(replyDeletedUserIds ?? [])]))
			: null;

	// BUG-FIX: when a thread is solved but no explicit accepted reply was set
	// (legacy rows with parent_reply_id IS NULL), mirror the backend JSON-LD
	// fallback and pick the top-voted reply as the accepted answer.
	let acceptedReplyId: string | null = null;
	if (thread.solved) {
		acceptedReplyId = thread.parent_reply_id ?? null;
		if (!acceptedReplyId && replies.length > 0) {
			const top = replies.reduce((best, r) => {
				if (r.vote_score > best.vote_score) return r;
				if (r.vote_score === best.vote_score && r.created_at < best.created_at) return r;
				return best;
			}, replies[0]);
			acceptedReplyId = top.id;
		}
	}

	return {
		kind: 'forum' as const,
		thread,
		indexable,
		seoIndexable,
		replies,
		tags,
		poll,
		related,
		authed,
		canSolve,
		canEdit,
		canDelete,
		canModerate,
		isBookmarked,
		bookmarkId,
		jsonLd,
		deletedUserIds: mergedDeletedUserIds,
		acceptedReplyId,
		userVotes,
		currentUserId
	};
};

/**
 * Entity-thread branch — resolves a contents row by slug and builds the
 * embed card + first page of comments. Covers:
 *   - thread_anchors rows (`discussion` bound to a tool/wiki entity)
 *   - §49 linked discussions (`discussion` about a parent content)
 *   - standalone `discussion` rows (no parent — thread is itself)
 *   - any other CPT reached via /thread/{slug} (card links to canonical)
 */
async function loadEmbedThread(event: ServerLoadEvent<{ slug: string }>, slug: string) {
	const ctx = apiContext(event);

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
			throw error(404, { code: 'not_found', message: 'Discussion not found' });
		}
		if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
			throw error(403, { code: 'forbidden', message: 'You do not have access to this thread.' });
		}
		throw error(502, { code: 'upstream_error', message: 'Failed to load thread' });
	}

	const currentUserId = event.locals.user?.id ?? null;
	const userGroups = event.locals.user?.groups ?? [];

	// Backed threads are normal threads: the author (or content.edit_any /
	// content.delete_any holders) can edit/delete the thread inline, and
	// author/comments.pin holders moderate replies — same affordances as
	// loadContentArticle. One best-effort /me/permissions call.
	const isOwner = !!article.author_id && currentUserId === article.author_id;
	const isSuperAdmin = userGroups.some((g) => g === 'super_admin' || g === 'Super Admin');
	let canModerate = isOwner || isSuperAdmin;
	let canEdit = isOwner || isSuperAdmin;
	let canDelete = isOwner || isSuperAdmin;
	if (!isOwner && !isSuperAdmin && event.locals.user) {
		try {
			const permRes = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			const perms = permRes?.permissions ?? [];
			canModerate = hasPerm(perms, 'comments.pin');
			canEdit = hasPerm(perms, 'content.edit_any');
			canDelete = hasPerm(perms, 'content.delete_any');
		} catch {
			canModerate = false;
		}
	}

	let comments: Comment[] = [];
	let commentsLoadError = false;
	let commentDeletedUserIds: string[] | null = null;
	let commentsTotal: number | null = null;
	try {
		const envelope = await apiGetEnvelope<Comment[]>(ctx, `/content/${article.id}/comments?page_size=50`);
		comments = envelope.data ?? [];
		commentDeletedUserIds = envelope.deleted_user_ids ?? null;
		commentsTotal = (envelope.meta as { total?: number } | undefined)?.total ?? null;
	} catch {
		commentsLoadError = true;
	}

	const allDeletedUserIds = new Set<string>();
	if (deletedUserIds) for (const id of deletedUserIds) allDeletedUserIds.add(id);
	if (commentDeletedUserIds) for (const id of commentDeletedUserIds) allDeletedUserIds.add(id);
	const mergedDeletedUserIds = allDeletedUserIds.size > 0 ? Array.from(allDeletedUserIds) : null;

	return {
		kind: 'embed' as const,
		article,
		embed: buildThreadEmbed(article),
		comments,
		commentsTotal,
		commentsLoadError,
		authed: !!event.locals.user,
		currentUserId,
		canModerate,
		canEdit,
		canDelete,
		deletedUserIds: mergedDeletedUserIds,
		jsonLd,
		indexable: article.indexable ?? true,
		seoIndexable: article.seo_indexable ?? true
	};
}

/**
 * The "what this thread is about" card. Priority: thread_anchors entity →
 * §49 thread_parent → the row itself (non-discussion rows only — a bare
 * discussion IS the thread, so no card).
 */
function buildThreadEmbed(article: ContentArticle): ThreadEmbed | null {
	if (article.thread_anchor) {
		const a = article.thread_anchor;
		return {
			kind: 'entity',
			title: a.entity_title,
			href: a.entity_url,
			excerpt: a.entity_summary ?? null,
			entityType: a.entity_type
		};
	}

	if (article.thread_parent) {
		const p = article.thread_parent;
		return {
			kind: 'parent',
			title: p.title,
			href: contentHref({ slug: p.slug, content_type: p.content_type }),
			excerpt: p.excerpt ?? null,
			imageUrl: p.cover_image_url ?? null,
			badge: p.content_type
		};
	}

	if (article.content_type === 'discussion') return null;

	return {
		kind: 'self',
		title: article.title,
		href: contentHref(article),
		excerpt: article.excerpt ?? null,
		imageUrl: article.featured_image_url ?? article.header_image_url ?? null,
		badge: article.content_type
	};
}
