<script lang="ts">
	// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { TriangleAlert, Plus, Pin, X, Check, Bell } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { relativeTime } from '$lib/feed/time';
	import { i18n } from '$lib/i18n';
	import { scamTypeLabel, scamPlatformLabel } from '$lib/enumOptions';
	import { EmptyState, LoadingSkeleton, ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import type { ScamReport, ScamAlertSubscription, ScamType, ScamPlatform, ScamStatus } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let alerts = $state<ScamReport[]>(untrack(() => data.alerts));
	let atEnd = $state(false);
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);
	let currentPage = $state(1);

	// Filter form state — seeded from SSR query params.
	let destFilter = $state(untrack(() => data.filters.dest));

	// Re-seed on a new SSR payload (e.g. after filter navigation).
	$effect(() => {
		const seed = data.alerts;
		untrack(() => {
			alerts = seed;
			atEnd = seed.length < data.pageSize;
			loading = false;
			currentPage = 1;
			destFilter = data.filters.dest;
		});
	});

	const title = $derived(t('scam_alerts.list_title', 'Scam Alerts — {site}'));
	const description = $derived(
		t(
			'scam_alerts.list_description',
			'Community-reported scam alerts to help the diaspora stay safe.'
		)
	);

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
		loading = true;
		const nextPage = currentPage + 1;
		const params = new URLSearchParams({
			page: String(nextPage),
			limit: String(data.pageSize)
		});
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (data.filters.scam_type) params.set('scam_type', data.filters.scam_type);
		const res = await api.get<ScamReport[]>(`/scam-alerts?${params.toString()}`);
		if (res.error) {
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		alerts = [...alerts, ...batch];
		currentPage = nextPage;
		atEnd = batch.length < data.pageSize;
		loading = false;
	}

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		const params = new URLSearchParams();
		if (destFilter) params.set('dest', destFilter);
		if (data.filters.scam_type) params.set('scam_type', data.filters.scam_type);
		const qs = params.toString();
		goto(qs ? `/scam-alerts?${qs}` : '/scam-alerts');
	}

	function clearFilters() {
		destFilter = '';
		goto('/scam-alerts');
	}

	const activeTab = $derived(data.filters.scam_type || 'all');
	const hasFilters = $derived(!!data.filters.dest);
	const hasActiveFilters = $derived(!!data.filters.dest || activeTab !== 'all');

	const tabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('scam_alerts.filter_all_types', 'All types') },
		...(data.scamTypes ?? []).map((v) => ({ value: v, label: scamTypeLabel(t, v) }))
	]);

	let showFilters = $state(false);
	let subscribingDest = $state(false);
	let mySubscriptions = $state<ScamAlertSubscription[]>(untrack(() => data.mySubscriptions) ?? []);
	const destSubscribed = $derived(
		mySubscriptions.some((sub) => sub.destination_space_id === destFilter)
	);

	async function subscribeDest() {
		if (!destFilter || subscribingDest) return;
		subscribingDest = true;
		const res = destSubscribed
			? await api.delete(`/me/scam-alert-subscriptions/${destFilter}`)
			: await api.post('/me/scam-alert-subscriptions', { destination_space_id: destFilter });
		if (!res.error) {
			mySubscriptions = destSubscribed
				? mySubscriptions.filter((sub) => sub.destination_space_id !== destFilter)
				: [...mySubscriptions, { destination_space_id: destFilter, created_at: new Date().toISOString() }];
		}
		subscribingDest = false;
	}
	let submittedDismissed = $state(false);
	const showSubmitted = $derived(data.submitted && !submittedDismissed);
	$effect(() => {
		if (hasFilters) showFilters = true;
	});

	// ?submitted=1 is a one-shot flag — strip it from the URL once the banner
	// is acknowledged so a refresh/back-nav does not resurrect it.
	$effect(() => {
		if (!showSubmitted && data.submitted) {
			const url = new URL(page.url);
			url.searchParams.delete('submitted');
			history.replaceState(history.state, '', url);
		}
	});

	function selectType(v: string) {
		const params = new URLSearchParams();
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (v !== 'all') params.set('scam_type', v);
		const qs = params.toString();
		goto(qs ? `/scam-alerts?${qs}` : '/scam-alerts');
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

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('scam_alerts.filter_all_countries', 'All countries');
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
		destFilter ? (data.countries ?? []).find((c) => c.id === destFilter)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<h1 class="sr-only">{t('scam_alerts.heading', 'Scam Alerts')}</h1>
	<ListPageBar>
		<UrlTabs
			{tabs}
			value={activeTab}
			onValueChange={(v) => selectType(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			{#if data.isAuthed}
				<a
					href="/scam-alerts/mine"
					class="rounded-[var(--radius)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
				>{t('scam_alerts.my_reports', 'My reports')}</a>
			{/if}
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="scam-alerts-filter-panel" />
			<PageAction
				href="/scam-alerts/new"
				icon={Plus}
				label={t('scam_alerts.report_button', 'Report a Scam')}
			/>
		{/snippet}
	</ListPageBar>

	<!-- List -->
	<div class="mx-auto w-full max-w-3xl p-4">
		{#if showSubmitted}
			<div class="mb-4 rounded-[var(--radius)] border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-700">
				<div class="flex items-start gap-2">
					<Check size={16} class="mt-0.5 shrink-0" />
					<div class="flex-1">
						<p class="font-medium">{t('scam_alerts.submitted_title', 'Report submitted')}</p>
						<p class="opacity-90">{t('scam_alerts.submitted_body', 'Your report is under review and will appear once a moderator confirms it.')}</p>
					</div>
					<button
						type="button"
						aria-label={t('common.dismiss', 'Dismiss')}
						onclick={() => (submittedDismissed = true)}
						class="shrink-0 text-emerald-700/70 hover:text-emerald-700"
					>
						<X size={14} />
					</button>
				</div>
			</div>
		{/if}

		{#if showFilters}
			<!-- Filter panel -->
			<div id="scam-alerts-filter-panel" class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<form onsubmit={applyFilters} class="flex flex-wrap items-center gap-3">
		<div class="w-52">
			<Autocomplete
				placeholder={t('scam_alerts.filter_destination', 'Destination country')}
				ariaLabel={t('scam_alerts.filter_destination', 'Destination country')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchDestinationCountries}
				initialFetcher={fetchDestinationCountriesInitial}
				selectedId={destFilter || null}
				selectedLabel={selectedDestinationLabel}
				onSelect={(item) => { destFilter = item.id; applyFilters(); }}
				onClear={() => { destFilter = ''; applyFilters(); }}
			/>
		</div>
		{#if data.isAuthed && destFilter}
			<button
				type="button"
				disabled={subscribingDest}
				onclick={subscribeDest}
				class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				<Bell size={13} />
				{destSubscribed ? t('scam_alerts.unsubscribe_dest', 'Unsubscribe from alerts') : t('scam_alerts.subscribe_dest', 'Get alerts for this destination')}
			</button>
		{/if}
		{#if hasActiveFilters}
			<button
				type="button"
				onclick={clearFilters}
				class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				<X size={13} /> {t('common.clear_filters', 'Clear filters')}
			</button>
		{/if}
	</form>
			</div>
		{/if}

		{#if data.loadError}
			<EmptyState
				icon={TriangleAlert}
				title={t('scam_alerts.load_error_title', 'Failed to load alerts')}
				body={t('scam_alerts.load_error_body', 'Please try again later.')}
			/>
		{:else if alerts.length === 0}
			{#if hasActiveFilters}
				<EmptyState
					icon={TriangleAlert}
					title={t('scam_alerts.empty_filtered_title', 'No alerts match your filters')}
					body={t('scam_alerts.empty_filtered_body', 'Try a different destination or scam type.')}
					action={{ label: t('common.clear_filters', 'Clear filters'), onclick: clearFilters }}
				/>
			{:else}
				<EmptyState
					icon={TriangleAlert}
					title={t('scam_alerts.empty_title', 'No scam alerts yet')}
					body={t('scam_alerts.empty_body', 'Be the first to report a scam and help protect the community.')}
					action={{ label: t('scam_alerts.report_button', 'Report a Scam'), href: '/scam-alerts/new' }}
				/>
			{/if}
		{:else}
			<div class="space-y-3">
				{#each alerts as alert (alert.id)}
					<a
						href="/scam-alerts/{alert.id}"
						class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40"
					>
						<div class="flex flex-wrap items-center gap-2">
							<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-semibold text-[var(--color-primary)]">
								{scamTypeLabel(t, alert.scam_type as ScamType)}
							</span>
							{#if alert.is_pinned}
								<span class="inline-flex items-center gap-0.5 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-semibold text-amber-600">
									<Pin size={11} /> {t('scam_alerts.pinned', 'Pinned')}
								</span>
							{/if}
							<span class={`rounded-full px-2 py-0.5 text-xs font-semibold ${statusClass(alert.status as ScamStatus)}`}>
								{statusLabel(alert.status as ScamStatus)}
							</span>
							<span class="ms-auto text-xs text-[var(--color-muted)]">
								{relativeTime(alert.created_at, new Date(), i18n.locale)}
							</span>
						</div>

						<p class="mt-2 text-sm text-[var(--color-text)]">{excerpt(alert.description)}</p>

						<div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--color-text-subtle)]">
							{#if alert.destination_space_name}
								<span>{t('scam_alerts.destination', 'Destination')}: <span class="font-medium text-[var(--color-text)]">{alert.destination_space_name}</span></span>
							{/if}
							{#if alert.platform_used}
								<span>{t('scam_alerts.platform', 'Platform')}: <span class="font-medium text-[var(--color-text)]">{scamPlatformLabel(t, alert.platform_used as ScamPlatform)}</span></span>
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
