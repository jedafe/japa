<script lang="ts">
	import {
		ArrowUp,
		ArrowDown,
		CornerUpLeft,
		Pin,
		Pencil,
		Trash2,
		Minus,
		Plus,
		Share2,
		MoreHorizontal,
		Check
	} from 'lucide-svelte';
	import { DropdownMenu } from 'bits-ui';
	import { relativeTime } from '$lib/feed/time';
	import { api } from '$lib/api/client';
	import { t, i18n } from '$lib/i18n';
	import type { Comment } from '$lib/types';
	import { tiptapBodyToHtml } from '$lib/tiptap/serialize';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import CommentItem from './CommentItem.svelte';
	import CommentEditor from './CommentEditor.svelte';

	interface Props {
		comment: Comment;
		/** Content id (for posting replies via POST /content/{id}/comments). */
		contentId: string;
		/** Parent content type — retained for backwards compatibility; no longer gates voting. */
		contentType: string;
		/** Current nesting depth (0 = top-level). */
		depth: number;
		/** Auth gating: only render interactive controls when true. */
		authed: boolean;
		/** Current user id — gates edit/delete affordances to the author. */
		currentUserId: string | null;
		/** Content author or moderator — gates the pin affordance. */
		canModerate: boolean;
		/** Erased-user IDs for GDPR/ADR-029 mention redaction (Section 15). */
		deletedUserIds?: Set<string>;
		/** Called when any reply in this subtree is successfully posted. */
		onReply?: () => void;
		/** Called when this comment or any nested reply is deleted. */
		onDelete?: () => void;
	}
	let {
		comment,
		contentId,
		contentType,
		depth,
		authed,
		currentUserId,
		canModerate,
		deletedUserIds,
		onReply = () => {},
		onDelete = () => {}
	}: Props = $props();

	const MAX_DEPTH = 3;
	const time = $derived(relativeTime(comment.created_at, new Date(), i18n.locale));
	const authorName = $derived(
		comment.is_anonymous
			? t('comments.anonymous', 'Anonymous')
			: comment.display_name || comment.username || t('forum.member', 'Member')
	);
	const authorHandle = $derived(
		comment.is_anonymous || !comment.username ? '' : `@${comment.username}`
	);
	const authorHref = $derived(
		comment.is_anonymous || !comment.username ? '#' : `/u/${comment.username}`
	);
	const isAuthor = $derived(
		!!currentUserId && !!comment.author_user_id && currentUserId === comment.author_user_id
	);
	const beyondMaxDepth = $derived(depth >= MAX_DEPTH);
	const bodyHtml = $derived(
		tiptapBodyToHtml(comment.body, {
			deletedUserIds,
			deletedUserLabel: t('mentions.deleted_user', '[deleted user]')
		})
	);

	// Vote state (optimistic + rollback). The list endpoint now returns the
	// viewer's user_vote and the denormalized upvote/downvote counts, so the
	// optimistic math is always anchored to the real baseline.
	let upvotes = $state(0);
	let downvotes = $state(0);
	let userVote = $state<1 | -1 | 0>(0);
	let votePending = $state(false);
	let voteError = $state<string | null>(null);

	$effect(() => {
		upvotes = comment.upvote_count ?? 0;
		downvotes = comment.downvote_count ?? 0;
		userVote = comment.user_vote ?? 0;
	});

	async function vote(value: 1 | -1) {
		if (!authed || votePending) return;
		voteError = null;
		const prev = userVote;

		let nextUp = upvotes;
		let nextDown = downvotes;
		let nextVote: 1 | -1 | 0 = 0;

		if (value === 1) {
			if (prev === 1) {
				nextUp--;
				nextVote = 0;
			} else {
				if (prev === -1) nextDown--;
				nextUp++;
				nextVote = 1;
			}
		} else {
			if (prev === -1) {
				nextDown--;
				nextVote = 0;
			} else {
				if (prev === 1) nextUp--;
				nextDown++;
				nextVote = -1;
			}
		}

		upvotes = nextUp;
		downvotes = nextDown;
		userVote = nextVote;
		votePending = true;

		const res = nextVote === 0
			? await api.delete(`/comments/${comment.id}/vote`)
			: await api.post(`/comments/${comment.id}/vote`, { value });
		votePending = false;

		if (res.error) {
			upvotes = comment.upvote_count ?? 0;
			downvotes = comment.downvote_count ?? 0;
			userVote = comment.user_vote ?? 0;
			voteError = res.error.message;
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

	// Whether this comment has (or may have) nested replies. Controls the
	// vertical thread rail and the collapse/expand affordances.
	const hasChildren = $derived(comment.reply_count > 0 || allReplies.length > 0);

	// "View reply" for exactly one, "View X replies" otherwise. Used for the
	// lazy-load button (server count) and the collapsed thread's expand
	// affordance (loaded-children count).
	function viewRepliesText(count: number) {
		return count === 1
			? t('comments.view_reply', 'View reply')
			: t('comments.view_replies_count', 'View {count} replies').replace('{count}', String(count));
	}
	const viewRepliesLabel = $derived(viewRepliesText(comment.reply_count));
	const collapsedLabel = $derived(viewRepliesText(allReplies.length));

	async function loadReplies() {
		if (repliesLoading || repliesLoaded) return;
		repliesLoading = true;
		repliesError = null;
		// BUG-005: use getEnvelope to extract deleted_user_ids for GDPR redaction.
		const res = await api.getEnvelope<Comment[]>(`/content/${contentId}/comments?parent_id=${encodeURIComponent(comment.id)}`);
		repliesLoading = false;
		repliesLoaded = true;
		if (res.error || !res.data) {
			repliesError = res.error?.message ?? t('comments.load_error');
			return;
		}
		loadedReplies = res.data;
		// Merge any new deleted_user_ids from the reply envelope into the set.
		const replyDeletedIds = (res.envelope as Record<string, unknown> | null)?.deleted_user_ids;
		if (Array.isArray(replyDeletedIds) && deletedUserIds) {
			for (const id of replyDeletedIds as string[]) deletedUserIds.add(id);
		}
	}

	async function submitReply() {
		if (!replyBody || submitting || !authed) return;
		submitting = true;
		replyError = null;
		const res = await api.post<Comment>(`/content/${contentId}/comments`, {
			parent_id: comment.id,
			body: replyBody
		});
		submitting = false;
		if (res.error || !res.data) {
			replyError = res.error?.message ?? t('comments.load_error');
			return;
		}
		localReplies = [...localReplies, res.data];
		replyBody = '';
		composing = false;
		collapsed = false;
		onReply();
	}

	// Edit / delete (author only).
	let editing = $state(false);
	let editBody = $state<string>('');
	let editPending = $state(false);
	let deleted = $state(false);
	let deletePending = $state(false);
	let repliesError = $state<string | null>(null);
	let replyError = $state<string | null>(null);
	let editError = $state<string | null>(null);
	let deleteError = $state<string | null>(null);
	let copied = $state(false);

	function startEdit() {
		editBody = comment.body;
		editing = true;
	}

	async function saveEdit() {
		if (!editBody || editPending) return;
		editPending = true;
		editError = null;
		const res = await api.patch<Comment>(`/comments/${comment.id}`, { body: editBody });
		editPending = false;
		if (res.error || !res.data) {
			editError = res.error?.message ?? t('comments.load_error');
			return;
		}
		comment = { ...comment, body: res.data.body, is_edited: res.data.is_edited, edited_at: res.data.edited_at };
		editing = false;
	}

	async function deleteComment() {
		if (deletePending) return;
		deletePending = true;
		deleteError = null;
		const res = await api.delete(`/comments/${comment.id}`);
		deletePending = false;
		if (res.error) {
			deleteError = res.error?.message ?? t('comments.load_error');
			return;
		}
		deleted = true;
		onDelete();
	}

	function shareComment() {
		if (typeof window === 'undefined' || !navigator.clipboard) return;
		const url = `${window.location.origin}${window.location.pathname}#comment-${comment.id}`;
		navigator.clipboard.writeText(url).catch(() => {});
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	// Thread rail termination: the vertical line ends at the LAST child's
	// connector junction (the "└" elbow) rather than running to the bottom
	// of the article. CSS can't express "stop at the last child's avatar",
	// so measure it and feed the rail a bottom offset.
	let articleEl = $state<HTMLElement>();
	let childrenWrap = $state<HTMLElement>();
	let railEndEl = $state<HTMLElement>();
	let railBottom = $state(0);

	$effect(() => {
		const art = articleEl;
		if (!art) {
			railBottom = 0;
			return;
		}
		const wrap = childrenWrap;
		const endEl = railEndEl;
		const measure = () => {
			const last = wrap?.firstElementChild?.lastElementChild as HTMLElement | null;
			const artBottom = art.getBoundingClientRect().bottom;
			if (last && wrap) {
				// Rail stops where the last child's elbow starts bending: the elbow
				// box bottom is at +35 and the corner radius is 12px, so the curve
				// leaves the rail's column at +23.
				railBottom = Math.max(0, artBottom - last.getBoundingClientRect().top - 23);
			} else if (endEl) {
				// No rendered children — the "+" affordance (view replies /
				// continue thread / collapsed toggle) is the terminal node on the rail.
				const r = endEl.getBoundingClientRect();
				railBottom = Math.max(0, artBottom - r.top - r.height / 2);
			} else {
				railBottom = 0;
			}
		};
		measure();
		const ro = new ResizeObserver(measure);
		if (wrap) ro.observe(wrap);
		if (endEl) ro.observe(endEl);
		ro.observe(art);
		return () => ro.disconnect();
	});

	const iconBtn =
		'grid h-8 w-8 place-items-center rounded-[var(--radius)] transition-colors disabled:opacity-50';
	const menuItem =
		'flex w-full cursor-pointer items-center gap-2 rounded-[6px] px-3 py-2 text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]';
</script>

<article
	id={`comment-${comment.id}`}
	bind:this={articleEl}
	class="relative isolate flex flex-col rounded-[var(--radius-card)] p-4 {depth > 0 ? 'comment-reply' : ''}"
>
	{#if hasChildren}
		<!-- Vertical thread rail: runs from the commenter avatar down through descendants. -->
		<div
			class="thread-rail absolute left-9 top-9 -z-10 w-px bg-[var(--color-border)]"
			style="bottom: {railBottom}px"
		></div>
	{/if}

	<header class="z-[1] flex w-full flex-row self-start gap-3">
		{#if !comment.is_anonymous && comment.username && comment.author_user_id}
			<EntityHoverChip
				type="user"
				id={comment.author_user_id}
				slug={comment.username}
				name={authorName}
				avatar={comment.avatar_url}
				variant="row"
			>
				{#snippet trigger()}
					{#if comment.avatar_url}
						<img
							src={comment.avatar_url}
							alt=""
							class="h-10 w-10 shrink-0 rounded-full object-cover"
							loading="lazy"
						/>
					{:else}
						<span
							class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-bold text-[var(--color-primary-contrast)]"
						>
							{authorName.charAt(0).toUpperCase()}
						</span>
					{/if}
					<div class="flex min-w-0 flex-1 flex-col justify-center">
						<div class="flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
							{#if pinned}
								<span
									class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-1.5 py-0.5 font-semibold text-[var(--color-primary)]"
								>
									<Pin size={10} /> {t('comments.pinned')}
								</span>
							{/if}
							<span class="font-bold text-[var(--color-text)]">{authorName}</span>
							{#if authorHandle}
								<span class="text-[var(--color-text-subtle)]">{authorHandle}</span>
							{/if}
							{#if time}<span>•</span><span>{time}</span>{/if}
							{#if comment.is_edited}<span>•</span><span class="italic">{t('comments.edited')}</span>{/if}
						</div>
					</div>
				{/snippet}
			</EntityHoverChip>
		{:else}
			<a href={authorHref} class="shrink-0 no-underline" aria-label={authorName}>
				{#if comment.avatar_url && !comment.is_anonymous}
					<img
						src={comment.avatar_url}
						alt=""
						class="h-10 w-10 shrink-0 rounded-full object-cover"
						loading="lazy"
					/>
				{:else}
					<span
						class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-bold text-[var(--color-primary-contrast)]"
					>
						{authorName.charAt(0).toUpperCase()}
					</span>
				{/if}
			</a>
			<div class="flex min-w-0 flex-1 flex-col justify-center">
				<div class="flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
					{#if pinned}
						<span
							class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-primary)]/10 px-1.5 py-0.5 font-semibold text-[var(--color-primary)]"
						>
							<Pin size={10} /> {t('comments.pinned')}
						</span>
					{/if}
					<a
						href={authorHref}
						class="font-bold text-[var(--color-text)] no-underline hover:underline"
					>
						{authorName}
					</a>
					{#if authorHandle}
						<a href={authorHref} class="text-[var(--color-text-subtle)] no-underline hover:underline">
							{authorHandle}
						</a>
					{/if}
					{#if time}<span>•</span><span>{time}</span>{/if}
					{#if comment.is_edited}<span>•</span><span class="italic">{t('comments.edited')}</span>{/if}
				</div>
			</div>
		{/if}
	</header>

	<div class="break-words z-[1] mt-1 ml-[52px]">
		{#if deleted}
			<p class="text-sm italic text-[var(--color-muted)]">{t('comments.deleted')}</p>
		{:else}
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
					{#if editError}
						<p class="mt-1.5 text-xs text-[var(--color-danger)]">{editError}</p>
					{/if}
				</div>
			{:else}
				<div class="prose-comment text-[0.9375rem] leading-[1.55] text-[var(--color-text)]">
					{@html bodyHtml}
				</div>
			{/if}

			<!-- Actions -->
			{#if !editing}
				<div class="relative mt-2 flex flex-row items-center gap-0.5">
					{#if allReplies.length > 0 && !collapsed}
						<button
							type="button"
							onclick={() => (collapsed = true)}
							class="{iconBtn} absolute -left-12 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-muted)] hover:text-[var(--color-text)]"
							aria-label={t('comments.collapse')}
							title={t('comments.collapse')}
						>
							<Minus size={16} />
						</button>
					{/if}

					<button
						type="button"
						onclick={() => vote(1)}
						disabled={!authed || votePending}
						aria-pressed={userVote === 1}
						aria-label={t('comments.upvote')}
						class="{iconBtn} {userVote === 1 ? 'text-[var(--color-success)] bg-[var(--color-success)]/10' : 'text-[var(--color-muted)] hover:text-[var(--color-success)]'}"
					>
						<ArrowUp size={18} />
					</button>
					<button
						type="button"
						onclick={() => vote(-1)}
						disabled={!authed || votePending}
						aria-pressed={userVote === -1}
						aria-label={t('comments.downvote')}
						class="{iconBtn} {userVote === -1 ? 'text-[var(--color-danger)] bg-[var(--color-danger)]/10' : 'text-[var(--color-muted)] hover:text-[var(--color-danger)]'}"
					>
						<ArrowDown size={18} />
					</button>

					{#if authed}
						<button
							type="button"
							onclick={() => (composing = !composing)}
							class="{iconBtn} text-[var(--color-muted)] hover:text-[var(--color-text)]"
							aria-label={t('comments.reply')}
							title={t('comments.reply')}
						>
							<CornerUpLeft size={18} />
						</button>
					{/if}

					<button
						type="button"
						onclick={shareComment}
						class="{iconBtn} text-[var(--color-muted)] hover:text-[var(--color-text)]"
						aria-label={copied ? t('comments.copied', 'Link copied') : t('comments.share', 'Share comment')}
						title={copied ? t('comments.copied', 'Link copied') : t('comments.share', 'Share comment')}
					>
						{#if copied}<Check size={18} class="text-[var(--color-success)]" />{:else}<Share2 size={18} />{/if}
					</button>

					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							class="{iconBtn} text-[var(--color-muted)] hover:text-[var(--color-text)]"
							aria-label={t('comments.options', 'More options')}
							title={t('comments.options', 'More options')}
						>
							<MoreHorizontal size={18} />
						</DropdownMenu.Trigger>
						<DropdownMenu.Portal>
							<DropdownMenu.Content
								align="end"
								sideOffset={8}
								class="z-50 min-w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
							>
								{#if isAuthor && !deleted}
									<DropdownMenu.Item class={menuItem}>
										{#snippet child({ props })}
											<button type="button" onclick={startEdit} {...props}>
												<Pencil size={16} /> {t('comments.edit')}
											</button>
										{/snippet}
									</DropdownMenu.Item>
									<DropdownMenu.Item class={menuItem}>
										{#snippet child({ props })}
											<button type="button" onclick={deleteComment} disabled={deletePending} {...props}>
												<Trash2 size={16} /> {t('comments.delete')}
											</button>
										{/snippet}
									</DropdownMenu.Item>
								{/if}
								{#if canModerate && !deleted}
									<DropdownMenu.Item class={menuItem}>
										{#snippet child({ props })}
											<button type="button" onclick={togglePin} disabled={pinPending} {...props}>
												<Pin size={16} /> {pinned ? t('comments.unpin') : t('comments.pin')}
											</button>
										{/snippet}
									</DropdownMenu.Item>
								{/if}
							</DropdownMenu.Content>
						</DropdownMenu.Portal>
					</DropdownMenu.Root>

					<span class="ml-auto text-xs text-[var(--color-muted)]" title={t('comments.see_upvoters', 'See who upvoted')}>
						{upvotes === 1 ? '1 upvote' : t('comments.upvote_count', '{count} upvotes').replace('{count}', String(upvotes))}
					</span>
				</div>

				{#if voteError}
					<p class="mt-1 text-xs text-[var(--color-danger)]">{voteError}</p>
				{/if}
				{#if deleteError}
					<p class="mt-1 text-xs text-[var(--color-danger)]">{deleteError}</p>
				{/if}
				{#if repliesError}
					<p class="mt-1 text-xs text-[var(--color-danger)]">{repliesError}</p>
				{/if}
			{/if}

			<!-- Inline reply composer -->
			{#if composing && authed}
				<div class="mt-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2.5">
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
					{#if replyError}
						<p class="mt-1.5 text-xs text-[var(--color-danger)]">{replyError}</p>
					{/if}
				</div>
			{/if}

			<!-- Nested children -->
			{#if collapsed && allReplies.length > 0}
				<button
					type="button"
					bind:this={railEndEl}
					onclick={() => (collapsed = false)}
					class="relative -left-12 mt-2 inline-flex items-center gap-2 text-xs font-medium text-[var(--color-primary)] transition-colors hover:text-[var(--color-primary-hover)]"
				>
					<span class="{iconBtn} rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-primary)]">
						<Plus size={14} />
					</span>
					{collapsedLabel}
				</button>
			{:else if !collapsed && !beyondMaxDepth}
				{#if allReplies.length > 0}
					<div class="relative -left-8 mt-3 pl-5" bind:this={childrenWrap}>
						<div class="space-y-3">
							{#each allReplies as child (child.id)}
								<CommentItem
									comment={child}
									{contentId}
									{contentType}
									depth={depth + 1}
									{authed}
									{currentUserId}
									{canModerate}
									{deletedUserIds}
									onReply={() => onReply()}
									onDelete={() => onDelete()}
								/>
							{/each}
						</div>
					</div>
				{/if}
				{#if !repliesLoaded && allReplies.length === 0 && comment.reply_count > 0}
					<button
						type="button"
						bind:this={railEndEl}
						onclick={loadReplies}
						disabled={repliesLoading}
						class="relative -left-12 mt-2 inline-flex items-center gap-2 text-xs font-medium text-[var(--color-primary)] transition-colors hover:text-[var(--color-primary-hover)] disabled:opacity-50"
					>
						<span class="{iconBtn} rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-primary)]">
							<Plus size={14} />
						</span>
						{repliesLoading ? t('common.loading') : viewRepliesLabel}
					</button>
				{/if}
			{:else if !collapsed && allReplies.length > 0}
				<a
					href={`#comment-${allReplies[0].id}`}
					bind:this={railEndEl}
					class="relative -left-12 mt-2 inline-flex items-center gap-2 text-xs font-medium text-[var(--color-primary)] hover:underline"
				>
					<span class="{iconBtn} rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-primary)]">
						<Plus size={14} />
					</span>
					{t('comments.continue_thread')}
				</a>
			{/if}
		{/if}
	</div>
</article>

<style>
	:global(.prose-comment p) {
		margin: 0.65rem 0;
	}
	:global(.prose-comment p:first-child) {
		margin-top: 0;
	}
	:global(.prose-comment p:last-child) {
		margin-bottom: 0;
	}
	:global(.prose-comment a) {
		color: var(--color-primary);
		text-decoration: underline;
		text-underline-offset: 2px;
	}
	:global(.prose-comment a:hover) {
		color: var(--color-primary-hover);
	}

	/* Curved thread branch (╰ elbow): the parent's rail (left-9 / avatar center)
	 * bends right into the child reply avatar. Paired with the .thread-rail
	 * vertical line, which terminates at the last child's junction. */
	.comment-reply::before {
		content: '';
		position: absolute;
		left: -20px;
		top: 20px;
		width: 36px;
		height: 15px;
		border-left: 1px solid var(--color-border);
		border-bottom: 1px solid var(--color-border);
		border-bottom-left-radius: 12px;
		z-index: -10;
	}
</style>
