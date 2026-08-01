<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { ArrowLeft, Clock, Eye } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import ReactionRail from '$lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '$lib/components/blog/CreatorCard.svelte';
	import SidebarWidgetRenderer from '$lib/components/shell/SidebarWidgetRenderer.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const article = $derived(data.article);

	// Normalize sidebar widgets to the typed shape expected by SidebarWidgetRenderer.
	const sidebarWidgets = $derived(
		(data.rightSidebarWidgets as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number }[]).filter(
			(w) => w && typeof w.widget_type === 'string'
		)
	);
	const publishedDate = $derived(new Date(article.published_at || article.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' }));
	const title = $derived(`${article.title} — ${t('app.name')}`);
	const description = $derived(article.excerpt ?? article.body.slice(0, 160));

	// JSON-LD structured data (Section 14, Section 26 — SEO). Uses the
	// admin-configured template from GET /content/jsonld-templates when
	// available; falls back to a basic Article schema.
	const jsonLd = $derived(
		data.jsonldTemplate
			? JSON.stringify({
					...data.jsonldTemplate,
					headline: article.title,
					datePublished: article.published_at || article.created_at,
					author: { '@type': 'Person', name: article.author_name ?? 'JapaConnect' },
					url: page.url.href,
					description
				})
			: JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Article',
					headline: article.title,
					datePublished: article.published_at || article.created_at,
					author: { '@type': 'Person', name: article.author_name ?? 'JapaConnect' },
					url: page.url.href,
					description
				})
	);

	// View tracking (Section 14 — POST /content/{id}/view). Fire-and-forget
	// on mount; the backend deduplicates per user/session per day.
	$effect(() => {
		if (!article.id) return;
		api.post(`/content/${article.id}/view`, {});
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={article.title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{#if article.featured_image_url}<meta property="og:image" content={article.featured_image_url} />{/if}
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={article.title} />
	<meta name="twitter:description" content={description} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<!-- ADR-058 Exception #1: dev.to reading layout (same as blog singles).
     Left nav is auto-collapsed by the layout (isExceptionRoute covers /content/[slug]). -->
<div class="mx-auto flex max-w-6xl gap-8 px-4 py-8">
	<!-- Left margin: borderless ReactionRail (distinct from the collapsed nav) -->
	<div class="hidden w-14 shrink-0 lg:block">
		<ReactionRail contentId={article.id} voteScore={article.reshare_count} commentCount={article.comment_count} authed={data.authed} />
	</div>

	<!-- Center: article -->
	<article class="min-w-0 flex-1 max-w-3xl">
		<a href="/explore" class="mb-8 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]">
			<ArrowLeft size={16} /> {t('content.back', 'Back to explore')}
		</a>

		{#if article.featured_image_url}
			<img src={article.featured_image_url} alt={article.title} class="mb-8 aspect-[16/9] w-full rounded-[var(--radius-card)] object-cover" />
		{/if}

		<h1 class="text-balance text-4xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-5xl" style="font-family: var(--font-serif);">{article.title}</h1>

		<div class="mt-6 flex flex-wrap items-center gap-4 border-b border-[var(--color-border)] pb-6 text-sm text-[var(--color-text-subtle)]">
			{#if article.author_name}
				<span class="flex items-center gap-2">
					{#if article.author_avatar_url}
						<img src={article.author_avatar_url} alt="" class="h-9 w-9 rounded-full object-cover" />
					{:else}
						<span class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-bold text-[var(--color-primary)]">{article.author_name.charAt(0)}</span>
					{/if}
					<span class="font-medium text-[var(--color-text)]">{article.author_name}</span>
				</span>
			{/if}
			<span>·</span>
			<span>{publishedDate}</span>
			{#if article.read_time_minutes > 0}
				<span class="flex items-center gap-1"><Clock size={14} /> {article.read_time_minutes} {t('blog.min_read', 'min read')}</span>
			{/if}
			<span class="flex items-center gap-1"><Eye size={14} /> {article.view_count} views</span>
		</div>

		{#if article.tags && article.tags.length > 0}
			<div class="mt-4 flex flex-wrap gap-2">
				{#each article.tags as tag (tag)}
					<a href={`/tags/${tag}`} class="rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-primary)]/10 hover:text-[var(--color-primary)]">#{tag}</a>
				{/each}
			</div>
		{/if}

		<!-- Article body: prose TipTap HTML with serif reading font -->
		<div class="prose mt-8">
			{@html article.body_html || article.body}
		</div>

		<!-- Mobile reaction bar (visible when ReactionRail is hidden) -->
		<div class="mt-8 flex items-center gap-4 border-t border-[var(--color-border)] pt-4 lg:hidden">
			<span class="text-sm text-[var(--color-text-subtle)]">{article.reshare_count} reactions</span>
			<span class="text-sm text-[var(--color-text-subtle)]">{article.comment_count} comments</span>
		</div>

		<!-- Unified comment engine (Section 49) — Reddit-style nested comments. -->
		<CommentThread
			comments={data.comments}
			contentId={article.id}
			authed={data.authed}
			currentUserId={data.currentUserId}
			canModerate={data.canModerate}
			loadError={data.commentsLoadError}
		/>

		<!-- Related content (Section 14 — GET /content/{id}/related). -->
		{#if data.relatedContent && data.relatedContent.length > 0}
			<section class="mt-8">
				<h2 class="mb-4 text-lg font-semibold text-[var(--color-text)]">{t('content.related', 'Related')}</h2>
				<div class="grid gap-4 sm:grid-cols-2">
					{#each data.relatedContent as rel (rel.id)}
						<a
							href={rel.content_type === 'blog_post' ? `/blog/${rel.slug}` : `/content/${rel.slug}`}
							class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/30"
						>
							{#if rel.featured_image_url}
								<img src={rel.featured_image_url} alt="" class="mb-3 aspect-[16/9] w-full rounded-[var(--radius)] object-cover" />
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

	<!-- Right: CreatorCard replaces the search/avatar bar -->
	<aside class="hidden w-80 shrink-0 xl:block">
		<div class="sticky top-24 flex flex-col gap-4">
			<CreatorCard
				authorName={article.author_name}
				authorAvatarUrl={article.author_avatar_url}
				spaceName={article.space_name}
				authorId={article.author_id}
				initiallyFollowing={data.initiallyFollowing}
				authed={data.authed}
				currentUserId={data.currentUserId}
			/>
			<!-- Admin-managed sidebar widgets (Section 59, ADR-066). -->
			{#each sidebarWidgets as widget (widget.id)}
				<SidebarWidgetRenderer {widget} user={null} />
			{/each}
		</div>
	</aside>
</div>
