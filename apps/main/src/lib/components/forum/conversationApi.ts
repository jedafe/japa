/**
 * Conversation backend adapter — ReplyThread/ReplyItem render one UI for
 * every discussion surface (thread-rename spec: "use ReplyThread in all
 * conversation"). The forum backend is the default and preserves the §08
 * calls verbatim; the comments backend swaps endpoints and normalizes the
 * §49 Comment shape into ForumReply so the components stay agnostic.
 */
import { api } from '#lib/api/client.js';
import { queueCommentView } from '#lib/comments/viewTracker.js';
import { actAsPersona } from '#lib/stores/actAsPersona.svelte.js';
import type { Comment, ForumReply } from '#lib/types/index.js';

export interface ConversationBackend {
	/** 'forum' — §08 forum_replies; 'comments' — §49 contents comments. */
	kind: 'forum' | 'comments';
	/** forum_threads.id | contents.id — the endpoint root id. */
	entityId: string;
	/** VoteRail backend — 'comment' hits /comments/{id}/vote. */
	voteBackend: 'forum' | 'comments';
	/** ReportDialog entity_type ('forum_reply' | 'comment'). */
	reportEntity: 'forum_reply' | 'comment';
	/** Backend reply-depth cap — replies beyond this are rejected
	 *  (comments allow depth 0–3; forum replies are uncapped). */
	maxDepth: number;
	/** Whether the reply edit window hides the edit affordance once it
	 *  lapses (forum hard-blocks; comments only flag is_edited). */
	editWindowApplies: boolean;
	/** Comment pin affordance (comments backend only). */
	pin?: (id: string, pinned: boolean) => Promise<boolean>;
	/** Raw (literal-unresolved) body for the edit prefill. */
	fetchRawBody: (id: string) => Promise<string | null>;
	/** Post a reply — parentId=null for a top-level entry. */
	postReply: (parentId: string | null, body: string) => Promise<ForumReply | null>;
	editReply: (id: string, body: string, isAnonymous: boolean) => Promise<{ error?: string; code?: string } | null>;
	deleteReply: (id: string) => Promise<{ error?: string } | null>;
	recordView: (id: string) => void;
}

/** Normalize a §49 Comment into the ForumReply shape ReplyItem renders. */
export function commentToReply(c: Comment, contentId: string): ForumReply {
	return {
		id: c.id,
		thread_id: contentId,
		parent_reply_id: c.parent_id,
		author_user_id: c.author_user_id ?? '',
		body: c.body,
		vote_score: c.vote_score ?? 0,
		view_count: c.view_count ?? 0,
		is_anonymous: c.is_anonymous,
		is_edited: c.is_edited,
		created_at: c.created_at,
		updated_at: c.updated_at,
		deleted_at: c.deleted_at,
		author_username: c.username || null,
		author_display_name: c.display_name || null,
		author_avatar_url: c.avatar_url || null,
		is_pinned: c.is_pinned,
		viewer_can_manage: c.viewer_can_manage
	};
}

export function forumApi(threadId: string): ConversationBackend {
	return {
		kind: 'forum',
		entityId: threadId,
		voteBackend: 'forum',
		reportEntity: 'forum_reply',
		maxDepth: Number.POSITIVE_INFINITY,
		editWindowApplies: true,
		fetchRawBody: async (id) => {
			const res = await api.get<ForumReply[]>(`/thread/${threadId}/replies?raw=1`);
			return res.data?.find((r) => r.id === id)?.body ?? null;
		},
		postReply: async (parentId, body) => {
			const res = await api.post<ForumReply>(`/thread/${threadId}/replies`, {
				body,
				...(parentId ? { parent_reply_id: parentId } : {}),
				// Delegated authorship: the persona authors the reply.
				...(actAsPersona.actorId ? { author_id: actAsPersona.actorId } : {})
			});
			return res.error ? null : (res.data ?? null);
		},
		editReply: async (id, body, isAnonymous) => {
			const res = await api.patch<ForumReply>(`/thread/replies/${id}`, {
				body,
				is_anonymous: isAnonymous
			});
			if (res.error || !res.data)
				return { error: res.error?.message, code: res.error?.code };
			return null;
		},
		deleteReply: async (id) => {
			const res = await api.delete(`/thread/replies/${id}`);
			return res.error ? { error: res.error.message } : null;
		},
		recordView: (id) => {
			void api.post(`/thread/replies/${id}/view`, {}, { 'x-silent-error': '1' });
		}
	};
}

async function fetchCommentPage(contentId: string, parentId: string | null, offset: number) {
	const actor = actAsPersona.actorId ? `&actor_id=${actAsPersona.actorId}` : '';
	const qs = parentId
		? `parent_id=${parentId}&page_size=100&page_offset=${offset}${actor}`
		: `sort=top&page_size=100&page_offset=${offset}${actor}`;
	const res = await api.getEnvelope<Comment[]>(`/content/${contentId}/comments?${qs}`);
	const meta = (res.envelope as { meta?: { total?: number } } | null)?.meta;
	return { items: res.data ?? [], total: meta?.total ?? res.data?.length ?? 0 };
}

export async function listAllComments(contentId: string): Promise<Comment[]> {
	const out: Comment[] = [];
	// Page every top-level comment (sort=top — ReplyThread re-ranks anyway).
	for (let offset = 0; ; ) {
		const { items, total } = await fetchCommentPage(contentId, null, offset);
		out.push(...items);
		offset += items.length;
		if (items.length === 0 || offset >= total) break;
	}
	// Depth is capped at 3 — walk reply_count'd parents level by level.
	// Each level's parents fetch in parallel; sequential walks made the
	// client hydration pass N+1-slow on busy threads.
	let parents = out.filter((c) => (c.reply_count ?? 0) > 0);
	while (parents.length > 0) {
		const subtrees = await Promise.all(
			parents.map(async (p) => {
				const branch: Comment[] = [];
				for (let offset = 0; ; ) {
					const { items, total } = await fetchCommentPage(contentId, p.id, offset);
					branch.push(...items);
					offset += items.length;
					if (items.length === 0 || offset >= total) break;
				}
				return branch;
			})
		);
		const next = subtrees.flat();
		out.push(...next);
		parents = next.filter((c) => (c.reply_count ?? 0) > 0);
	}
	return out;
}

export function commentsApi(contentId: string): ConversationBackend {
	return {
		kind: 'comments',
		entityId: contentId,
		voteBackend: 'comments',
		reportEntity: 'comment',
		maxDepth: 3,
		editWindowApplies: false,
		pin: async (id, pinned) => {
			const res = pinned
				? await api.post(`/comments/${id}/pin`)
				: await api.delete(`/comments/${id}/pin`);
			return !res.error;
		},
		fetchRawBody: async (id) => {
			const res = await api.get<Comment>(`/comments/${id}?raw=1`);
			return res.error ? null : (res.data?.body ?? null);
		},
		postReply: async (parentId, body) => {
			const res = await api.post<Comment>(`/content/${contentId}/comments`, {
				body,
				...(parentId ? { parent_id: parentId } : {}),
				// Delegated authorship: the persona authors the comment.
				...(actAsPersona.actorId ? { author_id: actAsPersona.actorId } : {})
			});
			return res.error || !res.data ? null : commentToReply(res.data, contentId);
		},
		editReply: async (id, body) => {
			const res = await api.patch<Comment>(`/comments/${id}`, { body });
			if (res.error || !res.data)
				return { error: res.error?.message, code: res.error?.code };
			return null;
		},
		deleteReply: async (id) => {
			const res = await api.delete(`/comments/${id}`);
			return res.error ? { error: res.error.message } : null;
		},
		recordView: (id) => queueCommentView(id, true)
	};
}

/** Build the reply list + per-user vote map from raw comments. */
export function commentsConversation(
	comments: Comment[],
	contentId: string
): { replies: ForumReply[]; userVotes: Record<string, number> } {
	const userVotes: Record<string, number> = {};
	const replies = comments.map((c) => {
		userVotes[c.id] = c.user_vote ?? 0;
		return commentToReply(c, contentId);
	});
	return { replies, userVotes };
}
