<script lang="ts">
	// Shared content single-page layout — used by /content/[slug] (fallback
	// family) and /guide/[slug]. Ref: §14, §49, ADR-058 Exception #1.
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { ArrowLeft, Clock, Eye, Pencil } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import ReactionRail from '$lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '$lib/components/blog/CreatorCard.svelte';
	import BlogSeriesNav from '$lib/components/blog/BlogSeriesNav.svelte';
	import SidebarWidgetRenderer from '$lib/components/shell/SidebarWidgetRenderer.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import OwnerStatsCard from '$lib/components/content/OwnerStatsCard.svelte';
	import VideoEmbed from '$lib/components/content/VideoEmbed.svelte';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { safeJsonLdScript, sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { contentHref } from '$lib/utils/contentLink';
	import type { ContentArticle, ContentAnalytics, SeriesPost } from '$lib/types/content';
	import type { Comment, SessionUser, TrendingTag, SpaceSuggestion, VideoSource } from '$lib/types';

	// Mirrors the loadContentArticle() return shape ($lib/server/content-article.ts).
	// Type-only imports from $lib/server are avoided so the component stays
	// outside SvelteKit's server-module boundary.
	export interface ContentArticlePageData {
		article: ContentArticle;
		authed: boolean;
		currentUserId: string | null;
		canModerate: boolean;
		canEdit: boolean;
		comments: Comment[];
		commentsLoadError: boolean;
		rightSidebarWidgets: unknown[];
		relatedContent: ContentArticle[];
		analytics: ContentAnalytics | null;
		seriesPosts: SeriesPost[];
		jsonLd: unknown;
		deletedUserIds: string[] | null;
		initiallyFollowing: boolean;
		siteSettings?: Record<string, unknown>;
		/** §106 video — the linked video_sources row, loaded by
		 *  /content/[slug]/+page.server.ts when content_type === 'video'. */
		videoSource?: VideoSource | null;
	}

	type PageData = ContentArticlePageData;

	let { data }: { data: PageData } = $props();

	const article = $derived(data.article);
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds as string[]) : undefined
	);

	// Normalize sidebar widgets to the typed shape expected by SidebarWidgetRenderer.
	const sidebarWidgets = $derived(
		(data.rightSidebarWidgets as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number }[]).filter(
			(w) => w && typeof w.widget_type === 'string'
		)
	);

	// Section 59 widget data — provided by the root layout (user) and the
	// (app) layout (trending tags / suggested spaces). Passing null/[] here
	// silently hid every user- and data-dependent widget on content pages.
	const sidebarUser = $derived((page.data.user as SessionUser | null | undefined) ?? null);
	const sidebarTrendingTags = $derived((page.data.trendingTags as TrendingTag[] | undefined) ?? []);
	const sidebarSuggestedSpaces = $derived((page.data.suggestedSpaces as SpaceSuggestion[] | undefined) ?? []);
	const sidebarLanguages = $derived(
		(page.data.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? 'en,fr'
	);

	// Humanize a slug-style field key (e.g. 'visa_fee_usd' → 'Visa Fee Usd')
	// for the guide details card — field labels are admin-managed via custom
	// field groups, which are auth-gated, so the public page falls back to a
	// readable rendering of the key.
	function humanizeKey(key: string): string {
		return key
			.split('_')
			.filter(Boolean)
			.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
			.join(' ');
	}

	// §14 guide amendment — category-specific custom field values rendered as
	// a "key details" card. Internal keys (leading '_') and nested objects are
	// skipped; scalars and flat lists are shown.
	const guideDetails = $derived.by(() => {
		if (article.content_type !== 'guide' || !article.field_values) return [] as { key: string; label: string; value: string }[];
		const out: { key: string; label: string; value: string }[] = [];
		for (const [k, v] of Object.entries(article.field_values)) {
			if (k.startsWith('_') || v == null || v === '' || typeof v === 'function') continue;
			let text = '';
			if (Array.isArray(v)) {
				text = v.filter((x) => x != null && typeof x !== 'object').map(String).join(', ');
			} else if (typeof v === 'object') {
				continue;
			} else if (typeof v === 'boolean') {
				text = v ? t('common.yes', 'Yes') : t('common.no', 'No');
			} else {
				text = String(v);
			}
			if (text) out.push({ key: k, label: humanizeKey(k), value: text });
		}
		return out;
	});

	// Journey-aware metadata chips (§32) — shown for any content that carries
	// them, most commonly guides.
	const journeyChips = $derived(
		[
			article.journey_stage ? humanizeKey(article.journey_stage) : null,
			article.visa_route ? humanizeKey(article.visa_route) : null,
			article.source_country && article.target_country
				? `${article.source_country} → ${article.target_country}`
				: (article.target_country ?? null)
		].filter(Boolean) as string[]
	);
	const publishedDate = $derived(new Date(article.published_at || article.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' }));
	const title = $derived(`${article.title} — ${t('app.name')}`);
	const description = $derived(article.excerpt ?? article.body.slice(0, 160));
	const allowedIframeHosts = $derived(
		(data.siteSettings?.['content.sanitization.allowed_iframe_hosts'] as string | undefined) ?? undefined
	);
	const sanitizedBody = $derived(sanitizeContentHtml(article.body_html || article.body, allowedIframeHosts));

	// Series part links route into this article's own route family
	// (ADR-058): /guide/{slug} for guides, /content/{slug} for generic types.
	const seriesBasePath = $derived(
		contentHref(article).split('/').slice(0, -1).join('/') || '/content'
	);

	// JSON-LD structured data (Section 14, Section 26 — SEO). The backend
	// resolves the admin-managed template (matching the content's
	// content_type) server-side and returns it in the `json_ld` sidecar.
	// Fall back to a basic Article schema when absent.
	const jsonLd = $derived(
		data.jsonLd ?? {
			'@context': 'https://schema.org',
			'@type': 'Article',
			headline: article.title,
			datePublished: article.published_at || article.created_at,
			author: { '@type': 'Person', name: article.author_name || t('app.name') },
			url: page.url.href,
			description
		}
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
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<!-- ADR-058 Exception #1: dev.to reading layout (same as blog singles). -->
{#snippet rightSidebar()}
	<div class="flex flex-col gap-4">
		<CreatorCard
			authorName={article.author_name}
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
				user={sidebarUser}
				trendingTags={sidebarTrendingTags}
				suggestedSpaces={sidebarSuggestedSpaces}
				enabledLanguages={sidebarLanguages}
			/>
		{/each}
	</div>
{/snippet}

<div class="mx-auto flex max-w-full gap-8 px-4 py-8 pb-24 lg:pb-8">
	<!-- Left margin: borderless ReactionRail (distinct from the collapsed nav) -->
	<div class="hidden w-14 shrink-0 lg:block">
		<ReactionRail contentId={article.id} voteScore={article.reshare_count} commentCount={article.comment_count} authed={data.authed} />
	</div>

	<!-- Center: article -->
	<article class="min-w-0 flex-1 max-w-3xl">
		<div class="mb-8 flex items-center justify-between gap-3">
			<a href="/explore" class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]">
				<ArrowLeft size={16} /> {t('content.back', 'Back to explore')}
			</a>
			{#if data.canEdit}
				<a
					href={`/create?draft=${article.id}`}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
				>
					<Pencil size={14} /> {t('content.edit', 'Edit')}
				</a>
			{/if}
		</div>

		<!-- §106 video — the player replaces the featured image on video
		     posts; the stored body only has an empty .video-embed placeholder. -->
		{#if article.content_type === 'video' && data.videoSource}
			<div class="mb-8">
				<VideoEmbed source={data.videoSource} />
			</div>
		{:else if article.featured_image_url}
			<img src={article.featured_image_url} alt={article.title} class="mb-8 aspect-[16/9] w-full rounded-[var(--radius-card)] object-cover" />
		{/if}

		<h1 class="text-balance text-4xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-5xl" style="font-family: var(--font-serif);">{article.title}</h1>

		{#if article.content_type === 'guide' && article.guide_category || journeyChips.length > 0}
			<div class="mt-4 flex flex-wrap items-center gap-2">
				{#if article.content_type === 'guide' && article.guide_category}
					<a
						href="/guides?category={article.guide_category}"
						class="rounded-full bg-[var(--color-primary)]/10 px-3 py-1 text-xs font-medium text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/20"
					>
						{article.guide_category_label ?? humanizeKey(article.guide_category)}
					</a>
				{/if}
				{#each journeyChips as chip (chip)}
					<span class="rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1 text-xs text-[var(--color-text-subtle)]">{chip}</span>
				{/each}
			</div>
		{/if}

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
			<span class="flex items-center gap-1"><Eye size={14} /> {article.view_count} {t('content.views', 'views')}</span>
		</div>

		{#if article.tags && article.tags.length > 0}
			<div class="mt-4 flex flex-wrap gap-2">
				{#each article.tags as tag (tag)}
					<a href={`/tags/${tag}`} class="rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-primary)]/10 hover:text-[var(--color-primary)]">#{tag}</a>
				{/each}
			</div>
		{/if}

		<!-- §14 guide amendment: category-specific custom fields at a glance. -->
		{#if guideDetails.length > 0}
			<dl class="mt-6 grid gap-x-6 gap-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:grid-cols-2">
				{#each guideDetails as field (field.key)}
					<div class="min-w-0">
						<dt class="text-xs font-medium uppercase text-[var(--color-muted)]">{field.label}</dt>
						<dd class="mt-0.5 truncate text-sm text-[var(--color-text)]" title={field.value}>{field.value}</dd>
					</div>
				{/each}
			</dl>
		{/if}

		<!-- Series navigation (Phase 06.10) — dev.to style part list, same as
		     the blog single; part links stay inside this route family. -->
		{#if article.series_id && article.series_title && article.series_slug}
			<div class="mt-6">
				<BlogSeriesNav
					title={article.series_title}
					slug={article.series_slug}
					total={data.seriesPosts?.length ?? article.series_total ?? 0}
					posts={data.seriesPosts ?? []}
					currentSlug={article.slug}
					currentPart={article.series_part ?? 0}
					basePath={seriesBasePath}
				/>
			</div>
		{/if}

		<!-- Article body: prose TipTap HTML with serif reading font -->
		<div class="prose mt-8" use:hydrateLinkPreviews use:hydrateMentions>
			{@html sanitizedBody}
		</div>

		<!-- Mobile reaction/action bar (fixed at the bottom) -->
		<div class="lg:hidden">
			<ReactionRail
				contentId={article.id}
				voteScore={article.reshare_count}
				commentCount={article.comment_count}
				authed={data.authed}
				bottom
			/>
		</div>

		<!-- Mobile right sidebar: author card + widgets, stacked below the body on small screens. -->
		<div class="mt-8 flex flex-col gap-4 xl:hidden">
			{@render rightSidebar()}
		</div>

		<!-- Owner analytics (Section 40 — GET /content/{id}/analytics). -->
		{#if data.analytics}
			<OwnerStatsCard stats={data.analytics} />
		{/if}

		<!-- Unified comment engine (Section 49) — Reddit-style nested comments. -->
		<CommentThread
			comments={data.comments}
			totalCount={article.comment_count}
			contentId={article.id}
			contentType={article.content_type}
			authed={data.authed}
			currentUserId={data.currentUserId}
			canModerate={data.canModerate}
			loadError={data.commentsLoadError}
			deletedUserIds={deletedUserIds}
		/>

		<!-- Related content (Section 14 — GET /content/{id}/related). -->
		{#if data.relatedContent && data.relatedContent.length > 0}
			<section class="mt-8">
				<h2 class="mb-4 text-lg font-semibold text-[var(--color-text)]">{t('content.related', 'Related')}</h2>
				<div class="grid gap-4 sm:grid-cols-2">
					{#each data.relatedContent as rel (rel.id)}
						<a
							href={contentHref(rel)}
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

	<!-- Right: CreatorCard + admin-managed widgets -->
	<aside class="hidden w-80 shrink-0 xl:block">
		<div class="sticky top-24 flex flex-col gap-4">
			{@render rightSidebar()}
		</div>
	</aside>
</div>
