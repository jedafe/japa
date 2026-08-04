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
	import type { ColCity, ColComparison, ColAttributeDef } from '$lib/types';

	/**
	 * Cost of Living Calculator tool — §72, ADR-072.
	 * City selector (max 3 from system_settings) → comparison matrix vs baseline.
	 * Shareable URL via city_ids query param.
	 */
	let { data } = $props();

	let cities = $state<ColCity[]>(untrack(() => data.cities));
	let comparison = $state<ColComparison | null>(untrack(() => data.comparison));
	let selectedCityIds = $state<string[]>([]);
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

	async function runComparison() {
		if (selectedCityIds.length === 0) return;
		loading = true;
		error = null;
		const idsParam = selectedCityIds.join(',');
		const res = await api.get<ColComparison>(
			`/calculator/cost-of-living/compare?city_ids=${encodeURIComponent(idsParam)}`
		);
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		comparison = res.data;
		// Update URL for shareability (SvelteKit replaceState, not window.history)
		const url = new URL(page.url);
		url.searchParams.set('city_ids', idsParam);
		replaceState(url, {});
	}

	function toggleCity(cityId: string) {
		if (selectedCityIds.includes(cityId)) {
			selectedCityIds = selectedCityIds.filter((id) => id !== cityId);
		} else if (selectedCityIds.length < maxCities) {
			selectedCityIds = [...selectedCityIds, cityId];
		}
	}

	function formatCurrency(value: number | null | undefined, currency: string): string {
		if (value === null || value === undefined) return '—';
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: currency.toUpperCase(),
			maximumFractionDigits: 0
		}).format(value);
	}

	function ratioLabel(ratio: number | null | undefined): string {
		if (ratio === null || ratio === undefined) return '—';
		const pct = Math.round((ratio - 1) * 100);
		if (pct === 0) return t('col.same', 'Same');
		return pct > 0 ? `+${pct}%` : `${pct}%`;
	}

	function ratioColor(ratio: number | null | undefined): string {
		if (ratio === null || ratio === undefined) return 'text-muted';
		const pct = (ratio - 1) * 100;
		if (pct > 5) return 'text-red-500';
		if (pct < -5) return 'text-green-500';
		return 'text-muted';
	}

	// Group attributes by category
	const groupedAttributes = $derived.by(() => {
		if (!comparison?.attributes) return [];
		const groups: Record<string, ColAttributeDef[]> = {};
		for (const attr of comparison.attributes) {
			const cat = attr.category ?? t('col.general_category', 'General');
			if (!groups[cat]) groups[cat] = [];
			groups[cat].push(attr);
		}
		return Object.entries(groups);
	});

	// Check if data is stale (threshold from col_calculator.stale_days setting).
	const isStale = $derived.by(() => {
		if (!comparison) return false;
		for (const city of comparison.cities) {
			if (city.last_updated_at) {
				const age = Date.now() - new Date(city.last_updated_at).getTime();
				if (age > staleDays * 24 * 60 * 60 * 1000) return true;
			}
		}
		return false;
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
					{t('col.selected_count', '{count}/{max} selected').replace('{count}', String(selectedCityIds.length)).replace('{max}', String(maxCities))}
				</span>
			</div>
			<div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
				{#each cities as city (city.id)}
					<button
						onclick={() => toggleCity(city.id)}
						disabled={!selectedCityIds.includes(city.id) && selectedCityIds.length >= maxCities}
						class="p-3 text-left border rounded-lg transition-colors disabled:opacity-50 {selectedCityIds.includes(city.id) ? 'border-[var(--tool-accent)] bg-[var(--tool-accent)]/5' : 'border-border hover:border-[var(--tool-accent)]'}"
					>
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium text-text">{city.name}</span>
							{#if city.is_baseline}
								<span class="text-xs px-1.5 py-0.5 bg-surface-subtle text-muted rounded">{t('col.baseline_label', 'Baseline')}</span>
							{/if}
						</div>
						<p class="text-xs text-muted mt-0.5">{city.country}</p>
					</button>
				{/each}
			</div>
			<button
				onclick={runComparison}
				disabled={selectedCityIds.length === 0 || loading}
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
		{:else if comparison}
			{#if isStale}
				<div class="mb-4 p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg flex items-center gap-2">
					<Icon name="alert-triangle" size={18} class="text-yellow-600 dark:text-yellow-400 flex-shrink-0" />
					<p class="text-sm text-yellow-700 dark:text-yellow-300">
						{t('col.stale_warning', 'Some city data may be outdated. Values are curated manually and updated periodically.')}
					</p>
				</div>
			{/if}

			<!-- Comparison matrix -->
			<div class="space-y-6">
				{#each groupedAttributes as [category, attrs] (category)}
					<div>
						<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
							{category}
						</h3>
						<div class="overflow-x-auto">
							<table class="w-full border-collapse">
								<thead>
									<tr class="border-b border-border">
										<th class="text-left py-2 px-3 text-sm font-medium text-text">{t('col.attribute_header', 'Attribute')}</th>
										<th class="text-right py-2 px-3 text-sm font-medium text-text">
											{comparison!.baseline.name}
											<span class="block text-xs font-normal text-muted">{t('col.baseline_label', 'Baseline')}</span>
										</th>
										{#each comparison!.cities.filter((c) => c.id !== comparison!.baseline.id) as city (city.id)}
											<th class="text-right py-2 px-3 text-sm font-medium text-text">
												{city.name}
												<span class="block text-xs font-normal text-muted">{city.currency_code}</span>
											</th>
										{/each}
									</tr>
								</thead>
								<tbody>
									{#each attrs as attr (attr.slug)}
										<tr class="border-b border-border">
											<td class="py-2 px-3 text-sm text-text">{attr.label}</td>
											<td class="py-2 px-3 text-sm text-right text-muted">
												{formatCurrency(comparison!.baseline.attributes?.[attr.slug] ?? null, comparison!.baseline.currency_code)}
											</td>
											{#each comparison!.cities.filter((c) => c.id !== comparison!.baseline.id) as city (city.id)}
												<td class="py-2 px-3 text-sm text-right">
													<div class="font-medium text-text">
														{formatCurrency(city.attributes?.[attr.slug] ?? null, city.currency_code)}
													</div>
													<div class="text-xs {ratioColor(city.ratios?.[attr.slug])}">
														{ratioLabel(city.ratios?.[attr.slug])}
													</div>
												</td>
											{/each}
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
	{/if}
</div>
