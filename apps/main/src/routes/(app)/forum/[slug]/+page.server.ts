import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, apiList, ApiError } from '$lib/server/api';
import type { ForumThread, ForumReply, ForumTag, PollWithResults } from '$lib/types';

/**
 * Single forum discussion at /forum/[slug] — rendered inside the app shell
 * (NOT a reading-layout exception; only blog/CPT singles and /create break
 * the shell per ADR-058). Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
 *
 * Parallel fetches:
 *   - GET /forum/{slug}         → the thread
 *   - GET /forum/{id}/replies   → flat reply list (nested via parent_reply_id)
 *   - GET /forum/{id}/tags      → tags attached to the thread (Section 08)
 *   - GET /forum/polls/{id}     → poll + results (if the thread has a poll)
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
		const envelope = await apiGetEnvelope<ForumThread>(ctx, `/forum/${slug}`);
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
			throw error(404, { code: 'not_found', message: 'Discussion not found' });
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
		const repliesEnvelope = await apiGetEnvelope<ForumReply[]>(ctx, `/forum/${thread.id}/replies`);
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
		// The thread's poll (GET /forum/polls/{threadId}) — 404s when none is
		// attached; degrade to null.
		apiGet<PollWithResults>(ctx, `/forum/polls/${thread.id}`).catch(() => null),
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
