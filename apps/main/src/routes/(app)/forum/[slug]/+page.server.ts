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
	let isBookmarked = false;
	let bookmarkId = '';
	let deletedUserIds: string[] | null = null;
	let userVotes: Record<string, number> = {};
	try {
		// BUG-013/BUG-014: use the full envelope to capture the backend-provided
		// json_ld (QAPage structured data with acceptedAnswer text) and the
		// can_solve capability flag (which includes moderator scope, not just
		// the author). Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
		const envelope = await apiGetEnvelope<ForumThread>(ctx, `/forum/${slug}`);
		thread = envelope.data;
		jsonLd = envelope.json_ld ?? null;
		backendCanSolve = envelope.can_solve ?? false;
		canEdit = envelope.can_edit ?? false;
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

	// Fetch the tags attached to the thread (GET /forum/{id}/tags).
	// Best-effort — degrades to empty if the endpoint is unavailable.
	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Tags)
	let tags: ForumTag[] = [];
	try {
		const { items } = await apiList<ForumTag>(ctx, `/forum/${thread.id}/tags`);
		tags = items;
	} catch {
		tags = [];
	}

	// Fetch the thread's poll if one exists (GET /forum/polls/{threadId}).
	// The endpoint returns 404 when no poll is attached — degrade to null.
	let poll: PollWithResults | null = null;
	try {
		poll = await apiGet<PollWithResults>(ctx, `/forum/polls/${thread.id}`);
	} catch {
		poll = null;
	}

	// Fetch related content for the sidebar (GET /content/{content_id}/related).
	// Best-effort — degrades to empty if the endpoint is unavailable.
	// Ref: Phase 05.14. The forum thread has a backing contents row; related
	// content must be looked up by that content_id, not the forum thread id.
	let related: { id: string; title: string; slug: string; content_type: string }[] = [];
	try {
		if (thread.content_id) {
			related = (
				await apiList<{ id: string; title: string; slug: string; content_type: string }>(
					ctx,
					`/content/${thread.content_id}/related?limit=5`
				)
			).items;
		}
	} catch {
		related = [];
	}

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

	return {
		thread,
		replies,
		tags,
		poll,
		related,
		authed,
		canSolve,
		canEdit,
		isBookmarked,
		bookmarkId,
		jsonLd,
		deletedUserIds: mergedDeletedUserIds,
		acceptedReplyId: thread.solved ? (thread.parent_reply_id ?? null) : null,
		userVotes
	};
};
