<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ChevronRight, GitCompare, Scale } from 'lucide-svelte';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t, siteName } from '#lib/i18n/index.js';
	import JceSelect from '#lib/components/ui/JceSelect.svelte';
	import { EmptyState, PaginationControls } from '#lib/components/ui/index.js';
	import CompareRefineRail from '#lib/components/compare/CompareRefineRail.svelte';
	import ComparisonVsCard from '#lib/components/compare/ComparisonVsCard.svelte';
	import { badgeFor } from '#lib/components/compare/compareBadge.js';
	import { familyForSlug } from '#lib/components/compare/compareFamilies.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let family = $derived(familyForSlug(data.familySlug)!);
	let FamIcon = $derived(family.icon);
	let memberTypes = $derived(data.memberTypes);
	let creatableTypes = $derived(data.creatableTypes);
	let meta = $derived(data.meta);
	let query = $state('');
	// Resync after client-side navigation (rail links keep this page mounted).
	$effect(() => {
		query = data.filters.q ?? '';
	});

	// Type facet counts, scoped server-side to this family's member types
	// (facet_scope). When `sub` is active the list total only covers that
	// subtype, so the "All comparisons" rail count must sum facets — falling
	// back to meta.total only when facets are absent and no sub is active.
	const facetTypes = $derived(meta.facets?.types ?? []);
	const facetCountBySlug = $derived.by(() => {
		const map: Record<string, number> = {};
		for (const f of facetTypes) if (f.slug) map[f.slug] = f.count;
		return map;
	});
	const allCount = $derived(
		facetTypes.length > 0
			? facetTypes.reduce((sum, f) => sum + f.count, 0)
			: data.sub
				? undefined
				: (meta.total ?? 0)
	);

	function url(updates: Record<string, string | null>): string {
		const u = new URL(page.url.href);
		for (const [k, v] of Object.entries(updates)) {
			if (v === null || v === '') u.searchParams.delete(k);
			else u.searchParams.set(k, v);
		}
		return u.pathname + u.search;
	}

	const railCategories = $derived([
		{
			label: t('jce.compare.category.all', 'All comparisons'),
			count: allCount,
			href: url({ sub: null, page: null }),
			active: !data.sub
		},
		...memberTypes.map((mt) => ({
			label: mt.name,
			count: facetCountBySlug[mt.slug] as number | undefined,
			href: url({ sub: mt.slug, page: null }),
			active: data.sub === mt.slug
		}))
	]);

	function onQuery(q: string) {
		goto(url({ q: q || null, page: null }), { replace: true, reset: false });
	}

	const sortOptions = $derived(
		(meta.sort_options ?? ['popularity']).map((v) => ({
			value: v,
			label: t(`jce.comparisons.sort.${v}`, v)
		}))
	);

	const totalPages = $derived(Math.max(1, Math.ceil((meta.total ?? 0) / (meta.limit ?? 12))));
</script>

<SeoHead
	title={`${t(`jce.compare.family.${family.key}`, family.key)} — ${t('jce.compare.heading')} — ${siteName()}`}
	description={t(`jce.compare.family_desc.${family.key}`, '')}
/>

<div class="mx-auto w-full max-w-6xl space-y-6 p-4">
	<!-- Breadcrumb + category header -->
	<nav class="flex items-center gap-1.5 text-sm text-[var(--color-muted)]" aria-label="breadcrumb">
		<a href="/" class="hover:text-[var(--color-text)]">{t('nav.home', 'Home')}</a>
		<ChevronRight size={13} class="rtl:-scale-x-100" />
		<a href="/compare" class="hover:text-[var(--color-text)]">{t('jce.compare.heading')}</a>
		<ChevronRight size={13} class="rtl:-scale-x-100" />
		<span class="text-[var(--color-text)]">{t(`jce.compare.family.${family.key}`, family.key)}</span>
	</nav>

	<header class="flex flex-wrap items-start justify-between gap-4">
		<div class="flex items-start gap-4">
			<span
				class="grid h-14 w-14 shrink-0 place-items-center rounded-[var(--radius-card)]"
				style="background: color-mix(in srgb, {family.tint} 14%, transparent); color: {family.tint}"
			>
				<FamIcon size={28} />
			</span>
			<div>
				<h1 class="text-2xl font-bold text-[var(--color-text)] sm:text-3xl">
					{t(`jce.compare.family.${family.key}`, family.key)}
				</h1>
				<p class="mt-1 max-w-md text-sm text-[var(--color-muted)]">
					{t(`jce.compare.family_desc.${family.key}`, '')}
				</p>
				<p class="mt-2 flex flex-wrap items-center gap-3 text-xs text-[var(--color-muted)]">
					<span class="inline-flex items-center gap-1">
						<GitCompare size={12} />
						{t('jce.compare.category.comparisons_count', '{n} comparisons', { n: String(meta.total ?? 0) })}
					</span>
					<span>{t('jce.compare.category.updated_weekly', 'Updated weekly')}</span>
				</p>
			</div>
		</div>

		<!-- Can't decide? → picker handoff (only creatable member types) -->
		{#if creatableTypes.length > 0}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<p class="text-sm font-semibold text-[var(--color-text)]">{t('jce.compare.category.cant_decide', "Can't decide?")}</p>
				<p class="mt-0.5 text-xs text-[var(--color-muted)]">
					{t('jce.compare.category.cant_decide_body', 'Compare up to {max} entries side by side.', { max: '3' })}
				</p>
				{#if creatableTypes.length === 1}
					<a
						href="/compare?type={creatableTypes[0].slug}"
						class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)]"
					>
						<Scale size={14} /> {t('jce.compare.category.start_custom', 'Start custom comparison')}
					</a>
				{:else}
					<div class="mt-3 flex flex-wrap gap-2">
						{#each creatableTypes as mt (mt.id)}
							<a
								href="/compare?type={mt.slug}"
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>{mt.name}</a>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</header>

	<div class="flex flex-col gap-6 lg:flex-row">
		<CompareRefineRail bind:query categories={railCategories} {onQuery} />

		<main class="min-w-0 flex-1 space-y-4">
			<div class="flex items-center justify-between gap-3">
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('jce.compare.category.comparisons_count', '{n} comparisons', { n: String(meta.total ?? 0) })}
				</p>
				<JceSelect
					options={sortOptions}
					value={data.filters.sort}
					onchange={(v) => goto(url({ sort: v, page: null }), { replace: true })}
					class="w-44"
				/>
			</div>

			{#if data.loadError}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{t(data.loadError)}
				</p>
			{:else if data.items.length === 0}
				<EmptyState
					icon={GitCompare}
					title={t('jce.comparisons.empty_public')}
					action={creatableTypes.length > 0
						? { label: t('jce.compare.category.start_custom', 'Start custom comparison'), href: `/compare?type=${creatableTypes[0].slug}` }
						: undefined}
				/>
			{:else}
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
					{#each data.items as item, i (item.id)}
						<ComparisonVsCard
							summary={item}
							badge={data.filters.sort === 'popularity' ? badgeFor(item, { popularityIndex: i }) : badgeFor(item)}
							authed={Boolean(data.user)}
						/>
					{/each}
				</div>
				<PaginationControls
					page={meta.page ?? 1}
					{totalPages}
					onPageChange={(p) => goto(url({ page: String(p) }), { replace: true })}
				/>
			{/if}
		</main>
	</div>

	<!-- Bottom CTA strip -->
	{#if creatableTypes.length > 0}
		<section class="flex flex-wrap items-center justify-between gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-primary)]/5 p-5">
			<div class="flex items-center gap-3">
				<span class="grid h-10 w-10 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]">
					<Scale size={18} />
				</span>
				<div>
					<p class="font-semibold text-[var(--color-text)]">
						{t('jce.compare.category.create_own_title', 'Create your own comparison')}
					</p>
					<p class="text-sm text-[var(--color-muted)]">
						{t('jce.compare.category.create_own_body', 'Choose your entries and compare the metrics that matter most to you.')}
					</p>
				</div>
			</div>
			<a
				href="/compare?type={creatableTypes[0].slug}"
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)]"
			>
				{t('jce.compare.category.start_custom', 'Start custom comparison')}
			</a>
		</section>
	{/if}
</div>
