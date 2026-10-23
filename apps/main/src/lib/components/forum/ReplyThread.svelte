<script lang="ts">
	import { CornerUpLeft } from 'lucide-svelte';
	import { invalidateAll } from '$app/navigation';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import type { ForumReply } from '$lib/types';
	import ReplyItem from './ReplyItem.svelte';
	import CommentEditor from '$lib/components/comments/CommentEditor.svelte';

	interface Props {
		/** Flat list of all replies for the thread (parent_reply_id links them). */
		replies: ForumReply[];
		threadId: string;
		authed: boolean;
		/** OP or moderator — controls the "mark as solved" affordance. */
		canSolve: boolean;
		/** ID of the accepted answer reply, if the thread is solved. */
		acceptedReplyId: string | null;
		/** Map of reply_id -> current user's vote value (1, -1, 0). */
		userVotes?: Record<string, number>;
		/** Moderator flag — allows edit/delete on other users' replies. */
		canModerate?: boolean;
		/** Current user id (to detect own replies). */
		currentUserId?: string | null;
		/** Optional set of erased user UUIDs for GDPR/ADR-029 mention
		 * redaction (Section 15). Passed down to ReplyItem. */
		deletedUserIds?: Set<string>;
		/** §88 AMA: the host's user ID. When set, host replies are highlighted
		 * and pinned above regular replies. */
		hostUserId?: string;
		/** §88 AMA: whether the current viewer is the host (for approve UI). */
		isHost?: boolean;
		/** Callback to mark a specific reply as the accepted answer. */
		onMarkSolved?: (replyId: string) => void;
	}
	let { replies, threadId, authed, canSolve, acceptedReplyId, userVotes, canModerate, currentUserId, deletedUserIds, hostUserId, isHost, onMarkSolved }: Props = $props();

	// Build a parent → children map from the flat reply list. Uses $derived so
	// the tree is computed synchronously during SSR (not deferred to $effect,
	// which only runs client-side after the first paint).
	const byParent = $derived.by(() => {
		const map = new Map<string | null, ForumReply[]>();
		for (const r of replies) {
			const key = r.parent_reply_id ?? null;
			const arr = map.get(key) ?? [];
			arr.push(r);
			map.set(key, arr);
		}
		return map;
	});

	const topLevel = $derived(byParent.get(null) ?? []);

	function childrenOf(id: string): ForumReply[] {
		return byParent.get(id) ?? [];
	}

	// Top-level reply composer state — body is base64 TipTap JSON.
	let topBody = $state<string>('');
	let topSubmitting = $state(false);
	let addedTop: ForumReply[] = $state([]);

	async function submitTopReply() {
		if (topSubmitting) return;
		// Guests click Post reply → redirect to /login with a return-to
		// param (the button stays enabled for them — see the markup below).
		if (!(await requireAuthOrRedirect())) return;
		if (!topBody) return;
		topSubmitting = true;
		const res = await api.post<ForumReply>(`/forum/${threadId}/replies`, { body: topBody });
		topSubmitting = false;
		if (res.error || !res.data) return;
		topBody = '';
		// Refetch so the new reply renders with the joined author row — the
		// POST response is a bare forum_replies row (no author fields), and a
		// locally-appended copy would show "Member" + a blank avatar plus
		// duplicate once the server list refreshes.
		await invalidateAll();
		addedTop = [];
	}

	const allTop = $derived.by(() => {
		// §88 BUG-008: pin host replies above regular replies when hostUserId
		// is set (AMA threads). Otherwise rank top-level replies by vote
		// score so upvoted answers surface first — parity with the comment
		// thread's default "top" sort. Children stay chronological.
		const combined = [...topLevel, ...addedTop];
		const rank = (a: ForumReply, b: ForumReply) =>
			b.vote_score - a.vote_score || a.created_at.localeCompare(b.created_at);
		if (!hostUserId) return combined.sort(rank);
		return combined.sort((a, b) => {
			const aHost = a.author_user_id === hostUserId ? 0 : 1;
			const bHost = b.author_user_id === hostUserId ? 0 : 1;
			if (aHost !== bHost) return aHost - bHost;
			return rank(a, b);
		});
	});
	const replyCount = $derived(replies.length + addedTop.length);
</script>

<div id="replies" class="mt-10 border-t border-[var(--color-border)] pt-6">
	<h2 class="mb-4 text-base font-semibold text-[var(--color-text)]">
		{t('forum.replies_heading', 'Replies')}
		<span class="ms-1.5 text-sm font-normal text-[var(--color-muted)]">{replyCount}</span>
	</h2>

	<!-- Composer renders for guests too — the submit button stays enabled
	     and redirects to /login on click (see submitTopReply). -->
	<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<CommentEditor placeholder={t('forum.reply_placeholder', 'Write a reply…')} oninput={(b) => (topBody = b)} />
		<div class="mt-2 flex items-center justify-between">
			<span class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)]">
				<CornerUpLeft class="rtl:-scale-x-100" size={12} /> {t('forum.replying_to_thread', 'Replying to this thread')}
			</span>
			<button
				type="button"
				onclick={submitTopReply}
				disabled={authed && (!topBody || topSubmitting)}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3.5 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{topSubmitting ? t('common.loading', 'Loading') : t('forum.post_reply', 'Post reply')}
			</button>
		</div>
	</div>

	{#if allTop.length === 0}
		<p class="py-8 text-center text-sm text-[var(--color-muted)]">
			{t('forum.no_replies', 'No replies yet — be the first to respond.')}
		</p>
	{:else}
		<div class="space-y-1">
			{#each allTop as reply (reply.id)}
				<ReplyItem
					{reply}
					{childrenOf}
					depth={0}
					{threadId}
					{authed}
					{canSolve}
					isAccepted={acceptedReplyId === reply.id}
					{userVotes}
					{canModerate}
					{currentUserId}
					{deletedUserIds}
					{hostUserId}
					{isHost}
					onMarkSolved={onMarkSolved}
				/>
			{/each}
		</div>
	{/if}
</div>
