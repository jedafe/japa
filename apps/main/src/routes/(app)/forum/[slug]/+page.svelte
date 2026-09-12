<script lang="ts">
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { t, i18n } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { ArrowLeft, CheckCircle2, Eye, MessageSquare, Bookmark, Share2, Flag, Pencil, Save, Trash2, X, Loader2, Plus } from 'lucide-svelte';
	import { Dialog } from 'bits-ui';
	import { relativeTime } from '$lib/feed/time';
	import { tiptapBodyToHtml, tiptapExcerpt } from '$lib/tiptap/serialize';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import VoteRail from '$lib/components/forum/VoteRail.svelte';
	import ReplyThread from '$lib/components/forum/ReplyThread.svelte';
	import PollBlock from '$lib/components/forum/PollBlock.svelte';
	import PostEditor from '$lib/components/create/editors/PostEditor.svelte';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { hydrateVideoEmbeds } from '$lib/tiptap/hydrateVideoEmbeds';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';
	import type { ForumThread } from '$lib/types';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';
import { contentHref } from '$lib/utils/contentLink';

	let { data }: { data: PageData } = $props();

	const thread = $derived(data.thread);
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds as string[]) : undefined
	);
	const threadUserVote = $derived((data.userVotes?.[thread.id] ?? 0) as 0 | 1 | -1);
	const bodyHtml = $derived(
		tiptapBodyToHtml(thread.body, {
			deletedUserIds,
			deletedUserLabel: t('mentions.deleted_user', '[deleted user]')
		})
	);
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

	// BreadcrumbList JSON-LD (Section 26 — all pages with hierarchy).
	// Forum threads live under /forum, optionally within a space.
	const breadcrumbLd = $derived(
		JSON.stringify(
			breadcrumbJsonLd(
				thread.space_slug
					? [
							{ name: 'Home', path: page.url.origin },
							{ name: 'Forum', path: `${page.url.origin}/forum` },
							{ name: thread.space_name ?? thread.space_slug, path: `${page.url.origin}/spaces/${thread.space_slug}` },
							{ name: thread.title, path: page.url.href }
						]
					: [
							{ name: 'Home', path: page.url.origin },
							{ name: 'Forum', path: `${page.url.origin}/forum` },
							{ name: thread.title, path: page.url.href }
						]
			)
		)
	);

	// Solve / un-solve (OP or moderator only — backend enforces the permission).
	let solved = $state(false);
	let solving = $state(false);

	// Sync the solved flag from the server payload on mount and on navigation.
	$effect(() => {
		solved = thread.solved;
	});

	let acceptedReplyId = $state<string | null>(null);
	$effect(() => {
		acceptedReplyId = data.acceptedReplyId;
	});

	let showPollForm = $state(false);
	let pollQuestion = $state('');
	let pollMulti = $state(false);
	let pollCloseAt = $state('');
	let pollOptions = $state<string[]>(['', '']);
	let pollSubmitting = $state(false);
	let pollError = $state<string | null>(null);

	const canCreatePoll = $derived.by(
		() => data.authed && !data.poll && (data.canModerate || data.currentUserId === thread.author_user_id)
	);

	async function createPoll(e: Event) {
		e.preventDefault();
		if (pollSubmitting) return;
		pollSubmitting = true;
		pollError = null;

		const options = pollOptions.map((label) => label.trim()).filter(Boolean);
		if (pollQuestion.trim() === '') {
			pollError = t('forum.poll_question_required', 'Poll question is required');
			pollSubmitting = false;
			return;
		}
		if (options.length < 2) {
			pollError = t('forum.poll_options_min', 'Add at least two options');
			pollSubmitting = false;
			return;
		}

		const body: Record<string, any> = {
			question: pollQuestion.trim(),
			is_multi_select: pollMulti,
			options: options.map((label, i) => ({ label, position: i }))
		};
		if (pollCloseAt) body.close_at = new Date(pollCloseAt).toISOString();

		const res = await api.post(`/forum/${thread.id}/poll`, body);
		pollSubmitting = false;
		if (res.error) {
			pollError = res.error.message ?? t('forum.poll_create_error', 'Could not create poll');
		} else {
			pollQuestion = '';
			pollMulti = false;
			pollCloseAt = '';
			pollOptions = ['', ''];
			showPollForm = false;
			await invalidateAll();
		}
	}

	async function toggleSolve(replyId?: string) {
		if (solving || !data.canSolve) return;
		solving = true;
		const wasSolved = solved;
		solved = !wasSolved; // optimistic
		const res = wasSolved
			? await api.delete(`/forum/${thread.id}/solve`)
			: await api.post(`/forum/${thread.id}/solve`, replyId ? { reply_id: replyId } : {});
		solving = false;
		if (res.error) {
			solved = wasSolved;
		} else if (!wasSolved && replyId) {
			acceptedReplyId = replyId;
		}
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

	// Report thread — submits a content report via POST /content/{content_id}/report.
	// A forum thread is reported as entity_type 'content' using its content_id
	// (the contents row backing the thread). Ref: japaconnect-engineering/sections/51-content-reports-flagging.md
	let reportOpen = $state(false);
	let reportReason = $state('');
	let reportNotes = $state('');
	let reportSubmitting = $state(false);
	let reportError = $state('');
	let reportSubmitted = $state(false);

	function openReport() {
		if (!thread.content_id) return;
		reportOpen = true;
		reportSubmitted = false;
		reportError = '';
	}

	function closeReport() {
		reportOpen = false;
		reportSubmitted = false;
		reportError = '';
		reportReason = '';
		reportNotes = '';
	}

	async function submitReport() {
		if (!(await requireAuthOrRedirect())) return;
		if (!thread.content_id || !reportReason.trim() || reportSubmitting) return;
		reportSubmitting = true;
		reportError = '';
		const res = await api.post<{ id: string }>(`/content/${thread.content_id}/report`, {
			reason: reportReason.trim(),
			notes: reportNotes.trim() || null,
			evidence_urls: []
		});
		if (res.error) {
			reportError = res.error.message;
		} else {
			reportSubmitted = true;
			reportReason = '';
			reportNotes = '';
		}
		reportSubmitting = false;
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
	let deleting = $state(false);
	let deleteError = $state<string | null>(null);

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

	async function deleteThread() {
		if (deleting || !data.canDelete) return;
		if (!confirm(t('forum.delete_confirm', 'Delete this discussion? This cannot be undone.'))) return;
		deleting = true;
		deleteError = null;
		const res = await api.delete(`/forum/${thread.id}`);
		deleting = false;
		if (res.error) {
			deleteError = res.error.message || t('forum.delete_error', 'Failed to delete discussion.');
			return;
		}
		await goto('/forum');
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={thread.title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{@html safeJsonLdScript(jsonLd)}
	{@html safeJsonLdScript(breadcrumbLd)}
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
					userVote={threadUserVote}
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
					{#if data.canDelete && !editing}
						<button
							type="button"
							disabled={deleting}
							onclick={deleteThread}
							class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{#if deleting}<Loader2 size={12} class="animate-spin" />{:else}<Trash2 size={12} />{/if}
							{t('forum.delete', 'Delete')}
						</button>
					{/if}
					{#if deleteError}
						<p class="mt-2 rounded-[var(--radius)] border border-[var(--color-error)]/30 bg-[color-mix(in_srgb,var(--color-error)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-error)]">
							{deleteError}
						</p>
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
					{#if thread.is_anonymous}
						<span class="font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
					{:else}
						<UserHoverCard
							id={thread.author_user_id}
							slug={thread.author_username}
							name={authorLabel}
							variant="row"
						>
							{#snippet trigger()}
								<span class="font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
							{/snippet}
						</UserHoverCard>
					{/if}
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
				userVote={threadUserVote}
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
			<div class="prose mt-5 max-w-none text-[var(--color-text)]" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds>
				{@html bodyHtml}
			</div>
		{/if}

		<!-- Poll (if the thread has one) -->
		{#if data.poll}
			<div class="mt-5">
				<PollBlock pollData={data.poll} spaceId={data.thread.space_id} />
			</div>
		{:else if canCreatePoll}
			<div class="mt-5">
				{#if showPollForm}
					<form onsubmit={createPoll} class="space-y-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3">
						<div>
							<label for="poll-question" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('forum.poll_question', 'Poll question')}</label>
							<input id="poll-question" type="text" bind:value={pollQuestion} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]" placeholder={t('forum.poll_question_placeholder', 'Ask a question…')} />
						</div>
						<div class="flex items-center gap-2">
							<input id="poll-multi" type="checkbox" bind:checked={pollMulti} class="h-4 w-4" />
							<label for="poll-multi" class="text-sm text-[var(--color-text)]">{t('forum.poll_multi_select', 'Allow multiple choices')}</label>
						</div>
						<div>
							<label for="poll-close" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('forum.poll_close_at', 'Close at (optional)')}</label>
							<input id="poll-close" type="datetime-local" bind:value={pollCloseAt} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]" />
						</div>
						<div class="space-y-2">
							<p class="block text-sm font-medium text-[var(--color-text)]">{t('forum.poll_options', 'Options')}</p>
							{#each pollOptions as label, i (i)}
								<div class="flex gap-2">
									<input type="text" bind:value={pollOptions[i]} class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]" placeholder={`${t('forum.poll_option', 'Option')} ${i + 1}`} />
									{#if pollOptions.length > 2}
										<button type="button" onclick={() => pollOptions = pollOptions.filter((_, idx) => idx !== i)} class="text-[var(--color-error)]"><X size={18} /></button>
									{/if}
								</div>
							{/each}
							<button type="button" onclick={() => pollOptions = [...pollOptions, '']} class="inline-flex items-center gap-1 text-sm text-[var(--color-primary)]">
								<Plus size={14} /> {t('forum.poll_add_option', 'Add option')}
							</button>
						</div>
						{#if pollError}
							<p class="text-sm text-[var(--color-error)]">{pollError}</p>
						{/if}
						<div class="flex gap-2">
							<button type="submit" disabled={pollSubmitting} class="inline-flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-white disabled:opacity-50">
								{#if pollSubmitting}<Loader2 size={14} class="animate-spin" />{/if}
								{t('forum.create_poll', 'Create poll')}
							</button>
							<button type="button" onclick={() => showPollForm = false} class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)]">{t('common.cancel', 'Cancel')}</button>
						</div>
					</form>
				{:else}
					<button type="button" onclick={() => showPollForm = true} class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline">
						<Plus size={14} /> {t('forum.add_poll', 'Add poll')}
					</button>
				{/if}
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
			<button type="button" onclick={openReport} disabled={!data.authed || !thread.content_id} class="flex items-center gap-1 transition-colors hover:text-[var(--color-text)] disabled:cursor-not-allowed disabled:opacity-50" title={!data.authed ? t('forum.login_to_report', 'Log in to report') : thread.content_id ? t('forum.report', 'Report') : t('forum.report_unavailable', 'Reporting unavailable')}>
				<Flag size={14} /> {t('forum.report', 'Report')}
			</button>
			{#if data.canSolve}
				<button
					type="button"
					onclick={() => toggleSolve()}
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
		acceptedReplyId={acceptedReplyId}
		userVotes={data.userVotes}
		canModerate={data.canModerate}
		currentUserId={data.currentUserId}
		{deletedUserIds}
		onMarkSolved={toggleSolve}
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
							href={contentHref(rel)}
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

<!-- Report thread dialog — POST /content/{content_id}/report (Section 51) -->
{#if reportOpen}
	<Dialog.Root open={reportOpen}>
		<Dialog.Portal>
			<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
			<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl">
				<Dialog.Close class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={closeReport}>
					<X size={20} />
				</Dialog.Close>
				<div class="mb-4 flex items-center gap-2">
					<Flag size={20} class="text-red-500" />
					<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('forum.report_title', 'Report this discussion')}</h2>
				</div>
				{#if reportSubmitted}
					<p class="rounded-lg bg-green-50 px-4 py-3 text-sm text-green-700 dark:bg-green-950/30 dark:text-green-400">
						{t('forum.report_submitted', 'Your report has been submitted. Our team will review it.')}
					</p>
					<button
						type="button"
						onclick={closeReport}
						class="mt-4 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
					>
						{t('common.close', 'Close')}
					</button>
				{:else}
					<form onsubmit={(e) => { e.preventDefault(); submitReport(); }} class="space-y-3">
						{#if reportError}
							<p class="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{reportError}</p>
						{/if}
						<div>
							<label for="report-reason" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('forum.report_reason', 'Reason')}
							</label>
							<select
								id="report-reason"
								bind:value={reportReason}
								class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
								required
							>
								<option value="">{t('forum.report_reason_select', 'Select a reason…')}</option>
								<option value="spam">{t('forum.report_spam', 'Spam')}</option>
								<option value="harassment">{t('forum.report_harassment', 'Harassment')}</option>
								<option value="hate_speech">{t('forum.report_hate_speech', 'Hate speech')}</option>
								<option value="misinformation">{t('forum.report_misinformation', 'Misinformation')}</option>
								<option value="illegal_content">{t('forum.report_illegal', 'Illegal content')}</option>
								<option value="self_harm">{t('forum.report_self_harm', 'Self harm')}</option>
								<option value="copyright">{t('forum.report_copyright', 'Copyright')}</option>
								<option value="other">{t('forum.report_other', 'Other')}</option>
							</select>
						</div>
						<div>
							<label for="report-notes" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('forum.report_notes', 'Additional notes (optional)')}
							</label>
							<textarea
								id="report-notes"
								bind:value={reportNotes}
								rows={3}
								class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
							></textarea>
						</div>
						<div class="flex items-center gap-2">
							<button
								type="submit"
								disabled={reportSubmitting || !reportReason.trim()}
								class="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
							>
								{#if reportSubmitting}<Loader2 size={14} class="animate-spin" />{:else}
								{t('forum.report_submit', 'Submit report')}{/if}
							</button>
							<button
								type="button"
								onclick={closeReport}
								class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('common.cancel', 'Cancel')}
							</button>
						</div>
					</form>
				{/if}
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
{/if}
