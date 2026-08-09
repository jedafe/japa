// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
<script lang="ts">
	import { t } from '$lib/i18n';
	import { Spline } from 'layerchart';
	import { CalendarClock, Filter, X } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { VisaProcessingStats, TrendPoint } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const stats = $derived<VisaProcessingStats | null>(data.stats);
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
	let fDest = $state(filters.dest);
	let fOrigin = $state(filters.origin);
	let fVisa = $state(filters.visa);
	let fConsulate = $state(filters.consulate);
	let fMonths = $state(filters.months);

	$effect(() => {
		fDest = filters.dest;
		fOrigin = filters.origin;
		fVisa = filters.visa;
		fConsulate = filters.consulate;
		fMonths = filters.months;
	});

	const hasFilters = $derived(
		!!(filters.dest || filters.origin || filters.visa || filters.consulate || filters.months)
	);

	const trendData = $derived(
		trend
			.filter((p) => p.avg_days != null)
			.map((p) => ({ period: new Date(p.period_start).getTime(), avg: p.avg_days as number }))
	);

	const statCards = $derived([
		{ key: 'avg_days', label: t('visa_timelines.avg_days', 'Average'), value: stats?.avg_days },
		{ key: 'median_days', label: t('visa_timelines.median_days', 'Median'), value: stats?.median_days },
		{ key: 'p10_days', label: t('visa_timelines.p10_days', 'Fastest (P10)'), value: stats?.p10_days },
		{ key: 'p90_days', label: t('visa_timelines.p90_days', 'Slowest (P90)'), value: stats?.p90_days },
		{ key: 'sample_count', label: t('visa_timelines.sample_count', 'Samples'), value: stats?.sample_count }
	]);

	function applyFilters(e: SubmitEvent) {
		e.preventDefault();
		const qs = new URLSearchParams();
		if (fDest.trim()) qs.set('dest', fDest.trim());
		if (fOrigin.trim()) qs.set('origin', fOrigin.trim());
		if (fVisa.trim()) qs.set('visa', fVisa.trim());
		if (fConsulate.trim()) qs.set('consulate', fConsulate.trim());
		if (fMonths.trim()) qs.set('months', fMonths.trim());
		const q = qs.toString();
		window.location.href = `/visa-timelines${q ? `?${q}` : ''}`;
	}

	function fmtDays(v: number | null | undefined): string {
		if (v == null) return '—';
		return `${Math.round(v)} ${t('visa_timelines.days', 'days')}`;
	}

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<CalendarClock size={20} /> {t('visa_timelines.heading', 'Visa Timeline Tracker')}
		</h1>
		{#if data.authed}
			<a
				href="/visa-timelines/my"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<CalendarClock size={14} /> {t('visa_timelines.track_mine', 'Track My Timeline')}
			</a>
		{:else}
			<a
				href="/login?redirect=/visa-timelines/my"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<CalendarClock size={14} /> {t('visa_timelines.track_mine', 'Track My Timeline')}
			</a>
		{/if}
	</div>

	<div class="space-y-5 p-4">
		<!-- Filter bar -->
		<form
			onsubmit={applyFilters}
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<div class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Filter size={15} /> {t('visa_timelines.filters', 'Filters')}
			</div>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				<div class="flex flex-col gap-1">
					<label for="f-dest" class="text-xs text-[var(--color-text-subtle)]">
						{t('visa_timelines.destination', 'Destination')}
					</label>
					<input id="f-dest" type="text" bind:value={fDest} class={inputClass} placeholder="e.g. GB" />
				</div>
				<div class="flex flex-col gap-1">
					<label for="f-origin" class="text-xs text-[var(--color-text-subtle)]">
						{t('visa_timelines.origin', 'Origin')}
					</label>
					<input id="f-origin" type="text" bind:value={fOrigin} class={inputClass} placeholder="e.g. NG" />
				</div>
				<div class="flex flex-col gap-1">
					<label for="f-visa" class="text-xs text-[var(--color-text-subtle)]">
						{t('visa_timelines.visa_category', 'Visa category')}
					</label>
					<input id="f-visa" type="text" bind:value={fVisa} class={inputClass} placeholder="e.g. student" />
				</div>
				<div class="flex flex-col gap-1">
					<label for="f-consulate" class="text-xs text-[var(--color-text-subtle)]">
						{t('visa_timelines.consulate', 'Consulate')}
					</label>
					<input id="f-consulate" type="text" bind:value={fConsulate} class={inputClass} placeholder="e.g. Lagos" />
				</div>
				<div class="flex flex-col gap-1">
					<label for="f-months" class="text-xs text-[var(--color-text-subtle)]">
						{t('visa_timelines.months', 'Months')}
					</label>
					<input id="f-months" type="number" min="1" bind:value={fMonths} class={inputClass} placeholder="e.g. 12" />
				</div>
			</div>
			<div class="mt-3 flex items-center justify-end gap-2">
				{#if hasFilters}
					<a
						href="/visa-timelines"
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]"
					>
						<X size={13} /> {t('common.clear', 'Clear')}
					</a>
				{/if}
				<button
					type="submit"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition hover:bg-[var(--color-primary-hover)]"
				>
					{t('common.filter', 'Filter')}
				</button>
			</div>
		</form>

		{#if data.loadError}
			<div class="rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 px-4 py-3 text-sm text-[var(--color-danger)]">
				{t('visa_timelines.load_error', 'Could not load statistics. Please try again later.')}
			</div>
		{:else if !stats || stats.sample_count === 0}
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
					{#if filters.dest}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{filters.dest}</span>{/if}
					{#if filters.origin}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{filters.origin}</span>{/if}
					{#if filters.visa}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{filters.visa}</span>{/if}
					{#if filters.consulate}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{filters.consulate}</span>{/if}
					{#if filters.months}<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{filters.months} {t('visa_timelines.months_label', 'mo')}</span>{/if}
				</div>
			{/if}

			<!-- Trend chart -->
			{#if trendData.length > 1}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
						{t('visa_timelines.trend_heading', 'Average processing time trend')}
					</h2>
					<div class="h-[200px]">
						<Spline
							data={trendData}
							x="period"
							y="avg"
							stroke="var(--color-primary)"
						/>
					</div>
				</div>
			{/if}
		{/if}
	</div>
</div>
