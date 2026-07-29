<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { ArrowLeft, CheckCircle2, Eye, MessageSquare, Bookmark, Share2, Flag } from 'lucide-svelte';
	import { relativeTime } from '$lib/feed/time';
	import { tiptapBodyToHtml, tiptapExcerpt } from '$lib/tiptap/serialize';
	import { api } from '$lib/api/client';
	import VoteRail from '$lib/components/forum/VoteRail.svelte';
	import ReplyThread from '$lib/components/forum/ReplyThread.svelte';
	import PollBlock from '$lib/components/forum/PollBlock.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const thread = $derived(data.thread);
	const bodyHtml = $derived(tiptapBodyToHtml(thread.body));
	const time = $derived(relativeTime(thread.created_at));
	const authorLabel = $derived(
		thread.is_anonymous
			? t('forum.anonymous', 'Anonymous')
			: thread.author_display_name || thread.author_username || t('forum.member', 'Member')
	);
	const spaceHref = $derived(thread.space_slug ? `/spaces/${thread.space_slug}` : null);

	const title = $derived(`${thread.title} — ${t('forum.list_title', 'Forum')}`);
	const description = $derived(tiptapExcerpt(thread.body, 160));

	// JSON-LD: QAPage when the thread is solved, DiscussionForumPosting otherwise
	// (H3 — audit finding). QAPage is the correct schema for solved Q&A threads.
	const jsonLd = $derived(
		thread.solved
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'QAPage',
					mainEntity: {
						'@type': 'Question',
						name: thread.title,
						text: tiptapExcerpt(thread.body, 500),
						dateCreated: thread.created_at,
						author: { '@type': 'Person', name: authorLabel },
						acceptedAnswer: {
							'@type': 'Answer',
							dateCreated: thread.created_at,
							author: { '@type': 'Person', name: authorLabel }
						},
						answerCount: thread.reply_count
					}
				})
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

<div class="mx-auto w-full max-w-3xl px-4 py-6">
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
				</div>

				<h1 class="mt-2 text-balance text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-3xl">
					{thread.title}
				</h1>

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

		<!-- Body -->
		<div class="prose mt-5 max-w-none text-[var(--color-text)]">
			{@html bodyHtml}
		</div>

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
			<span class="flex items-center gap-1 [font-variant-numeric:tabular-nums]">
				<Bookmark size={14} /> {thread.bookmark_count}
			</span>
			<button type="button" class="flex items-center gap-1 transition-colors hover:text-[var(--color-text)]">
				<Share2 size={14} /> {t('forum.share', 'Share')}
			</button>
			<button type="button" class="flex items-center gap-1 transition-colors hover:text-[var(--color-text)]">
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
