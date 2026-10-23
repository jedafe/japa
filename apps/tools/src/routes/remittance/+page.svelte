<script lang="ts">
	import { untrack, onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { mainAppUrl } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { formatMoney, currencyLabel, currencySymbol } from '$lib/utils/currency';
	import { LineChart } from 'layerchart';
	import type {
		RemittanceCorridor,
		RemittanceProvider,
		RemittanceRate,
		RemittanceAlert,
		RemittanceHistoryPoint,
		RemittanceCalculation,
		CatalogOption
	} from '$lib/types';

	/**
	 * Remittance Rate Tracker tool — §74, ADR-075.
	 * Corridor selector → provider rate table → trend chart (LayerChart) →
	 * calculator widget → rate alerts (auth only).
	 */
	let { data } = $props();

	let corridors = $state<RemittanceCorridor[]>(untrack(() => data.corridors));
	let providers = $state<RemittanceProvider[]>(untrack(() => data.providers));
	let rates = $state<RemittanceRate[]>(untrack(() => data.rates));
	let alerts = $state<RemittanceAlert[]>(untrack(() => data.alerts));
	let user = $state(untrack(() => data.user));

	let selectedCorridor = $state<RemittanceCorridor>(untrack(() => data.defaultCorridor));
	let directionOptions = $state<CatalogOption[]>(untrack(() => data.directionOptions));
	let history = $state<RemittanceHistoryPoint[]>([]);
	let loadingRates = $state(false);
	let loadingHistory = $state(false);

	// Calculator state
	let calcSendAmount = $state(1000);
	let calcResult = $state<RemittanceCalculation | null>(null);
	let calcLoading = $state(false);
	// Debounce timer for the calculator input — prevents an API call per
	// keystroke (§4.1 performance). The timer is cleaned up on component
	// destroy via the $effect cleanup.
	let calcTimer: ReturnType<typeof setTimeout> | null = null;

	// Alert creation state
	let alertTargetRate = $state('');
	let alertDirection = $state(untrack(() => data.directionOptions[0]?.value ?? 'above'));
	let showRegisterPrompt = $state(false);
	let alertError = $state<string | null>(null);
	let ratesError = $state<string | null>(null);

	const seo = $derived(
		buildSeoMeta({
			title: t('remittance.seo_title', 'Remittance Rate Tracker — {site} Tools'),
			description: t(
				'remittance.seo_description',
				'Compare live remittance rates across providers, view historical trends, calculate transfer costs, and set rate alerts.'
			),
			path: '/remittance',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-remittance.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'SoftwareApplication',
				name: 'Remittance Rate Tracker',
				applicationCategory: 'FinanceApplication',
				operatingSystem: 'Web',
				offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
				description: 'Compare live remittance rates across providers with historical trends and alerts.'
			}
		})
	);

	// Best rate — §112: benchmark rows are a mid-market reference, never a
	// sellable quote, so they can't win the best-rate highlight (ADR-125).
	const bestRate = $derived.by(() => {
		const sellable = rates.filter((r) => !r.is_benchmark);
		if (sellable.length === 0) return null;
		return sellable.reduce((best, r) => (r.rate > best.rate ? r : best));
	});

	// Rates render sorted: sellable quotes best-rate-first, mid-market
	// benchmark rows pinned last as a reference (§112, ADR-125).
	const sortedRates = $derived.by(() => {
		const sellable = rates.filter((r) => !r.is_benchmark).sort((a, b) => b.rate - a.rate);
		const benchmark = rates.filter((r) => r.is_benchmark).sort((a, b) => b.rate - a.rate);
		return [...sellable, ...benchmark];
	});

	async function loadRates() {
		loadingRates = true;
		ratesError = null;
		const res = await api.get<RemittanceRate[]>(
			`/remittance/rates?send=${selectedCorridor.send_currency}&receive=${selectedCorridor.receive_currency}`
		);
		loadingRates = false;
		if (res.error) {
			rates = [];
			ratesError = res.error.message;
		} else {
			rates = res.data ?? [];
		}
		loadHistory();
		calculate();
	}

	async function loadHistory() {
		loadingHistory = true;
		const res = await api.get<RemittanceHistoryPoint[]>(
			`/remittance/history?send=${selectedCorridor.send_currency}&receive=${selectedCorridor.receive_currency}&days=30`
		);
		loadingHistory = false;
		if (!res.error) history = res.data ?? [];
	}

	async function calculate() {
		if (!Number.isFinite(calcSendAmount) || calcSendAmount <= 0) {
			calcResult = null;
			return;
		}
		calcLoading = true;
		const res = await api.get<RemittanceCalculation>(
			`/remittance/calculate?send_amount=${calcSendAmount}&send=${selectedCorridor.send_currency}&receive=${selectedCorridor.receive_currency}`
		);
		calcLoading = false;
		if (!res.error) calcResult = res.data;
	}

	/** Debounced calculate — waits 400ms after the last keystroke before
	 *  hitting the API (§4.1 performance — no API call per keystroke). */
	function debouncedCalculate() {
		if (calcTimer) clearTimeout(calcTimer);
		calcTimer = setTimeout(() => calculate(), 400);
	}

	async function createAlert() {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const targetRate = parseFloat(alertTargetRate);
		if (!(targetRate > 0)) return;
		alertError = null;
		const res = await api.post<RemittanceAlert>('/remittance/alerts', {
			send_currency: selectedCorridor.send_currency,
			receive_currency: selectedCorridor.receive_currency,
			target_rate: targetRate,
			direction: alertDirection as 'above' | 'below'
		});
		if (res.error) {
			alertError = res.error.message;
		} else if (res.data) {
			alerts = [...alerts, res.data];
			alertTargetRate = '';
		}
	}

	async function deleteAlert(id: string) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		alertError = null;
		const res = await api.delete(`/remittance/alerts/${id}`);
		if (res.error) {
			alertError = res.error.message;
		} else {
			alerts = alerts.filter((a) => a.id !== id);
		}
	}

	/** Toggle an alert's active state via PATCH (§74). */
	async function toggleAlertActive(id: string, currentActive: boolean) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		alertError = null;
		const res = await api.patch<RemittanceAlert>(`/remittance/alerts/${id}`, {
			is_active: !currentActive
		});
		if (res.error) {
			alertError = res.error.message;
		} else {
			alerts = alerts.map((a) => (a.id === id ? { ...a, is_active: !currentActive } : a));
		}
	}

	/** Edit an alert's target rate and direction via PATCH (§74). */
	let editingAlertId = $state<string | null>(null);
	let editTargetRate = $state('');
	let editDirection = $state('above');

	function startEditAlert(alert: RemittanceAlert) {
		editingAlertId = alert.id;
		editTargetRate = String(alert.target_rate);
		editDirection = alert.direction;
	}

	async function saveAlertEdit(id: string) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const targetRate = parseFloat(editTargetRate);
		if (!(targetRate > 0)) return;
		alertError = null;
		const res = await api.patch<RemittanceAlert>(`/remittance/alerts/${id}`, {
			target_rate: targetRate,
			direction: editDirection as 'above' | 'below'
		});
		if (res.error) {
			alertError = res.error.message;
			return;
		}
		editingAlertId = null;
		alerts = alerts.map((a) =>
			a.id === id
				? { ...a, target_rate: targetRate, direction: editDirection as 'above' | 'below' }
				: a
		);
	}

	// Corridor rates span orders of magnitude (GBP→NGN ~1500 vs inverse
	// corridors below 1) — a fixed toFixed(2) renders small rates as "0.00".
	function formatRate(rate: number): string {
		if (!Number.isFinite(rate)) return '—';
		if (rate >= 100) return rate.toFixed(2);
		if (rate >= 1) return rate.toFixed(4);
		return rate.toPrecision(4);
	}

	function directionLabel(v: string): string {
		const opt = directionOptions.find((o) => o.value === v);
		return t(`remittance.direction_${v}`, opt?.label ?? v);
	}

	// Alert direction picker — every dropdown on this page uses Autocomplete.
	function fetchDirectionOptions(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return Promise.resolve(
			directionOptions
				.filter((o) => query === '' || directionLabel(o.value).toLowerCase().includes(query))
				.map((o) => ({ id: o.value, label: directionLabel(o.value) }))
		);
	}
	function fetchDirectionOptionsInitial(): Promise<AutocompleteItem[]> {
		return fetchDirectionOptions('');
	}

	// §58 — managed currency symbols via the shared util (page.data.currencies
	// comes from the root layout's GET /currencies fetch).
	function fmtMoney(amount: number, currency: string): string {
		return formatMoney(amount, currency, data.currencies, { fractionDigits: 2 });
	}

	/**
	 * Manual-rate staleness threshold from system_settings (Rule 3 — never
	 * hardcode config values). Seeded from the SSR load which reads
	 * remittance.manual_stale_hours (default 24h).
	 * Ref: conventions/system-settings-catalog.md (remittance.manual_stale_hours).
	 */
	const manualStaleHours = untrack(() => data.manualStaleHours);
	function isManualRateStale(rate: RemittanceRate): boolean {
		if (!rate.is_manual || !rate.fetched_at) return false;
		const ageHours = (Date.now() - new Date(rate.fetched_at).getTime()) / (1000 * 60 * 60);
		return ageHours > manualStaleHours;
	}

	/** Look up a provider by ID for logo/website display (§74). */
	function getProvider(providerId: string): RemittanceProvider | undefined {
		return providers.find((p) => p.id === providerId);
	}

	function onCorridorChange(value: string) {
		const corridor = corridors.find(
			(c) => `${c.send_currency}-${c.receive_currency}` === value
		);
		if (corridor) {
			selectedCorridor = corridor;
			loadRates();
		}
	}

	function corridorId(c: RemittanceCorridor): string {
		return `${c.send_currency}-${c.receive_currency}`;
	}

	function fetchCorridors(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? corridors
			: corridors.filter((c) => getCorridorLabel(corridorId(c)).toLowerCase().includes(query));
		return Promise.resolve(
			filtered.map((c) => ({
				id: corridorId(c),
				label:
					`${currencyLabel(c.send_currency, data.currencies)} → ${currencyLabel(c.receive_currency, data.currencies)}`
			}))
		);
	}

	function fetchCorridorsInitial(): Promise<AutocompleteItem[]> {
		return fetchCorridors('');
	}

	function getCorridorLabel(id: string): string {
		const c = corridors.find((x) => corridorId(x) === id);
		return c
			? `${currencyLabel(c.send_currency, data.currencies)} → ${currencyLabel(c.receive_currency, data.currencies)}`
			: id;
	}

	// Prepare history data for LayerChart — daily aggregate (avg rate per day).
	// The backend returns { day, max_rate, min_rate, avg_rate } per ADR-075 §6.
	const chartData = $derived.by(() => {
		return history.map((p) => ({
			date: new Date(p.day),
			avg: p.avg_rate,
			min: p.min_rate,
			max: p.max_rate
		}));
	});

	// On mount: load the 30-day history and run the initial calculation for the
	// default corridor. The SSR load fetches corridors/providers/rates, but the
	// history chart and calculator are client-only (interactive) — without this
	// they stay empty until the user manually changes the corridor.
	// Uses onMount (not $effect) so it runs exactly once — an $effect would
	// re-fire whenever selectedCorridor changes (read synchronously inside
	// loadHistory/calculate before the first await), duplicating the calls
	// already made by loadRates() during onCorridorChange.
	// Ref: §74 (Remittance Tracker), ROADMAP-TOOLS 02.06.
	onMount(() => {
		if (corridors.length > 0) {
			loadHistory();
			calculate();
		}
	});

	// Clean up the calculator debounce timer on destroy so a pending callback
	// can't fire after the component is gone (§4.1 performance / no leaked timers).
	onDestroy(() => {
		if (calcTimer) clearTimeout(calcTimer);
	});
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">{t('remittance.title', 'Remittance Rate Tracker')}</h1>
		<p class="mt-2 text-text-subtle">
			{t('remittance.subtitle', 'Compare live rates across providers, view trends, and set alerts.')}
		</p>
	</div>

	{#if corridors.length === 0}
		<EmptyState
			icon="arrow-left-right"
			title={t('remittance.no_corridors', 'No corridors available')}
			description={t('remittance.no_corridors_desc', "Remittance data hasn't been configured yet. Please check back later.")}
		/>
	{:else}
		<!-- Corridor selector -->
		<div class="mb-6 flex items-center gap-3">
			<label for="corridor" class="text-sm font-medium text-text whitespace-nowrap">{t('remittance.corridor', 'Corridor:')}</label>
			<Autocomplete
				id="corridor"
				placeholder={t('remittance.corridor_placeholder', 'Search corridors…')}
				emptyMessage={t('remittance.no_corridors_found', 'No corridors found')}
				fetcher={fetchCorridors}
				initialFetcher={fetchCorridorsInitial}
				selectedId={corridorId(selectedCorridor)}
				selectedLabel={getCorridorLabel(corridorId(selectedCorridor))}
				onSelect={(item) => onCorridorChange(item.id)}
			/>
		</div>

		<div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
			<!-- Rate table -->
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('remittance.current_rates', 'Current rates')}
				</h2>
				{#if loadingRates}
					<div class="space-y-2">
						<LoadingSkeleton height="60px" />
						<LoadingSkeleton height="60px" />
						<LoadingSkeleton height="60px" />
					</div>
				{:else if ratesError}
					<EmptyState
						icon="trending-down"
						title={t('remittance.load_error', 'Could not load rates')}
						description={ratesError}
					/>
				{:else if rates.length === 0}
					<EmptyState
						icon="trending-down"
						title={t('remittance.no_rates', 'No rates available')}
						description={t('remittance.no_rates_desc', 'No providers have rates for this corridor right now.')}
					/>
				{:else}
					<div class="space-y-2">
						{#if bestRate}
							<div class="p-3 bg-[var(--tool-accent)]/5 border border-[var(--tool-accent)]/30 rounded-lg flex items-center justify-between">
								<div>
									<span class="text-xs font-medium text-[var(--tool-accent)]">{t('remittance.best_rate', 'Best rate')}</span>
									<p class="text-sm font-semibold text-text">{bestRate.provider_name}</p>
								</div>
								<span class="text-lg font-bold text-[var(--tool-accent)]">
									{formatRate(bestRate.rate)}
								</span>
							</div>
						{/if}
						{#each sortedRates as rate (rate.id)}
							{@const provider = getProvider(rate.provider_id)}
							<div class="p-3 bg-surface border border-border rounded-lg flex items-center justify-between">
								<div class="flex items-center gap-3 min-w-0">
									<div class="min-w-0">
										<div class="flex items-center gap-1.5">
											<p class="text-sm font-medium text-text truncate">{rate.provider_name}</p>
											{#if rate.is_benchmark}
												<span class="rounded-full border border-border px-1.5 py-0.5 text-xs text-muted flex-shrink-0">{t('remittance.mid_market', 'Mid-market')}</span>
											{/if}
											{#if provider?.website_url}
												<a
													href={provider.website_url}
													target="_blank"
													rel="noopener noreferrer"
													class="text-muted hover:text-text flex-shrink-0"
													aria-label={t('remittance.provider_website', 'Visit provider website')}
												>
													<Icon name="external-link" size={12} />
												</a>
											{/if}
										</div>
										<div class="flex items-center gap-2 text-xs text-muted mt-0.5">
											{#if rate.fee_flat || rate.fee_pct}
												<span>
													{t('remittance.fee', 'Fee:')}
													{#if rate.fee_flat}{fmtMoney(rate.fee_flat, rate.send_currency)}{/if}
													{#if rate.fee_pct}{rate.fee_flat ? ' ' : ''}+{rate.fee_pct}%{/if}
												</span>
											{/if}
											{#if rate.transfer_time}
												<span>· {rate.transfer_time}</span>
											{/if}
											{#if rate.is_manual}
												<span class="{isManualRateStale(rate) ? 'text-red-500' : 'text-yellow-600'}">
													· {isManualRateStale(rate) ? t('remittance.manual_stale', 'manual · stale') : t('remittance.manual', 'manual')}
												</span>
											{/if}
										</div>
										{#if rate.source?.slug}
											<a href={mainAppUrl(`/sources/${rate.source.slug}`)} class="inline-flex items-center gap-0.5 text-primary hover:underline">
												{t('remittance.source', 'Source')}: {rate.source.name}
											</a>
										{/if}
									</div>
								</div>
								<div class="text-end flex-shrink-0">
									<span class="text-base font-semibold text-text">{formatRate(rate.rate)}</span>
									<p class="text-xs text-muted">{rate.receive_currency} {t('remittance.per', 'per')} {rate.send_currency}</p>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Calculator -->
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					{t('remittance.calculator', 'Calculator')}
				</h2>
				<div class="p-4 bg-surface border border-border rounded-lg">
					<div class="flex items-center gap-2 mb-3">
						<input
							type="number"
							bind:value={calcSendAmount}
							oninput={debouncedCalculate}
							min="1"
							class="w-32 px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
						/>
						<span class="text-sm text-muted">{currencySymbol(selectedCorridor.send_currency, data.currencies) || selectedCorridor.send_currency}</span>
						<Icon name="arrow-right" size={16} class="text-muted" />
						<span class="text-sm text-muted">{currencySymbol(selectedCorridor.receive_currency, data.currencies) || selectedCorridor.receive_currency}</span>
					</div>

					{#if calcLoading}
						<LoadingSkeleton height="40px" />
					{:else if calcResult?.results?.length}
						<!-- §112 — mid-market benchmark reference (ADR-125). Shown
						     above the ranked provider results; never ranked itself. -->
						{#if calcResult.benchmark}
							<p class="mb-2 text-xs text-muted">
								{t('remittance.mid_market_would_give', 'Mid-market would give')}
								<span class="font-medium text-text">{fmtMoney(calcResult.benchmark.receive_amount, calcResult.receive_currency)}</span>
								(1 {calcResult.send_currency} = {formatRate(calcResult.benchmark.rate)} {calcResult.receive_currency})
							</p>
						{/if}
						<div class="space-y-2">
							{#each calcResult.results as row (row.provider_id)}
								<div class="flex items-center justify-between text-sm">
									<span class="text-text">{row.provider_name}</span>
									<span class="font-semibold text-text">
										{fmtMoney(row.receive_amount, calcResult.receive_currency)}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		</div>

		<!-- Trend chart -->
		<div class="mt-8">
			<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
				{t('remittance.history_title', '30-day rate history')}
			</h2>
			{#if loadingHistory}
				<LoadingSkeleton height="240px" />
			{:else if history.length > 0}
				<div class="p-4 bg-surface border border-border rounded-lg">
					<div class="h-60 w-full">
						<LineChart
							data={chartData}
							x="date"
							y="avg"
							props={{
								line: { class: 'stroke-[var(--chart-1)]' },
								area: { class: 'fill-[var(--chart-1)]/10' }
							}}
						/>
					</div>
				</div>
			{:else}
				<EmptyState
					icon="chart-line"
					title={t('remittance.no_history', 'No history available')}
					description={t('remittance.no_history_desc', 'Historical rate data will appear here once enough data is collected.')}
				/>
			{/if}
		</div>

		<!-- Rate alerts -->
		<div class="mt-8">
			<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
				{t('remittance.alerts', 'Rate alerts')}
			</h2>
			{#if !user}
				<div class="p-4 bg-surface border border-border rounded-lg text-center">
					<p class="text-sm text-text-subtle mb-3">
						{t('remittance.alerts_signin', 'Sign in to create rate alerts and get notified when your target rate is hit.')}
					</p>
					<a
						href={mainAppUrl('/login')}
						class="inline-flex items-center gap-1.5 px-4 py-2 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
					>
						{t('common.sign_in', 'Sign in')}
					</a>
				</div>
			{:else}
				<div class="space-y-4">
					<!-- Create alert -->
					{#if alertError}
						<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
							{alertError}
						</div>
					{/if}
					<div class="p-4 bg-surface border border-border rounded-lg flex flex-wrap items-center gap-3">
						<span class="text-sm text-text">{t('remittance.notify_when', 'Notify me when')}</span>
						<div class="w-40">
							<Autocomplete
								placeholder={t('remittance.direction', 'Direction')}
								emptyMessage={t('common.no_results', 'No results found')}
								fetcher={fetchDirectionOptions}
								initialFetcher={fetchDirectionOptionsInitial}
								selectedId={alertDirection}
								selectedLabel={directionLabel(alertDirection)}
								onSelect={(item) => (alertDirection = item.id)}
							/>
						</div>
						<input
							type="number"
							bind:value={alertTargetRate}
							placeholder={t('remittance.target_rate', 'target rate')}
							step="any"
							class="w-28 px-3 py-1.5 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
						<span class="text-sm text-muted">
							{selectedCorridor.receive_currency}/{selectedCorridor.send_currency}
						</span>
						<button
							onclick={createAlert}
							disabled={!(parseFloat(alertTargetRate) > 0)}
							class="px-4 py-1.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
						>
							{t('remittance.create_alert', 'Create alert')}
						</button>
					</div>

					<!-- Existing alerts -->
					{#if alerts.length > 0}
						<div class="space-y-2">
							{#each alerts.filter((a) => a.send_currency === selectedCorridor.send_currency && a.receive_currency === selectedCorridor.receive_currency) as alert (alert.id)}
								<div class="p-3 bg-surface border border-border rounded-lg {alert.is_active ? '' : 'opacity-60'}">
									{#if editingAlertId === alert.id}
										<!-- Edit mode (PATCH /remittance/alerts/:id — §74) -->
										<div class="flex flex-wrap items-center gap-2">
											<div class="w-40">
												<Autocomplete
													placeholder={t('remittance.direction', 'Direction')}
													emptyMessage={t('common.no_results', 'No results found')}
													fetcher={fetchDirectionOptions}
													initialFetcher={fetchDirectionOptionsInitial}
													selectedId={editDirection}
													selectedLabel={directionLabel(editDirection)}
													onSelect={(item) => (editDirection = item.id)}
												/>
											</div>
											<input
												type="number"
												bind:value={editTargetRate}
												step="any"
												class="w-24 px-2 py-1 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
											/>
											<button
												onclick={() => saveAlertEdit(alert.id)}
												class="px-3 py-1 text-sm bg-[var(--tool-accent)] text-white rounded hover:bg-[var(--tool-accent-hover)]"
											>
												{t('common.save', 'Save')}
											</button>
											<button
												onclick={() => (editingAlertId = null)}
												class="px-3 py-1 text-sm text-muted hover:text-text"
											>
												{t('common.cancel', 'Cancel')}
											</button>
										</div>
									{:else}
										<div class="flex items-center justify-between">
											<div class="text-sm">
												<span class="text-text">{alert.direction === 'above' ? t('remittance.alert_rate_above', 'Rate above') : t('remittance.alert_rate_below', 'Rate below')} </span>
												<span class="font-semibold text-text">{formatRate(alert.target_rate)}</span>
												<span class="text-muted"> ({alert.send_currency}→{alert.receive_currency})</span>
												{#if !alert.is_active}
													<span class="ms-2 text-xs text-muted">{t('remittance.alert_inactive', '(inactive)')}</span>
												{/if}
											</div>
											<div class="flex items-center gap-1">
												<button
													onclick={() => toggleAlertActive(alert.id, alert.is_active)}
													class="p-1.5 text-muted hover:text-text transition-colors"
													aria-label={alert.is_active ? t('remittance.pause_alert', 'Pause alert') : t('remittance.activate_alert', 'Activate alert')}
												>
													<Icon name={alert.is_active ? 'pause' : 'play'} size={16} />
												</button>
												<button
													onclick={() => startEditAlert(alert)}
													class="p-1.5 text-muted hover:text-text transition-colors"
													aria-label={t('remittance.edit_alert', 'Edit alert')}
												>
													<Icon name="pencil" size={16} />
												</button>
												<button
													onclick={() => deleteAlert(alert.id)}
													class="p-1.5 text-muted hover:text-red-500 transition-colors"
													aria-label={t('remittance.delete_alert', 'Delete alert')}
												>
													<Icon name="trash-2" size={16} />
												</button>
											</div>
										</div>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{/if}

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="bell"
			title={t('register.create_alerts', 'Create rate alerts')}
			description={t('register.create_alerts_desc', 'Create a free account to set rate alerts and get notified when your target rate is hit.')}
		/>
	{/if}
	{/if}
</div>
