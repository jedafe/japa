<script lang="ts">
	import { ChevronDown, ChevronRight, CornerUpLeft, CheckCircle2, Pencil, Trash2, Loader2 } from 'lucide-svelte';
	import { relativeTime } from '$lib/feed/time';
	import { tiptapBodyToHtml } from '$lib/tiptap/serialize';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { api } from '$lib/api/client';
	import { t, i18n } from '$lib/i18n';
	import type { ForumReply } from '$lib/types';
	import VoteRail from './VoteRail.svelte';
	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
	// Reply view count is recorded via POST /forum/replies/{id}/view when the
	// reply is first visible in the viewport.
	// Self-import replaces the deprecated <svelte:self> for recursive nesting.
	import ReplyItem from './ReplyItem.svelte';
	import CommentEditor from '$lib/components/comments/CommentEditor.svelte';

	interface Props {
		reply: ForumReply;
		/** Resolves the child replies for a given reply id (from the flat list). */
		childrenOf: (id: string) => ForumReply[];
		depth: number;
		threadId: string;
		authed: boolean;
		/** Whether the current user can mark this thread solved (OP/mod). */
		canSolve: boolean;
		/** Whether this reply is the accepted answer. */
		isAccepted: boolean;
		/** Map of reply_id -> current user's vote value (1, -1, 0). */
		userVotes?: Record<string, number>;
		/** Moderator flag — allows edit/delete on other users' replies. */
		canModerate?: boolean;
		/** Current user id (to detect own replies). */
		currentUserId?: string | null;
		/** Optional set of erased user UUIDs for GDPR/ADR-029 mention
		 * redaction (Section 15). Passed down from the thread page. */
		deletedUserIds?: Set<string>;
		/** §88 AMA: the host's user ID. When set and this reply's author
		 * matches, the reply is highlighted as a host answer. */
		hostUserId?: string;
		/** §88 AMA: whether the current viewer is the host (for approve UI). */
		isHost?: boolean;
		/** Callback to mark this reply as the accepted answer. */
		onMarkSolved?: (replyId: string) => void;
	}
	let {
		reply,
		childrenOf,
		depth,
		threadId,
		authed,
		canSolve,
		isAccepted,
		userVotes,
		canModerate,
		currentUserId,
		deletedUserIds,
		hostUserId,
		isHost,
		onMarkSolved
	}: Props = $props();

	const myVote = $derived.by(() => (userVotes?.[reply.id] ?? 0) as 1 | -1 | 0);

	const MAX_DEPTH = 6;
	const bodyHtml = $derived(
		tiptapBodyToHtml(reply.body, {
			deletedUserIds,
			deletedUserLabel: t('mentions.deleted_user', '[deleted user]')
		})
	);
	const time = $derived(relativeTime(reply.created_at, new Date(), i18n.locale));
	const authorLabel = $derived(
		reply.is_anonymous
			? t('forum.anonymous', 'Anonymous')
			: reply.author_display_name || reply.author_username || t('forum.member', 'Member')
	);
	const beyondMaxDepth = $derived(depth >= MAX_DEPTH);

	// §88 AMA: is this reply from the AMA host? When true, highlight it.
	const isHostReply = $derived(!!hostUserId && reply.author_user_id === hostUserId);

	// §88 AMA: is this a pending question awaiting host approval?
	const isPending = $derived(!!reply.ama_pending);

	let approving = $state(false);

	// §88: host approves a pending question via POST /forum/ama/:id/questions/:replyId/approve
	async function approveQuestion() {
		if (approving) return;
		approving = true;
		await api.post(`/forum/ama/${threadId}/questions/${reply.id}/approve`, {});
		approving = false;
		reply.ama_pending = false;
	}

	let collapsed = $state(false);
	let composing = $state(false);
	let replyBody = $state<string>('');
	let submitting = $state(false);
	let localChildren = $state<ForumReply[]>([]);

	// Edit/delete state.
	let editing = $state(false);
	let editBody = $state<string>('');
	let savingEdit = $state(false);
	let editError = $state<string | null>(null);
	let deleting = $state(false);
	let deleteError = $state<string | null>(null);
	let removed = $state(false);
	let viewRecorded = $state(false);
	let replyEl = $state<HTMLElement | null>(null);

	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
	// Record a view once this reply enters the viewport. Only for authenticated
	// viewers; the route requires auth, so guests would just 401.
	$effect(() => {
		if (!authed) return;
		const el = replyEl;
		if (!el || viewRecorded) return;

		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					api.post(`/forum/replies/${reply.id}/view`, {});
					viewRecorded = true;
					observer.disconnect();
				}
			},
			{ threshold: 0.5 }
		);
		observer.observe(el);
		return () => observer.disconnect();
	});

	const canEditReply = $derived.by(() => authed && (canModerate || currentUserId === reply.author_user_id));
	const canDeleteReply = $derived.by(() => authed && (canModerate || currentUserId === reply.author_user_id));
	const canMarkSolved = $derived.by(() => canSolve && !isAccepted && !removed && typeof onMarkSolved === 'function');

	// Server-resolved children plus any posted locally during this session.
	const allChildren = $derived([...childrenOf(reply.id), ...localChildren]);

	async function submitReply() {
		if (!replyBody || submitting || !authed) return;
		submitting = true;
		const res = await api.post<ForumReply>(`/forum/${threadId}/replies`, {
			body: replyBody,
			parent_reply_id: reply.id
		});
		submitting = false;
		if (res.error || !res.data) return;
		localChildren = [...localChildren, res.data];
		replyBody = '';
		composing = false;
	}

	async function saveEdit() {
		if (savingEdit || !canEditReply || !editBody) return;
		savingEdit = true;
		editError = null;
		const res = await api.patch<ForumReply>(`/forum/replies/${reply.id}`, {
			body: editBody,
			is_anonymous: reply.is_anonymous
		});
		savingEdit = false;
		if (res.error || !res.data) {
			editError = res.error?.message || t('forum.edit_error', 'Failed to save edits.');
			return;
		}
		// Mutate the local reply so the UI updates without a full reload.
		reply.body = res.data.body;
		reply.is_edited = res.data.is_edited;
		reply.updated_at = res.data.updated_at;
		editing = false;
	}

	async function deleteReply() {
		if (deleting || !canDeleteReply) return;
		if (!confirm(t('forum.delete_reply_confirm', 'Delete this reply? This cannot be undone.'))) return;
		deleting = true;
		deleteError = null;
		const res = await api.delete(`/forum/replies/${reply.id}`);
		deleting = false;
		if (res.error) {
			deleteError = res.error.message || t('forum.delete_error', 'Failed to delete reply.');
			return;
		}
		removed = true;
	}
</script>

<div bind:this={replyEl} class={`flex gap-3 ${isHostReply ? 'rounded-[var(--radius-card)] border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/[0.03] p-3' : ''}`} class:pl-1={depth > 0 && !isHostReply}>
	<!-- Vote rail (vertical) -->
	<VoteRail
		entityType="reply"
		entityId={reply.id}
		score={reply.vote_score}
		layout="vertical"
		{authed}
		userVote={myVote}
	/>

	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
			{#if isAccepted}
				<span
					class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-success)]/10 px-1.5 py-0.5 font-semibold text-[var(--color-success)]"
				>
					<CheckCircle2 size={11} /> {t('forum.accepted', 'Accepted')}
				</span>
			{/if}
			{#if isHostReply}
				<span
					class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-1.5 py-0.5 font-semibold text-[var(--color-primary)]"
				>
					{t('ama.host_badge', 'Host')}
				</span>
			{/if}
			{#if isPending}
				<span
					class="inline-flex items-center gap-0.5 rounded-full bg-amber-500/10 px-1.5 py-0.5 font-semibold text-amber-600"
				>
					{t('ama.pending', 'Pending')}
				</span>
			{/if}
			<span class="font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
			{#if time}<span>·</span><span>{time}</span>{/if}
			{#if reply.is_edited}<span>·</span><span class="italic">{t('forum.edited', 'edited')}</span>{/if}
			{#if reply.view_count > 0}<span>·</span><span class="italic">{reply.view_count} {t('forum.views', 'views')}</span>{/if}
		</div>

		<!-- Body -->
		{#if !collapsed}
			{#if removed}
				<p class="mt-1.5 text-sm italic text-[var(--color-muted)]">
					{t('forum.deleted_reply', '[deleted]')}
				</p>
			{:else if editing}
				<div class="mt-2 space-y-2">
					{#if editError}
						<p class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-primary)]">
							{editError}
						</p>
					{/if}
					<CommentEditor
						initialBody={reply.body}
						oninput={(b) => (editBody = b)}
						placeholder={t('forum.edit_placeholder', 'Edit your reply…')}
					/>
					<div class="flex items-center justify-end gap-2">
						<button
							type="button"
							onclick={() => { editing = false; editBody = reply.body; editError = null; }}
							class="rounded-[var(--radius)] px-2.5 py-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
						>
							{t('common.cancel', 'Cancel')}
						</button>
						<button
							type="button"
							onclick={saveEdit}
							disabled={!editBody || savingEdit}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							{savingEdit ? t('common.loading', 'Loading') : t('common.save', 'Save')}
						</button>
					</div>
				</div>
			{:else}
				<div class="prose prose-sm mt-1.5 max-w-none text-[14px] leading-relaxed text-[var(--color-text)]" use:hydrateMentions>
					{@html bodyHtml}
				</div>
			{/if}

			<!-- Actions -->
			<div class="mt-2 flex items-center gap-3 text-xs text-[var(--color-muted)]">
				{#if authed}
					<button
						type="button"
						onclick={() => (composing = !composing)}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-text)]"
					>
						<CornerUpLeft size={12} /> {t('forum.reply', 'Reply')}
					</button>
				{/if}
				{#if isPending && isHost}
					<button
						type="button"
						onclick={approveQuestion}
						disabled={approving}
						class="inline-flex items-center gap-1 font-medium text-[var(--color-primary)] transition-colors hover:text-[var(--color-primary-hover)] disabled:opacity-50"
					>
						<CheckCircle2 size={12} /> {approving ? t('common.loading', 'Loading') : t('ama.approve', 'Approve')}
					</button>
				{/if}
				{#if allChildren.length > 0}
					<button
						type="button"
						onclick={() => (collapsed = !collapsed)}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-text)]"
						aria-expanded={!collapsed}
					>
						{#if collapsed}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
						{allChildren.length} {t('forum.replies', 'replies')}
					</button>
				{/if}
				{#if canEditReply && !editing && !removed}
					<button
						type="button"
						onclick={() => { editing = true; editBody = reply.body; editError = null; }}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-text)]"
					>
						<Pencil size={12} /> {t('forum.edit', 'Edit')}
					</button>
				{/if}
				{#if canDeleteReply && !editing && !removed}
					<button
						type="button"
						onclick={deleteReply}
						disabled={deleting}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-error)] disabled:opacity-50"
					>
						{#if deleting}<Loader2 size={12} class="animate-spin" />{:else}<Trash2 size={12} />{/if}
						{t('forum.delete', 'Delete')}
					</button>
				{/if}
				{#if canMarkSolved}
					<button
						type="button"
						onclick={() => onMarkSolved?.(reply.id)}
						class="inline-flex items-center gap-1 text-[var(--color-success)] transition-colors hover:text-[var(--color-text)]"
					>
						<CheckCircle2 size={12} /> {t('forum.mark_answer', 'Mark as answer')}
					</button>
				{/if}
			</div>
			{#if deleteError}
				<p class="mt-2 rounded-[var(--radius)] border border-[var(--color-error)]/30 bg-[color-mix(in_srgb,var(--color-error)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-error)]">
					{deleteError}
				</p>
			{/if}

			<!-- Inline reply composer -->
			{#if composing && authed}
				<div class="mt-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-2.5">
					<CommentEditor placeholder={t('forum.reply_placeholder', 'Write a reply…')} oninput={(b) => (replyBody = b)} />
					<div class="mt-2 flex items-center justify-end gap-2">
						<button
							type="button"
							onclick={() => { composing = false; replyBody = ''; }}
							class="rounded-[var(--radius)] px-2.5 py-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
						>
							{t('common.cancel', 'Cancel')}
						</button>
						<button
							type="button"
							onclick={submitReply}
							disabled={!replyBody || submitting}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							{submitting ? t('common.loading', 'Loading') : t('forum.post_reply', 'Post reply')}
						</button>
					</div>
				</div>
			{/if}

			<!-- Nested children -->
			{#if allChildren.length > 0 && !beyondMaxDepth}
				<div class="mt-3 space-y-3 border-l border-[var(--color-border)] pl-3">
					{#each allChildren as child (child.id)}
						<ReplyItem
							reply={child}
							{childrenOf}
							depth={depth + 1}
							{threadId}
							{authed}
							{canSolve}
							isAccepted={false}
							{userVotes}
							{canModerate}
							{currentUserId}
							{deletedUserIds}
							{hostUserId}
							{isHost}
						/>
					{/each}
				</div>
			{:else if allChildren.length > 0 && beyondMaxDepth}
				<a
					href={`/forum/${threadId}?reply=${allChildren[0].id}#continue`}
					class="mt-2 inline-block text-xs font-medium text-[var(--color-primary)] hover:underline"
				>
					{t('forum.continue_thread', 'Continue this thread →')}
				</a>
			{/if}
		{:else}
			<button
				type="button"
				onclick={() => (collapsed = !collapsed)}
				class="mt-1 inline-flex items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				<ChevronRight size={12} /> {t('forum.expand', 'Expand')}
			</button>
		{/if}
	</div>
</div>
