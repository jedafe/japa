<script lang="ts">
	import { ChevronDown, ChevronRight, CornerUpLeft, Pin, ArrowUp, ArrowDown, Pencil, Trash2 } from 'lucide-svelte';
	import { relativeTime } from '$lib/feed/time';
	import { api } from '$lib/api/client';
	import { t, i18n } from '$lib/i18n';
	import type { Comment } from '$lib/types';
	import { tiptapBodyToHtml } from '$lib/tiptap/serialize';
	// Self-import replaces the deprecated <svelte:self> for recursive nesting.
	import CommentItem from './CommentItem.svelte';
	import CommentEditor from './CommentEditor.svelte';

	interface Props {
		comment: Comment;
		/** Content id (for posting replies via POST /content/{id}/comments). */
		contentId: string;
		/** Current nesting depth (0 = top-level). */
		depth: number;
		/** Auth gating: only render interactive controls when true. */
		authed: boolean;
		/** Current user id — gates edit/delete affordances to the author. */
		currentUserId: string | null;
		/** Content author or moderator — gates the pin affordance. */
		canModerate: boolean;
	}
	let { comment, contentId, depth, authed, currentUserId, canModerate }: Props = $props();

	const MAX_DEPTH = 3;
	const time = $derived(relativeTime(comment.created_at, new Date(), i18n.locale));
	const authorLabel = $derived(comment.display_name || comment.username || t('forum.member', 'Member'));
	const isAuthor = $derived(!!currentUserId && currentUserId === comment.author_user_id);
	const beyondMaxDepth = $derived(depth >= MAX_DEPTH);
	const bodyHtml = $derived(tiptapBodyToHtml(comment.body));

	// Vote state (optimistic + rollback). The backend uses upsert semantics;
	// DELETE /comments/{id}/vote retracts. We track the user's current vote
	// locally because the list endpoint does not return it.
	let score = $state(0);
	let userVote = $state<1 | -1 | 0>(0);
	let votePending = $state(false);

	// Sync from the server payload on mount and whenever the prop reference
	// changes (e.g. a parent re-fetch). Mirrors the VoteRail pattern.
	$effect(() => {
		score = comment.vote_score;
		userVote = 0;
	});

	async function vote(value: 1 | -1) {
		if (!authed || votePending) return;
		const prev = userVote;
		const optimisticScore = score + (value - prev);
		score = optimisticScore;
		userVote = value === prev ? 0 : value;
		votePending = true;

		const res = value === prev
			? await api.delete(`/comments/${comment.id}/vote`)
			: await api.post(`/comments/${comment.id}/vote`, { value });
		votePending = false;

		if (res.error) {
			score = comment.vote_score;
			userVote = prev;
		}
	}

	// Pin / unpin (content author or moderator — backend enforces).
	let pinned = $state(false);
	let pinPending = $state(false);

	$effect(() => {
		pinned = comment.is_pinned;
	});

	async function togglePin() {
		if (pinPending) return;
		pinPending = true;
		const wasPinned = pinned;
		pinned = !wasPinned; // optimistic
		const res = wasPinned
			? await api.delete(`/comments/${comment.id}/pin`)
			: await api.post(`/comments/${comment.id}/pin`);
		pinPending = false;
		if (res.error) pinned = wasPinned; // rollback
	}

	// Reply composer.
	let collapsed = $state(false);
	let composing = $state(false);
	let replyBody = $state<string>('');
	let submitting = $state(false);
	let localReplies = $state<Comment[]>([]);

	// Lazily-loaded child replies (fetched via GET /content/{id}/comments?parent_id=).
	let loadedReplies = $state<Comment[]>([]);
	let repliesLoading = $state(false);
	let repliesLoaded = $state(false);

	const allReplies = $derived([...loadedReplies, ...localReplies]);

	async function loadReplies() {
		if (repliesLoading || repliesLoaded) return;
		repliesLoading = true;
		const res = await api.get<Comment[]>(`/content/${contentId}/comments?parent_id=${encodeURIComponent(comment.id)}`);
		repliesLoading = false;
		repliesLoaded = true;
		if (res.error || !res.data) return;
		loadedReplies = res.data;
	}

	async function submitReply() {
		if (!replyBody || submitting || !authed) return;
		submitting = true;
		const res = await api.post<Comment>(`/content/${contentId}/comments`, {
			parent_id: comment.id,
			body: replyBody
		});
		submitting = false;
		if (res.error || !res.data) return;
		localReplies = [...localReplies, res.data];
		replyBody = '';
		composing = false;
	}

	// Edit / delete (author only).
	let editing = $state(false);
	let editBody = $state<string>('');
	let editPending = $state(false);
	let deleted = $state(false);
	let deletePending = $state(false);

	function startEdit() {
		editBody = comment.body;
		editing = true;
	}

	async function saveEdit() {
		if (!editBody || editPending) return;
		editPending = true;
		const res = await api.patch<Comment>(`/comments/${comment.id}`, { body: editBody });
		editPending = false;
		if (res.error || !res.data) return;
		comment = { ...comment, body: res.data.body, is_edited: res.data.is_edited, edited_at: res.data.edited_at };
		editing = false;
	}

	async function deleteComment() {
		if (deletePending) return;
		deletePending = true;
		const res = await api.delete(`/comments/${comment.id}`);
		deletePending = false;
		if (res.error) return;
		deleted = true;
	}

	const upActive = $derived(userVote === 1);
	const downActive = $derived(userVote === -1);
	const baseBtn = 'grid place-items-center transition-colors disabled:opacity-50 disabled:cursor-not-allowed';
	const upClass = $derived(
		`${baseBtn} ${upActive ? 'text-[var(--color-success)]' : 'text-[var(--color-muted)] hover:text-[var(--color-success)]'}`
	);
	const downClass = $derived(
		`${baseBtn} ${downActive ? 'text-[var(--color-danger)]' : 'text-[var(--color-muted)] hover:text-[var(--color-danger)]'}`
	);
</script>

<div class="flex gap-3" class:pl-1={depth > 0}>
	<!-- Vertical vote rail -->
	<div class="flex w-7 flex-col items-center gap-0.5 pt-0.5 text-[var(--color-muted)]">
		<button
			type="button"
			onclick={() => vote(1)}
			disabled={!authed || votePending}
			aria-pressed={upActive}
			aria-label={t('comments.upvote')}
			class={upClass}
		>
			<ArrowUp size={16} />
		</button>
		<span class="text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">{score}</span>
		<button
			type="button"
			onclick={() => vote(-1)}
			disabled={!authed || votePending}
			aria-pressed={downActive}
			aria-label={t('comments.downvote')}
			class={downClass}
		>
			<ArrowDown size={16} />
		</button>
	</div>

	<div class="min-w-0 flex-1">
		<div class="flex flex-wrap items-center gap-2 text-xs text-[var(--color-muted)]">
			{#if pinned}
				<span class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-1.5 py-0.5 font-semibold text-[var(--color-primary)]">
					<Pin size={11} /> {t('comments.pinned')}
				</span>
			{/if}
			{#if comment.avatar_url}
				<img src={comment.avatar_url} alt="" class="h-5 w-5 rounded-full object-cover" />
			{/if}
			<span class="font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
			{#if time}<span>·</span><span>{time}</span>{/if}
			{#if comment.is_edited}<span>·</span><span class="italic">{t('comments.edited')}</span>{/if}
		</div>

		{#if deleted}
			<p class="mt-1.5 text-sm italic text-[var(--color-muted)]">{t('comments.deleted')}</p>
		{:else if !collapsed}
			{#if editing}
				<div class="mt-2">
					<CommentEditor initialBody={editBody} oninput={(b) => (editBody = b)} />
					<div class="mt-2 flex items-center justify-end gap-2">
						<button
							type="button"
							onclick={() => (editing = false)}
							class="rounded-[var(--radius)] px-2.5 py-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
						>
							{t('comments.cancel')}
						</button>
						<button
							type="button"
							onclick={saveEdit}
							disabled={!editBody || editPending}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							{editPending ? t('common.loading') : t('comments.save')}
						</button>
					</div>
				</div>
			{:else}
				<!-- Body: TipTap JSON rendered to HTML (Section 49). -->
				<div class="mt-1.5 break-words text-[14px] leading-relaxed text-[var(--color-text)] prose-comment">{@html bodyHtml}</div>
			{/if}

			<!-- Actions -->
			<div class="mt-2 flex flex-wrap items-center gap-3 text-xs text-[var(--color-muted)]">
				{#if authed}
					<button
						type="button"
						onclick={() => (composing = !composing)}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-text)]"
					>
						<CornerUpLeft size={12} /> {t('comments.reply')}
					</button>
				{/if}
				{#if isAuthor && !deleted}
					<button
						type="button"
						onclick={startEdit}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-text)]"
					>
						<Pencil size={12} /> {t('comments.edit')}
					</button>
					<button
						type="button"
						onclick={deleteComment}
						disabled={deletePending}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-danger)] disabled:opacity-50"
					>
						<Trash2 size={12} /> {t('comments.delete')}
					</button>
				{/if}
				{#if canModerate && !deleted}
					<button
						type="button"
						onclick={togglePin}
						disabled={pinPending}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-text)] disabled:opacity-50"
					>
						<Pin size={12} /> {pinned ? t('comments.unpin') : t('comments.pin')}
					</button>
				{/if}
				{#if allReplies.length > 0}
					<button
						type="button"
						onclick={() => (collapsed = !collapsed)}
						class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-text)]"
						aria-expanded={!collapsed}
					>
						{#if collapsed}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
						{allReplies.length} {t('comments.replies')}
					</button>
				{/if}
			</div>

			<!-- Inline reply composer -->
			{#if composing && authed}
				<div class="mt-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-2.5">
					<CommentEditor placeholder={t('comments.reply_placeholder')} oninput={(b) => (replyBody = b)} />
					<div class="mt-2 flex items-center justify-end gap-2">
						<button
							type="button"
							onclick={() => { composing = false; replyBody = ''; }}
							class="rounded-[var(--radius)] px-2.5 py-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
						>
							{t('comments.cancel')}
						</button>
						<button
							type="button"
							onclick={submitReply}
							disabled={!replyBody || submitting}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							{submitting ? t('common.loading') : t('comments.post_reply')}
						</button>
					</div>
				</div>
			{/if}

			<!-- Nested children -->
			{#if !beyondMaxDepth}
				{#if allReplies.length > 0}
					<div class="mt-3 space-y-3 border-l border-[var(--color-border)] pl-3">
						{#each allReplies as child (child.id)}
							<CommentItem
								comment={child}
								{contentId}
								depth={depth + 1}
								{authed}
								{currentUserId}
								{canModerate}
							/>
						{/each}
					</div>
				{/if}
				{#if !repliesLoaded && allReplies.length === 0}
					<button
						type="button"
						onclick={loadReplies}
						disabled={repliesLoading}
						class="mt-2 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] transition-colors hover:underline disabled:opacity-50"
					>
						{repliesLoading ? t('common.loading') : t('comments.view_replies')}
					</button>
				{/if}
			{:else if allReplies.length > 0}
				<a
					href={`#comment-${allReplies[0].id}`}
					class="mt-2 inline-block text-xs font-medium text-[var(--color-primary)] hover:underline"
				>
					{t('comments.continue_thread')}
				</a>
			{/if}
		{:else}
			<button
				type="button"
				onclick={() => (collapsed = !collapsed)}
				class="mt-1 inline-flex items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				<ChevronRight size={12} /> {t('comments.expand')}
			</button>
		{/if}
	</div>
</div>
