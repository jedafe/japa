<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta, mainAppUrl } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t, i18n } from '$lib/i18n';
	import { formatMoney } from '$lib/utils/currency';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import type { ColCity, ColCityData, ColComparisonMatrix } from '$lib/types';

	/**
	 * Cost of Living Calculator tool — §72, ADR-072.
	 * City selector (max 3 from system_settings) → comparison matrix vs baseline.
	 * Shareable URL via city_ids query param.
	 */
	let { data } = $props();

	let cities = $state<ColCity[]>(untrack(() => data.cities));
	let matrix = $state<ColComparisonMatrix | null>(untrack(() => data.matrix));
	let loading = $state(false);
	let error = $state<string | null>(null);

	// Max cities and staleness threshold from system_settings (col_calculator.*
	// keys), passed from the SSR load. Falls back to the spec defaults if the
	// backend didn't return them. Ref: §72, Rule 3 / Rule 16.
	const maxCities = untrack(() => data.maxCities) ?? 3;
	const staleDays = untrack(() => data.staleDays) ?? 180;

	// The city ratios are computed against — user-selectable (§72 amendment),
	// defaulting to the seeded is_baseline city (Lagos).
	const seededBaselineId = $derived(cities.find((c) => c.is_baseline)?.id ?? null);
	let baselineId = $state<string | null>(untrack(() => data.preselectedBaseline ?? null));
	// Destination cities compared against the baseline. The baseline is
	// excluded in case an old-style share URL carries it inside city_ids.
	let selectedIds = $state<string[]>(
		untrack(() => {
			const base =
				data.preselectedBaseline ?? data.cities.find((c) => c.is_baseline)?.id ?? null;
			return (data.preselected ?? []).filter((id) => id !== base).slice(0, maxCities);
		})
	);

	const seo = $derived(
		buildSeoMeta({
			title: t('col.seo_title', 'Cost of Living Calculator — {site} Tools'),
			description: t(
				'col.seo_description',
				'Compare cost of living between your destination cities and a baseline city. Rent, groceries, transport, utilities, and more.'
			),
			path: '/cost-of-living',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-cost-of-living.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'SoftwareApplication',
				name: 'Cost of Living Calculator',
				applicationCategory: 'UtilitiesApplication',
				operatingSystem: 'Web',
				offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
				description: 'Compare cost of living between destination cities — rent, groceries, transport, utilities.'
			}
		})
	);

	const effectiveBaselineId = $derived(baselineId ?? seededBaselineId);
	const baselineCity = $derived(cities.find((c) => c.id === effectiveBaselineId) ?? null);

	async function runComparison() {
		if (selectedIds.length < 1) {
			matrix = null;
			return;
		}
		loading = true;
		error = null;
		const params = new URLSearchParams({ city_ids: selectedIds.join(',') });
		if (effectiveBaselineId) params.set('baseline_id', effectiveBaselineId);
		const res = await api.get<ColComparisonMatrix>(
			`/calculator/cost-of-living/compare?${params.toString()}`
		);
		loading = false;
		if (res.error) {
			error = res.error.message;
			matrix = null;
			return;
		}
		matrix = res.data;
		// If the requested baseline didn't resolve, the API falls back to the
		// seeded baseline — mirror the effective one.
		if (res.data?.baseline_city_id && res.data.baseline_city_id !== baselineId) {
			baselineId = res.data.baseline_city_id;
		}
		// Update URL for shareability (SvelteKit replaceState, not window.history)
		const url = new URL(page.url);
		url.searchParams.set('city_ids', selectedIds.join(','));
		if (effectiveBaselineId) url.searchParams.set('baseline_id', effectiveBaselineId);
		else url.searchParams.delete('baseline_id');
		replaceState(url, {});
	}

	function setBaseline(cityId: string) {
		if (cityId === effectiveBaselineId) return;
		baselineId = cityId;
		// A city can't be both the reference and a destination.
		selectedIds = selectedIds.filter((id) => id !== cityId);
	}

	function addCity(cityId: string) {
		if (cityId === effectiveBaselineId || selectedIds.includes(cityId) || selectedIds.length >= maxCities) return;
		selectedIds = [...selectedIds, cityId];
	}

	function removeCity(cityId: string) {
		selectedIds = selectedIds.filter((id) => id !== cityId);
	}

	// Autocomplete fetchers — filter the SSR-loaded city list client-side
	// (Rule 16: the list comes from the API). The destination picker excludes
	// already-selected cities and the baseline; the baseline picker lists all.
	function cityItem(c: ColCity): AutocompleteItem {
		return {
			id: c.id,
			label: cityLabel(c),
			sublabel: c.is_baseline
				? t('col.baseline_label', 'Baseline')
				: c.geo_space_name || undefined
		};
	}

	function matchesCity(c: ColCity, query: string): boolean {
		return (
			query === '' ||
			cityLabel(c).toLowerCase().includes(query) ||
			c.geo_space_name.toLowerCase().includes(query) ||
			c.description.toLowerCase().includes(query)
		);
	}

	function fetchCityItems(q: string): Promise<AutocompleteItem[]> {
		const query = q.trim().toLowerCase();
		const items = cities
			.filter((c) => !selectedIds.includes(c.id) && c.id !== effectiveBaselineId)
			.filter((c) => matchesCity(c, query))
			.map(cityItem);
		return Promise.resolve(items);
	}

	function fetchBaselineItems(q: string): Promise<AutocompleteItem[]> {
		const query = q.trim().toLowerCase();
		return Promise.resolve(cities.filter((c) => matchesCity(c, query)).map(cityItem));
	}

	const selectedCities = $derived(
		selectedIds.map((id) => cities.find((c) => c.id === id)).filter(Boolean) as ColCity[]
	);

	// City wiki pages live on the main app — the tools surface has no /wiki
	// routes, so links must go through mainAppUrl (cross-app).
	function wikiUrl(slug: string): string {
		return mainAppUrl(`/wiki/${slug}/cost-of-living`);
	}

	function cityLabel(c: { title?: string; geo_space_name?: string }): string {
		return c.title || c.geo_space_name || t('col.untitled_city', 'Untitled city');
	}

	function formatRatio(r: number): string {
		return r.toFixed(2) + '×';
	}

	// §112 — matrix cells are denominated in matrix.display_currency when
	// the city converted; unconverted cells keep their raw local value.
	function formatCellValue(city: ColCityData, v: number | undefined): string {
		if (v === undefined) return '—';
		if (matrix?.display_currency && city.converted) {
			return formatMoney(v, matrix.display_currency, data.currencies);
		}
		const n = v.toLocaleString(i18n.locale, { maximumFractionDigits: 0 });
		return city.currency_code ? `${n} ${city.currency_code}` : n;
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

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-4xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">{t('col.title', 'Cost of Living Calculator')}</h1>
		<p class="mt-2 text-text-subtle">
			{t('col.subtitle', 'Compare cities against a baseline city. Select up to {max} cities.').replace('{max}', String(maxCities))}
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
			<!-- Baseline picker — the city all ratios are computed against
			     (§72 amendment: user-selectable, defaults to the seeded
			     is_baseline city). No onClear: a baseline is required. -->
			<div class="mb-3">
				<label for="col-baseline-search" class="mb-1 block text-xs text-muted">
					{t('col.baseline_compare_against', 'Compare against (baseline)')}
				</label>
				<div class="max-w-md">
					<Autocomplete
						id="col-baseline-search"
						placeholder={t('col.search_baseline', 'Search baseline city…')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchBaselineItems}
						initialFetcher={() => fetchBaselineItems('')}
						selectedId={effectiveBaselineId}
						selectedLabel={baselineCity ? cityLabel(baselineCity) : null}
						selectedBadge={baselineCity?.id === seededBaselineId
							? t('col.baseline_label', 'Baseline')
							: t('col.baseline_custom', 'custom')}
						onSelect={(item) => setBaseline(item.id)}
						ariaLabel={t('col.baseline_compare_against', 'Compare against (baseline)')}
					/>
				</div>
			</div>

			<!-- Selected destination chips -->
			{#if selectedCities.length > 0}
				<div class="mb-3 flex flex-wrap gap-2">
					{#each selectedCities as city (city.id)}
						<span
							class="inline-flex items-center gap-1.5 rounded-full border border-border bg-surface-subtle px-2.5 py-1 text-xs text-text"
						>
							{#if city.geo_space_slug}
								<a
									href={wikiUrl(city.geo_space_slug)}
									class="hover:underline"
									title={t('col.view_wiki', 'View {city} cost of living wiki').replace('{city}', cityLabel(city))}
								>
									{cityLabel(city)}
								</a>
							{:else}
								{cityLabel(city)}
							{/if}
							<button
								type="button"
								onclick={() => removeCity(city.id)}
								class="text-muted hover:text-text"
								aria-label="{t('common.remove', 'Remove')} {cityLabel(city)}"
							>
								<Icon name="x" size={12} />
							</button>
						</span>
					{/each}
				</div>
			{/if}

			<!-- Destination picker (Autocomplete dropdown — searchable, scales
			     to the full city list; selected cities and the baseline are
			     filtered out). -->
			<div class="max-w-md">
				<Autocomplete
					id="col-city-search"
					placeholder={t('col.search_cities', 'Search cities to compare…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchCityItems}
					initialFetcher={() => fetchCityItems('')}
					selectedId={null}
					onSelect={(item) => addCity(item.id)}
					disabled={selectedIds.length >= maxCities}
					ariaLabel={t('col.search_cities', 'Search cities to compare')}
				/>
			</div>
			<button
				onclick={runComparison}
				disabled={selectedIds.length < 1 || loading}
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
				<div
					class="mb-4 p-3 rounded-lg flex items-center gap-2"
					style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); border: 1px solid var(--color-warning)"
				>
					<Icon name="alert-triangle" size={18} class="flex-shrink-0" style="color: var(--color-warning)" />
					<p class="text-sm" style="color: var(--color-warning)">
						{t('col.stale_warning', 'Some city data may be outdated. Values are curated manually and updated periodically.')}
					</p>
				</div>
			{/if}

			{#if matrix.fx_stale}
				<div class="mb-4 flex items-center gap-2 rounded-lg border border-border bg-surface-subtle px-3 py-2">
					<Icon name="alert-triangle" size={14} class="flex-shrink-0 text-muted" />
					<p class="text-xs text-muted">
						{t('col.fx_stale', 'Exchange rates may be outdated — displayed conversions are indicative.')}
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
									{#if city.geo_space_slug}
										<a
											href={wikiUrl(city.geo_space_slug)}
											class="hover:underline"
											title={t('col.view_wiki', 'View {city} cost of living wiki').replace('{city}', cityLabel(city))}
										>
											{cityLabel(city)}
										</a>
									{:else}
										{cityLabel(city)}
									{/if}
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
						<tr class="border-b border-border text-start text-xs text-muted">
							<th class="px-3 py-2 font-medium">{t('col.attribute_header', 'Attribute')}</th>
							{#each matrix.cities as city (city.id)}
								<th class="px-3 py-2 text-end font-medium">
									{#if city.geo_space_slug}
										<a
											href={wikiUrl(city.geo_space_slug)}
											class="hover:underline"
											title={t('col.view_wiki', 'View {city} cost of living wiki').replace('{city}', cityLabel(city))}
										>
											{cityLabel(city)}
										</a>
									{:else}
										{cityLabel(city)}
									{/if}
								</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each matrix.categories as cat (cat.key)}
							<tr class="border-b border-border last:border-0">
								<td class="px-3 py-2 text-text">{cat.label}</td>
								{#each matrix.cities as city (city.id)}
									<td class="px-3 py-2 text-end text-text">
										<div class="font-medium">{formatCellValue(city, cat.values[city.id])}</div>
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
		{:else if selectedIds.length < 1}
			<EmptyState
				icon="git-compare"
				title={t('col.select_more', 'Select at least 1 city to compare')}
			/>
		{/if}
	{/if}
	{/if}
</div>
