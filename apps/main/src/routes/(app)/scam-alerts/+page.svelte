// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { TriangleAlert, Plus, Pin, Search, X } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { i18n } from '$lib/i18n';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import type { ScamReport, ScamType, ScamStatus } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const SCAM_TYPES: ScamType[] = [
		'fraudulent_agent',
		'fake_job',
		'housing_fraud',
		'fake_scholarship',
		'whatsapp_telegram',
		'other'
	];

	let alerts = $state<ScamReport[]>(untrack(() => data.alerts));
	let atEnd = $state(false);
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);

	// Filter form state — seeded from SSR query params.
	let destFilter = $state(untrack(() => data.filters.dest));
	let typeFilter = $state(untrack(() => data.filters.type));

	// Re-seed on a new SSR payload (e.g. after filter navigation).
	$effect(() => {
		const seed = data.alerts;
		untrack(() => {
			alerts = seed;
			atEnd = seed.length < data.pageSize;
			loading = false;
			destFilter = data.filters.dest;
			typeFilter = data.filters.type;
		});
	});

	const title = $derived(t('scam_alerts.list_title', 'Scam Alerts — JapaConnect'));
	const description = $derived(
		t(
			'scam_alerts.list_description',
			'Community-reported scam alerts to help the diaspora stay safe.'
		)
	);

	function cursor(): string | null {
		const last = alerts[alerts.length - 1];
		return last ? last.created_at : null;
	}

	$effect(() => {
		const el = sentinel;
		if (!el || atEnd) return;
		const obs = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) loadMore();
			},
			{ rootMargin: '600px' }
		);
		obs.observe(el);
		return () => obs.disconnect();
	});

	async function loadMore() {
		if (loading || atEnd) return;
		const c = cursor();
		if (!c) return;
		loading = true;
		const params = new URLSearchParams({ page_size: String(data.pageSize), cursor_created_at: c });
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (data.filters.type) params.set('type', data.filters.type);
		const res = await api.get<ScamReport[]>(`/scam-alerts?${params.toString()}`);
		if (res.error) {
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		alerts = [...alerts, ...batch];
		atEnd = batch.length < data.pageSize;
		loading = false;
	}

	function applyFilters(e: SubmitEvent) {
		e.preventDefault();
		const params = new URLSearchParams();
		if (destFilter.trim()) params.set('dest', destFilter.trim());
		if (typeFilter) params.set('type', typeFilter);
		const qs = params.toString();
		window.location.href = qs ? `/scam-alerts?${qs}` : '/scam-alerts';
	}

	function clearFilters() {
		destFilter = '';
		typeFilter = '';
		window.location.href = '/scam-alerts';
	}

	const hasFilters = $derived(!!data.filters.dest || !!data.filters.type);

	function scamTypeLabel(type: ScamType): string {
		return t(`scam_alerts.type.${type}`, type.replace(/_/g, ' '));
	}

	function statusClass(status: ScamStatus): string {
		if (status === 'confirmed') return 'bg-red-500/10 text-red-600';
		if (status === 'under_review') return 'bg-amber-500/10 text-amber-600';
		return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
	}

	function statusLabel(status: ScamStatus): string {
		return t(`scam_alerts.status.${status}`, status.replace(/_/g, ' '));
	}

	function excerpt(text: string, max = 180): string {
		if (text.length <= max) return text;
		return text.slice(0, max).trimEnd() + '…';
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('scam_alerts.heading', 'Scam Alerts')}
		</h1>
		<a
			href="/scam-alerts/new"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			<Plus size={14} /> {t('scam_alerts.report_button', 'Report a Scam')}
		</a>
	</div>

	<!-- Filter bar -->
	<form onsubmit={applyFilters} class="flex flex-wrap items-end gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<div class="flex flex-col gap-1">
			<label for="filter-dest" class="text-xs font-medium text-[var(--color-text-subtle)]">
				{t('scam_alerts.filter_destination', 'Destination country')}
			</label>
			<input
				id="filter-dest"
				type="text"
				value={destFilter}
				oninput={(e) => (destFilter = e.currentTarget.value)}
				placeholder={t('scam_alerts.filter_destination_placeholder', 'e.g. UK')}
				class="w-40 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>
		<div class="flex flex-col gap-1">
			<label for="filter-type" class="text-xs font-medium text-[var(--color-text-subtle)]">
				{t('scam_alerts.filter_type', 'Scam type')}
			</label>
			<select
				id="filter-type"
				value={typeFilter}
				onchange={(e) => (typeFilter = e.currentTarget.value)}
				class="w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
			>
				<option value="">{t('scam_alerts.filter_all_types', 'All types')}</option>
				{#each SCAM_TYPES as st (st)}
					<option value={st}>{scamTypeLabel(st)}</option>
				{/each}
			</select>
		</div>
		<button
			type="submit"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			<Search size={14} /> {t('common.filter', 'Filter')}
		</button>
		{#if hasFilters}
			<button
				type="button"
				onclick={clearFilters}
				class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				<X size={13} /> {t('common.clear_filters', 'Clear filters')}
			</button>
		{/if}
	</form>

	<!-- List -->
	<div class="p-3">
		{#if data.loadError}
			<EmptyState
				icon={TriangleAlert}
				title={t('scam_alerts.load_error_title', 'Failed to load alerts')}
				body={t('scam_alerts.load_error_body', 'Please try again later.')}
			/>
		{:else if alerts.length === 0}
			<EmptyState
				icon={TriangleAlert}
				title={t('scam_alerts.empty_title', 'No scam alerts yet')}
				body={t('scam_alerts.empty_body', 'Be the first to report a scam and help protect the community.')}
				action={{ label: t('scam_alerts.report_button', 'Report a Scam'), href: '/scam-alerts/new' }}
			/>
		{:else}
			<div class="space-y-3">
				{#each alerts as alert (alert.id)}
					<a
						href="/scam-alerts/{alert.id}"
						class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40"
					>
						<div class="flex flex-wrap items-center gap-2">
							<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-semibold text-[var(--color-primary)]">
								{scamTypeLabel(alert.scam_type as ScamType)}
							</span>
							{#if alert.is_pinned}
								<span class="inline-flex items-center gap-0.5 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-semibold text-amber-600">
									<Pin size={11} /> {t('scam_alerts.pinned', 'Pinned')}
								</span>
							{/if}
							<span class={`rounded-full px-2 py-0.5 text-xs font-semibold ${statusClass(alert.status as ScamStatus)}`}>
								{statusLabel(alert.status as ScamStatus)}
							</span>
							<span class="ml-auto text-xs text-[var(--color-muted)]">
								{relativeTime(alert.created_at, new Date(), i18n.locale)}
							</span>
						</div>

						<p class="mt-2 text-sm text-[var(--color-text)]">{excerpt(alert.description)}</p>

						<div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--color-text-subtle)]">
							{#if alert.destination_space_name}
								<span>{t('scam_alerts.destination', 'Destination')}: <span class="font-medium text-[var(--color-text)]">{alert.destination_space_name}</span></span>
							{/if}
							{#if alert.platform_used}
								<span>{t('scam_alerts.platform', 'Platform')}: <span class="font-medium text-[var(--color-text)]">{alert.platform_used}</span></span>
							{/if}
							{#if alert.approximate_date}
								<span>{t('scam_alerts.date', 'Date')}: <span class="font-medium text-[var(--color-text)]">{alert.approximate_date}</span></span>
							{/if}
							{#if alert.amount_range}
								<span>{t('scam_alerts.amount', 'Amount')}: <span class="font-medium text-[var(--color-text)]">{alert.amount_range}</span></span>
							{/if}
						</div>
					</a>
				{/each}
			</div>

			{#if !atEnd}
				<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
				{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
			{:else}
				<p class="py-6 text-center text-xs text-[var(--color-muted)]">
					{t('feed.end', "You're all caught up")}
				</p>
			{/if}
		{/if}
	</div>
</div>
