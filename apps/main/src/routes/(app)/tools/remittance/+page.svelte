<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import {
		Banknote,
		Lock,
		Bell,
		ArrowRight,
		TrendingUp,
		Pencil,
		Building2,
		ChevronDown,
		ChevronUp,
		ToggleLeft,
		ToggleRight,
		Loader2,
		GitCompare
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { LineChart } from 'layerchart';
	import type {
		RemittanceCorridor,
		RemittanceRate,
		RemittanceHistoryPoint,
		RemittanceAlert,
		RemittanceProvider,
		RemittanceCalculation,
		RemittanceProviderCalc,
		CatalogOption
	} from '$lib/types';
	import type { PageData, ActionData } from './$types';
	import { formatMoney, currencyLabel, currencySymbol } from '$lib/utils/currency';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('tools.remittance', 'Remittance Rate Tracker — {site}'));
	const description = $derived(
		t('tools.remittance_desc', 'Compare provider rates and set alerts.')
	);

	const corridors = $derived<RemittanceCorridor[]>(data.corridors);
	const providers = $derived<RemittanceProvider[]>(data.providers ?? []);
	// §112 — benchmark providers are a mid-market reference, not bookable
	// providers: exclude them from the JCE compare picker (ADR-125).
	const sellableProviders = $derived(providers.filter((p) => !p.is_benchmark));
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	// Pre-select the configured default send currency when it participates
	// in a corridor (remittance.default_send_currency, §74).
	let sendCurrency = $state(
		untrack(() =>
			data.defaultSendCurrency && corridors.some((c) => c.send_currency === data.defaultSendCurrency)
				? data.defaultSendCurrency
				: ''
		)
	);
	let receiveCurrency = $state('');
	let rates = $state<RemittanceRate[]>([]);
	let history = $state<RemittanceHistoryPoint[]>([]);
	let alerts = $state<RemittanceAlert[]>(untrack(() => data.alerts ?? []));
	let loading = $state(false);
	let error = $state<string | null>(null);
	let actionError = $state<string | null>(null);

	// Providers and calculator.
	let showProviders = $state(false);
	let calculation = $state<RemittanceCalculation | null>(null);
	let calcLoading = $state(false);

	// JCE comparison state.
	let savingJCE = $state(false);
	let selectedProviderIds = $state<Set<string>>(new Set());
	const canCompare = $derived(selectedProviderIds.size >= 2 && selectedProviderIds.size <= 4);

	function toggleProvider(id: string) {
		const next = new Set(selectedProviderIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			if (next.size >= 4) return;
			next.add(id);
		}
		selectedProviderIds = next;
	}

	// Calculator widget.
	let sendAmount = $state('1000');
	let calcDebounce: ReturnType<typeof setTimeout> | null = null;

	// Alert creation.
	let alertRate = $state('');
	const directionOptions = $derived<CatalogOption[]>(data.directionOptions ?? []);
	let alertDirection = $state(untrack(() => data.directionOptions[0]?.value ?? 'above'));
	let creatingAlert = $state(false);

	// Alert editing.
	let editingAlert = $state<RemittanceAlert | null>(null);
	let editAlertRate = $state('');
	let editAlertDirection = $state('above');
	let savingEdit = $state(false);

	// §112 — benchmark rows are a mid-market reference, never a sellable
	// quote: exclude them from the best-rate highlight (ADR-125).
	const bestRate = $derived.by(() => {
		const sellable = rates.filter((r) => !r.is_benchmark);
		if (sellable.length === 0) return null;
		return sellable.reduce((best, r) => (r.rate > best.rate ? r : best), sellable[0]);
	});

	const chartData = $derived(
		history.map((h) => ({ date: new Date(h.day), avg: h.avg_rate, min: h.min_rate, max: h.max_rate }))
	);

	// Rates are rendered sorted: sellable quotes best-rate-first, then any
	// mid-market benchmark rows pinned at the bottom as a reference (§112).
	const sortedRates = $derived.by(() => {
		const sellable = rates.filter((r) => !r.is_benchmark).sort((a, b) => b.rate - a.rate);
		const benchmark = rates.filter((r) => r.is_benchmark).sort((a, b) => b.rate - a.rate);
		return [...sellable, ...benchmark];
	});

	// A manual rate is stale once fetched_at is older than the configured
	// threshold (remittance.manual_stale_hours, §74).
	function isStale(r: RemittanceRate): boolean {
		if (!r.is_manual || !r.fetched_at) return false;
		return Date.now() - new Date(r.fetched_at).getTime() > data.manualStaleHours * 3600_000;
	}

	// Corridor rates span orders of magnitude (GBP→NGN ~1500 vs inverse
	// corridors below 1) — a fixed toFixed(2) renders small rates as "0.00".
	function formatRate(rate: number): string {
		if (!Number.isFinite(rate)) return '—';
		if (rate >= 100) return rate.toFixed(2);
		if (rate >= 1) return rate.toFixed(4);
		return rate.toPrecision(4);
	}

	// Fee cell — flat fees render in send currency; a percentage fee renders
	// as "+x%" alongside any flat amount (fee_pct was previously invisible).
	function feeLabel(feeFlat: number | undefined, feePct: number | undefined, currency: string): string {
		const parts: string[] = [];
		if (feeFlat && feeFlat > 0) parts.push(formatMoney(feeFlat, currency, data.currencies));
		if (feePct && feePct > 0) parts.push(`+${feePct}%`);
		return parts.length > 0 ? parts.join(' ') : '—';
	}

	function directionLabel(v: string): string {
		return directionOptions.find((o) => o.value === v)?.label ?? v;
	}

	// Alert direction picker — every dropdown on this page uses Autocomplete.
	function fetchDirectionOptions(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return Promise.resolve(
			directionOptions
				.filter((o) => query === '' || o.label.toLowerCase().includes(query))
				.map((o) => ({ id: o.value, label: o.label }))
		);
	}
	function fetchDirectionOptionsInitial(): Promise<AutocompleteItem[]> {
		return fetchDirectionOptions('');
	}

	async function loadCalculation() {
		if (!sendCurrency || !receiveCurrency) return;
		const amount = parseFloat(sendAmount || '0');
		if (!amount || amount <= 0) {
			calculation = null;
			return;
		}
		calcLoading = true;
		const res = await api.get<RemittanceCalculation>(
			`/remittance/calculate?send_amount=${amount}&send=${sendCurrency}&receive=${receiveCurrency}`
		);
		calcLoading = false;
		if (!res.error) calculation = res.data ?? null;
		else calculation = null;
	}

	function debouncedLoadCalculation() {
		if (calcDebounce) clearTimeout(calcDebounce);
		calcDebounce = setTimeout(() => loadCalculation(), 300);
	}

	async function loadCorridor() {
		if (!sendCurrency || !receiveCurrency || sendCurrency === receiveCurrency) return;
		loading = true;
		error = null;
		actionError = null;
		const [ratesRes, histRes] = await Promise.all([
			api.get<RemittanceRate[]>(`/remittance/rates?send=${sendCurrency}&receive=${receiveCurrency}`),
			api.get<RemittanceHistoryPoint[]>(`/remittance/history?send=${sendCurrency}&receive=${receiveCurrency}&days=30`)
		]);
		loading = false;
		if (ratesRes.error) {
			error = ratesRes.error.message;
			return;
		}
		rates = ratesRes.data ?? [];
		history = histRes.error ? [] : (histRes.data ?? []);
		if (histRes.error) {
			actionError = histRes.error.message;
		}
		loadCalculation();
	}

	// Corridor selection changed — reload data when a valid pair is chosen,
	// otherwise clear corridor-scoped state so stale results don't linger.
	function onCorridorChange() {
		rates = [];
		history = [];
		calculation = null;
		error = null;
		if (sendCurrency && receiveCurrency && sendCurrency !== receiveCurrency) {
			loadCorridor();
		}
	}

	async function createAlert() {
		if (!authed || !alertRate || !sendCurrency || !receiveCurrency) return;
		creatingAlert = true;
		actionError = null;
		const res = await api.post<RemittanceAlert>('/remittance/alerts', {
			send_currency: sendCurrency,
			receive_currency: receiveCurrency,
			target_rate: parseFloat(alertRate),
			direction: alertDirection
		});
		creatingAlert = false;
		if (res.error) {
			actionError = res.error.message;
		} else if (res.data) {
			const created = res.data!;
			alerts = [...alerts, created];
			alertRate = '';
		}
	}

	async function deleteAlert(id: string) {
		actionError = null;
		const res = await api.delete(`/remittance/alerts/${id}`);
		if (res.error) {
			actionError = res.error.message;
		} else {
			alerts = alerts.filter((a) => a.id !== id);
		}
	}

	async function toggleAlertActive(a: RemittanceAlert) {
		actionError = null;
		const res = await api.patch<RemittanceAlert>(`/remittance/alerts/${a.id}`, {
			is_active: !a.is_active
		});
		if (res.error) {
			actionError = res.error.message;
		} else if (res.data) {
			const updated = res.data!;
			alerts = alerts.map((alert) => (alert.id === updated.id ? updated : alert));
		}
	}

	function startEditAlert(a: RemittanceAlert) {
		editingAlert = a;
		editAlertRate = String(a.target_rate);
		editAlertDirection = a.direction;
	}

	function cancelEditAlert() {
		editingAlert = null;
		editAlertRate = '';
		editAlertDirection = directionOptions[0]?.value ?? 'above';
	}

	async function saveEditAlert() {
		if (!editingAlert) return;
		savingEdit = true;
		actionError = null;
		const res = await api.patch<RemittanceAlert>(`/remittance/alerts/${editingAlert.id}`, {
			target_rate: parseFloat(editAlertRate),
			direction: editAlertDirection
		});
		savingEdit = false;
		if (res.error) {
			actionError = res.error.message;
		} else if (res.data) {
			const updated = res.data!;
			alerts = alerts.map((a) => (a.id === updated.id ? updated : a));
			editingAlert = null;
		}
	}

	function ratioVsBest(r: RemittanceRate): string {
		if (!bestRate || bestRate.rate === 0) return '—';
		const ratio = (r.rate / bestRate.rate) * 100;
		return ratio.toFixed(1) + '%';
	}

	function bestCalcProvider(calc: RemittanceCalculation | null): RemittanceProviderCalc | null {
		if (!calc || !calc.results || calc.results.length === 0) return null;
		const sellable = calc.results.filter((p) => !p.is_benchmark);
		if (sellable.length === 0) return null;
		return sellable.reduce((best, p) => (p.receive_amount > best.receive_amount ? p : best), sellable[0]);
	}

	// Autocomplete fetchers — filter the already-loaded corridors client-side.
	// Labels resolve currency names from the managed currency list (§58) so
	// users see "GBP — British Pound", not a bare code.
	function currencyItem(code: string): AutocompleteItem {
		return { id: code, label: currencyLabel(code, data.currencies) };
	}
	function fetchSendCurrencies(q: string): Promise<AutocompleteItem[]> {
		const codes = [...new Set(corridors.map((c) => c.send_currency))];
		const query = q.toLowerCase();
		const filtered = query === ''
			? codes
			: codes.filter((c) => currencyItem(c).label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map(currencyItem));
	}
	function fetchSendCurrenciesInitial(): Promise<AutocompleteItem[]> {
		return fetchSendCurrencies('');
	}
	function fetchReceiveCurrencies(q: string): Promise<AutocompleteItem[]> {
		const codes = [...new Set(corridors.map((c) => c.receive_currency))];
		const query = q.toLowerCase();
		const filtered = query === ''
			? codes
			: codes.filter((c) => currencyItem(c).label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map(currencyItem));
	}
	function fetchReceiveCurrenciesInitial(): Promise<AutocompleteItem[]> {
		return fetchReceiveCurrencies('');
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Banknote size={20} /> {t('tools.remittance', 'Remittance Rate Tracker')}
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
	{:else}
		<!-- Corridor selector -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<div class="flex flex-wrap items-end gap-3">
				<div>
					<label for="rem-send" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
						{t('remittance.send', 'Send')}
					</label>
					<div class="mt-1">
						<Autocomplete
							placeholder="—"
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchSendCurrencies}
							initialFetcher={fetchSendCurrenciesInitial}
							selectedId={sendCurrency || null}
							selectedLabel={sendCurrency || null}
							onSelect={(item) => { sendCurrency = item.id; onCorridorChange(); }}
							onClear={() => { sendCurrency = ''; onCorridorChange(); }}
						/>
					</div>
				</div>
				<ArrowRight size={16} class="pb-2 text-[var(--color-muted)] rtl:-scale-x-100" />
				<div>
					<label for="rem-recv" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
						{t('remittance.receive', 'Receive')}
					</label>
					<div class="mt-1">
						<Autocomplete
							placeholder="—"
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchReceiveCurrencies}
							initialFetcher={fetchReceiveCurrenciesInitial}
							selectedId={receiveCurrency || null}
							selectedLabel={receiveCurrency || null}
							onSelect={(item) => { receiveCurrency = item.id; onCorridorChange(); }}
							onClear={() => { receiveCurrency = ''; onCorridorChange(); }}
						/>
					</div>
				</div>
			</div>
		</div>

		{#if actionError}
			<div class="px-4 pt-4">
				<div class="rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
					{actionError}
				</div>
			</div>
		{/if}

		{#if sendCurrency && receiveCurrency && sendCurrency === receiveCurrency}
			<div class="p-4">
				<EmptyState
					icon={ArrowRight} iconClass="rtl:-scale-x-100"
					title={t('remittance.same_currency', 'Choose two different currencies')}
					body={t('remittance.same_currency_hint', 'Send and receive currencies must differ.')}
				/>
			</div>
		{:else if sendCurrency && receiveCurrency}
			<div class="p-4">
				{#if loading}
					<LoadingSkeleton variant="lines" />
				{:else if error}
					<EmptyState icon={Banknote} title={t('remittance.load_error', 'Could not load rates')} body={error} />
				{:else}
					<!-- Rate table -->
					{#if rates.length > 0}
						<div class="mb-4 overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
							<table class="w-full text-sm">
								<thead>
									<tr class="border-b border-[var(--color-border)] text-start text-xs text-[var(--color-muted)]">
										<th class="px-3 py-2 font-medium">{t('remittance.provider', 'Provider')}</th>
										<th class="px-3 py-2 text-end font-medium">{t('remittance.rate', 'Rate')}</th>
										<th class="px-3 py-2 text-end font-medium">{t('remittance.fee', 'Fee')}</th>
										<th class="px-3 py-2 text-end font-medium">{t('remittance.transfer_time', 'Time')}</th>
										<th class="px-3 py-2 text-end font-medium">{t('remittance.vs_best', 'vs Best')}</th>
									</tr>
								</thead>
								<tbody>
									{#each sortedRates as r (r.id)}
										<tr class="border-b border-[var(--color-border)] last:border-0">
											<td class="px-3 py-2 text-[var(--color-text)]">
												{#if r.provider_website}
													<a href={r.provider_website} target="_blank" rel="noopener noreferrer" class="hover:underline">{r.provider_name}</a>
												{:else}
													{r.provider_name}
												{/if}
												{#if r.is_benchmark}
													<span class="ms-1 rounded-full border border-[var(--color-border)] px-1.5 py-0.5 text-xs text-[var(--color-muted)]">{t('remittance.mid_market', 'Mid-market')}</span>
												{/if}
												{#if r.is_manual}
													<span class="ms-1 rounded-full border border-[var(--color-border)] px-1.5 py-0.5 text-xs text-[var(--color-muted)]">{t('remittance.manual', 'Manual')}</span>
												{/if}
												{#if isStale(r)}
													<span class="ms-1 rounded-full border border-amber-300 px-1.5 py-0.5 text-xs text-amber-600 dark:border-amber-800 dark:text-amber-400">{t('remittance.stale', 'Stale')}</span>
												{/if}
												{#if bestRate && r.id === bestRate.id}
													<span class="ms-1 rounded-full bg-[var(--color-primary)] px-1.5 py-0.5 text-xs text-white">{t('remittance.best', 'Best')}</span>
												{/if}
												{#if r.source?.name}
													<a
														href={r.source.slug ? `/sources/${r.source.slug}` : r.source.source_url || r.source_url || '#'}
														target={r.source.slug ? '_self' : '_blank'}
														rel="noopener noreferrer"
														class="block text-xs text-[var(--color-muted)] hover:underline"
													>{t('remittance.via', 'via')} {r.source.name}</a>
												{/if}
											</td>
											<td class="px-3 py-2 text-end font-medium text-[var(--color-text)]">{formatRate(r.rate)}</td>
											<td class="px-3 py-2 text-end text-[var(--color-muted)]">{feeLabel(r.fee_flat, r.fee_pct, sendCurrency)}</td>
											<td class="px-3 py-2 text-end text-[var(--color-muted)]">{r.transfer_time || '—'}</td>
											<td class="px-3 py-2 text-end text-[var(--color-muted)]">{ratioVsBest(r)}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{:else}
						<div class="mb-4">
							<EmptyState icon={Banknote} title={t('remittance.no_rates', 'No rates available for this corridor')} />
						</div>
					{/if}

					<!-- Historical trend chart -->
					{#if chartData.length > 0}
						<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<h2 class="mb-3 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
								<TrendingUp size={16} /> {t('remittance.history', '30-day rate history')}
							</h2>
							<div class="h-56 w-full">
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
					{/if}

					<!-- Send amount calculator -->
					<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
							{t('remittance.calculator', 'Send amount calculator')}
						</h2>
						<div class="flex items-center gap-2">
							<input
								type="number"
								bind:value={sendAmount}
								oninput={debouncedLoadCalculation}
								min="1"
								class="w-32 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
							/>
							<span class="text-sm text-[var(--color-muted)]">{currencySymbol(sendCurrency, data.currencies) || sendCurrency}</span>
						</div>

						{#if calcLoading}
							<p class="mt-2 text-sm text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
						{:else}
							<!-- §112 — mid-market benchmark reference (ADR-125). Shown
							     above the ranked provider results; never ranked itself. -->
							{#if calculation?.benchmark}
								<p class="mt-2 text-sm text-[var(--color-muted)]">
									{t('remittance.mid_market_would_give', 'Mid-market would give')}
									<strong class="font-medium text-[var(--color-text)]">{formatMoney(calculation.benchmark.receive_amount, receiveCurrency, data.currencies)}</strong>
									<span>({t('remittance.rate', 'Rate')}: {formatRate(calculation.benchmark.rate)})</span>
								</p>
							{/if}
							{#if calculation && calculation.results.length > 0}
								{@const best = bestCalcProvider(calculation)}
								<p class="mt-2 text-sm text-[var(--color-text)]">
									{t('remittance.best_rate_label', 'Best value')}:
									<strong class="font-semibold">{formatMoney(best?.receive_amount, receiveCurrency, data.currencies)}</strong>
									<span class="text-[var(--color-muted)]">({t('remittance.via', 'via')} {best?.provider_name})</span>
								</p>
								<div class="mt-3 overflow-x-auto rounded-[var(--radius)] border border-[var(--color-border)]">
									<table class="w-full text-sm">
										<thead>
											<tr class="border-b border-[var(--color-border)] text-start text-xs text-[var(--color-muted)]">
												<th class="px-3 py-2 font-medium">{t('remittance.provider', 'Provider')}</th>
												<th class="px-3 py-2 text-end font-medium">{t('remittance.rate', 'Rate')}</th>
												<th class="px-3 py-2 text-end font-medium">{t('remittance.fee', 'Fee')}</th>
												<th class="px-3 py-2 text-end font-medium">{t('remittance.receive_amount', 'Receive')}</th>
												<th class="px-3 py-2 text-end font-medium">{t('remittance.total_cost', 'Total cost')}</th>
											</tr>
										</thead>
										<tbody>
											{#each calculation.results as p (p.provider_id)}
												<tr class="border-b border-[var(--color-border)] last:border-0">
													<td class="px-3 py-2 text-[var(--color-text)]">
														{p.provider_name}
														{#if p.is_benchmark}
															<span class="ms-1 rounded-full border border-[var(--color-border)] px-1.5 py-0.5 text-xs text-[var(--color-muted)]">{t('remittance.mid_market', 'Mid-market')}</span>
														{/if}
													</td>
													<td class="px-3 py-2 text-end">{formatRate(p.rate)}</td>
													<td class="px-3 py-2 text-end text-[var(--color-muted)]">{feeLabel(p.fee_flat, p.fee_pct, sendCurrency)}</td>
													<td class="px-3 py-2 text-end font-medium">{formatMoney(p.receive_amount, receiveCurrency, data.currencies)}</td>
													<td class="px-3 py-2 text-end text-[var(--color-muted)]">{formatMoney(p.total_fees, sendCurrency, data.currencies)}</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							{:else if bestRate}
								<p class="mt-2 text-sm text-[var(--color-text)]">
									≈ {formatMoney(parseFloat(sendAmount || '0') * bestRate.rate, receiveCurrency, data.currencies)}
									<span class="text-[var(--color-muted)]"> ({t('remittance.via', 'via')} {bestRate.provider_name})</span>
								</p>
							{/if}
						{/if}
					</div>

					<!-- Providers list (collapsible) -->
					<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<button
							type="button"
							onclick={() => (showProviders = !showProviders)}
							class="mb-2 flex w-full items-center justify-between text-sm font-medium text-[var(--color-text)]"
						>
							<span class="flex items-center gap-1.5">
								<Building2 size={16} />
								{t('remittance.providers', 'Providers')}
							</span>
							{#if showProviders}
								<ChevronUp size={16} class="text-[var(--color-muted)]" />
							{:else}
								<ChevronDown size={16} class="text-[var(--color-muted)]" />
							{/if}
						</button>
						{#if showProviders}
							{#if sellableProviders.length === 0}
								<p class="text-sm text-[var(--color-muted)]">{t('remittance.no_providers', 'No providers configured.')}</p>
							{:else}
								{#if form?.error}
									<div class="mb-3 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
										{form.message || t('remittance.jce_error', 'Could not create comparison')}
									</div>
								{/if}

								<form
									method="POST"
									use:enhance={() => {
										savingJCE = true;
										return async ({ update }) => {
											savingJCE = false;
											await update();
										};
									}}
									class="space-y-2"
								>
									<input type="hidden" name="send_currency" value={sendCurrency} />
									<input type="hidden" name="receive_currency" value={receiveCurrency} />
									<input type="hidden" name="send_amount" value={sendAmount} />
									{#each sellableProviders as p (p.id)}
										{#if selectedProviderIds.has(p.id)}
											<input type="hidden" name="provider_id" value={p.id} />
										{/if}
									{/each}

									<div class="mb-3 flex items-center justify-between flex-wrap gap-3">
										<p class="text-sm text-[var(--color-muted)]">
											{t('remittance.selected', 'Selected')}: {selectedProviderIds.size} / 4
										</p>
										<button
											type="submit"
											disabled={!canCompare || !authed}
											title={authed
												? t('remittance.compare_in_jce_hint', 'Open in JCE comparison')
												: t('remittance.sign_in_to_compare', 'Sign in to compare')}
											class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
										>
											{#if savingJCE}
												<Loader2 size={16} class="animate-spin" />
											{:else}
												<GitCompare size={16} />
											{/if}
											{t('remittance.compare_in_jce', 'Compare in JCE')}
										</button>
									</div>

									<ul class="space-y-2">
										{#each sellableProviders as p (p.id)}
											<li class="flex items-start gap-3 text-sm text-[var(--color-text)]">
												<input
													type="checkbox"
													checked={selectedProviderIds.has(p.id)}
													onchange={() => toggleProvider(p.id)}
													disabled={selectedProviderIds.size >= 4 && !selectedProviderIds.has(p.id)}
													class="mt-1 accent-[var(--color-primary)]"
												/>
												<div class="min-w-0 flex-1">
													<div class="font-medium">{p.name}</div>
													{#if p.website_url}
														<a
															href={p.website_url}
															target="_blank"
															rel="noopener noreferrer"
															class="text-xs text-[var(--color-primary)] hover:underline"
														>
															{p.website_url}
														</a>
													{/if}
													<span class="text-xs text-[var(--color-muted)]">{p.data_source}</span>
												</div>
											</li>
										{/each}
									</ul>
								</form>
							{/if}
						{/if}
					</div>

				{/if}
			</div>
		{/if}

		<!-- Rate alerts — user-scoped, so the list renders regardless of
		     corridor selection; creating an alert targets the selected
		     corridor. -->
		{#if authed}
			<div class="px-4 pb-4">
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-3 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
						<Bell size={16} /> {t('remittance.alerts', 'Rate alerts')}
					</h2>
					{#if sendCurrency && receiveCurrency && sendCurrency !== receiveCurrency}
						<form onsubmit={(e) => { e.preventDefault(); createAlert(); }} class="mb-3 flex flex-wrap items-end gap-2">
							<div>
								<label for="alert-rate" class="block text-xs text-[var(--color-muted)]">
									{t('remittance.target_rate', 'Target rate')} ({sendCurrency}→{receiveCurrency})
								</label>
								<input
									id="alert-rate"
									type="number"
									bind:value={alertRate}
									step="any"
									class="mt-1 w-28 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
								/>
							</div>
							<div>
								<label for="alert-direction" class="block text-xs text-[var(--color-muted)]">{t('remittance.direction', 'Direction')}</label>
								<div class="mt-1 w-40">
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
							</div>
							<button
								type="submit"
								disabled={creatingAlert || !(parseFloat(alertRate) > 0)}
								class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
							>
								{t('remittance.create_alert', 'Create alert')}
							</button>
						</form>
					{:else}
						<p class="mb-3 text-xs text-[var(--color-muted)]">
							{t('remittance.alert_select_corridor', 'Select a corridor above to create an alert.')}
						</p>
					{/if}
					{#if alerts.length > 0}
						<ul class="space-y-2">
							{#each alerts as a (a.id)}
								<li class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
									{#if editingAlert?.id === a.id}
										<form onsubmit={(e) => { e.preventDefault(); saveEditAlert(); }} class="flex flex-wrap items-end gap-2">
											<input
												type="number"
												bind:value={editAlertRate}
												step="any"
												class="w-28 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
											/>
											<div class="w-40">
												<Autocomplete
													placeholder={t('remittance.direction', 'Direction')}
													emptyMessage={t('common.no_results', 'No results found')}
													fetcher={fetchDirectionOptions}
													initialFetcher={fetchDirectionOptionsInitial}
													selectedId={editAlertDirection}
													selectedLabel={directionLabel(editAlertDirection)}
													onSelect={(item) => (editAlertDirection = item.id)}
												/>
											</div>
											<button
												type="submit"
												disabled={savingEdit || !(parseFloat(editAlertRate) > 0)}
												class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
											>
												{t('common.save', 'Save')}
											</button>
											<button
												type="button"
												onclick={cancelEditAlert}
												class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
											>
												{t('common.cancel', 'Cancel')}
											</button>
										</form>
									{:else}
										<div class="flex items-center justify-between gap-3">
											<span class="text-[var(--color-text)]">
												{a.send_currency}→{a.receive_currency} @ {formatRate(a.target_rate)}
												<span class="text-[var(--color-muted)]">({directionLabel(a.direction)})</span>
												{#if !a.is_active}
													<span class="ms-1 rounded-full border border-[var(--color-border)] px-1.5 py-0.5 text-xs text-[var(--color-muted)]">{t('remittance.paused', 'Paused')}</span>
												{/if}
											</span>
											<div class="flex items-center gap-2">
												<button
													type="button"
													onclick={() => toggleAlertActive(a)}
													class="text-xs {a.is_active ? 'text-[var(--color-primary)]' : 'text-[var(--color-muted)]'} hover:text-[var(--color-text)]"
													aria-label={a.is_active ? t('remittance.pause_alert', 'Pause alert') : t('remittance.activate_alert', 'Activate alert')}
												>
													{#if a.is_active}
														<ToggleRight size={16} />
													{:else}
														<ToggleLeft size={16} />
													{/if}
												</button>
												<button
														type="button"
														onclick={() => startEditAlert(a)}
														class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
													>
														<Pencil size={14} />
													</button>
												<button
													type="button"
													onclick={() => deleteAlert(a.id)}
													class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
												>
													{t('common.remove', 'Remove')}
												</button>
											</div>
										</div>
									{/if}
								</li>
							{/each}
						</ul>
					{:else}
						<p class="text-xs text-[var(--color-muted)]">{t('remittance.no_alerts', 'No alerts set.')}</p>
					{/if}
				</div>
			</div>
		{/if}
	{/if}
</div>
