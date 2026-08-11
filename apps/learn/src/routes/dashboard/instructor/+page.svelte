<script lang="ts">
	// Instructor dashboard — Japa University creator economy (ADR-063).
	// CSR ONLY: instructor profile, tracks, earnings, payouts.
	// LayerChart for revenue over time (ADR-059).
	import { LineChart } from 'layerchart';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { formatPriceKobo, formatCount } from '$lib/format';
	import { GraduationCap, DollarSign, Users, TrendingUp, PenLine, CheckCircle, Clock, XCircle } from 'lucide-svelte';
	import type { InstructorProfile, InstructorTrack, EarningsSummary, Payout } from '$lib/types';

	let profile = $state<InstructorProfile | null>(null);
	let tracks = $state<InstructorTrack[]>([]);
	let earnings = $state<EarningsSummary | null>(null);
	let payouts = $state<Payout[]>([]);
	let loading = $state(true);

	$effect(() => {
		(async () => {
			const [profileR, tracksR, earningsR, payoutsR] = await Promise.all([
				api.get<InstructorProfile>('/learn/university/instructor/me'),
				api.get<InstructorTrack[]>('/learn/university/my/tracks'),
				api.get<EarningsSummary>('/learn/university/my/earnings'),
				api.get<Payout[]>('/learn/university/my/payouts')
			]);
			loading = false;
			if (profileR.data) profile = profileR.data;
			if (tracksR.data) tracks = tracksR.data;
			if (earningsR.data) earnings = earningsR.data;
			if (payoutsR.data) payouts = payoutsR.data;
		})();
	});

	// Revenue chart data (dates → Date for LayerChart).
	const revenueData = $derived(
		(earnings?.revenue_series ?? []).map((d) => ({
			date: new Date(d.date),
			kobo: d.kobo / 100 // Convert to naira for display
		}))
	);

	function submissionStatusIcon(status: string) {
		switch (status) {
			case 'approved':
				return CheckCircle;
			case 'pending':
				return Clock;
			case 'rejected':
			case 'suspended':
				return XCircle;
			default:
				return PenLine;
		}
	}

	function submissionStatusColor(status: string): string {
		switch (status) {
			case 'approved':
				return 'var(--color-progress)';
			case 'pending':
				return 'var(--level-intermediate)';
			case 'rejected':
			case 'suspended':
				return 'var(--color-destructive)';
			default:
				return 'var(--color-muted)';
		}
	}
</script>

<svelte:head>
	<title>{t('university.dashboard')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<GraduationCap size={24} class="text-[var(--color-primary)]" />
			{t('university.dashboard')}
		</h1>
	</header>

	{#if loading}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
			{#each Array(3) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="mb-3 h-4 w-24" />
					<LoadingSkeleton class="h-8 w-16" />
				</div>
			{/each}
		</div>
	{:else if profile?.status === 'pending'}
		<EmptyState
			title={t('university.application_pending')}
			description={t('university.application_pending_description')}
		/>
	{:else if profile?.status === 'rejected'}
		<EmptyState
			title={t('university.application_rejected')}
			description={t('university.application_rejected_description')}
		>
			{#snippet action()}
				<a
					href="/dashboard/instructor/apply"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('university.apply')}
				</a>
			{/snippet}
		</EmptyState>
	{:else if !profile || profile.status !== 'approved'}
		<EmptyState
			title={t('university.become_instructor')}
			description={t('university.become_instructor_description')}
		>
			{#snippet action()}
				<a
					href="/dashboard/instructor/apply"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('university.apply')}
				</a>
			{/snippet}
		</EmptyState>
	{:else}
		<!-- Stats cards -->
		<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<Users size={16} class="text-[var(--color-primary)]" />
					{t('university.total_enrolled')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">{formatCount(profile.total_enrolled)}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<DollarSign size={16} class="text-[var(--level-intermediate)]" />
					{t('university.total_revenue')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">{formatPriceKobo(profile.total_revenue_kobo)}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<PenLine size={16} class="text-[var(--color-primary)]" />
					{t('university.my_tracks')}
				</div>
				<p class="mt-2 text-3xl font-bold text-[var(--color-text)]">{tracks.length}</p>
			</div>
		</div>

		<!-- Revenue over time (LayerChart) -->
		{#if revenueData.length > 0}
			<section class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
					<TrendingUp size={16} class="text-[var(--chart-1)]" />
					{t('university.revenue_over_time')}
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

		<!-- My tracks -->
		<section class="mt-6">
			<div class="mb-3 flex items-center justify-between">
				<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('university.my_tracks')}</h2>
			</div>
			{#if tracks.length === 0}
				<p class="text-sm text-[var(--color-muted)]">{t('common.empty')}</p>
			{:else}
				<ul class="space-y-3">
					{#each tracks as tr (tr.id)}
						{@const StatusIcon = submissionStatusIcon(tr.submission_status)}
						<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-start gap-3">
								{#if tr.thumbnail_url}
									<img src={tr.thumbnail_url} alt="" class="h-16 w-24 shrink-0 rounded-[var(--radius)] object-cover" />
								{/if}
								<div class="flex-1">
									<a href="/tracks/{tr.slug}" class="font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]">
										{tr.title}
									</a>
									<div class="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
										<span class="inline-flex items-center gap-1">
											<Users size={12} /> {t('learn.enrolled_count', '{count} enrolled').replace('{count}', formatCount(tr.total_enrolled))}
										</span>
										<span>{formatPriceKobo(tr.total_revenue_kobo)}</span>
									</div>
								</div>
								<div class="flex flex-col items-end gap-2">
									<span class="inline-flex items-center gap-1 text-xs font-medium" style="color: {submissionStatusColor(tr.submission_status)}">
										<StatusIcon size={14} />
										{t(`university.submission_${tr.submission_status}`)}
									</span>
									{#if tr.submission_status === 'draft' || tr.submission_status === 'rejected'}
										<button
											type="button"
											class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
											onclick={() => {
												(async () => {
													await api.post(`/learn/university/my/tracks/${tr.id}/submit`);
													goto('/dashboard/instructor');
												})();
											}}
										>
											{t('university.submit_course')}
										</button>
									{/if}
								</div>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<!-- Payouts -->
		{#if payouts.length > 0}
			<section class="mt-6">
				<h2 class="mb-3 text-lg font-semibold text-[var(--color-text)]">{t('university.payouts')}</h2>
				<ul class="space-y-2">
					{#each payouts as p (p.id)}
						<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-sm">
							<span class="text-[var(--color-text-subtle)]">{new Date(p.period_start).toLocaleDateString()} – {new Date(p.period_end).toLocaleDateString()}</span>
							<span class="font-medium text-[var(--color-text)]">{formatPriceKobo(p.amount_kobo)}</span>
							<span class="text-xs text-[var(--color-muted)]">{p.status}</span>
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	{/if}
</div>
