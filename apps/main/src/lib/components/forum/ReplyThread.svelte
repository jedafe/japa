<script lang="ts">
	import { CornerUpLeft } from 'lucide-svelte';
	import { invalidateAll } from '$app/navigation';
	import { requireAuthOrRedirect } from '#lib/auth/guest.js';
	import { t } from '#lib/i18n/index.js';
	import type { ForumReply } from '#lib/types/index.js';
	import ReplyItem from './ReplyItem.svelte';
	import LazyCommentEditor from '#lib/components/comments/LazyCommentEditor.svelte';
	import { forumApi, type ConversationBackend } from './conversationApi.js';

	interface Props {
		/** Flat list of all replies for the thread (parent_reply_id links them). */
		replies: ForumReply[];
		threadId: string;
		/** Conversation backend — defaults to the §08 forum replies API.
		 *  Entity threads (thread-rename spec) pass commentsApi(contentId). */
		backend?: ConversationBackend;
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
		/** Preview mode (entity pages): hides the top-level composer and the
		 *  "Replies" heading — new conversations start on /thread/{slug},
		 *  while per-reply interactions (vote/reply/edit) stay intact. */
		preview?: boolean;
		/** Top-level ordering: 'top' ranks by votes, 'newest'/'oldest' are
		 *  chronological. Children always stay chronological. Entity
		 *  previews pass the order matching their fetch sort; the canonical
		 *  page leaves the default and lets the sort chips drive. */
		sort?: 'top' | 'newest' | 'oldest';
	}
	let { replies, threadId, backend, authed, canSolve, acceptedReplyId, userVotes, canModerate, currentUserId, deletedUserIds, hostUserId, isHost, onMarkSolved, preview = false, sort = 'top' }: Props = $props();

	const convApi = $derived(backend ?? forumApi(threadId));

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

	// Top-level reply composer state — body is base64 TipTap JSON. resetNonce
	// clears the editor's internal document after a successful submit.
	let topBody = $state<string>('');
	let topSubmitting = $state(false);
	let resetNonce = $state(0);
	let addedTop: ForumReply[] = $state([]);

	async function submitTopReply() {
		if (topSubmitting) return;
		// Guests click Post reply → redirect to /login with a return-to
		// param (the button stays enabled for them — see the markup below).
		if (!(await requireAuthOrRedirect())) return;
		if (!topBody) return;
		topSubmitting = true;
		const created = await convApi.postReply(null, topBody);
		topSubmitting = false;
		if (!created) return;
		topBody = '';
		resetNonce++;
		// Refetch so the new reply renders with the joined author row — the
		// POST response is a bare forum_replies row (no author fields), and a
		// locally-appended copy would show "Member" + a blank avatar plus
		// duplicate once the server list refreshes.
		await invalidateAll();
		addedTop = [];
	}

	// Reply sort — the chips on the canonical page set the override; entity
	// previews drive it via the `sort` prop (chips are hidden there).
	const sortOptions = $derived([
		{ value: 'top', label: t('comments.sort.top', 'Top') },
		{ value: 'newest', label: t('comments.sort.new', 'New') },
		{ value: 'oldest', label: t('comments.sort.oldest', 'Oldest') }
	] as const);
	let sortOverride = $state<'top' | 'newest' | 'oldest' | null>(null);
	const activeSort = $derived(sortOverride ?? sort);

	const allTop = $derived.by(() => {
		// §88 BUG-008: pin host replies above regular replies when hostUserId
		// is set (AMA threads). Otherwise rank top-level replies by the active
		// sort — vote score by default so upvoted answers surface first.
		// Children render in fetch order (top-voted first — parent_id fetches
		// default to sort=top server-side); only top-level order is re-ranked.
		const combined = [...topLevel, ...addedTop];
		const rank = (a: ForumReply, b: ForumReply) =>
			b.vote_score - a.vote_score || a.created_at.localeCompare(b.created_at);
		const order = activeSort === 'newest'
			? (a: ForumReply, b: ForumReply) => b.created_at.localeCompare(a.created_at) || rank(a, b)
			: activeSort === 'oldest'
				? (a: ForumReply, b: ForumReply) => a.created_at.localeCompare(b.created_at) || rank(a, b)
				: rank;
		if (!hostUserId) return combined.sort(order);
		return combined.sort((a, b) => {
			const aHost = a.author_user_id === hostUserId ? 0 : 1;
			const bHost = b.author_user_id === hostUserId ? 0 : 1;
			if (aHost !== bHost) return aHost - bHost;
			return order(a, b);
		});
	});
	const replyCount = $derived(replies.length + addedTop.length);
</script>

<div id="replies" class={preview ? 'mt-2' : 'mt-10 border-t border-[var(--color-border)] pt-6'}>
	{#if !preview}
		<div class="mb-4 flex items-center justify-between gap-3">
			<h2 class="text-base font-semibold text-[var(--color-text)]">
				{t('forum.replies_heading', 'Replies')}
				<span class="ms-1.5 text-sm font-normal text-[var(--color-muted)]">{replyCount}</span>
			</h2>
			{#if allTop.length > 1}
				<div class="flex items-center gap-1" role="tablist" aria-label={t('comments.sort_by', 'Sort by')}>
					{#each sortOptions as opt (opt.value)}
						<button
							type="button"
							role="tab"
							aria-selected={activeSort === opt.value}
							onclick={() => (sortOverride = opt.value)}
							class="rounded-full px-3 py-1 text-xs font-medium transition-colors {activeSort === opt.value
								? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
								: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							{opt.label}
						</button>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Composer renders for guests too — the submit button stays enabled
		     and redirects to /login on click (see submitTopReply). -->
		<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
			<LazyCommentEditor placeholder={t('forum.reply_placeholder', 'Write a reply…')} oninput={(b) => (topBody = b)} {resetNonce} />
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
	{/if}

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
					backend={convApi}
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
