<script lang="ts">
	import {
		Reply,
		CheckCircle2,
		Pencil,
		Trash2,
		Minus,
		Plus,
		Share2,
		MoreHorizontal,
		Check
	} from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { Menu, MenuItem } from '$lib/components/ui';
	import { relativeTime } from '$lib/feed/time';
	import { tiptapBodyToHtml } from '$lib/tiptap/serialize';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { imgLightbox } from '$lib/lightbox.svelte';
	import { api } from '$lib/api/client';
	import { t, i18n } from '$lib/i18n';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import type { ForumReply } from '$lib/types';
	import VoteRail from './VoteRail.svelte';
	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
	// Reply view count is recorded via POST /forum/replies/{id}/view when the
	// reply is first visible in the viewport.
	// Self-import replaces the deprecated <svelte:self> for recursive nesting.
	import ReplyItem from './ReplyItem.svelte';
	import LazyCommentEditor from '$lib/components/comments/LazyCommentEditor.svelte';

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
	const authorHandle = $derived(
		reply.is_anonymous || !reply.author_username ? '' : `@${reply.author_username}`
	);
	const beyondMaxDepth = $derived(depth >= MAX_DEPTH);

	// §88 AMA: is this reply from the AMA host? When true, highlight it.
	const isHostReply = $derived(!!hostUserId && reply.author_user_id === hostUserId);

	let approving = $state(false);
	let approvedLocally = $state(false);

	// §88 AMA: is this a pending question awaiting host approval?
	const isPending = $derived(!!reply.ama_pending && !approvedLocally);

	// §88: host approves a pending question via POST /forum/ama/:id/questions/:replyId/approve
	async function approveQuestion() {
		if (approving) return;
		approving = true;
		const res = await api.post(`/forum/ama/${threadId}/questions/${reply.id}/approve`, {});
		approving = false;
		if (!res.error) approvedLocally = true;
	}

	// Replies with children start collapsed — same "View N replies" pattern as
	// the comments section. untrack: intentional one-time snapshot of props.
	let collapsed = $state(untrack(() => childrenOf(reply.id).length > 0));
	let deepExpanded = $state(false);
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
	let copied = $state(false);
	let childrenWrap = $state<HTMLElement>();
	let railEndEl = $state<HTMLElement>();
	let railBottom = $state(0);

	// Thread rail termination: the vertical line ends at the LAST child's
	// connector junction (the "└" elbow) rather than running to the bottom
	// of the article. CSS can't express "stop at the last child's avatar",
	// so measure it and feed the rail a bottom offset.
	$effect(() => {
		const art = replyEl;
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
				// No rendered children — the "+" affordance (continue thread /
				// collapsed toggle) is the terminal node on the rail.
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
	// Reply edit window mirrors the parent thread's edit_window_minutes: the
	// thread page passes it via `reply.edit_window_minutes` when present,
	// defaulting to 30. Authors see a countdown; the edit entry disables
	// once the window lapses (moderators bypass it).
	const replyWindowMinutes = $derived((reply as { edit_window_minutes?: number }).edit_window_minutes ?? 30);
	const replyEditExpired = $derived.by(() => {
		if (canModerate || currentUserId !== reply.author_user_id) return false;
		const created = new Date(reply.created_at).getTime();
		if (Number.isNaN(created)) return false;
		return Date.now() - created > replyWindowMinutes * 60 * 1000;
	});
	const replyEditCountdown = $derived.by(() => {
		if (replyEditExpired || canModerate || currentUserId !== reply.author_user_id) return '';
		const created = new Date(reply.created_at).getTime();
		const left = replyWindowMinutes * 60 * 1000 - (Date.now() - created);
		if (left <= 0) return '';
		const mins = Math.floor(left / 60000);
		if (mins >= 60) return t('forum.edit_hours_left', '{n}h left to edit').replace('{n}', String(Math.floor(mins / 60)));
		return t('forum.edit_minutes_left', '{n}m left to edit').replace('{n}', String(Math.max(1, mins)));
	});
	const canEditReplyInWindow = $derived(canEditReply && !replyEditExpired);
	const canDeleteReply = $derived.by(() => authed && (canModerate || currentUserId === reply.author_user_id));
	const canMarkSolved = $derived.by(() => canSolve && !isAccepted && !removed && typeof onMarkSolved === 'function');

	// Server-resolved children plus any posted locally during this session.
	const allChildren = $derived([...childrenOf(reply.id), ...localChildren]);

	// Label for the collapsed thread's expand affordance — same copy as the
	// comments lazy-load button: "View reply" for one, "View N replies" else.
	const viewRepliesLabel = $derived(
		allChildren.length === 1
			? t('forum.view_reply', 'View reply')
			: t('forum.view_replies_count', 'View {count} replies').replace('{count}', String(allChildren.length))
	);

	async function submitReply() {
		if (!replyBody || submitting) return;
		if (!(await requireAuthOrRedirect())) return;
		submitting = true;
		const res = await api.post<ForumReply>(`/forum/${threadId}/replies`, {
			body: replyBody,
			parent_reply_id: reply.id
		});
		submitting = false;
		if (res.error || !res.data) return;
		replyBody = '';
		composing = false;
		collapsed = false;
		// Refetch so the new reply renders with the joined author row — the
		// POST response is a bare row without author fields (see ReplyThread).
		await invalidateAll();
		localChildren = [];
	}

	async function startEdit() {
		editError = null;
		// Fetch the raw (unresolved) body so authored {{...}} literals survive a
		// re-save — the display payload is resolved and CommentEditor reads
		// initialBody once at mount.
		const res = await api.get<ForumReply[]>(`/forum/${threadId}/replies?raw=1`);
		const raw = res.data?.find((r) => r.id === reply.id);
		editBody = raw?.body ?? reply.body;
		editing = true;
	}

	async function saveEdit() {
		if (savingEdit || !canEditReplyInWindow || !editBody) return;
		savingEdit = true;
		editError = null;
		const res = await api.patch<ForumReply>(`/forum/replies/${reply.id}`, {
			body: editBody,
			is_anonymous: reply.is_anonymous
		});
		savingEdit = false;
		if (res.error || !res.data) {
			editError =
				res.error?.code === 'edit_window_expired'
					? t('forum.edit_window_expired', 'The edit window has expired.')
					: res.error?.message || t('forum.edit_error', 'Failed to save edits.');
			return;
		}
		// Load-data props aren't deeply reactive — mutating reply.* fields
		// would never re-render, so refetch to show the edited body + flag.
		editing = false;
		await invalidateAll();
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

	function shareReply() {
		if (typeof window === 'undefined' || !navigator.clipboard) return;
		const url = `${window.location.origin}${window.location.pathname}#reply-${reply.id}`;
		navigator.clipboard.writeText(url).catch(() => {});
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	const iconBtn =
		'grid h-8 w-8 place-items-center rounded-[var(--radius)] transition-colors disabled:opacity-50';
</script>

<!-- Replies render through the universal comment layout (Section 49 parity):
     avatar on the left, header meta line, indented body + icon action row,
     thread rails connecting nested children. -->
<article
	id={`reply-${reply.id}`}
	bind:this={replyEl}
	class="relative isolate flex flex-col rounded-[var(--radius-card)] p-4 {depth > 0 ? 'comment-reply' : ''} {isHostReply ? 'border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/[0.03]' : ''}"
>
	{#if allChildren.length > 0}
		<!-- Vertical thread rail: runs from the replier avatar down through descendants. -->
		<div
			class="thread-rail absolute start-9 top-9 -z-10 w-px bg-[var(--color-border)]"
			style="bottom: {railBottom}px"
		></div>
	{/if}

	<header class="z-[1] flex w-full flex-row self-start gap-3">
		{#if !reply.is_anonymous && reply.author_username && reply.author_user_id}
			<EntityHoverChip
				type="user"
				id={reply.author_user_id}
				slug={reply.author_username}
				name={authorLabel}
				avatar={reply.author_avatar_url}
				variant="row"
			>
				{#snippet trigger()}
					{#if reply.author_avatar_url}
						<img
							src={reply.author_avatar_url}
							alt=""
							class="h-10 w-10 shrink-0 rounded-full object-cover"
							loading="lazy"
						/>
					{:else}
						<span
							class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-bold text-[var(--color-primary-contrast)]"
						>
							{authorLabel.charAt(0).toUpperCase()}
						</span>
					{/if}
					<div class="flex min-w-0 flex-1 flex-col justify-center">
						<div class="flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
							{#if isAccepted}
								<span
									class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-success)]/10 px-1.5 py-0.5 font-semibold text-[var(--color-success)]"
								>
									<CheckCircle2 size={10} /> {t('forum.accepted', 'Accepted')}
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
							<span class="font-bold text-[var(--color-text)]">{authorLabel}</span>
							{#if authorHandle}
								<span class="text-[var(--color-text-subtle)]">{authorHandle}</span>
							{/if}
							{#if time}<span>•</span><span>{time}</span>{/if}
							{#if reply.is_edited}<span>•</span><span class="italic">{t('forum.edited', 'edited')}</span>{/if}
							{#if reply.view_count > 0}<span>•</span><span class="italic">{reply.view_count} {t('forum.views', 'views')}</span>{/if}
						</div>
					</div>
				{/snippet}
			</EntityHoverChip>
		{:else}
			<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-bold text-[var(--color-primary-contrast)]">
				{authorLabel.charAt(0).toUpperCase()}
			</span>
			<div class="flex min-w-0 flex-1 flex-col justify-center">
				<div class="flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
					{#if isAccepted}
						<span
							class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-success)]/10 px-1.5 py-0.5 font-semibold text-[var(--color-success)]"
						>
							<CheckCircle2 size={10} /> {t('forum.accepted', 'Accepted')}
						</span>
					{/if}
					{#if isPending}
						<span
							class="inline-flex items-center gap-0.5 rounded-full bg-amber-500/10 px-1.5 py-0.5 font-semibold text-amber-600"
						>
							{t('ama.pending', 'Pending')}
						</span>
					{/if}
					<span class="font-bold text-[var(--color-text)]">{authorLabel}</span>
					{#if time}<span>•</span><span>{time}</span>{/if}
					{#if reply.is_edited}<span>•</span><span class="italic">{t('forum.edited', 'edited')}</span>{/if}
					{#if reply.view_count > 0}<span>•</span><span class="italic">{reply.view_count} {t('forum.views', 'views')}</span>{/if}
				</div>
			</div>
		{/if}
	</header>

	<div class="break-words z-[1] mt-1 ms-[52px]">
		{#if removed}
			<p class="text-sm italic text-[var(--color-muted)]">
				{t('forum.deleted_reply', '[deleted]')}
			</p>
		{:else}
			{#if editing}
				<div class="mt-2 space-y-2">
					{#if editError}
						<p class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-primary)]">
							{editError}
						</p>
					{/if}
					{#if replyEditCountdown}
						<p class="text-xs text-[var(--color-muted)]">{replyEditCountdown}</p>
					{/if}
					<LazyCommentEditor
						initialBody={editBody}
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
				<div class="prose-comment text-[0.9375rem] leading-[1.55] text-[var(--color-text)]" use:hydrateMentions use:imgLightbox>
					{@html bodyHtml}
				</div>
			{/if}

			<!-- Actions -->
			{#if !editing && replyEditExpired && currentUserId === reply.author_user_id && !canModerate}
				<p class="mt-1 text-xs italic text-[var(--color-muted)]">{t('forum.edit_window_expired', 'The edit window has expired.')}</p>
			{/if}
			{#if !editing}
				<div class="relative mt-2 flex flex-row items-center gap-3">
					{#if allChildren.length > 0 && !collapsed}
						<button
							type="button"
							onclick={() => (collapsed = true)}
							class="{iconBtn} absolute -start-12 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-muted)] hover:text-[var(--color-text)]"
							aria-label={t('comments.collapse', 'Collapse')}
							title={t('comments.collapse', 'Collapse')}
						>
							<Minus size={16} />
						</button>
					{/if}

					<VoteRail
						entityType="reply"
						entityId={reply.id}
						score={reply.vote_score}
						layout="horizontal"
						{authed}
						userVote={myVote}
					/>

					<!-- Visible for guests — clicking redirects to /login. -->
					<button
						type="button"
						onclick={() => {
							if (!authed) {
								void requireAuthOrRedirect();
								return;
							}
							composing = !composing;
						}}
						class="{iconBtn} text-[var(--color-muted)] hover:text-[var(--color-text)]"
						aria-label={t('forum.reply', 'Reply')}
						title={t('forum.reply', 'Reply')}
					>
						<Reply class="rtl:-scale-x-100" size={18} />
					</button>

					<button
						type="button"
						onclick={shareReply}
						class="{iconBtn} text-[var(--color-muted)] hover:text-[var(--color-text)]"
						aria-label={copied ? t('comments.copied', 'Link copied') : t('comments.share', 'Share')}
						title={copied ? t('comments.copied', 'Link copied') : t('comments.share', 'Share')}
					>
						{#if copied}<Check size={18} class="text-[var(--color-success)]" />{:else}<Share2 size={18} />{/if}
					</button>

					{#if (canEditReply || canDeleteReply || canMarkSolved || (isPending && isHost)) && !removed}
						<Menu contentClass="min-w-44">
							{#snippet trigger(props)}
								<button
									type="button"
									{...props}
									class="{iconBtn} text-[var(--color-muted)] hover:text-[var(--color-text)]"
									aria-label={t('comments.options', 'More options')}
									title={t('comments.options', 'More options')}
								>
									<MoreHorizontal size={18} />
								</button>
							{/snippet}
							{#if canEditReplyInWindow}
								<MenuItem icon={Pencil} onSelect={startEdit}>{t('forum.edit', 'Edit')}</MenuItem>
							{/if}
							{#if canDeleteReply}
								<MenuItem icon={Trash2} onSelect={deleteReply} disabled={deleting}>{t('forum.delete', 'Delete')}</MenuItem>
							{/if}
							{#if canMarkSolved}
								<MenuItem icon={CheckCircle2} onSelect={() => onMarkSolved?.(reply.id)}>{t('forum.mark_answer', 'Mark as answer')}</MenuItem>
							{/if}
							{#if isPending && isHost}
								<MenuItem icon={CheckCircle2} onSelect={approveQuestion} disabled={approving}>
									{approving ? t('common.loading', 'Loading') : t('ama.approve', 'Approve')}
								</MenuItem>
							{/if}
						</Menu>
					{/if}
				</div>

				{#if deleteError}
					<p class="mt-1 text-xs text-[var(--color-danger)]">{deleteError}</p>
				{/if}
			{/if}

			<!-- Inline reply composer -->
			{#if composing && authed}
				<div class="mt-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2.5">
					<LazyCommentEditor eager placeholder={t('forum.reply_placeholder', 'Write a reply…')} oninput={(b) => (replyBody = b)} />
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
			{#if collapsed && allChildren.length > 0}
				<button
					type="button"
					bind:this={railEndEl}
					onclick={() => (collapsed = false)}
					class="relative -start-12 mt-2 inline-flex items-center gap-2 text-xs font-medium text-[var(--color-primary)] transition-colors hover:text-[var(--color-primary-hover)]"
				>
					<span class="{iconBtn} rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-primary)]">
						<Plus size={14} />
					</span>
					{viewRepliesLabel}
				</button>
			{:else if !collapsed && (!beyondMaxDepth || deepExpanded)}
				{#if allChildren.length > 0}
					<div class="relative -start-8 mt-3 ps-5" bind:this={childrenWrap}>
						<div class="space-y-3">
							{#each allChildren as child (child.id)}
								<!-- At MAX_DEPTH children stay at the same visual depth —
								     each one renders its own "Continue this thread"
								     affordance for the level below. -->
								<ReplyItem
									reply={child}
									{childrenOf}
									depth={beyondMaxDepth ? depth : depth + 1}
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
					</div>
				{/if}
			{:else if allChildren.length > 0}
				<button
					type="button"
					bind:this={railEndEl}
					onclick={() => (deepExpanded = true)}
					class="relative -start-12 mt-2 inline-flex items-center gap-2 text-xs font-medium text-[var(--color-primary)] hover:underline"
				>
					<span class="{iconBtn} rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-primary)]">
						<Plus size={14} />
					</span>
					{t('forum.continue_thread', 'Continue this thread →')}
				</button>
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

	/* Curved thread branch (╰ elbow): the parent's rail (start-9 / avatar center)
	 * bends right into the child reply avatar. Paired with the .thread-rail
	 * vertical line, which terminates at the last child's junction. */
	.comment-reply::before {
		content: '';
		position: absolute;
		inset-inline-start: -20px;
		top: 20px;
		width: 36px;
		height: 15px;
		border-inline-start: 1px solid var(--color-border);
		border-bottom: 1px solid var(--color-border);
		border-end-start-radius: 12px;
		z-index: -10;
	}
</style>
