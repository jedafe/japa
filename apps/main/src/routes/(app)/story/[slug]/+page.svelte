<script lang="ts">
	// Ref: japaconnect-engineering/sections/79-community-story-wall.md
	//      ADR-058 Exception #1 (dev.to reading layout — CPT singles)
	import { t, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { relativeTime } from '$lib/feed/time';
	import { ArrowLeft, BadgeCheck, MapPin, Calendar, CreditCard, Briefcase, Lightbulb, BookMarked, Quote, Pencil, Trash2, Loader2, Eye, ExternalLink, Maximize2 } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ReactionRail from '$lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '$lib/components/blog/CreatorCard.svelte';
	import RecommendedContentCard from '$lib/components/content/RecommendedContentCard.svelte';
	import SidebarWidgetRenderer from '$lib/components/shell/SidebarWidgetRenderer.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import BlogSeriesNav from '$lib/components/blog/BlogSeriesNav.svelte';
	import InArticleAd from '$lib/components/ads/InArticleAd.svelte';
	import NextActionInline from '$lib/components/shell/NextActionInline.svelte';
	import AdSlot from '$lib/components/ui/AdSlot.svelte';
	import { safeJsonLdScript, sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';
	import { contentHref } from '$lib/utils/contentLink';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { hydrateVideoEmbeds } from '$lib/tiptap/hydrateVideoEmbeds';
	import { imgLightbox, openLightbox } from '$lib/lightbox.svelte';
	import { parseMostUsefulResource } from '$lib/utils/storyResources';
	import type { PageData } from './$types';
	import type { SessionUser, TrendingTag, SpaceSuggestion } from '$lib/types';

	let { data }: { data: PageData } = $props();

	const story = $derived(data.story);
	const isAuthor = $derived(!!story && !!data.currentUserId && story.author_id === data.currentUserId);
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds as string[]) : undefined
	);

	// Normalize sidebar widgets to the typed shape expected by SidebarWidgetRenderer.
	const sidebarWidgets = $derived(
		((data.rightSidebarWidgets ?? []) as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number }[]).filter(
			(w) => w && typeof w.widget_type === 'string'
		)
	);

	// Section 59 widget data — provided by the root layout (user) and the
	// (app) layout (trending tags / suggested spaces).
	const sidebarUser = $derived((page.data.user as SessionUser | null | undefined) ?? null);
	const sidebarTrendingTags = $derived((page.data.trendingTags as TrendingTag[] | undefined) ?? []);
	const sidebarSuggestedSpaces = $derived((page.data.suggestedSpaces as SpaceSuggestion[] | undefined) ?? []);
	const sidebarLanguages = $derived(
		(page.data.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? 'en,fr'
	);

	let deleting = $state(false);
	let showDeleteConfirm = $state(false);

	async function confirmDelete() {
		if (!story) return;
		deleting = true;
		try {
			const res = await api.delete(`/stories/${story.id}`);
			if (res.error === null) {
				deleting = false;
				showDeleteConfirm = false;
				goto('/stories');
			} else {
				deleting = false;
				showDeleteConfirm = false;
			}
		} catch {
			// stay on page — the error is transient
			deleting = false;
			showDeleteConfirm = false;
		}
	}

	const title = $derived(story ? `${story.title} — ${t('stories.heading', 'Community Stories')}` : t('stories.heading', 'Community Stories'));
	const description = $derived(story?.biggest_challenge ?? story?.title ?? '');
	// Section 26 — self-referencing canonical on the clean slug URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(story ? `${page.url.origin}/story/${story.slug}` : `${page.url.origin}/stories`);
	const publishedDate = $derived(
		story
			? new Date(story.published_at || story.created_at).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'long', day: 'numeric' })
			: ''
	);
	const allowedIframeHosts = $derived(
		(data.siteSettings?.['content.sanitization.allowed_iframe_hosts'] as string | undefined) ?? undefined
	);
	// Render the sanitized TipTap HTML like every other CPT single — the raw
	// Markdown body is the editing source, not display output.
	const sanitizedBody = $derived(story ? sanitizeContentHtml(story.body_html || story.body, allowedIframeHosts) : '');

	const jsonLd = $derived(
		story
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Article',
					headline: story.title,
					datePublished: story.published_at || story.created_at,
					author: { '@type': 'Person', name: story.author_name },
					url: canonical,
					articleBody: story.body,
					...(story?.destination_space_name ? { about: { '@type': 'Country', name: story.destination_space_name } } : {}),
					...(story?.biggest_challenge ? { description: story.biggest_challenge } : {})
				})
			: ''
	);

	function metaRow(icon: typeof MapPin, label: string, value: string | null | undefined) {
		return { icon, label, value, show: !!value };
	}

	const metaRows = $derived(
		story
			? [
					metaRow(MapPin, t('stories.origin', 'From'), story.origin_space_name),
					metaRow(MapPin, t('stories.destination', 'To'), story.destination_space_name),
					metaRow(Calendar, t('stories.relocation_year', 'Relocation year'), story.relocation_year ? String(story.relocation_year) : null),
					metaRow(CreditCard, t('stories.visa_category', 'Visa category'), story.visa_category),
					metaRow(Briefcase, t('stories.profession', 'Profession'), story.profession)
				].filter((r) => r.show)
			: []
	);

	// Most Useful Resources are stored as a single joined string (Name — Link; …)
	// so they can be edited through the text column while the composer stores a
	// repeater (§79). Parse into display-ready title/URL pairs for the single page.
	const usefulResources = $derived(parseMostUsefulResource(story?.most_useful_resource));

	// §79 "Integration points" — surface related tools for the story's
	// destination (visa routes, community locator, processing tracker). All
	// three accept a ?dest= country space ID.
	const relatedTools = $derived.by(() => {
		if (!story?.destination_space_id) return [] as { href: string; label: string }[];
		const dest = encodeURIComponent(story.destination_space_id);
		const params = new URLSearchParams({ dest: story.destination_space_id });
		if (story.origin_space_id) params.set('origin', story.origin_space_id);
		if (story.visa_category) params.set('visa', story.visa_category);
		return [
			{ href: `/tools/visa-routes?dest=${dest}`, label: t('tools.visa_routes', 'Visa Route Comparison') },
			{ href: `/tools/community-locator?dest=${dest}`, label: t('tools.community_locator', 'Diaspora Community Locator') },
			{ href: `/tools/processing-tracker?${params.toString()}`, label: t('tools.processing_tracker', 'Visa Processing Time Tracker') }
		];
	});

	// View tracking (Section 14 — POST /content/{id}/view). Fire-and-forget
	// on mount; the backend deduplicates per user/session per day.
	$effect(() => {
		if (!story?.id) return;
		api.post(`/content/${story.id}/view`, {});
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={story?.title ?? title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={story?.title ?? title} />
	<meta name="twitter:description" content={description} />
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
	{#if story}
		{@html safeJsonLdScript(
			breadcrumbJsonLd([
				{ name: 'Home', path: page.url.origin },
				{ name: 'Stories', path: `${page.url.origin}/stories` },
				{ name: story.title, path: canonical }
			])
		)}
	{/if}
</svelte:head>

{#if data.loadError || !story}
	<div class="mx-auto w-full max-w-3xl p-4">
		<EmptyState
			icon={BookMarked}
			title={t('stories.load_error', 'Could not load story')}
			body={t('stories.load_error_body', 'Please try again later.')}
			action={{ label: t('stories.back_to_wall', 'Back to stories'), href: '/stories' }}
		/>
	</div>
{:else}
	<!-- ADR-058 Exception #1: dev.to reading layout (same as /blog/[slug] and
	     /content/[slug]). Left nav is auto-collapsed by the layout. -->
	{#snippet rightSidebar()}
		<div class="flex flex-col gap-4">
			<CreatorCard
				authorName={story.author_display_name ?? story.author_name}
				authorUsername={story.author_username}
				authorAvatarUrl={story.author_avatar}
				spaceName={story.destination_space_name}
				authorId={story.author_id}
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
		<!-- Center: story -->
		<article class="min-w-0 flex-1 max-w-[41rem]">
			<div class="mb-8 flex items-center justify-between gap-3">
				<a href="/stories" class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]">
					<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('stories.back_to_wall', 'Back to stories')}
				</a>
				{#if isAuthor || data.canEdit}
					<div class="flex shrink-0 items-center gap-1">
						<a
							href="/studio/create?draft={story.id}&type=story"
							class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
						>
							<Pencil size={14} /> {t('stories.edit', 'Edit')}
						</a>
						{#if isAuthor}
							<button
								type="button"
								onclick={() => (showDeleteConfirm = true)}
								class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-xs font-medium text-red-500 transition-colors hover:bg-red-500/10"
							>
								<Trash2 size={14} /> {t('stories.delete', 'Delete')}
							</button>
						{/if}
					</div>
				{/if}
			</div>

			{#if story.status !== 'published'}
				<div class="mb-6 rounded-[var(--radius)] border border-[var(--color-warning)]/40 bg-[var(--color-warning)]/10 px-4 py-3 text-sm text-[var(--color-text)]" role="status">
					{t('stories.status_pending', 'This story is awaiting moderation and is only visible to you and moderators.')}
				</div>
			{/if}

			<div class="flex items-start gap-2">
				<h1 class="text-balance text-4xl font-bold leading-tight tracking-tight text-[var(--color-text)] md:text-5xl" style="font-family: var(--font-heading, var(--font-serif));">{story.title}</h1>
				{#if story.verified}
					<BadgeCheck size={22} class="mt-2 shrink-0 text-[var(--color-primary)]" />
				{/if}
			</div>

			{#if story.featured_image_url}
				<button
					type="button"
					class="group relative mt-6 block w-full cursor-zoom-in overflow-hidden rounded-[var(--radius-card)]"
					onclick={() => openLightbox([{ id: `hero-${story.id}`, url: story.featured_image_url!, alt_text: story.title }])}
					aria-label={t('lightbox.view_image', 'View image')}
				>
					<img src={story.featured_image_url} alt={story.title} class="aspect-[16/9] w-full object-cover" />
					<span class="absolute end-3 top-3 grid h-8 w-8 place-items-center rounded-full bg-black/55 text-white opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100">
						<Maximize2 size={15} />
					</span>
				</button>
			{/if}

			<!-- Author + date + views -->
			<div class="mt-6 flex flex-wrap items-center gap-4 border-b border-[var(--color-border)] pb-6 text-sm text-[var(--color-text-subtle)]">
				{#if story.author_name}
					{@const authorDisplay = story.author_display_name ?? story.author_name}
					<UserHoverCard
						id={story.author_id ?? ''}
						slug={story.author_username}
						name={authorDisplay}
						avatar={story.author_avatar}
						variant="row"
					>
						{#snippet trigger()}
							{#if story.author_avatar}
								<img src={story.author_avatar} alt="" class="h-9 w-9 rounded-full object-cover" />
							{:else}
								<span class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-bold text-[var(--color-primary)]">{authorDisplay.charAt(0)}</span>
							{/if}
							<span class="font-medium text-[var(--color-text)]">{authorDisplay}</span>
						{/snippet}
					</UserHoverCard>
					<span>·</span>
				{/if}
				<span>{publishedDate || relativeTime(story.created_at, new Date(), i18n.locale)}</span>
				<span class="flex items-center gap-1"><Eye size={14} /> {story.view_count} {t('content.views', 'views')}</span>
			</div>

			<!-- Story meta grid -->
			{#if metaRows.length > 0}
				<dl class="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
					{#each metaRows as row (row.label)}
						<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2">
							<row.icon size={16} class="shrink-0 text-[var(--color-muted)]" />
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{row.label}</dt>
								<dd class="truncate text-sm font-medium text-[var(--color-text)]">{row.value}</dd>
							</div>
						</div>
					{/each}
				</dl>
			{/if}

			<!-- Body: sanitized TipTap HTML with prose styling.
			     InArticleAd injects the article_inline slot between blocks (§25). -->
			{#if sanitizedBody}
				<div class="prose mt-8" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds use:imgLightbox>
					<NextActionInline />
			<InArticleAd html={sanitizedBody} />
				</div>
			{/if}

			<!-- Highlighted story fields -->
			<div class="mt-8 space-y-4">
				{#if story?.biggest_challenge}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
							<Lightbulb size={16} class="text-[var(--color-primary)]" />
							{t('stories.biggest_challenge', 'Biggest Challenge')}
						</h2>
						<p class="mt-2 whitespace-pre-wrap text-sm text-[var(--color-text-subtle)]">{story.biggest_challenge}</p>
					</div>
				{/if}

				{#if usefulResources.length > 0}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
							<BookMarked size={16} class="text-[var(--color-primary)]" />
							{t('stories.most_useful_resource', 'Most Useful Resource')}
						</h2>
						<ul class="mt-2 space-y-2">
							{#each usefulResources as resource (resource.name + resource.url)}
								<li class="text-sm">
									{#if resource.url}
										<a
											href={resource.url}
											target="_blank"
											rel="noopener noreferrer"
											class="inline-flex items-center gap-1.5 text-[var(--color-primary)] transition-colors hover:underline"
										>
											{resource.name}
											<ExternalLink class="rtl:-scale-x-100" size={12} />
										</a>
									{:else}
										<span class="text-[var(--color-text-subtle)]">{resource.name}</span>
									{/if}
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if story?.best_advice}
					<div class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/5 p-4">
						<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
							<Quote size={16} class="text-[var(--color-primary)]" />
							{t('stories.best_advice', 'Best Advice')}
						</h2>
						<p class="mt-2 whitespace-pre-wrap text-sm text-[var(--color-text-subtle)]">{story.best_advice}</p>
					</div>
				{/if}
			</div>

			<!-- §79 integration points: related tools for the story's destination -->
			{#if relatedTools.length > 0}
				<nav class="mt-8 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<p class="mb-3 text-xs font-medium uppercase text-[var(--color-muted)]">
						{t('stories.related_tools', 'Plan a similar move')}
						{#if story.destination_space_name}· {story.destination_space_name}{/if}
					</p>
					<div class="flex flex-wrap gap-2">
						{#each relatedTools as tool (tool.href)}
							<a
								href={tool.href}
								class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
							>
								<ExternalLink class="rtl:-scale-x-100" size={12} /> {tool.label}
							</a>
						{/each}
					</div>
				</nav>
			{/if}

			<!-- Reaction/action bar: fixed bottom bar on mobile (this route is an
			     ADR-058 exception, so the shell's mobile tab bar is absent), falling
			     back to the horizontal strip under the body on sm+. -->
			<div class="mt-8">
			<ReactionRail
				contentId={story.id}
				commentCount={story.comment_count}
				authed={data.authed}
				bottom
			/>
			</div>

			<!-- Mobile right sidebar: author card + widgets, stacked below the body on small screens. -->
			<div class="mt-8 flex flex-col gap-4 xl:hidden">
				{@render rightSidebar()}
			</div>

			<!-- Series navigation (§14) — same dev.to-style card as the blog
			     single; part links stay in the /stories route family. -->
			{#if story.series_id && story.series_title && story.series_slug}
				<div class="mt-8">
					<BlogSeriesNav
						title={story.series_title}
						slug={story.series_slug}
						total={data.seriesPosts?.length ?? story.series_total ?? 0}
						posts={data.seriesPosts ?? []}
						currentSlug={story.slug}
						currentPart={story.series_part ?? 0}
						basePath="/story"
					/>
				</div>
			{/if}

			<!-- Unified comment engine (Section 49) — Reddit-style nested comments. -->
			<CommentThread
				comments={data.comments}
				commentsTotal={data.commentsTotal}
				totalCount={story.comment_count}
				contentId={story.id}
				contentType="story"
				authed={data.authed}
				currentUserId={data.currentUserId}
				canModerate={data.canModerate}
				loadError={data.commentsLoadError}
				deletedUserIds={deletedUserIds}
			/>

			<!-- §25: engagement-area ad slot. -->
			<AdSlot slot="comments_below" format="banner" />

			<!-- Same-type recommendations, ranked by embedding similarity. -->
			<div class="mt-8">
				<RecommendedContentCard items={data.recommendedContent ?? []} contentType="story" />
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

	{#if showDeleteConfirm}
		<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
			<div class="w-full max-w-md rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
				<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('stories.delete_confirm_title', 'Delete your story?')}</h2>
				<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{t('stories.delete_confirm_body', 'This will permanently remove your story. This action cannot be undone.')}</p>
				<div class="mt-4 flex justify-end gap-2">
					<button
						type="button"
						onclick={() => (showDeleteConfirm = false)}
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						{t('common.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						onclick={confirmDelete}
						disabled={deleting}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-red-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
					>
						{#if deleting}
							<Loader2 size={16} class="animate-spin" />
							{t('stories.deleting', 'Deleting…')}
						{:else}
							<Trash2 size={16} />
							{t('stories.delete', 'Delete')}
						{/if}
					</button>
				</div>
			</div>
		</div>
	{/if}
{/if}
