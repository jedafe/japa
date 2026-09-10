<script lang="ts">
	import { getFreshness } from '$lib/api/jce';
	import { t } from '$lib/i18n';
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
	import OwnerStatsCard from '$lib/components/content/OwnerStatsCard.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import ReactionRail from '$lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '$lib/components/blog/CreatorCard.svelte';
	import SidebarWidgetRenderer from '$lib/components/shell/SidebarWidgetRenderer.svelte';
	import { ArrowLeft, ExternalLink, Pencil } from 'lucide-svelte';

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
</script>

<!-- ADR-058 Exception #1: dev.to reading layout. Left nav auto-collapses;
     right sidebar is CreatorCard + widgets; on mobile it is stacked below the body. -->
{#snippet rightSidebar()}
	<div class="flex flex-col gap-4">
		<CreatorCard
			{authorName}
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

<div class="mx-auto grid max-w-full gap-8 px-4 py-8 pb-24 lg:pb-8 lg:grid-cols-[4rem_1fr] xl:grid-cols-[4rem_1fr_20rem]">
	<!-- Left margin: borderless ReactionRail (distinct from the collapsed nav) -->
	<div class="hidden w-14 shrink-0 lg:block">
		<ReactionRail
			contentId={comparison.content.id}
			commentCount={comparison.content.comment_count}
			voteScore={comparison.content.reshare_count}
			authed={data.authed}
		/>
	</div>

	<!-- Center: comparison -->
	<article class="min-w-0 max-w-5xl">
		<div class="space-y-1">
		<a
			href="/comparisons"
			class="inline-flex items-center gap-1 text-sm transition-colors hover:opacity-80"
			style="color: var(--color-muted)"
		>
			<ArrowLeft size={14} /> {t('jce.comparisons.back')}
		</a>
		<div class="flex flex-wrap items-start justify-between gap-3">
			<h1 class="text-2xl sm:text-3xl font-bold" style="color: var(--color-text)">{comparison.content.title}</h1>
			{#if data.canEdit}
				<a
					href="/create?type=comparison&draft={comparison.content.id}"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
				>
					<Pencil size={14} /> {t('common.edit', 'Edit')}
				</a>
			{/if}
		</div>
		<p class="text-sm capitalize" style="color: var(--color-muted)">
			{comparison.metadata.depth} · {comparison.content.status} · {comparison.content.visibility}
		</p>
	</div>

	{#if comparison.content.featured_image_url}
		<img
			src={comparison.content.featured_image_url}
			alt={comparison.content.title}
			class="aspect-[16/9] w-full rounded-[var(--radius-card)] object-cover"
		/>
	{/if}

	{#if comparison.content.body_html}
		<!-- body_html is sanitized by the content service (SanitizeHTML) before storage. Ref: Rule 12, Section 30. -->
		<article
			class="prose prose-invert max-w-none rounded-[var(--radius-card)] border p-4"
			style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
		>
			{@html comparison.content.body_html}
		</article>
	{:else if comparison.content.body}
		<article
			class="prose prose-invert max-w-none rounded-[var(--radius-card)] border p-4"
			style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
		>
			{comparison.content.body}
		</article>
	{/if}

	<!-- Owner analytics (Section 40 — GET /content/{id}/analytics). -->
	{#if data.analytics}
		<OwnerStatsCard stats={data.analytics} />
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
		</section>
	{/if}

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

	<div class="flex gap-4">
		<a href="/comparisons" class="text-sm underline" style="color: var(--color-text)">← {t('jce.comparisons.back')}</a>
		<a href="/compare" class="text-sm underline" style="color: var(--color-muted)">{t('jce.comparisons.create_your_own')}</a>
	</div>

	{#if user && features['feature.jce.ai_advisor']}
		<AIAdvisorDrawer
			bind:open={advisorOpen}
			comparisonID={comparison.content.id}
			items={comparison.items ?? []}
		/>
	{/if}

	<!-- Mobile right sidebar: author card + widgets, stacked below the body on small screens. -->
	<div class="mt-8 flex flex-col gap-4 xl:hidden">
		{@render rightSidebar()}
	</div>

	<!-- Mobile reaction/action bar (fixed at the bottom) -->
	<div class="lg:hidden">
		<ReactionRail
			contentId={comparison.content.id}
			commentCount={comparison.content.comment_count}
			voteScore={comparison.content.reshare_count}
			authed={data.authed}
			bottom
		/>
	</div>
</article>

<!-- Right: CreatorCard + widgets. Hidden on mobile because a stacked copy
     is rendered inside the article body. -->
<aside class="hidden w-full min-w-0 xl:col-span-1 xl:col-start-3 xl:block xl:w-80 xl:shrink-0">
	<div class="sticky top-24 flex flex-col gap-4">
		{@render rightSidebar()}
	</div>
</aside>
</div>
