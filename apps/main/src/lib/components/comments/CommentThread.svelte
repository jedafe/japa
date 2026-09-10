<script lang="ts">
	import { untrack } from 'svelte';
	import { MessageCircle, TriangleAlert } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { EmptyState } from '$lib/components/ui';
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
		/** Total published comment count (top-level + replies). Falls back to the length of `comments` when omitted. */
		totalCount?: number;
		/** Content id — used for POST /content/{id}/comments and reply fetches. */
		contentId: string;
		/** Parent content type — retained for backwards compatibility; no longer gates voting. */
		contentType: string;
		/** Auth gating. */
		authed: boolean;
		/** Current user id — gates edit/delete to the author. */
		currentUserId: string | null;
		/** Content author or moderator — gates the pin affordance. */
		canModerate: boolean;
		/** SSR load failure flag — renders an error state instead of an empty list. */
		loadError?: boolean;
		/** Erased-user IDs for GDPR/ADR-029 mention redaction (Section 15). */
		deletedUserIds?: Set<string>;
	}
	let { comments: initial, totalCount, contentId, contentType, authed, currentUserId, canModerate, loadError = false, deletedUserIds }: Props = $props();

	let comments = $state<Comment[]>(untrack(() => initial));

	// Re-seed on a new SSR payload (navigation between articles).
	$effect(() => {
		const seed = initial;
		untrack(() => {
			comments = seed;
			sort = 'top';
			localDelta = 0;
		});
	});

	// Tracks optimistic local adjustments to the total comment count.
	let localDelta = $state(0);

	// Sort state (daily.dev-style dropdown). Backend supports top/new/oldest.
	let sort = $state<'top' | 'new' | 'oldest'>('top');
	let sortLoading = $state(false);
	let sortError = $state<string | null>(null);

	async function changeSort(value: 'top' | 'new' | 'oldest') {
		if (value === sort || sortLoading) return;
		sort = value;
		sortLoading = true;
		sortError = null;
		const res = await api.getEnvelope<Comment[]>(`/content/${contentId}/comments?sort=${encodeURIComponent(value)}`);
		sortLoading = false;
		if (res.error || !res.data) {
			sortError = res.error?.message ?? t('comments.load_error');
			return;
		}
		comments = res.data;
		const ids = (res.envelope as Record<string, unknown> | null)?.deleted_user_ids;
		if (Array.isArray(ids) && deletedUserIds) {
			for (const id of ids as string[]) deletedUserIds.add(id);
		}
	}

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
		if (totalCount != null) {
			localDelta++;
		}
	}

	// Total published comment count (top-level + replies). Falls back to the
	// number of top-level comments when totalCount is not provided.
	const count = $derived((totalCount ?? comments.length) + localDelta);
</script>

<div id="comments" class="mt-12 border-t border-[var(--color-border)] pt-8">
	<div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
		<h2 class="flex items-center gap-2 text-xl font-bold text-[var(--color-text)]">
			<MessageCircle size={20} />
			{t('comments.heading')}
			<span class="text-sm font-normal text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">{count}</span>
		</h2>

		{#if comments.length > 0}
			<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
				<span>{t('comments.sort_by', 'Sort by:')}</span>
				<select
					bind:value={sort}
					onchange={(e) => changeSort(e.currentTarget.value as 'top' | 'new' | 'oldest')}
					disabled={sortLoading}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-[var(--color-text)] outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				>
					<option value="top">{t('comments.sort.top', 'Top')}</option>
					<option value="new">{t('comments.sort.new', 'New')}</option>
					<option value="oldest">{t('comments.sort.oldest', 'Oldest')}</option>
				</select>
			</div>
		{/if}
	</div>

	{#if sortError}
		<p class="mb-4 text-xs text-[var(--color-danger)]">{sortError}</p>
	{/if}

	{#if authed}
		<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<CommentEditor placeholder={t('comments.placeholder')} oninput={(b) => (draftBody = b)} />
			<div class="mt-3 flex items-center justify-end">
				<button
					type="button"
					onclick={submitComment}
					disabled={!draftBody || submitting}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
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
		<section class="relative space-y-1" aria-label={t('comments.heading')}>
			<div class="absolute top-3 bottom-8 left-5 w-px bg-[var(--color-border)]"></div>
			{#each comments as comment (comment.id)}
				<div id={`comment-${comment.id}`} class="relative">
					<CommentItem
						{comment}
						{contentId}
						{contentType}
						depth={0}
						{authed}
						{currentUserId}
						{canModerate}
						{deletedUserIds}
						onReply={() => localDelta++}
						onDelete={() => localDelta--}
					/>
				</div>
			{/each}
		</section>
	{/if}
</div>
