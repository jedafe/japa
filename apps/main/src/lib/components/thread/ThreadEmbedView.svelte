<script lang="ts">
	// /thread/{slug} universal thread view — renders EVERY non-forum thread
	// (anchor threads, §49 linked discussions, content threads) exactly like
	// the former forum page: body section at the top (avatar, author meta,
	// title, description/excerpt), the canonical-content embed card at the
	// bottom of the body before the reaction rail, then replies.
	// Ref: docs/superpowers/specs/2026-11-08-thread-rename-unified-discussions-design.md
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { t, i18n } from '#lib/i18n/index.js';
	import { ArrowLeft, Eye, MessageSquare } from 'lucide-svelte';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import ThreadEmbedCard from '#lib/components/thread/ThreadEmbedCard.svelte';
	import ReplyThread from '#lib/components/forum/ReplyThread.svelte';
	import ReactionRail from '#lib/components/blog/ReactionRail.svelte';
	import UserHoverCard from '#lib/components/ui/UserHoverCard.svelte';
	import { commentsApi, commentsConversation, listAllComments } from '#lib/components/forum/conversationApi.js';
	import { tiptapBodyToHtml } from '#lib/tiptap/serialize.js';
	import { relativeTime } from '#lib/feed/time.js';
	import { hydrateLinkPreviews } from '#lib/tiptap/hydrateLinkPreviews.js';
	import { hydrateMentions } from '#lib/tiptap/hydrateMentions.js';
	import { hydrateVideoEmbeds } from '#lib/tiptap/hydrateVideoEmbeds.js';
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
	// Empty-body threads (minted anchors, linked discussions) fall back to the
	// embed excerpt so the body section still carries a description.
	const fallbackExcerpt = $derived(!bodyHtml ? (data.embed?.excerpt ?? article.excerpt ?? '') : '');

	const authorLabel = $derived(article.author_name ?? article.author_username ?? t('forum.member', 'Member'));
	const time = $derived(article.published_at ? relativeTime(article.published_at, new Date(), i18n.locale) : '');
	const typeBadge = $derived(t(`content_type.${article.content_type}`, article.content_type));

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

	<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6">
		<!-- Header — author avatar in the left gutter (same layout as the
		     former forum thread page). -->
		<div class="flex items-start gap-3">
			{#if article.author_username}
				<UserHoverCard
					id={article.author_id ?? ''}
					slug={article.author_username}
					name={authorLabel}
					avatar={article.author_avatar_url}
					variant="row"
					class="shrink-0"
				>
					{#snippet trigger()}
						{#if article.author_avatar_url}
							<img
								src={article.author_avatar_url}
								alt=""
								class="h-10 w-10 shrink-0 rounded-full object-cover"
								loading="lazy"
							/>
						{:else}
							<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-bold text-[var(--color-primary-contrast)]">
								{authorLabel.charAt(0).toUpperCase()}
							</span>
						{/if}
					{/snippet}
				</UserHoverCard>
			{:else}
				<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-bold text-[var(--color-primary-contrast)]">
					{authorLabel.charAt(0).toUpperCase()}
				</span>
			{/if}

			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--color-muted)]">
					{#if article.author_username}
						<UserHoverCard
							id={article.author_id ?? ''}
							slug={article.author_username}
							name={authorLabel}
							variant="row"
						>
							{#snippet trigger()}
								<span class="text-sm font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
							{/snippet}
						</UserHoverCard>
					{:else}
						<span class="text-sm font-medium text-[var(--color-text-subtle)]">{authorLabel}</span>
					{/if}
					{#if article.space_name}
						<span>·</span>
						<span class="text-[var(--color-text-subtle)]">
							{#if article.space_slug}<a href="/spaces/{article.space_slug}" class="hover:underline">{article.space_name}</a>{:else}{article.space_name}{/if}
						</span>
					{/if}
					{#if time}<span>·</span><span>{time}</span>{/if}
					{#if article.view_count != null}
						<span>·</span>
						<span class="flex items-center gap-1">
							<Eye size={12} /> {article.view_count} {t('content.views', 'views')}
						</span>
					{/if}

					<div class="ms-auto flex flex-wrap items-center gap-2">
						<span class="rounded border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
							{typeBadge}
						</span>
					</div>
				</div>

				<h1 class="mt-2 text-balance text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-3xl">
					{article.title}
				</h1>
			</div>
		</div>

		<!-- Body — TipTap prose when the thread carries one, otherwise the
		     canonical entity's excerpt (anchor threads have no body). -->
		{#if bodyHtml}
			<div class="prose mt-5 max-w-none text-[var(--color-text)]" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds>
				{@html bodyHtml}
			</div>
		{:else if fallbackExcerpt}
			<p class="mt-5 text-[var(--color-text-subtle)]">{fallbackExcerpt}</p>
		{/if}

		<!-- Canonical content embed — the CTA back to the entity this thread
		     discusses. Sits at the bottom of the body section, before the
		     reaction rail (unified-discussion spec). -->
		{#if data.embed}
			<div class="mt-5">
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

		<!-- Reaction rail — the §125 content rail (love / comments / share /
		     bookmark / tip); fixed bottom bar below sm like the forum page. -->
		<div class="mt-6 border-t border-[var(--color-border)] pt-2">
			<ReactionRail
				contentId={article.id}
				voteScore={article.reaction_count ?? 0}
				commentCount={article.comment_count ?? 0}
				authed={data.authed}
				horizontal
				bottom
				authorUsername={article.author_username}
				authorId={article.author_id}
				authorName={authorLabel}
			/>
		</div>
	</article>

	<!-- Replies — the unified conversation (§49 comments via the §08 reply
	     UI). Discussion happens here, not on the entity page. -->
	<div id="replies" class="mt-6">
		<h2 class="mb-3 flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<MessageSquare size={18} />
			{t('thread.discussions', 'Discussions')}
		</h2>
		{#if data.commentsLoadError}
			<p class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-sm text-[var(--color-muted)]">
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
	</div>
</div>
