<script lang="ts">
	import { ChevronDown, ChevronRight, CornerUpLeft, CheckCircle2 } from 'lucide-svelte';
	import { relativeTime } from '$lib/feed/time';
	import { tiptapBodyToHtml, encodeReplyBody } from '$lib/tiptap/serialize';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import type { ForumReply } from '$lib/types';
	import VoteRail from './VoteRail.svelte';
	// Self-import replaces the deprecated <svelte:self> for recursive nesting.
	import ReplyItem from './ReplyItem.svelte';

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
	}
	let {
		reply,
		childrenOf,
		depth,
		threadId,
		authed,
		canSolve,
		isAccepted
	}: Props = $props();

	const MAX_DEPTH = 6;
	const bodyHtml = $derived(tiptapBodyToHtml(reply.body));
	const time = $derived(relativeTime(reply.created_at));
	const authorLabel = $derived(
		reply.is_anonymous
			? t('forum.anonymous', 'Anonymous')
			: reply.author_display_name || reply.author_username || t('forum.member', 'Member')
	);
	const beyondMaxDepth = $derived(depth >= MAX_DEPTH);

	let collapsed = $state(false);
	let composing = $state(false);
	let draft = $state('');
	let submitting = $state(false);
	let localChildren = $state<ForumReply[]>([]);

	// Server-resolved children plus any posted locally during this session.
	const allChildren = $derived([...childrenOf(reply.id), ...localChildren]);

	async function submitReply() {
		const text = draft.trim();
		if (!text || submitting || !authed) return;
		submitting = true;
		const body = encodeReplyBody(text);
		const res = await api.post<ForumReply>(`/forum/${threadId}/replies`, {
			body,
			parent_reply_id: reply.id
		});
		submitting = false;
		if (res.error || !res.data) return;
		localChildren = [...localChildren, res.data];
		draft = '';
		composing = false;
	}
</script>

<div class="flex gap-3" class:pl-1={depth > 0}>
	<!-- Vote rail (vertical) -->
	<VoteRail
		entityType="reply"
		entityId={reply.id}
		score={reply.vote_score}
		layout="vertical"
		{authed}
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
			<span class="font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
			{#if time}<span>·</span><span>{time}</span>{/if}
			{#if reply.is_edited}<span>·</span><span class="italic">{t('forum.edited', 'edited')}</span>{/if}
		</div>

		<!-- Body -->
		{#if !collapsed}
			<div class="prose prose-sm mt-1.5 max-w-none text-[14px] leading-relaxed text-[var(--color-text)]">
				{@html bodyHtml}
			</div>

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
			</div>

			<!-- Inline reply composer -->
			{#if composing && authed}
				<div class="mt-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-2.5">
					<textarea
						bind:value={draft}
						rows="3"
						placeholder={t('forum.reply_placeholder', 'Write a reply…')}
						class="w-full resize-y rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					></textarea>
					<div class="mt-2 flex items-center justify-end gap-2">
						<button
							type="button"
							onclick={() => { composing = false; draft = ''; }}
							class="rounded-[var(--radius)] px-2.5 py-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
						>
							{t('common.cancel', 'Cancel')}
						</button>
						<button
							type="button"
							onclick={submitReply}
							disabled={!draft.trim() || submitting}
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
