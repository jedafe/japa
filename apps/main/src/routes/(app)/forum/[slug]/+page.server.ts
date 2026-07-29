import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ForumThread, ForumReply, PollWithResults } from '$lib/types';

/**
 * Single forum discussion at /forum/[slug] — rendered inside the app shell
 * (NOT a reading-layout exception; only blog/CPT singles and /create break
 * the shell per ADR-058). Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
 *
 * Parallel fetches:
 *   - GET /forum/{slug}         → the thread
 *   - GET /forum/{id}/replies   → flat reply list (nested via parent_reply_id)
 *   - GET /forum/polls/{id}     → poll + results (if the thread has a poll)
 *   - GET /content/{id}/related → related posts sidebar (Phase 05.14)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;

	let thread: ForumThread;
	try {
		thread = await apiGet<ForumThread>(ctx, `/forum/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Discussion not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load discussion' });
	}

	let replies: ForumReply[] = [];
	try {
		const { items } = await apiList<ForumReply>(ctx, `/forum/${thread.id}/replies`);
		replies = items;
	} catch {
		// A failed reply fetch must not blank the thread; render with no replies.
		replies = [];
	}

	// Fetch the thread's poll if one exists (GET /forum/polls/{threadId}).
	// The endpoint returns 404 when no poll is attached — degrade to null.
	let poll: PollWithResults | null = null;
	try {
		poll = await apiGet<PollWithResults>(ctx, `/forum/polls/${thread.id}`);
	} catch {
		poll = null;
	}

	// Fetch related content for the sidebar (GET /content/{id}/related).
	// Best-effort — degrades to empty if the endpoint is unavailable.
	let related: { id: string; title: string; slug: string; content_type: string }[] = [];
	try {
		related = (await apiList<{ id: string; title: string; slug: string; content_type: string }>(
			ctx,
			`/content/${thread.id}/related?limit=5`
		)).items;
	} catch {
		related = [];
	}

	// The current user may mark the thread solved if they are the author or a
	// moderator. The backend enforces the actual permission; this flag only
	// gates the UI affordance. The forum API does not join author details, so
	// we compare against the session user id directly.
	const currentUserId = event.locals.user?.id ?? null;
	const canSolve = authed && currentUserId === thread.author_user_id;

	return {
		thread,
		replies,
		poll,
		related,
		authed,
		canSolve,
		acceptedReplyId: thread.solved ? null : null
	};
};
