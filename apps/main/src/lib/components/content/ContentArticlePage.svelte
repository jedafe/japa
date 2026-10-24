<script lang="ts">
	// Shared content single-page layout — used by /content/[slug] (fallback
	// family) and /guide/[slug]. Ref: §14, §49, ADR-058 Exception #1.
	import { page } from '$app/state';
	import { t, i18n } from '$lib/i18n';
	import { ArrowLeft, Clock, Eye, ExternalLink, Pencil, GraduationCap, Maximize2 } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import ReactionRail from '$lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '$lib/components/blog/CreatorCard.svelte';
	import RecommendedContentCard from '$lib/components/content/RecommendedContentCard.svelte';
	import BlogSeriesNav from '$lib/components/blog/BlogSeriesNav.svelte';
	import SidebarWidgetRenderer from '$lib/components/shell/SidebarWidgetRenderer.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import VideoEmbed from '$lib/components/content/VideoEmbed.svelte';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { imgLightbox, openLightbox } from '$lib/lightbox.svelte';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { hydrateVideoEmbeds } from '$lib/tiptap/hydrateVideoEmbeds';
	import { hydrateContentEmbeds } from '$lib/tiptap/hydrateContentEmbeds';
	import { safeJsonLdScript, sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';
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
		recommendedContent: ContentArticle[];
		analytics: ContentAnalytics | null;
		seriesPosts: SeriesPost[];
		jsonLd: unknown;
		deletedUserIds: string[] | null;
		initiallyFollowing: boolean;
		siteSettings?: Record<string, unknown>;
		/** §106 video — the linked video_sources row, loaded by
		 *  /content/[slug]/+page.server.ts when content_type === 'video'. */
		videoSource?: VideoSource | null;
		/** §73 scholarship integration — set by /guide/[slug] when the guide
		 *  is also a scholarship in the finder. */
		scholarship?: { guide_id: string; slug: string; title: string } | null;
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
	// Render dates in the app-selected locale — `undefined` (server/client
	// default locale) diverges between SSR and hydration for non-en users.
	const publishedDate = $derived(new Date(article.published_at || article.created_at).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'long', day: 'numeric' }));
	const title = $derived(`${article.title} — ${t('app.name')}`);
	const description = $derived(article.excerpt ?? article.body.slice(0, 160));
	// Section 26 — self-referencing canonical on the clean content URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(`${page.url.origin}${contentHref(article)}`);
	const allowedIframeHosts = $derived(
		(data.siteSettings?.['content.sanitization.allowed_iframe_hosts'] as string | undefined) ?? undefined
	);
	const sanitizedBody = $derived(sanitizeContentHtml(article.body_html || article.body, allowedIframeHosts));

	// Series part links route into this article's own route family
	// (ADR-058): /guide/{slug} for guides, /content/{slug} for generic types.
	const seriesBasePath = $derived(
		contentHref(article).split('/').slice(0, -1).join('/') || '/content'
	);

	// BreadcrumbList JSON-LD (Section 26 — all pages with hierarchy). The
	// middle crumb is the content type's public index when one exists.
	const TYPE_HUBS: Record<string, { name: string; path: string }> = {
		guide: { name: 'Guides', path: '/guides' },
		list: { name: 'Lists', path: '/lists' },
		blog_post: { name: 'Blog', path: '/blog' },
		story: { name: 'Stories', path: '/stories' },
		comparison: { name: 'Comparisons', path: '/comparisons' },
		news: { name: 'Explore', path: '/explore' },
		video: { name: 'Explore', path: '/explore' }
	};
	// The visible back link and the BreadcrumbList share the same hub so the
	// on-page affordance and the JSON-LD never disagree (e.g. a guide used to
	// breadcrumb to /guides but link "Back to explore" → /explore).
	const typeHub = $derived(TYPE_HUBS[article.content_type ?? ''] ?? { name: 'Explore', path: '/explore' });
	const breadcrumbLd = $derived.by(() => {
		const hub = typeHub;
		return breadcrumbJsonLd([
			{ name: 'Home', path: page.url.origin },
			{ name: hub.name, path: `${page.url.origin}${hub.path}` },
			{ name: article.title, path: canonical }
		]);
	});

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
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={article.title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	{#if article.featured_image_url}<meta property="og:image" content={article.featured_image_url} />{/if}
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={article.title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
	{@html safeJsonLdScript(breadcrumbLd)}
</svelte:head>

<!-- ADR-058 Exception #1: dev.to reading layout (same as blog singles). -->
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
				user={sidebarUser}
				trendingTags={sidebarTrendingTags}
				suggestedSpaces={sidebarSuggestedSpaces}
				enabledLanguages={sidebarLanguages}
			/>
		{/each}
	</div>
{/snippet}

<!--
	Content single pages render the unified shell's left ad rail (Section 59) instead of a
	page-owned left reaction rail. The action bar is now a horizontal rail under the body.
-->
<div class="mx-auto flex max-w-[65rem] gap-8 px-4 py-8 pb-24 sm:px-6 lg:pb-8">
	<!-- Center: article -->
	<article class="min-w-0 flex-1 max-w-[41rem]">
		<div class="mb-8 flex items-center justify-between gap-3">
			<a href={typeHub.path} class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]">
				<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('content.back_to', 'Back to {hub}', { hub: typeHub.name })}
			</a>
			{#if data.canEdit}
				<a
					href={`/studio/create?draft=${article.id}`}
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
			<button
				type="button"
				class="group relative mb-8 block w-full cursor-zoom-in overflow-hidden rounded-[var(--radius-card)]"
				onclick={() => openLightbox([{ id: `hero-${article.id}`, url: article.featured_image_url!, alt_text: article.title }])}
				aria-label={t('lightbox.view_image', 'View image')}
			>
				<img src={article.featured_image_url} alt={article.title} class="aspect-[16/9] w-full object-cover" />
				<span class="absolute end-3 top-3 grid h-8 w-8 place-items-center rounded-full bg-black/55 text-white opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100">
					<Maximize2 size={15} />
				</span>
			</button>
		{/if}

		<h1 class="text-balance text-4xl font-bold leading-tight tracking-tight text-[var(--color-text)] md:text-5xl" style="font-family: var(--font-heading, var(--font-serif));">{article.title}</h1>

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

		{#if data.scholarship}
			<div class="mt-4">
				<a
					href={`/scholarships/${data.scholarship.guide_id}`}
					class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-opacity hover:opacity-90"
				>
					<GraduationCap size={18} />
					{t('scholarships.view_scholarship', 'View scholarship')}
				</a>
			</div>
		{/if}

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
			{#if article.source}
				{@const source = article.source}
				<span class="flex items-center gap-1">
					<EntityHoverChip
						type="source"
						id={source.id}
						slug={source.slug}
						name={source.name}
						avatar={source.logo_url}
						class="inline-flex items-center gap-1 font-medium text-[var(--color-primary)] hover:underline"
					>
						{#snippet trigger()}
							{t('content.source', 'Source')}: {source.name}
						{/snippet}
					</EntityHoverChip>
					{#if source.source_url}
						<a
							href={source.source_url}
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-0.5 text-[var(--color-text-muted)] hover:text-[var(--color-primary)]"
							aria-label={t('content.original_source', 'View original source')}
						>
							<ExternalLink class="rtl:-scale-x-100" size={12} />
						</a>
					{/if}
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

		<!-- §14 guide amendment: category-specific custom fields at a glance. -->
		{#if guideDetails.length > 0}
			<dl class="mt-6 grid grid-cols-1 gap-x-6 gap-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:grid-cols-2">
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
		<div class="prose mt-8" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds use:hydrateContentEmbeds use:imgLightbox>
			{@html sanitizedBody}
		</div>

		<!-- Reaction/action bar: fixed bottom bar on mobile (the routes using
		     this are ADR-058 exceptions, so the shell's mobile tab bar is
		     absent), falling back to the horizontal strip under the body on sm+. -->
		<div class="mt-8">
			<ReactionRail
				contentId={article.id}
				voteScore={article.reaction_count}
				commentCount={article.comment_count}
				authed={data.authed}
				bottom
			/>
		</div>

		<!-- Mobile right sidebar: author card + widgets, stacked below the body on small screens. -->
		<div class="mt-8 flex flex-col gap-4 xl:hidden">
			{@render rightSidebar()}
		</div>

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

		<!-- Same-type recommendations, ranked by embedding similarity. -->
		<div class="mt-8">
			<RecommendedContentCard items={data.recommendedContent ?? []} contentType={article.content_type} />
		</div>

		<!-- Related content (Section 14 — GET /content/{id}/related). -->
		{#if data.relatedContent && data.relatedContent.length > 0}
			<section class="mt-8">
				<h2 class="mb-4 text-lg font-semibold text-[var(--color-text)]">{t('content.related', 'Related')}</h2>
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
