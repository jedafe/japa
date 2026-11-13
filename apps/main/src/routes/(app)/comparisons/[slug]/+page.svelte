<script lang="ts">
	import { getFreshness } from '#lib/api/jce.js';
	import { i18n, t } from '#lib/i18n/index.js';
	import UserHoverCard from '#lib/components/ui/UserHoverCard.svelte';
	import {
		deduplicateMatrix,
		groupDimensionsByPool,
		matrixBestCells,
		uniqueDimensions,
		uniqueResolvedItems,
		type DimensionGroup
	} from '#lib/utils/comparison.js';
	import { flagUrl } from '#lib/utils/flags.js';
	import type {
		FreshnessSummary,
		JCEComparisonDetail,
		JCEItem,
		JCEDimension,
		MatrixDimension,
		MatrixItem,
		ResolvedItem
	} from '#lib/types/jce.js';
	import type { PageData } from './$types';
	import FreshnessChip from '#lib/components/compare/FreshnessChip.svelte';
	import RefreshButton from '#lib/components/compare/RefreshButton.svelte';
	import EntityDiscussions from '#lib/components/thread/EntityDiscussions.svelte';
	import AdSlot from '#lib/components/ui/AdSlot.svelte';
	import ReactionRail from '#lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '#lib/components/blog/CreatorCard.svelte';
	import RecommendedContentCard from '#lib/components/content/RecommendedContentCard.svelte';
	import SidebarWidgetRenderer from '#lib/components/shell/SidebarWidgetRenderer.svelte';
	import { ArrowLeft, BookOpen, Briefcase, CalendarClock, Check, ChevronDown, CircleDollarSign, Clock, Ellipsis, Eye, ExternalLink, FileText, GraduationCap, LayoutList, Pencil } from 'lucide-svelte';
	import type { Component } from 'svelte';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { api } from '#lib/api/client.js';
	import { safeJsonLdScript, sanitizeContentHtml } from '#lib/utils/safeHtml.js';
	import { breadcrumbJsonLd } from '#lib/seo/breadcrumbs.js';
	import { formatMoney } from '#lib/utils/currency.js';
	import { hydrateLinkPreviews } from '#lib/tiptap/hydrateLinkPreviews.js';
	import { hydrateMentions } from '#lib/tiptap/hydrateMentions.js';
	import { hydrateVideoEmbeds } from '#lib/tiptap/hydrateVideoEmbeds.js';
	import { hydrateContentEmbeds } from '#lib/tiptap/hydrateContentEmbeds.js';
	import { imgLightbox } from '#lib/lightbox.svelte.js';

	import { ogCardUrl, ogMediaUrl } from '#lib/seo/og.js';
	let { data }: { data: PageData } = $props();

	let comparison = $derived(data.comparison);
	let matrix = $derived(deduplicateMatrix(data.matrix ?? []));
	let uniqueDims = $derived(uniqueDimensions(comparison.dimensions ?? []));
	// §30 Sub-spec 6: when the comparison's dimensions span more than one
	// field-pool group, the matrix renders as a single-header accordion —
	// one shared table head, collapsible group sections, only one open at a
	// time. Empty pool → one unlabeled group = plain table.
	let dimGroups = $derived(groupDimensionsByPool(uniqueDims, data.fieldPoolGroups ?? []));
	// "Overview" expanded by default; falls back to the first group. One-time
	// snapshot — manual toggles own the state afterwards.
	let openGroupKey = $state<string | null>(
		untrack(() => dimGroups.find((g) => g.label.toLowerCase() === 'overview')?.key ?? dimGroups[0]?.key ?? null)
	);
	function toggleGroup(key: string) {
		openGroupKey = openGroupKey === key ? null : key;
	}

	// Accordion card visuals keyed by the pool group's semantic category —
	// keyword fallbacks keep unknown/type-specific categories on a generic icon.
	type GroupVisual = { icon: Component; tint: string };
	const asComponent = (icon: unknown): Component => icon as Component;
	const GROUP_VISUALS: [RegExp, GroupVisual][] = [
		[/overview|general/i, { icon: asComponent(LayoutList), tint: '#14b8a6' }],
		[/entry|requirement|admission/i, { icon: asComponent(FileText), tint: '#10b981' }],
		[/tuition|fee|cost|financ|price/i, { icon: asComponent(CircleDollarSign), tint: '#f59e0b' }],
		[/career|outcome|job|employ/i, { icon: asComponent(Briefcase), tint: '#8b5cf6' }],
		[/curriculum|course|academ|program|module/i, { icon: asComponent(BookOpen), tint: '#0ea5e9' }],
		[/universit|school|institut/i, { icon: asComponent(GraduationCap), tint: '#6366f1' }],
		[/deliver|study|schedule|mode|duration|format/i, { icon: asComponent(CalendarClock), tint: '#f43f5e' }]
	];
	const DEFAULT_GROUP_VISUAL: GroupVisual = { icon: asComponent(Ellipsis), tint: '#71717a' };
	function groupVisual(category?: string): GroupVisual {
		if (!category) return DEFAULT_GROUP_VISUAL;
		return GROUP_VISUALS.find(([re]) => re.test(category))?.[1] ?? DEFAULT_GROUP_VISUAL;
	}

	// Country display name for the "City, Country" header subtitle — the API
	// emits an ISO geo_code; Intl.DisplayNames localizes it.
	let regionNames = $derived(new Intl.DisplayNames([i18n.locale || 'en'], { type: 'region' }));
	function countryName(code?: string): string {
		if (!code) return '';
		try {
			return regionNames.of(code.toUpperCase()) ?? '';
		} catch {
			return '';
		}
	}
	let bestCells = $derived(matrixBestCells(matrix, uniqueDims));
	let resolvedItems = $derived(uniqueResolvedItems(data.resolvedItems ?? []));
	let user = $derived(data.user ?? null);
	let features = $derived((data.features as Record<string, boolean> | undefined) ?? {});
	// Discussion preview mounts EntityDiscussions; the full comment engine
	// lives at /thread/{slug}.
	let authorName = $derived(
		comparison.content.author_display_name ??
			comparison.content.author_name ??
			t('jce.comparisons.anonymous', 'Anonymous')
	);
	let publishedDate = $derived(
		new Date(
			comparison.content.published_at ?? comparison.content.created_at ?? Date.now()
		).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'long', day: 'numeric' })
	);
	let sidebarWidgets = $derived(
		(data.rightSidebarWidgets as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number }[]) ??
			[]
	);

	let freshness = $state<FreshnessSummary | null>(null);

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
		api.post(`/content/${comparison.content.id}/view`, {}, { 'x-silent-error': '1' });
	});

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}

	function formatValue(cell?: MatrixDimension) {
		if (!cell) return '—';
		// Currency dimensions carry a normalized rating + ISO code — render
		// through formatMoney like the quick-compare matrix does.
		if (cell.currency_code && cell.value_rating !== undefined && cell.value_rating !== null) {
			return formatMoney(cell.value_rating, cell.currency_code, page.data.currencies);
		}
		if (cell.value !== undefined && cell.value !== null) return cell.value;
		if (cell.value_rating !== undefined && cell.value_rating !== null) return cell.value_rating;
		if (cell.value_boolean !== undefined && cell.value_boolean !== null)
			return cell.value_boolean ? t('jce.common.yes') : t('jce.common.no');
		return '—';
	}

	const headTitle = $derived(`${comparison.content.title} — ${t('jce.comparisons.title', 'Comparisons')}`);
	// Fallback to the title/excerpt when the body is empty so the meta
	// description is never conditionally empty (ResolveMetadata fallback).
	const headDescription = $derived(
		(comparison.content.body_html ?? comparison.content.body ?? '')
			.replace(/<[^>]*>/g, ' ')
			.replace(/\s+/g, ' ')
			.trim()
			.slice(0, 160) || comparison.content.title
	);
	// Section 26 — self-referencing canonical on the clean slug URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	// Section 26 deliberately assigns comparison pages no JSON-LD — there is
	// no clean schema.org fit for this content shape.
	const canonical = $derived(`${page.url.origin}/comparisons/${comparison.content.slug}`);
	const sanitizedBody = $derived(
		sanitizeContentHtml(comparison.content.body_html ?? comparison.content.body ?? '')
	);

	const seoOgImage = $derived(
		ogMediaUrl(page.url.origin, comparison?.content?.featured_image_url) ||
		ogCardUrl(page.url.origin, 'comparison')
	);
</script>

<svelte:head>
	<title>{headTitle}</title>
	{#if headDescription}<meta name="description" content={headDescription} />{/if}
	<meta property="og:title" content={comparison.content.title} />
	{#if headDescription}<meta property="og:description" content={headDescription} />{/if}
	<meta property="og:type" content="article" />
	<link rel="canonical" href={canonical} />
	{#if data.indexable === false}
		<meta name="robots" content="noindex,nofollow" />
	{/if}
	<meta property="og:url" content={canonical} />
	<meta property="og:image" content={seoOgImage} />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={comparison.content.title} />
	<meta name="twitter:image" content={seoOgImage} />
	{#if headDescription}<meta name="twitter:description" content={headDescription} />{/if}
	{@html safeJsonLdScript(
		breadcrumbJsonLd([
			{ name: 'Home', path: page.url.origin },
			{ name: 'Comparisons', path: `${page.url.origin}/comparisons` },
			{ name: comparison.content.title, path: canonical }
		])
	)}
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
<div class="mx-auto flex max-w-[72rem] gap-8 px-4 py-8 pb-24 sm:px-6 lg:pb-8">
	<!-- Center: comparison -->
	<article class="min-w-0 flex-1 max-w-[41rem]">
		<div class="mb-8 flex flex-wrap items-center justify-between gap-3">
			<a
				href="/comparisons"
				class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			>
				<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('jce.comparisons.back')}
			</a>
			<div class="flex flex-wrap items-center gap-2">
				{#if data.canEdit}
					<a
						href="/studio/create?draft={comparison.content.id}&type=comparison"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary-text)]"
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
			style="font-family: var(--font-heading, var(--font-serif))"
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
							<span class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-bold text-[var(--color-primary-text)]">{authorName.charAt(0)}</span>
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
		</div>

	{#if sanitizedBody}
		<!-- body_html is sanitized by the content service (SanitizeHTML) before storage;
		     re-sanitized client-side for parity with the other content singles. -->
		<div class="prose mt-8" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds use:hydrateContentEmbeds use:imgLightbox>
			{@html sanitizedBody}
		</div>
	{/if}

	<section class="mt-8">
		<h2 class="mb-3 text-lg font-semibold text-[var(--color-text)]">{t('jce.compare.items_heading')}</h2>
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each resolvedItems as item (item.id)}
				{@const rawCode = item.metadata?.geo_code ?? item.metadata?.country_code}
				{@const flag = flagUrl(typeof rawCode === 'string' ? rawCode : undefined)}
				<div
					class="space-y-2 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				>
					{#if item.image_url || flag}
						<div class="mb-1 flex items-center">
							{#if item.image_url}
								<img src={item.image_url} alt="" class="h-9 w-9 rounded-full border border-[var(--color-border)] object-cover" />
							{:else if flag}
								<img src={flag} alt="" class="h-6 w-8 rounded-sm border border-[var(--color-border)] object-cover" />
							{/if}
						</div>
					{/if}
					<h3 class="font-medium text-[var(--color-text)]">{item.title}</h3>
					{#if item.description}
						<p class="text-sm text-[var(--color-muted)]">{item.description}</p>
					{/if}
					{#if item.url}
						<a
							href={item.url}
							target="_blank" rel="noopener noreferrer"
							class="inline-flex items-center gap-1 text-xs text-[var(--color-text)] underline"
						>
							<ExternalLink class="rtl:-scale-x-100" size={12} /> {t('jce.common.view')}
						</a>
					{/if}
				</div>
			{/each}
		</div>
	</section>

	{#if matrix.length > 0}
		<section class="mt-8">
			<div class="mb-3 flex flex-wrap items-center justify-end gap-3">
				<div class="flex flex-wrap items-center gap-2">
					{#if user && features['feature.jce.refresh']}
						<RefreshButton comparisonID={comparison.content.id} />
					{/if}
					{#if user && features['feature.jce.refresh'] && freshness}
						<FreshnessChip summary={freshness} />
					{/if}
				</div>
			</div>
			{#if dimGroups.length > 0}
				{#if dimGroups.length > 1}
					<div class="space-y-3">
						{#each dimGroups as group (group.key)}
							{@const open = openGroupKey === group.key}
							{#if open}
								{@render matrixTable(group, true)}
							{:else}
								{@const gv = groupVisual(group.category)}
								<button
									type="button"
									onclick={() => toggleGroup(group.key)}
									aria-expanded="false"
									class="flex w-full items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3.5 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<span
										class="flex h-8 w-8 shrink-0 items-center justify-center rounded-[calc(var(--radius)*0.8)]"
										style="background: color-mix(in srgb, {gv.tint} 14%, transparent); color: {gv.tint}"
									>
										<gv.icon size={15} />
									</span>
									<span class="text-xs font-semibold uppercase tracking-wide text-[var(--color-text)]">
										{group.label || t('jce.compare.family.other', 'Other')}
									</span>
									<span class="ms-auto flex shrink-0 items-center gap-2.5">
										<span class="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
											{t('jce.compare.group_items', '{count} items', { count: group.dims.length })}
										</span>
										<ChevronDown size={15} class="text-[var(--color-muted)]" />
									</span>
								</button>
							{/if}
						{/each}
					</div>
				{:else}
					{@render matrixTable(dimGroups[0], false)}
				{/if}
			{/if}
		</section>
	{/if}

	{#snippet matrixTable(group: DimensionGroup, banded: boolean)}
		{@const gv = groupVisual(group.category)}
		<div
			class="prose-table-wrapper compare-matrix rounded-[var(--radius-card)] border border-[var(--color-border)]"
		>
			<table class="text-sm">
				<thead>
					{#if banded}
						<tr>
							<th
								colspan={matrix.length + 1}
								class="text-start"
								style="padding: 0; background-color: var(--color-surface)"
							>
								<button
									type="button"
									onclick={() => toggleGroup(group.key)}
									aria-expanded="true"
									class="flex w-full items-center gap-3 px-4 py-3 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<span class="sticky start-0 flex min-w-0 items-center gap-3">
										<span
											class="flex h-8 w-8 shrink-0 items-center justify-center rounded-[calc(var(--radius)*0.8)]"
											style="background: color-mix(in srgb, {gv.tint} 14%, transparent); color: {gv.tint}"
										>
											<gv.icon size={15} />
										</span>
										<span class="truncate text-xs font-semibold uppercase tracking-wide text-[var(--color-text)]">
											{group.label || t('jce.compare.family.other', 'Other')}
										</span>
									</span>
									<span class="sticky end-0 ms-auto flex shrink-0 items-center gap-2.5">
										<span class="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
											{t('jce.compare.group_items', '{count} items', { count: group.dims.length })}
										</span>
										<ChevronDown size={15} class="rotate-180 text-[var(--color-muted)]" />
									</span>
								</button>
							</th>
						</tr>
					{/if}
					<tr>
						<th class="text-[var(--color-text)]"><span class="compare-pin compare-pin-head">{t('jce.compare.matrix_dimension')}</span></th>
						{#each matrix as item (item.id)}
							<th class="text-[var(--color-text)]">{@render itemHeader(item)}</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each group.dims as dim (dim.id)}
						{@render dimRow(dim)}
					{/each}
				</tbody>
			</table>
		</div>
	{/snippet}

	{#snippet itemHeader(item: MatrixItem)}
		{@const flag = flagUrl(item.geo_code)}
		{@const subtitle = [...new Set([item.geo_label, countryName(item.geo_code)].filter(Boolean))].join(', ')}
		<div class="flex items-center gap-3 py-1">
			{#if item.avatar_url}
				<img
					src={item.avatar_url}
					alt=""
					class="h-10 w-10 shrink-0 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] object-cover"
				/>
			{/if}
			<div class="min-w-0">
				<div class="font-semibold normal-case tracking-normal">{item.entity_name || item.title}</div>
				{#if subtitle}
					<div class="flex items-center gap-1.5 text-xs font-normal text-[var(--color-muted)]">
						{#if flag}
							<img src={flag} alt="" class="h-3 w-4 shrink-0 rounded-[2px] object-cover" />
						{/if}
						<span class="truncate">{subtitle}</span>
					</div>
				{/if}
			</div>
		</div>
	{/snippet}

	{#snippet dimRow(dim: JCEDimension)}
		<tr>
			<td class="font-medium text-[var(--color-text)]"><span class="compare-pin">{dim.label}</span></td>
			{#each matrix as item (item.id)}
				{@const best = bestCells.has(`${item.id}:${dim.id}`)}
				<td
					data-best={best || undefined}
					class={best
						? 'bg-emerald-500/10 font-semibold text-emerald-600 dark:text-emerald-400'
						: 'text-[var(--color-muted)]'}
				>
					{formatValue(cellFor(item, dim))}
					{#if best}
						<Check size={12} class="inline-block align-[-0.1em]" aria-hidden="true" />
						<span class="sr-only">{t('jce.compare.best_value', 'Best value')}</span>
					{/if}
				</td>
			{/each}
		</tr>
	{/snippet}

	<!-- Reaction/action bar: fixed bottom bar on mobile (this route is an
	     ADR-058 exception, so the shell's mobile tab bar is absent), falling
	     back to the horizontal strip under the body on sm+. -->
	<div class="mt-8">
		<ReactionRail
			contentId={comparison.content.id}
			commentCount={comparison.content.comment_count}
			authed={data.authed}
			bottom
			authorUsername={comparison.content.author_username}
			authorId={comparison.content.author_id}
			authorName={comparison.content.author_display_name ?? comparison.content.author_name}
			commentHref="/thread/{comparison.content.slug}#replies"
		/>
	</div>

	<!-- Mobile right sidebar: author card + widgets, stacked below the body on small screens. -->
	<div class="mt-8 flex flex-col gap-4 xl:hidden">
		{@render rightSidebar()}
	</div>

	<!-- Unified comment engine (Section 49) — top-level comments are SSR-seeded. -->
	<!-- Discussion preview — full engine lives at /thread/{slug}. -->
	<EntityDiscussions entityType="content" entityKey={comparison.content.id} contentSlug={comparison.content.slug} authed={data.authed} />

	<!-- §25: engagement-area ad slot. -->
	<AdSlot slot="comments_below" format="banner" />

	<!-- Same-type recommendations, ranked by embedding similarity. -->
	<div class="mt-8">
		<RecommendedContentCard items={data.recommendedContent ?? []} contentType="comparison" />
	</div>

	<div class="mt-6 flex gap-4">
		<a href="/comparisons" class="text-sm text-[var(--color-text)] underline">← {t('jce.comparisons.back')}</a>
		<a href="/compare" class="text-sm text-[var(--color-muted)] underline">{t('jce.comparisons.create_your_own')}</a>
	</div>
</article>

<!-- Right: CreatorCard + widgets. Hidden on mobile because a stacked copy
     is rendered inside the article body. -->
<aside class="ms-auto hidden w-80 shrink-0 xl:block">
	<div class="sticky top-24 flex flex-col gap-4">
		{@render rightSidebar()}
	</div>
</aside>
</div>

<style>
	/* border-collapse: collapse + sticky cells mis-stack in Chromium —
	   scrolled cells paint through the pinned column. separate + zero
	   spacing keeps the single-row separators while fixing occlusion. */
	.compare-matrix table {
		border-collapse: separate;
		border-spacing: 0;
	}

	/* Give columns a real minimum so the table overflows the card and
	   scrolls horizontally instead of squishing cells to fit. */
	.compare-matrix table :is(td, th) {
		min-width: 12rem;
	}
	.compare-matrix table :is(td, th):first-child {
		min-width: 11rem;
	}

	/* Pinned dimension column — sticky on a child element, not the cell:
	   Chromium under-paints sticky cell backgrounds beneath scrolled
	   siblings, but a positioned child stacks reliably (same mechanism
	   as the band label/pill pins). Cell padding moves onto the pin so
	   it covers the full column. */
	.compare-matrix table :is(td, th):first-child {
		padding: 0;
	}
	.compare-pin {
		display: block;
		position: sticky;
		inset-inline-start: 0;
		z-index: 1;
		height: 100%;
		padding: 0.75rem;
		background: var(--color-surface);
	}
	.compare-pin-head {
		background: var(--color-surface-subtle);
	}
	.compare-matrix tbody tr:nth-child(even) td:first-child .compare-pin {
		background: color-mix(in srgb, rgb(135 135 195) 5%, var(--color-surface));
	}

	/* Horizontal row separators only — vertical cell borders would grid the
	   matrix; the card's own border supplies the frame. */
	.compare-matrix :is(td, th) {
		border: 0;
		border-bottom: 1px solid var(--color-border);
	}
	.compare-matrix tbody:last-child tr:last-child :is(td, th) {
		border-bottom: 0;
	}

	/* The wrapper's ::after scroll-fade is absolutely positioned inside the
	   scroller, so it scrolls with the content and lands as a stray
	   vertical band mid-table once scrolled. The pinned column already
	   signals scrollability — drop it. */
	.compare-matrix::after {
		display: none;
	}

	/* Mockup zebra — a faint cool-lavender stripe (≈ #f9f9fc on white)
	   rather than the neutral surface-subtle; translucent so it adapts to
	   the dark theme too. The sticky first column needs the opaque
	   composite so scrolled cells don't show through it. */
	.compare-matrix tbody tr:nth-child(even) td:not([data-best]) {
		background: rgb(135 135 195 / 0.05);
	}
	.compare-matrix tbody tr:nth-child(even) td:first-child:not([data-best]) {
		background: color-mix(in srgb, rgb(135 135 195) 5%, var(--color-surface));
	}
</style>
