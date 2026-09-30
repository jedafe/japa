<script lang="ts">
	// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { LineChart } from 'layerchart';
	import { CalendarClock, X } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ListPageBar, FilterToggle, PageAction } from '$lib/components/ui';
	import { staticAutocompleteFetcher, autocompleteItems } from '$lib/autocomplete';
	import type { VisaProcessingStats, TrendPoint } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const stats = $derived<VisaProcessingStats[]>(data.stats);
	const trend = $derived<TrendPoint[]>(data.trend);
	const filters = $derived(data.filters);

	const title = $derived(t('visa_timelines.title', 'Visa Timeline Tracker — JapaConnect'));
	const description = $derived(
		t(
			'visa_timelines.description',
			'Community-powered visa processing time statistics. See how long visa applications take, filtered by destination, origin, visa category, and consulate.'
		)
	);

	// Local filter state — bound to inputs; the Apply button navigates with the
	// query string so SSR re-runs with the new filters.
	let fDest = $state(untrack(() => filters.dest));
	let fOrigin = $state(untrack(() => filters.origin));
	let fVisa = $state(untrack(() => filters.visa));
	let fConsulate = $state(untrack(() => filters.consulate));

	$effect(() => {
		fDest = filters.dest;
		fOrigin = filters.origin;
		fVisa = filters.visa;
		fConsulate = filters.consulate;
	});

	const hasFilters = $derived(!!(filters.dest || filters.origin || filters.visa || filters.consulate));

	let showFilters = $state(false);
	$effect(() => {
		if (hasFilters) showFilters = true;
	});

	// The stats endpoint returns an array of period rows (one per month
	// bucket). Derive the latest period for headline display, matching the
	// tools app /processing-tracker pattern.
	const latestStat = $derived.by<VisaProcessingStats | null>(() => {
		if (stats.length === 0) return null;
		return [...stats].sort(
			(a, b) => new Date(b.period_start).getTime() - new Date(a.period_start).getTime()
		)[0];
	});

	const totalSamples = $derived(stats.reduce((sum, s) => sum + s.sample_count, 0));

	const trendData = $derived(
		trend
			.filter((p) => p.avg_days != null)
			.map((p) => ({ period: new Date(p.period_start), avg: p.avg_days as number }))
	);

	const statCards = $derived([
		{ key: 'avg_days', label: t('visa_timelines.avg_days', 'Average'), value: latestStat?.avg_days },
		{ key: 'median_days', label: t('visa_timelines.median_days', 'Median'), value: latestStat?.median_days },
		{ key: 'p10_days', label: t('visa_timelines.p10_days', 'Fastest (P10)'), value: latestStat?.p10_days },
		{ key: 'p90_days', label: t('visa_timelines.p90_days', 'Slowest (P90)'), value: latestStat?.p90_days },
		{ key: 'sample_count', label: t('visa_timelines.sample_count', 'Samples'), value: totalSamples }
	]);

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		const qs = new URLSearchParams();
		if (fDest.trim()) qs.set('dest', fDest.trim());
		if (fOrigin.trim()) qs.set('origin', fOrigin.trim());
		if (fVisa.trim()) qs.set('visa', fVisa.trim());
		if (fConsulate.trim()) qs.set('consulate', fConsulate.trim());
		const q = qs.toString();
		goto(`/visa-timelines${q ? `?${q}` : ''}`);
	}

	function fmtDays(v: number | null | undefined): string {
		if (v == null) return '—';
		return `${Math.round(v)} ${t('visa_timelines.days', 'days')}`;
	}

	function fmtDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
		} catch {
			return iso;
		}
	}

	function countryName(id: string): string {
		return data.countries.find((c) => c.id === id)?.name ?? id;
	}

	// visa/consulate are free-text server filters — suggest values observed in
	// the loaded stats, and allow arbitrary input via allowFreeText.
	const visaFetcher = $derived(
		staticAutocompleteFetcher(autocompleteItems(stats.map((s) => s.visa_category)))
	);
	const consulateFetcher = $derived(
		staticAutocompleteFetcher(autocompleteItems(stats.map((s) => s.consulate ?? null)))
	);

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('visa_timelines.destination_all', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = (data.countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}
	const selectedDestinationLabel = $derived(
		fDest ? (data.countries ?? []).find((c) => c.id === fDest)?.name ?? null : null
	);

	function fetchOriginCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('visa_timelines.origin_all', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = (data.countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchOriginCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchOriginCountries('');
	}
	const selectedOriginLabel = $derived(
		fOrigin ? (data.countries ?? []).find((c) => c.id === fOrigin)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-4xl" style="--page-gutter: 1rem">
	<ListPageBar title={t('visa_timelines.heading', 'Visa Timeline Tracker')}>
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasFilters} controls="visa-timelines-filter-panel" />
			<PageAction
				href={data.authed ? '/visa-timelines/my' : '/login?redirect=/visa-timelines/my'}
				icon={CalendarClock}
				label={t('visa_timelines.track_mine', 'Track My Timeline')}
			/>
		{/snippet}
	</ListPageBar>

	<div class="space-y-5 p-4">
		{#if showFilters}
		<!-- Filter panel -->
		<form
			id="visa-timelines-filter-panel"
			onsubmit={applyFilters}
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
				<Autocomplete
					placeholder={t('visa_timelines.destination', 'Destination')}
					ariaLabel={t('visa_timelines.destination', 'Destination')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={fDest || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => { fDest = item.id; applyFilters(); }}
					onClear={() => { fDest = ''; applyFilters(); }}
				/>
				<Autocomplete
					placeholder={t('visa_timelines.origin', 'Origin')}
					ariaLabel={t('visa_timelines.origin', 'Origin')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchOriginCountries}
					initialFetcher={fetchOriginCountriesInitial}
					selectedId={fOrigin || null}
					selectedLabel={selectedOriginLabel}
					onSelect={(item) => { fOrigin = item.id; applyFilters(); }}
					onClear={() => { fOrigin = ''; applyFilters(); }}
				/>
				<Autocomplete
					placeholder={t('visa_timelines.visa_category', 'Visa category')}
					ariaLabel={t('visa_timelines.visa_category', 'Visa category')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={visaFetcher}
					initialFetcher={visaFetcher}
					selectedId={fVisa || null}
					selectedLabel={fVisa || null}
					allowFreeText
					onSelect={(item) => { fVisa = item.id; applyFilters(); }}
					onClear={() => { fVisa = ''; applyFilters(); }}
				/>
				<Autocomplete
					placeholder={t('visa_timelines.consulate', 'Consulate')}
					ariaLabel={t('visa_timelines.consulate', 'Consulate')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={consulateFetcher}
					initialFetcher={consulateFetcher}
					selectedId={fConsulate || null}
					selectedLabel={fConsulate || null}
					allowFreeText
					onSelect={(item) => { fConsulate = item.id; applyFilters(); }}
					onClear={() => { fConsulate = ''; applyFilters(); }}
				/>
			</div>
			{#if hasFilters}
				<div class="mt-3 flex items-center justify-end">
					<a
						href="/visa-timelines"
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]"
					>
						<X size={13} /> {t('common.clear', 'Clear')}
					</a>
				</div>
			{/if}
		</form>
		{/if}

		{#if data.loadError}
			<div class="rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 px-4 py-3 text-sm text-[var(--color-danger)]">
				{t('visa_timelines.load_error', 'Could not load statistics. Please try again later.')}
			</div>
		{:else if !latestStat}
			<EmptyState
				icon={CalendarClock}
				title={t('visa_timelines.empty_title', 'No data yet')}
				body={t(
					'visa_timelines.empty_body',
					'No visa timelines match these filters. Try broadening your search or contribute your own timeline.'
				)}
				action={{ label: t('visa_timelines.track_mine', 'Track My Timeline'), href: data.authed ? '/visa-timelines/my' : '/login?redirect=/visa-timelines/my' }}
			/>
		{:else}
			<!-- Stats cards -->
			<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
				{#each statCards as card (card.key)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<p class="text-xs text-[var(--color-text-subtle)]">{card.label}</p>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">
							{card.key === 'sample_count' ? card.value ?? 0 : fmtDays(card.value)}
						</p>
					</div>
				{/each}
			</div>

			<!-- Active filter summary -->
			{#if hasFilters}
				<div class="flex flex-wrap items-center gap-2 text-xs text-[var(--color-text-subtle)]">
					<span>{t('visa_timelines.showing_for', 'Showing data for')}:</span>
					{#if filters.dest}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{countryName(filters.dest)}</span>{/if}
					{#if filters.origin}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{countryName(filters.origin)}</span>{/if}
					{#if filters.visa}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{filters.visa}</span>{/if}
					{#if filters.consulate}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{filters.consulate}</span>{/if}
				</div>
			{/if}

			<!-- All periods table -->
			{#if stats.length > 1}
				<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[var(--color-border)] text-left text-xs text-[var(--color-text-subtle)]">
								<th class="py-2 pr-4 font-medium">{t('visa_timelines.period', 'Period')}</th>
								<th class="py-2 pr-4 font-medium">{t('visa_timelines.avg_days', 'Average')}</th>
								<th class="py-2 pr-4 font-medium">{t('visa_timelines.median_days', 'Median')}</th>
								<th class="py-2 pr-4 font-medium">{t('visa_timelines.p10_days', 'p10')}</th>
								<th class="py-2 pr-4 font-medium">{t('visa_timelines.p90_days', 'p90')}</th>
								<th class="py-2 font-medium">{t('visa_timelines.sample_count', 'Samples')}</th>
							</tr>
						</thead>
						<tbody>
							{#each [...stats].sort((a, b) => new Date(b.period_start).getTime() - new Date(a.period_start).getTime()) as s (s.period_start + (s.consulate ?? ''))}
								<tr class="border-b border-[var(--color-border)]/50">
									<td class="py-2 pr-4 text-[var(--color-text)]">{fmtDate(s.period_start)}</td>
									<td class="py-2 pr-4 text-[var(--color-text)]">{fmtDays(s.avg_days)}</td>
									<td class="py-2 pr-4 text-[var(--color-text)]">{fmtDays(s.median_days)}</td>
									<td class="py-2 pr-4 text-[var(--color-success)]">{fmtDays(s.p10_days)}</td>
									<td class="py-2 pr-4 text-[var(--color-warning)]">{fmtDays(s.p90_days)}</td>
									<td class="py-2 text-[var(--color-text-subtle)]">{s.sample_count}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

			<!-- Trend chart -->
			{#if trendData.length > 1}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
						{t('visa_timelines.trend_heading', 'Average processing time trend')}
					</h2>
					<div class="h-[200px]">
						<LineChart
							data={trendData}
							x="period"
							y="avg"
							props={{ spline: { stroke: 'var(--color-primary)' } }}
						/>
					</div>
				</div>
			{/if}
		{/if}
	</div>
</div>
