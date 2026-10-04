<script lang="ts">
	import { getFreshness } from '$lib/api/jce';
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import {
		deduplicateMatrix,
		uniqueDimensions,
		uniqueResolvedItems
	} from '$lib/utils/comparison';
	import type {
		FreshnessCell,
		FreshnessSummary,
		JCEComparisonDetail,
		JCEItem,
		JCEDimension,
		MatrixDimension,
		MatrixItem,
		ResolvedItem
	} from '$lib/types/jce';
	import type { PageData } from './$types';
	import FreshnessChip from '$lib/components/compare/FreshnessChip.svelte';
	import RefreshButton from '$lib/components/compare/RefreshButton.svelte';
	import AIAdvisorDrawer from '$lib/components/compare/AIAdvisorDrawer.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import ReactionRail from '$lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '$lib/components/blog/CreatorCard.svelte';
	import RecommendedContentCard from '$lib/components/content/RecommendedContentCard.svelte';
	import SidebarWidgetRenderer from '$lib/components/shell/SidebarWidgetRenderer.svelte';
	import { ArrowLeft, Clock, Eye, ExternalLink, Pencil } from 'lucide-svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { hydrateVideoEmbeds } from '$lib/tiptap/hydrateVideoEmbeds';
	import { hydrateContentEmbeds } from '$lib/tiptap/hydrateContentEmbeds';
	import { imgLightbox } from '$lib/lightbox.svelte';

	let { data }: { data: PageData } = $props();

	let comparison = $derived(data.comparison);
	let matrix = $derived(deduplicateMatrix(data.matrix ?? []));
	let uniqueDims = $derived(uniqueDimensions(comparison.dimensions ?? []));
	let resolvedItems = $derived(uniqueResolvedItems(data.resolvedItems ?? []));
	let user = $derived(data.user ?? null);
	let features = $derived((data.features as Record<string, boolean> | undefined) ?? {});
	let deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds as string[]) : undefined
	);
	let authorName = $derived(
		comparison.content.author_display_name ??
			comparison.content.author_name ??
			t('jce.comparisons.anonymous', 'Anonymous')
	);
	let publishedDate = $derived(
		new Date(
			comparison.content.published_at ?? comparison.content.created_at ?? Date.now()
		).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' })
	);
	let sidebarWidgets = $derived(
		(data.rightSidebarWidgets as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number }[]) ??
			[]
	);

	let freshness = $state<FreshnessSummary | null>(null);
	let advisorOpen = $state(false);

	$effect(() => {
		if (comparison?.content?.id && user && features['feature.jce.refresh']) {
			getFreshness(comparison.content.id).then((f) => {
				freshness = f;
			});
		}
	});

	// View tracking (Section 14 — POST /content/{id}/view). Fire-and-forget
	// on mount; the backend deduplicates per user/session per day.
	$effect(() => {
		if (!comparison?.content?.id) return;
		api.post(`/content/${comparison.content.id}/view`, {});
	});

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}

	function findCell(itemId: string, dimId: string): FreshnessCell | undefined {
		const item = freshness?.by_item.find((i) => i.item_id === itemId);
		return item?.cells.find((c) => c.dimension_id === dimId);
	}

	function formatValue(cell?: MatrixDimension) {
		if (!cell) return '—';
		if (cell.value !== undefined && cell.value !== null) return cell.value;
		if (cell.value_rating !== undefined && cell.value_rating !== null) return cell.value_rating;
		if (cell.value_boolean !== undefined && cell.value_boolean !== null)
			return cell.value_boolean ? t('jce.common.yes') : t('jce.common.no');
		return '—';
	}

	const headTitle = $derived(`${comparison.content.title} — ${t('jce.comparisons.title', 'Comparisons')}`);
	const headDescription = $derived(
		(comparison.content.body ?? '').replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 160)
	);
	const sanitizedBody = $derived(
		sanitizeContentHtml(comparison.content.body_html ?? comparison.content.body ?? '')
	);
</script>

<svelte:head>
	<title>{headTitle}</title>
	{#if headDescription}<meta name="description" content={headDescription} />{/if}
	<meta property="og:title" content={comparison.content.title} />
	{#if headDescription}<meta property="og:description" content={headDescription} />{/if}
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{#if comparison.content.featured_image_url}<meta property="og:image" content={comparison.content.featured_image_url} />{/if}
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={comparison.content.title} />
	{#if headDescription}<meta name="twitter:description" content={headDescription} />{/if}
</svelte:head>

<!-- ADR-058 Exception #1: dev.to reading layout. Left nav auto-collapses;
     right sidebar is CreatorCard + widgets; on mobile it is stacked below the body. -->
{#snippet rightSidebar()}
	<div class="flex flex-col gap-4">
		<CreatorCard
			{authorName}
			authorUsername={comparison.content.author_username}
			authorAvatarUrl={comparison.content.author_avatar_url}
			spaceName={comparison.content.space_name}
			spaceSlug={comparison.content.space_slug}
			authorId={comparison.content.author_id}
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
<div class="mx-auto flex max-w-[65rem] gap-8 py-8 pb-24 lg:pb-8">
	<!-- Center: comparison -->
	<article class="min-w-0 flex-1 max-w-[41rem]">
		<div class="mb-8 flex flex-wrap items-center justify-between gap-3">
			<a
				href="/comparisons"
				class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			>
				<ArrowLeft size={16} /> {t('jce.comparisons.back')}
			</a>
			<div class="flex flex-wrap items-center gap-2">
				{#if data.canEdit}
					<a
						href="/create?type=comparison&draft={comparison.content.id}"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
					>
						<Pencil size={14} /> {t('common.edit', 'Edit')}
					</a>
				{/if}
			</div>
		</div>

		{#if comparison.content.featured_image_url}
			<img
				src={comparison.content.featured_image_url}
				alt={comparison.content.title}
				class="mb-8 aspect-[16/9] w-full rounded-[var(--radius-card)] object-cover"
			/>
		{/if}

		<h1
			class="text-balance text-4xl font-bold leading-tight tracking-tight text-[var(--color-text)] md:text-5xl"
			style="font-family: var(--font-serif)"
		>
			{comparison.content.title}
		</h1>

		<div class="mt-6 flex flex-wrap items-center gap-4 border-b border-[var(--color-border)] pb-6 text-sm text-[var(--color-text-subtle)]">
			{#if authorName}
				<UserHoverCard
					id={comparison.content.author_id ?? ''}
					slug={comparison.content.author_username}
					name={authorName}
					avatar={comparison.content.author_avatar_url}
					variant="row"
				>
					{#snippet trigger()}
						{#if comparison.content.author_avatar_url}
							<img src={comparison.content.author_avatar_url} alt="" class="h-9 w-9 rounded-full object-cover" />
						{:else}
							<span class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-bold text-[var(--color-primary)]">{authorName.charAt(0)}</span>
						{/if}
						<span class="font-medium text-[var(--color-text)]">{authorName}</span>
					{/snippet}
				</UserHoverCard>
			{/if}
			<span>·</span>
			<span>{publishedDate}</span>
			{#if comparison.content.read_time_minutes && comparison.content.read_time_minutes > 0}
				<span class="flex items-center gap-1"><Clock size={14} /> {comparison.content.read_time_minutes} {t('blog.min_read', 'min read')}</span>
			{/if}
			<span class="flex items-center gap-1"><Eye size={14} /> {comparison.content.view_count} {t('content.views', 'views')}</span>
			<span class="ml-auto capitalize">{comparison.metadata.depth} · {comparison.content.status} · {comparison.content.visibility}</span>
		</div>

	{#if sanitizedBody}
		<!-- body_html is sanitized by the content service (SanitizeHTML) before storage;
		     re-sanitized client-side for parity with the other content singles. -->
		<div class="prose mt-8" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds use:hydrateContentEmbeds use:imgLightbox>
			{@html sanitizedBody}
		</div>
	{/if}

	<section>
		<h2 class="text-lg font-semibold mb-3" style="color: var(--color-text)">{t('jce.compare.items_heading')}</h2>
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each resolvedItems as item (item.id)}
				<div
					class="rounded-[var(--radius-card)] border p-4 space-y-2"
					style="background-color: var(--color-surface); border-color: var(--color-border)"
				>
					<h3 class="font-medium" style="color: var(--color-text)">{item.title}</h3>
					{#if item.description}
						<p class="text-sm" style="color: var(--color-muted)">{item.description}</p>
					{/if}
					{#if item.url}
						<a
							href={item.url}
							target="_blank"
							class="inline-flex items-center gap-1 text-xs underline"
							style="color: var(--color-text)"
						>
							<ExternalLink size={12} /> {t('jce.common.view')}
						</a>
					{/if}
				</div>
			{/each}
		</div>
	</section>

	{#if matrix.length > 0}
		<section>
			<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
				<h2 class="text-lg font-semibold" style="color: var(--color-text)">{t('jce.compare.matrix_heading')}</h2>
				<div class="flex flex-wrap items-center gap-2">
					{#if user && features['feature.jce.refresh']}
						<RefreshButton comparisonID={comparison.content.id} />
					{/if}
					{#if user && features['feature.jce.ai_advisor']}
						<button
							type="button"
							class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
							style="background-color: var(--color-primary, #3b82f6); color: white"
							onclick={() => (advisorOpen = true)}
						>
							{t('jce.advisor.open', 'Ask AI Advisor')}
						</button>
					{/if}
					{#if user && features['feature.jce.refresh'] && freshness}
						<FreshnessChip summary={freshness} />
					{/if}
				</div>
			</div>
			<div class="prose-table-wrapper">
				<table class="text-sm">
					<thead>
						<tr>
							<th style="color: var(--color-text)">{t('jce.compare.matrix_dimension')}</th>
							{#each matrix as item (item.id)}
								<th style="color: var(--color-text)">{item.title}</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each uniqueDims as dim (dim.id)}
							<tr>
								<td class="font-medium" style="color: var(--color-text)">{dim.label}</td>
								{#each matrix as item (item.id)}
									<td style="color: var(--color-muted)">
										<div class="space-y-1">
											<div>{formatValue(cellFor(item, dim))}</div>
											{#if user && features['feature.jce.refresh'] && freshness}
												<FreshnessChip cell={findCell(item.id, dim.id)} />
											{/if}
										</div>
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			{#if user && features['feature.jce.ai_advisor']}
				<AIAdvisorDrawer
					bind:open={advisorOpen}
					comparisonID={comparison.content.id}
					items={comparison.items ?? []}
				/>
			{/if}
		</section>
	{/if}

	<!-- Horizontal reaction/action bar under the article body. -->
	<div class="mt-8">
		<ReactionRail
			contentId={comparison.content.id}
			commentCount={comparison.content.comment_count}
			authed={data.authed}
			horizontal
		/>
	</div>

	<!-- Mobile right sidebar: author card + widgets, stacked below the body on small screens. -->
	<div class="mt-8 flex flex-col gap-4 xl:hidden">
		{@render rightSidebar()}
	</div>

	<!-- Unified comment engine (Section 49) — top-level comments are SSR-seeded. -->
	<CommentThread
		comments={data.comments}
		totalCount={comparison.content.comment_count}
		contentId={comparison.content.id}
		contentType="comparison"
		authed={data.authed}
		currentUserId={data.currentUserId}
		canModerate={data.canModerate}
		loadError={data.commentsLoadError}
		deletedUserIds={deletedUserIds}
	/>

	<!-- Same-type recommendations, ranked by embedding similarity. -->
	<div class="mt-8">
		<RecommendedContentCard items={data.recommendedContent ?? []} contentType="comparison" />
	</div>

	<div class="mt-6 flex gap-4">
		<a href="/comparisons" class="text-sm underline" style="color: var(--color-text)">← {t('jce.comparisons.back')}</a>
		<a href="/compare" class="text-sm underline" style="color: var(--color-muted)">{t('jce.comparisons.create_your_own')}</a>
	</div>
</article>

<!-- Right: CreatorCard + widgets. Hidden on mobile because a stacked copy
     is rendered inside the article body. -->
<aside class="hidden w-80 shrink-0 xl:block">
	<div class="sticky top-24 flex flex-col gap-4">
		{@render rightSidebar()}
	</div>
</aside>
</div>
