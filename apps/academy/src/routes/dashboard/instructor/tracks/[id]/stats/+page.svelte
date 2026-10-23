<script lang="ts">
	// Per-track stats — Japa Academy creator economy (ADR-063).
	// CSR ONLY: fetch stats on mount via $effect.
	// LayerChart for revenue & enrollment over time (ADR-059).
	import { LineChart } from 'layerchart';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { formatPriceKobo, formatCount } from '$lib/format';
	import { TrendingUp, Users, DollarSign, ArrowLeft, BarChart3 } from 'lucide-svelte';
	import type { InstructorTrackStats } from '$lib/types';

	const trackId = page.params.id;

	let stats = $state<InstructorTrackStats | null>(null);
	let loading = $state(true);
	let error = $state(false);

	async function loadStats() {
		loading = true;
		error = false;
		const res = await api.get<InstructorTrackStats>(
			`/academy/my/tracks/${trackId}/stats`
		);
		loading = false;
		if (res.data) {
			stats = res.data;
		} else {
			error = true;
		}
	}

	$effect(() => {
		loadStats();
	});

	// Revenue chart data (dates → Date, kobo → naira for display).
	const revenueData = $derived(
		(stats?.revenue_series ?? []).map((d) => ({
			date: new Date(d.date),
			kobo: d.kobo / 100
		}))
	);

	// Enrollment chart data (dates → Date, count stays as-is).
	const enrollmentData = $derived(
		(stats?.enrollment_series ?? []).map((d) => ({
			date: new Date(d.date),
			count: d.count
		}))
	);
</script>

<svelte:head>
	<title>{t('academy.track_stats')} · {t('app.name')}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<!-- Back link -->
	<a
		href="/dashboard/instructor"
		class="mb-4 inline-flex items-center gap-1 text-sm text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
	>
		<ArrowLeft class="rtl:-scale-x-100" size={16} />
		{t('common.back')}
	</a>

	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<BarChart3 size={24} class="text-[var(--color-primary)]" />
			{t('academy.track_stats')}
		</h1>
	</header>

	{#if loading}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
			{#each Array(3) as _}
				<div
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				>
					<LoadingSkeleton class="mb-3 h-4 w-24" />
					<LoadingSkeleton class="h-8 w-16" />
				</div>
			{/each}
		</div>
		<div
			class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<LoadingSkeleton class="mb-3 h-4 w-32" />
			<LoadingSkeleton class="h-64 w-full" />
		</div>
	{:else if error || !stats}
		<EmptyState title={t('common.error')} description={t('academy.track_stats_unavailable')}>
			{#snippet action()}
				<button
					type="button"
					onclick={loadStats}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('common.retry')}
				</button>
			{/snippet}
		</EmptyState>
	{:else}
		<!-- Stat cards -->
		<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
			<div
				class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<Users size={16} class="text-[var(--color-primary)]" />
					{t('academy.total_enrolled')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">
					{formatCount(stats.total_enrolled)}
				</p>
			</div>
			<div
				class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<BarChart3 size={16} class="text-[var(--color-primary)]" />
					{t('academy.completion_rate')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">
					{stats.completion_rate_pct.toFixed(1)}%
				</p>
			</div>
			<div
				class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<DollarSign size={16} class="text-[var(--chart-1)]" />
					{t('academy.total_revenue')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">
					{formatPriceKobo(stats.total_revenue_kobo, stats.currency, page.data.locale, page.data.currencies)}
				</p>
			</div>
		</div>

		<!-- Revenue over time (LayerChart) -->
		{#if revenueData.length > 0}
			<section
				class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
					<TrendingUp size={16} class="text-[var(--chart-1)]" />
					{t('academy.revenue_over_time')}
				</h2>
				<div class="h-64 w-full">
					<LineChart
						data={revenueData}
						x="date"
						y="kobo"
						props={{ spline: { class: 'stroke-[var(--chart-1)]' } }}
						tooltipContext
					/>
				</div>
			</section>
		{/if}

		<!-- Enrollment over time (LayerChart) -->
		{#if enrollmentData.length > 0}
			<section
				class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
					<Users size={16} class="text-[var(--chart-2)]" />
					{t('academy.enrollment_over_time')}
				</h2>
				<div class="h-64 w-full">
					<LineChart
						data={enrollmentData}
						x="date"
						y="count"
						props={{ spline: { class: 'stroke-[var(--chart-2)]' } }}
						tooltipContext
					/>
				</div>
			</section>
		{/if}
	{/if}
</div>
