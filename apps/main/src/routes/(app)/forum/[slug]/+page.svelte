<script lang="ts">
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { t, i18n } from '$lib/i18n';
	import { ArrowLeft, CheckCircle2, Eye, MessageSquare, Bookmark, Share2, Flag, Pencil, Save, X } from 'lucide-svelte';
	import { relativeTime } from '$lib/feed/time';
	import { tiptapBodyToHtml, tiptapExcerpt } from '$lib/tiptap/serialize';
	import { api } from '$lib/api/client';
	import VoteRail from '$lib/components/forum/VoteRail.svelte';
	import ReplyThread from '$lib/components/forum/ReplyThread.svelte';
	import PollBlock from '$lib/components/forum/PollBlock.svelte';
	import PostEditor from '$lib/components/create/editors/PostEditor.svelte';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import type { ForumThread } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const thread = $derived(data.thread);
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds as string[]) : undefined
	);
	const bodyHtml = $derived(tiptapBodyToHtml(thread.body, { deletedUserIds }));
	const time = $derived(relativeTime(thread.created_at, new Date(), i18n.locale));
	const authorLabel = $derived(
		thread.is_anonymous
			? t('forum.anonymous', 'Anonymous')
			: thread.author_display_name || thread.author_username || t('forum.member', 'Member')
	);
	const spaceHref = $derived(thread.space_slug ? `/spaces/${thread.space_slug}` : null);

	const title = $derived(`${thread.title} — ${t('forum.list_title', 'Forum')}`);
	const description = $derived(tiptapExcerpt(thread.body, 160));

	// BUG-013: use the backend-provided JSON-LD (which includes the
	// acceptedAnswer text from the top-voted reply) instead of computing a
	// client-side version that was missing the answer text and causing double
	// emission. Fall back to a minimal DiscussionForumPosting when the backend
	// didn't provide one. Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
	const jsonLd = $derived(
		data.jsonLd
			? JSON.stringify(data.jsonLd)
			: JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'DiscussionForumPosting',
					headline: thread.title,
					datePublished: thread.created_at,
					author: { '@type': 'Person', name: authorLabel },
					url: page.url.href,
					interactionStatistic: {
						'@type': 'InteractionCounter',
						interactionType: 'https://schema.org/CommentAction',
						userInteractionCount: thread.reply_count
					}
				})
	);

	// Solve / un-solve (OP or moderator only — backend enforces the permission).
	let solved = $state(false);
	let solving = $state(false);

	// Sync the solved flag from the server payload on mount and on navigation.
	$effect(() => {
		solved = thread.solved;
	});

	async function toggleSolve() {
		if (solving || !data.canSolve) return;
		solving = true;
		const wasSolved = solved;
		solved = !wasSolved; // optimistic
		const res = wasSolved
			? await api.delete(`/forum/${thread.id}/solve`)
			: await api.post(`/forum/${thread.id}/solve`);
		solving = false;
		if (res.error) solved = wasSolved; // rollback
	}

	// Bookmark toggle — POST/DELETE /forum/bookmarks with entity_type=thread.
	// The backend returns bookmark_id on GetThread so DELETE can target
	// /forum/bookmarks/{id} directly. Ref: sections/08-forum-qa-solved.md (Bookmarks)
	let bookmarked = $state(false);
	let bookmarking = $state(false);
	let bookmarkCount = $state(0);
	let bookmarkId = $state('');

	$effect(() => {
		bookmarked = data.isBookmarked;
		bookmarkCount = thread.bookmark_count;
		bookmarkId = data.bookmarkId ?? '';
	});

	async function toggleBookmark() {
		if (!data.authed || bookmarking) return;
		bookmarking = true;
		const wasBookmarked = bookmarked;
		bookmarked = !wasBookmarked; // optimistic
		bookmarkCount += wasBookmarked ? -1 : 1;
		if (wasBookmarked && bookmarkId) {
			const res = await api.delete(`/forum/bookmarks/${bookmarkId}`);
			if (res.error) {
				bookmarked = true;
				bookmarkCount = thread.bookmark_count;
			} else {
				bookmarkId = '';
			}
		} else {
			const res = await api.post<{ id: string }>(`/forum/bookmarks`, {
				entity_type: 'thread',
				entity_id: thread.id
			});
			if (res.error) {
				bookmarked = false;
				bookmarkCount = thread.bookmark_count;
			} else if (res.data?.id) {
				bookmarkId = res.data.id;
			}
		}
		bookmarking = false;
	}

	// BUG-015: wire up the Share button with the Web Share API (fallback to
	// clipboard copy) and the Report button with a navigation to the report
	// flow. Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
	let shared = $state(false);

	async function shareThread() {
		const url = page.url.href;
		const shareData = { title: thread.title, url };
		try {
			if (navigator.share) {
				await navigator.share(shareData);
			} else {
				await navigator.clipboard.writeText(url);
				shared = true;
				setTimeout(() => (shared = false), 2000);
			}
		} catch {
			// user dismissed the share sheet — no action needed
		}
	}

	function reportThread() {
		goto(`/report?entity_type=forum_thread&entity_id=${thread.id}`);
	}

	// Inline edit (author or moderator — backend enforces via can_edit).
	// Uses the TipTap PostEditor so edits produce valid TipTap JSON for
	// the forum_threads.body jsonb column.
	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Editing)
	let editing = $state(false);
	let editTitle = $state('');
	let editBodyHtml = $state('');
	let editBodyJson = $state<Record<string, unknown>>({ type: 'doc', content: [] });
	let editAnonymous = $state(false);
	let savingEdit = $state(false);
	let editError = $state<string | null>(null);

	function startEdit() {
		if (!data.canEdit) return;
		editTitle = thread.title;
		// The PostEditor initialises from HTML. Render the stored TipTap
		// JSON to HTML so the editor starts with the current content.
		editBodyHtml = bodyHtml;
		editBodyJson = { type: 'doc', content: [] };
		editAnonymous = thread.is_anonymous;
		editError = null;
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		editError = null;
	}

	async function saveEdit() {
		if (savingEdit || !editTitle.trim()) return;
		savingEdit = true;
		editError = null;
		const res = await api.patch<ForumThread>(`/forum/${thread.id}`, {
			title: editTitle.trim(),
			body: btoa(JSON.stringify(editBodyJson)),
			is_anonymous: editAnonymous
		});
		savingEdit = false;
		if (res.error) {
			editError = res.error.message || t('forum.edit_error', 'Failed to save edits.');
			return;
		}
		editing = false;
		// Reload to reflect the updated thread + is_edited flag.
		await invalidateAll();
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={thread.title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-full px-4 py-6">
	<a
		href="/forum"
		class="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft size={16} /> {t('forum.back', 'Back to forum')}
	</a>

	<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6">
		<!-- Header -->
		<div class="flex items-start gap-4">
			<!-- Desktop vertical vote rail -->
			<div class="hidden sm:block">
				<VoteRail
					entityType="thread"
					entityId={thread.id}
					score={thread.vote_score}
					layout="vertical"
					authed={data.authed}
				/>
			</div>

			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-2">
					{#if solved}
						<span
							class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-success)]/10 px-2 py-0.5 text-xs font-semibold text-[var(--color-success)]"
						>
							<CheckCircle2 size={12} /> {t('forum.solved', 'Solved')}
						</span>
					{/if}
					<span class="rounded border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
						{t('content_type.discussion', 'Discussion')}
					</span>

					<!-- Edit button (author or moderator — can_edit from backend) -->
					{#if data.canEdit && !editing}
						<button
							type="button"
							onclick={startEdit}
							class="ml-auto inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<Pencil size={12} /> {t('forum.edit', 'Edit')}
						</button>
					{/if}
				</div>

				{#if editing}
					<!-- Inline edit mode -->
					<input
						type="text"
						bind:value={editTitle}
						maxlength="120"
						class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-xl font-bold text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						placeholder={t('forum.new.placeholder_title', 'Title')}
					/>
				{:else}
					<h1 class="mt-2 text-balance text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-3xl">
						{thread.title}
					</h1>
				{/if}

				<div class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
					<span class="font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
					{#if thread.space_name}
						<span>·</span>
						<span class="text-[var(--color-text-subtle)]">
							{#if spaceHref}<a href={spaceHref} class="hover:underline">{thread.space_name}</a>{:else}{thread.space_name}{/if}
						</span>
					{/if}
					{#if time}<span>·</span><span>{time}</span>{/if}
					{#if thread.is_edited}<span>·</span><span class="italic">{t('forum.edited', 'edited')}</span>{/if}
				</div>

				<!-- Tags (Section 08 — tags are first-class on discussions) -->
				{#if data.tags && data.tags.length > 0}
					<div class="mt-3 flex flex-wrap gap-1.5">
						{#each data.tags as tag (tag.id)}
							<a
								href="/forum?tag={tag.slug}"
								class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
							>
								{tag.name}
							</a>
						{/each}
					</div>
				{/if}
			</div>
		</div>

		<!-- Mobile vote rail (horizontal) -->
		<div class="mt-3 flex items-center justify-between border-b border-[var(--color-border)] pb-3 sm:hidden">
			<VoteRail
				entityType="thread"
				entityId={thread.id}
				score={thread.vote_score}
				layout="horizontal"
				authed={data.authed}
			/>
			<span class="flex items-center gap-3 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
				<span class="flex items-center gap-1"><MessageSquare size={13} /> {thread.reply_count}</span>
				<span class="flex items-center gap-1"><Eye size={13} /> {thread.view_count}</span>
			</span>
		</div>

		<!-- Body (view mode) / Edit form (edit mode) -->
		{#if editing}
			<div class="mt-5 space-y-3">
				{#if editError}
					<p class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-primary)]">
						{editError}
					</p>
				{/if}
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] focus-within:border-[var(--color-primary)]">
					<PostEditor
						html={editBodyHtml}
						onedit={() => {}}
						onhtml={(v) => { editBodyHtml = v; }}
						onjson={(v) => { editBodyJson = v; }}
					/>
				</div>
				<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
					<input type="checkbox" bind:checked={editAnonymous} class="h-4 w-4 accent-[var(--color-primary)]" />
					{t('forum.new.anonymous', 'Post anonymously')}
				</label>
				<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-3">
					<button
						type="button"
						onclick={cancelEdit}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<X size={14} /> {t('common.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						onclick={saveEdit}
						disabled={savingEdit || !editTitle.trim()}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						<Save size={14} /> {savingEdit ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
					</button>
				</div>
			</div>
		{:else}
			<div class="prose mt-5 max-w-none text-[var(--color-text)]" use:hydrateLinkPreviews use:hydrateMentions>
				{@html bodyHtml}
			</div>
		{/if}

		<!-- Poll (if the thread has one) -->
		{#if data.poll}
			<div class="mt-5">
				<PollBlock pollData={data.poll} />
			</div>
		{/if}

		<!-- Footer actions -->
		<div class="mt-6 flex flex-wrap items-center gap-4 border-t border-[var(--color-border)] pt-4 text-xs text-[var(--color-muted)]">
			<span class="hidden items-center gap-1 sm:flex [font-variant-numeric:tabular-nums]">
				<MessageSquare size={14} /> {thread.reply_count} {t('forum.replies', 'replies')}
			</span>
			<span class="hidden items-center gap-1 sm:flex [font-variant-numeric:tabular-nums]">
				<Eye size={14} /> {thread.view_count}
			</span>
			<button
				type="button"
				onclick={toggleBookmark}
				disabled={!data.authed || bookmarking}
				class="flex items-center gap-1 transition-colors hover:text-[var(--color-text)] disabled:cursor-not-allowed disabled:opacity-50 [font-variant-numeric:tabular-nums]"
				title={data.authed ? (bookmarked ? t('forum.remove_bookmark', 'Remove bookmark') : t('forum.add_bookmark', 'Bookmark this discussion')) : t('forum.login_to_bookmark', 'Log in to bookmark')}
			>
				<Bookmark size={14} fill={bookmarked ? 'currentColor' : 'none'} class={bookmarked ? 'text-[var(--color-primary)]' : ''} />
				{bookmarkCount}
			</button>
			<button type="button" onclick={shareThread} class="flex items-center gap-1 transition-colors hover:text-[var(--color-text)]">
				<Share2 size={14} /> {shared ? t('forum.shared', 'Copied!') : t('forum.share', 'Share')}
			</button>
			<button type="button" onclick={reportThread} class="flex items-center gap-1 transition-colors hover:text-[var(--color-text)]">
				<Flag size={14} /> {t('forum.report', 'Report')}
			</button>
			{#if data.canSolve}
				<button
					type="button"
					onclick={toggleSolve}
					disabled={solving}
					class="ml-auto inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 font-medium transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					<CheckCircle2 size={13} />
					{solved ? t('forum.unsolve', 'Mark unsolved') : t('forum.mark_solved', 'Mark as solved')}
				</button>
			{/if}
		</div>
	</article>

	<!-- Threaded replies -->
	<ReplyThread
		replies={data.replies}
		threadId={thread.id}
		authed={data.authed}
		canSolve={data.canSolve}
		acceptedReplyId={null}
		{deletedUserIds}
	/>

	<!-- Related posts sidebar (Phase 05.14) -->
	{#if data.related && data.related.length > 0}
		<aside class="mt-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
				{t('forum.related', 'Related discussions')}
			</h2>
			<ul class="space-y-2">
				{#each data.related as rel (rel.id)}
					<li>
						<a
							href="/content/{rel.slug}"
							class="block rounded-[var(--radius)] p-2 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<span class="block truncate text-sm text-[var(--color-text)]">{rel.title}</span>
							<span class="mt-0.5 block text-xs capitalize text-[var(--color-muted)]">{rel.content_type}</span>
						</a>
					</li>
				{/each}
			</ul>
		</aside>
	{/if}
</div>
