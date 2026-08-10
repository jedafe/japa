<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import type { ColCity, ColComparisonMatrix } from '$lib/types';

	/**
	 * Cost of Living Calculator tool — §72, ADR-072.
	 * City selector (max 3 from system_settings) → comparison matrix vs baseline.
	 * Shareable URL via city_ids query param.
	 */
	let { data } = $props();

	let cities = $state<ColCity[]>(untrack(() => data.cities));
	let matrix = $state<ColComparisonMatrix | null>(untrack(() => data.matrix));
	let selectedIds = $state<string[]>(untrack(() => data.preselected ?? []));
	let loading = $state(false);
	let error = $state<string | null>(null);

	// Max cities and staleness threshold from system_settings (col_calculator.*
	// keys), passed from the SSR load. Falls back to the spec defaults if the
	// backend didn't return them. Ref: §72, Rule 3 / Rule 16.
	const maxCities = untrack(() => data.maxCities) ?? 3;
	const staleDays = untrack(() => data.staleDays) ?? 180;

	const seo = $derived(
		buildSeoMeta({
			title: t('col.seo_title', 'Cost of Living Calculator — JapaConnect Tools'),
			description: t(
				'col.seo_description',
				'Compare cost of living between your destination cities and the Lagos baseline. Rent, groceries, transport, utilities, and more.'
			),
			path: '/cost-of-living',
			origin: page.url.origin
		})
	);

	const baselineCity = $derived(cities.find((c) => c.is_baseline));

	// Ensure baseline is always in the selection.
	$effect(() => {
		if (baselineCity && !selectedIds.includes(baselineCity.id)) {
			selectedIds = [baselineCity.id, ...selectedIds.filter((id) => id !== baselineCity.id)].slice(0, maxCities);
		}
	});

	async function runComparison() {
		if (selectedIds.length < 2) {
			matrix = null;
			return;
		}
		loading = true;
		error = null;
		const idsParam = selectedIds.join(',');
		const res = await api.get<ColComparisonMatrix>(
			`/calculator/cost-of-living/compare?city_ids=${encodeURIComponent(idsParam)}`
		);
		loading = false;
		if (res.error) {
			error = res.error.message;
			matrix = null;
			return;
		}
		matrix = res.data;
		// Update URL for shareability (SvelteKit replaceState, not window.history)
		const url = new URL(page.url);
		url.searchParams.set('city_ids', idsParam);
		replaceState(url, {});
	}

	function toggleCity(cityId: string) {
		if (selectedIds.includes(cityId)) {
			if (baselineCity && cityId === baselineCity.id) return; // can't remove baseline
			selectedIds = selectedIds.filter((id) => id !== cityId);
		} else if (selectedIds.length < maxCities) {
			selectedIds = [...selectedIds, cityId];
		}
	}

	function cityLabel(c: { title?: string; geo_space_name?: string }): string {
		return c.title || c.geo_space_name || t('col.untitled_city', 'Untitled city');
	}

	function formatRatio(r: number): string {
		return r.toFixed(2) + '×';
	}

	// Check if data is stale (threshold from col_calculator.stale_days setting).
	const isStale = $derived.by(() => {
		if (!matrix) return false;
		for (const city of matrix.cities) {
			if (city.last_updated) {
				const age = Date.now() - new Date(city.last_updated).getTime();
				if (age > staleDays * 24 * 60 * 60 * 1000) return true;
			}
		}
		return false;
	});

	// Fetch comparison on mount if preselected from a shareable URL.
	$effect(() => {
		if (data.matrix && !matrix && !loading) {
			matrix = data.matrix;
		}
	});
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-4xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">{t('col.title', 'Cost of Living Calculator')}</h1>
		<p class="mt-2 text-text-subtle">
			{t('col.subtitle', 'Compare cities against the Lagos baseline. Select up to {max} cities.').replace('{max}', String(maxCities))}
		</p>
	</div>

	{#if cities.length === 0}
		<EmptyState
			icon="map-pin-off"
			title={t('col.no_cities', 'No cities available')}
			description={t('col.no_cities_desc', "City data hasn't been configured yet. Please check back later.")}
		/>
	{:else}
		<!-- City selector -->
		<div class="mb-6">
			<div class="flex items-center justify-between mb-3">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide">
					{t('col.select_cities', 'Select cities to compare')}
				</h2>
				<span class="text-sm text-muted">
					{t('col.selected_count', '{count}/{max} selected').replace('{count}', String(selectedIds.length)).replace('{max}', String(maxCities))}
				</span>
			</div>
			<div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
				{#each cities as city (city.id)}
					<button
						onclick={() => toggleCity(city.id)}
						disabled={!selectedIds.includes(city.id) && selectedIds.length >= maxCities}
						class="p-3 text-left border rounded-lg transition-colors disabled:opacity-50 {selectedIds.includes(city.id) ? 'border-[var(--tool-accent)] bg-[var(--tool-accent)]/5' : 'border-border hover:border-[var(--tool-accent)]'}"
					>
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium text-text">{cityLabel(city)}</span>
							{#if city.is_baseline}
								<span class="text-xs px-1.5 py-0.5 bg-surface-subtle text-muted rounded">{t('col.baseline_label', 'Baseline')}</span>
							{/if}
						</div>
						{#if city.geo_space_name}
							<p class="text-xs text-muted mt-0.5">{city.geo_space_name}</p>
						{/if}
					</button>
				{/each}
			</div>
			<button
				onclick={runComparison}
				disabled={selectedIds.length < 2 || loading}
				class="mt-4 inline-flex items-center gap-2 px-4 py-2.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
			>
				{#if loading}
					<Icon name="loader-2" size={16} class="animate-spin" />
					{t('col.comparing', 'Comparing...')}
				{:else}
					<Icon name="git-compare" size={16} />
					{t('col.compare', 'Compare cities')}
				{/if}
			</button>
		</div>

		{#if error}
			<p class="text-sm text-red-500 mb-4">{error}</p>
		{/if}

		{#if loading}
			<div class="space-y-3">
				<LoadingSkeleton height="60px" />
				<LoadingSkeleton height="200px" />
			</div>
		{:else if matrix}
			{#if isStale}
				<div class="mb-4 p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg flex items-center gap-2">
					<Icon name="alert-triangle" size={18} class="text-yellow-600 dark:text-yellow-400 flex-shrink-0" />
					<p class="text-sm text-yellow-700 dark:text-yellow-300">
						{t('col.stale_warning', 'Some city data may be outdated. Values are curated manually and updated periodically.')}
					</p>
				</div>
			{/if}

			<!-- Overall Cost Index -->
			{#if Object.keys(matrix.overall_index).length > 0}
				<div class="mb-6 rounded-lg border border-border bg-surface-subtle p-4">
					<h2 class="mb-3 text-sm font-semibold text-muted uppercase tracking-wide">
						{t('col.overall_index', 'Overall Cost Index')}
					</h2>
					<div class="space-y-2">
						{#each matrix.cities as city (city.id)}
							<div class="flex items-center justify-between text-sm">
								<span class="text-text">
									{cityLabel(city)}
									{#if city.is_baseline}
										<span class="text-muted">({t('col.baseline_label', 'Baseline')})</span>
									{/if}
								</span>
								<span class="font-medium text-text">{formatRatio(matrix.overall_index[city.id] ?? 1)}</span>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Category breakdown table -->
			<div class="overflow-x-auto rounded-lg border border-border">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-border text-left text-xs text-muted">
							<th class="px-3 py-2 font-medium">{t('col.attribute_header', 'Attribute')}</th>
							{#each matrix.cities as city (city.id)}
								<th class="px-3 py-2 text-right font-medium">{cityLabel(city)}</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each matrix.categories as cat (cat.key)}
							<tr class="border-b border-border last:border-0">
								<td class="px-3 py-2 text-text">{cat.label}</td>
								{#each matrix.cities as city (city.id)}
									<td class="px-3 py-2 text-right text-text">
										<div class="font-medium">{cat.values[city.id]?.toFixed(0) ?? '—'}</div>
										{#if !city.is_baseline && cat.ratios[city.id]}
											<div class="text-xs text-muted">{formatRatio(cat.ratios[city.id])}</div>
										{/if}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else if selectedIds.length < 2}
			<EmptyState
				icon="git-compare"
				title={t('col.select_more', 'Select at least 2 cities to compare')}
			/>
		{/if}
	{/if}
	{/if}
</div>
