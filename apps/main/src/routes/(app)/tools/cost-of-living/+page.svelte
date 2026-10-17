<script lang="ts">
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { replaceState } from '$app/navigation';
	import { Calculator, GitCompare, Loader2, Lock, Share2, X } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import type { ColCity, ColCityData, ColComparisonMatrix } from '$lib/types';
	import type { PageData, ActionData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { formatMoney, type CurrencyInfo } from '$lib/utils/currency';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('tools.cost_of_living', 'Cost of Living Calculator — {site}'));
	const description = $derived(
		t('tools.cost_of_living_desc', 'Compare living costs between cities.')
	);

	const cities = $derived<ColCity[]>(data.cities);
	const disabled = $derived(data.disabled);
	// Tool config from col_calculator.* system_settings (Rule 3 / Rule 16).
	const maxCities = $derived(data.maxCities ?? 3);
	const staleDays = $derived(data.staleDays ?? 180);

	// The city ratios are computed against — user-selectable (§72 amendment),
	// defaulting to the seeded is_baseline city (Lagos).
	const seededBaselineId = $derived(cities.find((c) => c.is_baseline)?.id ?? null);
	let baselineId = $state<string | null>(untrack(() => data.preselectedBaseline));
	// Up to maxCities destination cities compared against the baseline. The
	// baseline is excluded in case an old-style share URL carries it inside
	// city_ids.
	let selectedIds = $state<string[]>(
		untrack(() => {
			const base =
				data.preselectedBaseline ?? data.cities.find((c) => c.is_baseline)?.id ?? null;
			return data.preselected.filter((id) => id !== base).slice(0, maxCities);
		})
	);
	let matrix = $state<ColComparisonMatrix | null>(untrack(() => data.matrix));
	let loading = $state(false);
	let error = $state<string | null>(null);
	let copied = $state(false);
	let savingJCE = $state(false);
	// Race guard — a stale response must never overwrite a newer selection.
	let fetchSeq = 0;

	const effectiveBaselineId = $derived(baselineId ?? seededBaselineId);
	const baselineCity = $derived(cities.find((c) => c.id === effectiveBaselineId) ?? null);
	const selectedCities = $derived(
		selectedIds.map((id) => cities.find((c) => c.id === id)).filter(Boolean) as ColCity[]
	);
	const idsKey = $derived(selectedIds.join(','));

	// Single source of truth for the comparison fetch. Runs once per selection
	// or baseline change; the SSR-seeded matrix is reused whenever it already
	// covers the current selection (same effective baseline + all selected
	// cities) — including legacy share URLs that carried the baseline inside
	// city_ids, which strips it from selectedIds but not data.preselected.
	$effect(() => {
		const ids = idsKey;
		const base = effectiveBaselineId;
		const m = data.matrix;
		if (
			m &&
			ids !== '' &&
			base === m.baseline_city_id &&
			selectedIds.every((id) => m.cities.some((c) => c.id === id))
		) {
			matrix = m;
			error = null;
			// Mirror the effective baseline when the requested one didn't
			// resolve (deleted city, wrong-type id) so the picker never blanks.
			if (baselineId !== m.baseline_city_id) baselineId = m.baseline_city_id;
			return;
		}
		fetchComparison();
	});

	// Keep ?city_ids= + ?baseline_id= in sync so the share URL and reloads
	// restore the same comparison. Called only from user actions —
	// replaceState throws during hydration before the router is initialized.
	function syncUrl() {
		const url = new URL(page.url);
		if (selectedIds.length >= 1) url.searchParams.set('city_ids', idsKey);
		else url.searchParams.delete('city_ids');
		if (effectiveBaselineId) url.searchParams.set('baseline_id', effectiveBaselineId);
		else url.searchParams.delete('baseline_id');
		if (url.href !== page.url.href) replaceState(url, page.state);
	}

	async function fetchComparison() {
		// Invalidate any in-flight request before the early return so a stale
		// response can't re-render a comparison the user just cleared (and so
		// `loading` never gets stuck).
		const seq = ++fetchSeq;
		if (selectedIds.length < 1) {
			matrix = null;
			error = null;
			loading = false;
			return;
		}
		loading = true;
		error = null;
		const params = new URLSearchParams({ city_ids: idsKey });
		if (effectiveBaselineId) params.set('baseline_id', effectiveBaselineId);
		const res = await api.get<ColComparisonMatrix>(
			`/calculator/cost-of-living/compare?${params.toString()}`
		);
		if (seq !== fetchSeq) return; // superseded by a newer selection
		loading = false;
		if (res.error) {
			error = res.error.message;
			matrix = null;
			return;
		}
		matrix = res.data;
		// If the requested baseline didn't resolve (deleted city, stale link),
		// the API falls back to the seeded baseline — mirror the effective one.
		if (res.data?.baseline_city_id && res.data.baseline_city_id !== baselineId) {
			baselineId = res.data.baseline_city_id;
		}
	}

	function setBaseline(id: string) {
		if (id === effectiveBaselineId) return;
		baselineId = id;
		// A city can't be both the reference and a destination.
		selectedIds = selectedIds.filter((sid) => sid !== id);
		syncUrl();
	}

	function addCity(id: string) {
		if (id === effectiveBaselineId || selectedIds.includes(id) || selectedIds.length >= maxCities) return;
		selectedIds = [...selectedIds, id];
		syncUrl();
	}

	function removeCity(id: string) {
		selectedIds = selectedIds.filter((sid) => sid !== id);
		syncUrl();
	}

	// Autocomplete fetchers — filter the SSR-loaded city list client-side
	// (Rule 16: the list comes from the API). The destination picker excludes
	// already-selected cities and the baseline; the baseline picker lists all.
	function cityItem(c: ColCity): AutocompleteItem {
		return {
			id: c.id,
			label: cityLabel(c),
			sublabel: c.is_baseline ? t('col.baseline', 'baseline') : c.geo_space_name || undefined
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

	async function shareUrl() {
		const params = new URLSearchParams({ city_ids: idsKey });
		if (effectiveBaselineId) params.set('baseline_id', effectiveBaselineId);
		const url = `${page.url.origin}${page.url.pathname}?${params.toString()}`;
		try {
			await navigator.clipboard.writeText(url);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard may be blocked; ignore.
		}
	}

	function formatRatio(r: number): string {
		return r.toFixed(2) + '×';
	}

	// Active currency table (Section 58) from the root layout — formatMoney
	// resolves symbols from it. Money cells are denominated in the matrix's
	// display_currency (FX base) when the backend normalized them, else USD.
	const currencies = $derived((page.data.currencies as CurrencyInfo[] | undefined) ?? []);
	const displayCurrency = $derived(matrix?.display_currency || 'USD');

	function formatValue(v: number | undefined): string {
		if (v == null) return '—';
		return formatMoney(v, displayCurrency, currencies);
	}

	// §72/ADR-72 §5: surface each city's last_updated_at as a "last verified"
	// label for data-freshness transparency.
	function formatDate(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '';
		return d.toLocaleDateString((page.data.locale as string) || undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	function lastVerified(city: ColCityData): string {
		const d = city.last_updated ? formatDate(city.last_updated) : '';
		return d ? t('col.last_verified', 'Last verified {date}').replace('{date}', d) : '';
	}

	// Staleness check — §72 requires a "data may be outdated" notice when any
	// city's last_updated_at is older than col_calculator.stale_days (default 180).
	const isStale = $derived.by(() => {
		if (!matrix) return false;
		const thresholdMs = staleDays * 24 * 60 * 60 * 1000;
		for (const city of matrix.cities) {
			if (city.last_updated) {
				const age = Date.now() - new Date(city.last_updated).getTime();
				if (age > thresholdMs) return true;
			}
		}
		return false;
	});

	// Some seeded cities may have an empty title; fall back to the geo space
	// name so chips and buttons always have an accessible label (§H). Accepts
	// both ColCity (selector) and ColCityData (comparison matrix) since both
	// share title + geo_space_name.
	function cityLabel(c: { title?: string; geo_space_name?: string }): string {
		return c.title || c.geo_space_name || t('col.untitled_city', 'Untitled city');
	}

	// JSON-LD for the shareable comparison result (Ref: Section 72, ROADMAP-MAIN 36.03).
	// A cost-of-living comparison matrix maps most honestly to schema.org Dataset.
	const jsonLd = $derived(
		matrix
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Dataset',
					name: t('tools.cost_of_living', 'Cost of Living Comparison'),
					description: `Cost of living comparison between ${matrix.cities
						.map((c) => c.title || c.geo_space_name)
						.join(', ')}`,
					url: page.url.href,
					variableMeasured: matrix.categories.map((cat) => ({
						'@type': 'PropertyValue',
						name: cat.label,
						value: Object.entries(cat.ratios)
							.map(([k, v]) => `${k}: ${formatRatio(v)}`)
							.join('; ')
					}))
				})
			: ''
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Calculator size={20} /> {t('tools.cost_of_living', 'Cost of Living Calculator')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if cities.length === 0}
		<div class="p-4">
			<EmptyState
				icon={Calculator}
				title={t('col.no_cities', 'No cities available')}
				body={t(
					'col.no_cities_desc',
					"City data hasn't been configured yet. Please check back later."
				)}
			/>
		</div>
	{:else}
		<!-- City selector -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<div class="mb-2 flex items-center justify-between">
				<span class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('col.select_cities', 'Select up to {max} cities').replace('{max}', String(maxCities))}
				</span>
				{#if selectedIds.length >= 1}
					<div class="flex items-center gap-3">
						{#if data.authed}
							<form
								method="POST"
								use:enhance={() => {
									savingJCE = true;
									return async ({ update }) => {
										savingJCE = false;
										await update();
									};
								}}
								class="contents"
							>
								{#if effectiveBaselineId}
									<input type="hidden" name="city_id" value={effectiveBaselineId} />
									<input type="hidden" name="baseline_id" value={effectiveBaselineId} />
								{/if}
								{#each selectedIds as cityId}
									<input type="hidden" name="city_id" value={cityId} />
								{/each}
								<button
									type="submit"
									disabled={savingJCE}
									title={t('col.compare_in_jce_hint', 'Open in JCE comparison')}
									class="flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline disabled:opacity-50"
								>
									{#if savingJCE}
										<Loader2 size={12} class="animate-spin" />
									{:else}
										<GitCompare size={12} />
									{/if}
									{t('col.compare_in_jce', 'Compare in JCE')}
								</button>
							</form>
						{:else}
							<!-- Guests get a working sign-in path back to this selection
							     instead of a dead disabled button (§72: tool is public,
							     only the JCE export needs auth). -->
							<a
								href="/login?next={encodeURIComponent(page.url.pathname + page.url.search)}"
								title={t('col.compare_in_jce_hint', 'Open in JCE comparison')}
								class="flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
							>
								<GitCompare size={12} />
								{t('col.sign_in_to_compare', 'Sign in to compare')}
							</a>
						{/if}
						<button
							type="button"
							onclick={shareUrl}
							class="flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
						>
							<Share2 size={12} />
							{copied ? t('common.copied', 'Copied!') : t('common.share', 'Share')}
						</button>
					</div>
				{/if}
			</div>

			{#if form?.error}
				<div class="mb-2 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
					{form.message || t('col.jce_error', 'Could not create comparison')}
				</div>
			{/if}

			<!-- Baseline + destination pickers share one row on desktop
			     (stacked on mobile). Baseline: the city all ratios are computed
			     against (§72 amendment: user-selectable, defaults to the seeded
			     is_baseline city). No onClear: a baseline is required. -->
			<div class="mb-2 flex flex-col gap-3 sm:flex-row sm:items-end">
				<div class="min-w-0 sm:flex-1">
					<label for="col-baseline-search" class="mb-1 block text-xs text-[var(--color-muted)]">
						{t('col.baseline_label', 'Compare against (baseline)')}
					</label>
					<Autocomplete
						id="col-baseline-search"
						placeholder={t('col.search_baseline', 'Search baseline city…')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchBaselineItems}
						initialFetcher={() => fetchBaselineItems('')}
						selectedId={effectiveBaselineId}
						selectedLabel={baselineCity ? cityLabel(baselineCity) : null}
						selectedBadge={baselineCity?.id === seededBaselineId
							? t('col.baseline', 'baseline')
							: t('col.baseline_custom', 'custom')}
						onSelect={(item) => setBaseline(item.id)}
						ariaLabel={t('col.baseline_label', 'Compare against (baseline)')}
					/>
				</div>

				<!-- Destination picker (Autocomplete dropdown — searchable, scales
				     to the full city list; selected cities and the baseline are
				     filtered out). -->
				<div class="min-w-0 sm:flex-1">
					<label for="col-city-search" class="mb-1 block text-xs text-[var(--color-muted)]">
						{t('col.destinations_label', 'Compare with')}
					</label>
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
			</div>

			<!-- Selected destination chips -->
			{#if selectedCities.length > 0}
				<div class="mb-2 flex flex-wrap gap-2">
					{#each selectedCities as city (city.id)}
						<span
							class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1 text-xs text-[var(--color-text)]"
						>
							{#if city.geo_space_slug}
								<a
									href="/wiki/{city.geo_space_slug}/cost-of-living"
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
								class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
								aria-label="{t('common.remove', 'Remove')} {cityLabel(city)}"
							>
								<X size={12} />
							</button>
						</span>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Comparison matrix -->
		<div class="p-4">
			{#if selectedIds.length < 1}
				<EmptyState
					icon={Calculator}
					title={t('col.select_more', 'Select at least 1 city to compare')}
				/>
			{:else if loading}
				<LoadingSkeleton variant="lines" />
			{:else if error}
				<EmptyState icon={Calculator} title={t('col.load_error', 'Could not load comparison')} body={error} />
			{:else if matrix}
				{#if isStale}
					<div
						class="mb-4 rounded-[var(--radius-card)] border px-4 py-3 text-sm"
						style="border-color: var(--color-warning); background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)"
					>
						{t('col.stale_warning', 'Some city data may be outdated. Values are curated manually and updated periodically.')}
					</div>
				{/if}
				<!-- Overall index -->
				<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
						{t('col.overall_index', 'Overall Cost Index')}
					</h2>
					<div class="space-y-2">
						{#each matrix.cities as city (city.id)}
							<div class="flex items-center justify-between text-sm">
								<span class="text-[var(--color-text)]">
									{#if city.geo_space_slug}
										<a
											href="/wiki/{city.geo_space_slug}/cost-of-living"
											class="hover:underline"
											title={t('col.view_wiki', 'View {city} cost of living wiki').replace('{city}', cityLabel(city))}
										>
											{cityLabel(city)}
										</a>
									{:else}
										{cityLabel(city)}
									{/if}
									{#if city.is_baseline}
										<span class="text-[var(--color-muted)]">({t('col.baseline', 'baseline')})</span>
									{/if}
									{#if lastVerified(city)}
										<span class="block text-xs font-normal text-[var(--color-muted)]">
											{lastVerified(city)}
										</span>
									{/if}
								</span>
								<span class="font-medium text-[var(--color-text)]">
									{matrix.overall_index[city.id] != null
										? formatRatio(matrix.overall_index[city.id])
										: '—'}
								</span>
							</div>
						{/each}
					</div>
				</div>

				<!-- Category breakdown table -->
				<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[var(--color-border)] text-left text-xs text-[var(--color-muted)]">
								<th class="px-3 py-2 font-medium">{t('col.category', 'Category')}</th>
								{#each matrix.cities as city (city.id)}
									<th class="px-3 py-2 text-right font-medium">
										{#if city.geo_space_slug}
											<a
												href="/wiki/{city.geo_space_slug}/cost-of-living"
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
								<tr class="border-b border-[var(--color-border)] last:border-0">
									<td class="px-3 py-2 text-[var(--color-text)]">{cat.label}</td>
									{#each matrix.cities as city (city.id)}
										<td class="px-3 py-2 text-right text-[var(--color-text)]">
											<div class="font-medium">{formatValue(cat.values[city.id])}</div>
											{#if !city.is_baseline && cat.ratios[city.id]}
												<div class="text-xs text-[var(--color-muted)]">{formatRatio(cat.ratios[city.id])}</div>
											{/if}
										</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
					<p class="mt-2 text-xs text-[var(--color-muted)]">
						{t('col.amounts_currency', 'All amounts in {currency}').replace(
							'{currency}',
							displayCurrency
						)}
						{#if matrix.fx_fetched_at}
							{t('col.fx_note', '— converted at mid-market rates')}
						{/if}
					</p>
				</div>
			{/if}
		</div>
	{/if}
</div>
