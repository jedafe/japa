<script lang="ts">
	// /thread/{slug} entity-thread view — the single place where discussion
	// happens for non-forum entities (anchor threads, §49 linked discussions,
	// and any contents row reached via a thread URL). Renders the entity
	// embed card, the thread body when present, then the §49 CommentThread
	// engine (comments post to the backing contents row).
	// Ref: docs/superpowers/specs/2026-11-08-thread-rename-unified-discussions-design.md
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { ArrowLeft } from 'lucide-svelte';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import ThreadEmbedCard from '#lib/components/thread/ThreadEmbedCard.svelte';
	import ReplyThread from '#lib/components/forum/ReplyThread.svelte';
	import { commentsApi, commentsConversation, listAllComments } from '#lib/components/forum/conversationApi.js';
	import { tiptapBodyToHtml } from '#lib/tiptap/serialize.js';
	import type { ContentArticle } from '#lib/types/content.js';
	import type { Comment } from '#lib/types/index.js';

	export interface ThreadEmbed {
		kind: 'entity' | 'parent' | 'self';
		title: string;
		href: string;
		excerpt?: string | null;
		imageUrl?: string | null;
		badge?: string | null;
		entityType?: string;
	}

	interface Props {
		data: {
			article: ContentArticle;
			embed: ThreadEmbed | null;
			comments: Comment[];
			commentsTotal: number | null;
			commentsLoadError: boolean;
			authed: boolean;
			currentUserId: string | null;
			canModerate: boolean;
			deletedUserIds: string[] | null;
			jsonLd: unknown;
			indexable: boolean;
			seoIndexable: boolean;
		};
	}

	let { data }: Props = $props();

	const article = $derived(data.article);
	const canonical = $derived(`${page.url.origin}/thread/${article.slug}`);
	const description = $derived(article.excerpt ?? data.embed?.excerpt ?? '');
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds) : undefined
	);
	const bodyHtml = $derived(
		article.body
			? tiptapBodyToHtml(article.body, {
					deletedUserIds,
					deletedUserLabel: t('mentions.deleted_user', '[deleted user]')
				})
			: ''
	);

	// Unified conversation (thread-rename spec: ReplyThread renders every
	// discussion). The SSR seed is the top-level page only — a client pass
	// pulls descendants so the tree renders complete. invalidateAll() after
	// any post/edit/delete re-seeds and re-runs this effect.
	const convApi = $derived(commentsApi(article.id));
	let conversation = $state(
		untrack(() => commentsConversation(data.comments, data.article.id))
	);
	$effect(() => {
		const seeded = data.comments;
		conversation = commentsConversation(seeded, data.article.id);
		if (seeded.some((c) => (c.reply_count ?? 0) > 0) || (data.commentsTotal ?? 0) > seeded.length) {
			void listAllComments(data.article.id).then((all) => {
				conversation = commentsConversation(all, data.article.id);
			});
		}
	});
	const robots = $derived(
		data.indexable === false || data.seoIndexable === false ? 'noindex,nofollow' : ''
	);
</script>

<SeoHead
	title={article.title}
	{description}
	ogTitle={article.title}
	{canonical}
	ogType="article"
	{robots}
	jsonLd={data.jsonLd}
/>

<div class="mx-auto w-full max-w-3xl px-4 py-6 pb-20 sm:pb-6">
	<a
		href="/thread"
		class="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('thread.back', 'Back to threads')}
	</a>

	<!-- Entity embed card — what this thread is about. -->
	{#if data.embed}
		<div class="mb-4">
			<ThreadEmbedCard
				title={data.embed.title}
				href={data.embed.href}
				excerpt={data.embed.excerpt ?? null}
				badge={data.embed.badge ?? null}
				imageUrl={data.embed.imageUrl ?? null}
				entityType={data.embed.entityType ?? ''}
			/>
		</div>
	{/if}

	<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6">
		<h1 class="text-xl font-bold text-[var(--color-text)]">{article.title}</h1>
		{#if article.author_name || article.author_username}
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('thread.started_by', 'Started by')} {article.author_name ?? article.author_username}
			</p>
		{/if}
		{#if bodyHtml}
			<div class="tiptap-body mt-4">{@html bodyHtml}</div>
		{/if}

		<!-- Unified conversation (Section 49 comments via the §08 reply UI)
		     — the whole point of the thread page: discussion happens here,
		     not on the entity page. -->
		{#if data.commentsLoadError}
			<p class="mt-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-sm text-[var(--color-muted)]">
				{t('comments.load_error', 'Could not load comments right now.')}
			</p>
		{/if}
		<ReplyThread
			replies={conversation.replies}
			threadId={article.id}
			backend={convApi}
			authed={data.authed}
			canSolve={false}
			acceptedReplyId={null}
			userVotes={conversation.userVotes}
			canModerate={data.canModerate}
			currentUserId={data.currentUserId}
			{deletedUserIds}
		/>
	</article>
</div>

<style>
	.tiptap-body {
		font-size: 0.9375rem;
		line-height: 1.6;
		color: var(--color-text);
		word-break: break-word;
	}
	.tiptap-body :global(img) {
		max-width: 100%;
		height: auto;
		border-radius: 0.5rem;
	}
</style>
