<script lang="ts">
	import { untrack } from 'svelte';
	import { CornerUpLeft, MessageCircle, TriangleAlert } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import type { Comment } from '$lib/types';
	import CommentItem from './CommentItem.svelte';
	import CommentEditor from './CommentEditor.svelte';

	/**
	 * Unified comment engine for content singles (blog + CPTs) — Section 49.
	 * Renders inside the dev.to reading layout (ADR-058 Exception #1), below
	 * the article body. Top-level comments are SSR-seeded; replies are
	 * lazy-loaded by each CommentItem via GET /content/{id}/comments?parent_id=.
	 *
	 * Ref: japaconnect-engineering/sections/49-unified-comments.md
	 */
	interface Props {
		/** SSR-seeded top-level comments (parent_id IS NULL). */
		comments: Comment[];
		/** Content id — used for POST /content/{id}/comments and reply fetches. */
		contentId: string;
		/** Auth gating. */
		authed: boolean;
		/** Current user id — gates edit/delete to the author. */
		currentUserId: string | null;
		/** Content author or moderator — gates the pin affordance. */
		canModerate: boolean;
		/** SSR load failure flag — renders an error state instead of an empty list. */
		loadError?: boolean;
	}
	let { comments: initial, contentId, authed, currentUserId, canModerate, loadError = false }: Props = $props();

	let comments = $state<Comment[]>(untrack(() => initial));

	// Re-seed on a new SSR payload (navigation between articles).
	$effect(() => {
		const seed = initial;
		untrack(() => {
			comments = seed;
		});
	});

	// Top-level composer state — body is base64 TipTap JSON.
	let draftBody = $state<string>('');
	let submitting = $state(false);
	let composeError = $state<string | null>(null);

	async function submitComment() {
		if (!draftBody || submitting || !authed) return;
		submitting = true;
		composeError = null;
		const res = await api.post<Comment>(`/content/${contentId}/comments`, { body: draftBody });
		submitting = false;
		if (res.error || !res.data) {
			composeError = res.error?.message ?? t('comments.load_error');
			return;
		}
		comments = [res.data, ...comments];
		draftBody = '';
	}

	const count = $derived(comments.length);
</script>

<div id="comments" class="mt-12 border-t border-[var(--color-border)] pt-8">
	<h2 class="mb-4 flex items-center gap-2 text-xl font-bold text-[var(--color-text)]">
		<MessageCircle size={20} />
		{t('comments.heading')}
		<span class="text-sm font-normal text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">{count}</span>
	</h2>

	{#if authed}
		<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
			<CommentEditor placeholder={t('comments.placeholder')} oninput={(b) => (draftBody = b)} />
			<div class="mt-2 flex items-center justify-between">
				<span class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)]">
					<CornerUpLeft size={12} /> {t('comments.replying_to')}
				</span>
				<button
					type="button"
					onclick={submitComment}
					disabled={!draftBody || submitting}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3.5 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{submitting ? t('common.loading') : t('comments.post')}
				</button>
			</div>
			{#if composeError}
				<p class="mt-2 text-xs text-[var(--color-danger)]">{composeError}</p>
			{/if}
		</div>
	{:else}
		<p class="mb-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
			{t('comments.sign_in_to_comment')}
		</p>
	{/if}

	{#if loadError}
		<EmptyState icon={TriangleAlert} title={t('comments.load_error')} body={t('error.500_body', 'Something went wrong. Please try again.')} />
	{:else if comments.length === 0}
		<EmptyState
			icon={MessageCircle}
			title={t('comments.empty')}
		/>
	{:else}
		<div class="space-y-5">
			{#each comments as comment (comment.id)}
				<div id={`comment-${comment.id}`}>
					<CommentItem
						{comment}
						{contentId}
						depth={0}
						{authed}
						{currentUserId}
						{canModerate}
					/>
				</div>
			{/each}
		</div>
	{/if}
</div>
