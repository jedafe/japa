<script lang="ts">
	import { page } from '$app/state';
	import { goto, invalidate } from '$app/navigation';
	import { t, i18n } from '$lib/i18n';
	import { ArrowLeft, Check, Clock, Eye, Loader2, Pencil, Send, Trash2 } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import ReactionRail from '$lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '$lib/components/blog/CreatorCard.svelte';
	import RecommendedContentCard from '$lib/components/content/RecommendedContentCard.svelte';
	import SidebarWidgetRenderer from '$lib/components/shell/SidebarWidgetRenderer.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import BlogSeriesNav from '$lib/components/blog/BlogSeriesNav.svelte';
	import InArticleAd from '$lib/components/ads/InArticleAd.svelte';
	import NextActionInline from '$lib/components/shell/NextActionInline.svelte';
	import AdSlot from '$lib/components/ui/AdSlot.svelte';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { imgLightbox } from '$lib/lightbox.svelte';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { hydrateVideoEmbeds } from '$lib/tiptap/hydrateVideoEmbeds';
	import { safeJsonLdScript, sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { excerpt } from '$lib/utils/seo-score';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';
	import { contentHref } from '$lib/utils/contentLink';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const article = $derived(data.article);
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds as string[]) : undefined
	);

	const isDraft = $derived(article.status === 'draft');
	const canSubmitForReview = $derived(data.canEdit && isDraft);
	let submittingForReview = $state(false);
	let reviewSubmitted = $state(false);
	let actionError = $state<string | null>(null);

	async function submitForReview() {
		if (!canSubmitForReview || submittingForReview) return;
		submittingForReview = true;
		actionError = null;
		const res = await api.post(`/blog/${article.id}/submit-for-review`);
		if (!res.error) {
			reviewSubmitted = true;
			await invalidate(`blog:${article.slug}`);
		} else {
			actionError = res.error.message;
		}
		submittingForReview = false;
	}

	let deleting = $state(false);
	async function deletePost() {
		if (!data.canDelete || deleting) return;
		const confirmed = window.confirm(t('blog.delete_confirm', 'Are you sure you want to delete this post?'));
		if (!confirmed) return;
		deleting = true;
		actionError = null;
		const res = await api.delete(`/blog/${article.id}`);
		if (!res.error) {
			await goto('/blog');
		} else {
			actionError = res.error.message;
			deleting = false;
		}
	}

	// Normalize sidebar widgets to the typed shape expected by SidebarWidgetRenderer.
	const sidebarWidgets = $derived(
		(data.rightSidebarWidgets as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number }[]).filter(
			(w) => w && typeof w.widget_type === 'string'
		)
	);
	const publishedDate = $derived(new Date(article.published_at || article.created_at).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'long', day: 'numeric' }));
	const title = $derived(`${article.title} — ${t('app.name')}`);
	const description = $derived(
		article.excerpt ?? excerpt(article.body_html || article.body, 160)
	);
	// Section 26 — self-referencing canonical on the clean slug URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(`${page.url.origin}/blog/${article.slug}`);
	// The featured image is the LCP element on most posts — preconnect its
	// (usually cross-origin) host and preload it so discovery isn't blocked
	// behind stylesheet fetches. Guarded because URL parsing can throw on
	// relative/empty URLs.
	const featuredImageOrigin = $derived.by(() => {
		try {
			return article.featured_image_url ? new URL(article.featured_image_url).origin : '';
		} catch {
			return '';
		}
	});
	const allowedIframeHosts = $derived(
		(data.siteSettings?.['content.sanitization.allowed_iframe_hosts'] as string | undefined) ?? undefined
	);
	const sanitizedBody = $derived(sanitizeContentHtml(article.body_html || article.body, allowedIframeHosts));

	// JSON-LD structured data (Section 14, Section 26 — SEO). The backend
	// resolves the admin-managed Article template server-side and returns it
	// in the response envelope's `json_ld` sidecar. Fall back to a basic
	// BlogPosting schema when the backend didn't emit one (no active template).
	const jsonLd = $derived(
		data.jsonLd ?? {
			'@context': 'https://schema.org',
			'@type': 'BlogPosting',
			headline: article.title,
			datePublished: article.published_at || article.created_at,
			author: { '@type': 'Person', name: article.author_name || t('app.name') },
			url: canonical,
			description
		}
	);

	// View tracking (Section 14 — POST /content/{id}/view). Fire-and-forget
	// on mount; the backend deduplicates per user/session per day.
	$effect(() => {
		if (!article.id) return;
		api.post(`/content/${article.id}/view`, {});
	});

	// BreadcrumbList JSON-LD (Section 26 — all pages with hierarchy).
	const breadcrumbLd = $derived(
		breadcrumbJsonLd([
			{ name: 'Home', path: page.url.origin },
			{ name: 'Blog', path: `${page.url.origin}/blog` },
			{ name: article.title, path: canonical }
		])
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={article.title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	{#if article.featured_image_url}
		<meta property="og:image" content={article.featured_image_url} />
		{#if featuredImageOrigin && featuredImageOrigin !== page.url.origin}<link rel="preconnect" href={featuredImageOrigin} crossorigin="anonymous" />{/if}
		<link rel="preload" as="image" href={article.featured_image_url} fetchpriority="high" />
	{/if}
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={article.title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
	{@html safeJsonLdScript(breadcrumbLd)}
	{#if article?.seo_indexable === false || article?.indexable === false}
		<meta name="robots" content="noindex,follow" />
	{/if}
</svelte:head>

<!-- ADR-058 Exception #1: dev.to reading layout.
     Left nav is auto-collapsed by the layout (isExceptionRoute handles /create,
     but blog single uses a responsive grid here that works within the shell).
     On mobile the right sidebar is stacked below the article body. -->
{#snippet rightSidebar()}
	<div class="flex flex-col gap-4">
		<CreatorCard
			authorName={article.author_name}
			authorUsername={article.author_username}
			authorAvatarUrl={article.author_avatar_url}
			spaceName={article.space_name}
			spaceSlug={article.space_slug}
			authorId={article.author_id}
			initiallyFollowing={data.initiallyFollowing}
			authed={data.authed}
			currentUserId={data.currentUserId}
		/>
		<!-- Admin-managed sidebar widgets (Section 59, ADR-066). -->
		{#each sidebarWidgets as widget (widget.id)}
			<SidebarWidgetRenderer
				{widget}
				user={data.user}
				trendingTags={data.trendingTags}
				suggestedSpaces={data.suggestedSpaces}
				enabledLanguages={data.siteSettings?.['i18n.enabled_languages'] as string | undefined}
				isDashboard={false}
			/>
		{/each}
	</div>
{/snippet}

<!--
	Content single pages render the unified shell's left ad rail (Section 59) instead of a
	page-owned left reaction rail. The action bar is now a horizontal rail under the body.
-->
<div class="mx-auto grid grid-cols-1 max-w-[65rem] gap-8 px-4 py-8 pb-24 sm:px-6 lg:pb-8 lg:grid-cols-1 xl:grid-cols-[1fr_22rem]">
	<!-- Center: article -->
	<article class="min-w-0 max-w-[41rem]">
		<div class="mb-8 flex flex-wrap items-center justify-between gap-3">
			<a href="/blog" class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]">
				<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('blog.back', 'Back to blog')}
			</a>
			<div class="flex flex-wrap items-center gap-2">
				{#if data.canEdit}
					<a
						href={`/studio/create?draft=${article.id}`}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
					>
						<Pencil size={14} /> {t('content.edit', 'Edit')}
					</a>
				{/if}
				{#if canSubmitForReview}
					<button
						type="button"
						onclick={submitForReview}
						disabled={submittingForReview || reviewSubmitted}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)] disabled:opacity-50"
					>
						{#if reviewSubmitted}
							<Check size={14} /> {t('blog.submitted', 'Submitted')}
						{:else if submittingForReview}
							<Loader2 size={14} class="animate-spin" /> {t('blog.submitting', 'Submitting…')}
						{:else}
							<Send class="rtl:-scale-x-100" size={14} /> {t('blog.submit_for_review', 'Submit for review')}
						{/if}
					</button>
				{/if}
				{#if data.canDelete}
					<button
						type="button"
						onclick={deletePost}
						disabled={deleting}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-destructive)] transition-colors hover:border-[var(--color-destructive)]/40 hover:bg-[var(--color-destructive)]/10 disabled:opacity-50"
					>
						{#if deleting}
							<Loader2 size={14} class="animate-spin" />
						{:else}
							<Trash2 size={14} />
						{/if}
						{t('content.delete', 'Delete')}
					</button>
				{/if}
			</div>
		</div>

		{#if actionError}
			<p class="mb-8 -mt-4 text-sm text-[var(--color-destructive)]" role="alert">{actionError}</p>
		{/if}

		{#if article.featured_image_url}
			<img src={article.featured_image_url} alt={article.title} fetchpriority="high" class="mb-8 aspect-[16/9] w-full rounded-[var(--radius-card)] object-cover" />
		{/if}

		<h1 class="text-balance text-4xl font-bold leading-tight tracking-tight text-[var(--color-text)] md:text-5xl" style="font-family: var(--font-heading, var(--font-serif));">{article.title}</h1>

		<div class="mt-6 flex flex-wrap items-center gap-4 border-b border-[var(--color-border)] pb-6 text-sm text-[var(--color-text-subtle)]">
			{#if article.author_name}
				{@const authorName = article.author_name}
				<UserHoverCard
					id={article.author_id ?? ''}
					slug={article.author_username}
					name={authorName}
					avatar={article.author_avatar_url}
					variant="row"
				>
					{#snippet trigger()}
						{#if article.author_avatar_url}
							<img src={article.author_avatar_url} alt="" class="h-9 w-9 rounded-full object-cover" />
						{:else}
							<span class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-bold text-[var(--color-primary)]">{authorName.charAt(0)}</span>
						{/if}
						<span class="font-medium text-[var(--color-text)]">{authorName}</span>
					{/snippet}
				</UserHoverCard>
			{/if}
			<span>·</span>
			<span>{publishedDate}</span>
			{#if article.read_time_minutes > 0}
				<span class="flex items-center gap-1"><Clock size={14} /> {article.read_time_minutes} {t('blog.min_read', 'min read')}</span>
			{/if}
			<span class="flex items-center gap-1"><Eye size={14} /> {article.view_count} {t('content.views', 'views')}</span>
			{#if data.canEdit && article.status !== 'published'}
				{@const statusClass =
					article.status === 'draft'
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]'
						: article.status === 'submitted' || article.status === 'in_review'
							? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'}
				<span class="rounded-full px-2.5 py-0.5 text-xs font-medium capitalize {statusClass}">
					{article.status.replace('_', ' ')}
				</span>
			{/if}
		</div>

		{#if article.tags && article.tags.length > 0}
			<div class="mt-4 flex flex-wrap gap-2">
				{#each article.tags as tag (tag)}
					<a href={`/tags/${tag}`} class="rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-primary)]/10 hover:text-[var(--color-primary)]">#{tag}</a>
				{/each}
			</div>
		{/if}

		{#if article.series_id && article.series_title && article.series_slug}
			<div class="mt-6">
				<BlogSeriesNav
					title={article.series_title}
					slug={article.series_slug}
					total={data.seriesPosts?.length ?? article.series_total ?? 0}
					posts={data.seriesPosts ?? []}
					currentSlug={article.slug}
					currentPart={article.series_part ?? 0}
				/>
			</div>
		{/if}

		<!-- Article body: prose TipTap HTML with serif reading font.
		     InArticleAd splits the sanitized body on block boundaries and
		     injects the article_inline ad slot (§25 amendment). -->
		<div class="prose mt-8" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds use:imgLightbox>
			<NextActionInline />
			<InArticleAd html={sanitizedBody} />
		</div>

		<!-- Reaction/action bar: fixed bottom bar on mobile (this route is an
		     ADR-058 exception, so the shell's mobile tab bar is absent), falling
		     back to the horizontal strip under the body on sm+. -->
		<div class="mt-8">
			<ReactionRail
				contentId={article.id}
				voteScore={article.reaction_count}
				commentCount={article.comment_count}
				authed={data.authed}
				bottom
				authorUsername={article.author_username}
				authorId={article.author_id}
				authorName={article.author_name}
			/>
		</div>

		<!-- Mobile right sidebar: author card + widgets, stacked below the body on small screens. -->
		<div class="mt-8 flex flex-col gap-4 xl:hidden">
			{@render rightSidebar()}
		</div>

		<!-- Unified comment engine (Section 49) — Reddit-style nested comments. -->
		<CommentThread
			comments={data.comments}
			commentsTotal={data.commentsTotal}
			totalCount={article.comment_count}
			contentId={article.id}
			contentType={article.content_type}
			authed={data.authed}
			currentUserId={data.currentUserId}
			canModerate={data.canModerate}
			loadError={data.commentsLoadError}
			deletedUserIds={deletedUserIds}
		/>

		<!-- §25: engagement-area ad slots — hidden when no campaign fills them. -->
		<AdSlot slot="comments_below" format="banner" />
		<AdSlot slot="article_below_comments" format="banner" />

		<!-- Same-type recommendations, ranked by embedding similarity. -->
		<div class="mt-8">
			<RecommendedContentCard items={data.recommendedContent ?? []} contentType={article.content_type} />
		</div>

		<!-- Related content (Section 14 — GET /content/{id}/related). -->
		{#if data.relatedContent && data.relatedContent.length > 0}
			<section class="mt-8">
				<h2 class="mb-4 text-lg font-semibold text-[var(--color-text)]">{t('blog.related', 'Related')}</h2>
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					{#each data.relatedContent as rel (rel.id)}
						<a
							href={contentHref(rel)}
							class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/30"
						>
							{#if rel.featured_image_url}
								<img src={rel.featured_image_url} alt="" loading="lazy" class="mb-3 aspect-[16/9] w-full rounded-[var(--radius)] object-cover" />
							{/if}
							<h3 class="line-clamp-2 font-medium text-[var(--color-text)]">{rel.title}</h3>
							{#if rel.author_name}
								<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{rel.author_name}</p>
							{/if}
						</a>
					{/each}
				</div>
			</section>
		{/if}
	</article>

	<!-- Right: CreatorCard + admin-managed widgets. Hidden on mobile because a
	     stacked copy is rendered inside the article body. -->
	<aside class="hidden w-80 shrink-0 xl:block">
		<div class="flex flex-col gap-4 xl:sticky xl:top-24">
			{@render rightSidebar()}
		</div>
	</aside>
</div>
