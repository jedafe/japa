<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
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
		ToggleRight
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { LineChart } from 'layerchart';
	import type {
		RemittanceCorridor,
		RemittanceRate,
		RemittanceHistoryPoint,
		RemittanceAlert,
		RemittanceProvider,
		RemittanceCalculation,
		RemittanceProviderCalc
	} from '$lib/types';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.remittance', 'Remittance Rate Tracker — JapaConnect'));
	const description = $derived(
		t('tools.remittance_desc', 'Compare provider rates and set alerts.')
	);

	const corridors = $derived<RemittanceCorridor[]>(data.corridors);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let sendCurrency = $state('');
	let receiveCurrency = $state('');
	let rates = $state<RemittanceRate[]>([]);
	let history = $state<RemittanceHistoryPoint[]>([]);
	let alerts = $state<RemittanceAlert[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);

	// Providers and calculator.
	let providers = $state<RemittanceProvider[]>([]);
	let showProviders = $state(false);
	let calculation = $state<RemittanceCalculation | null>(null);
	let calcLoading = $state(false);

	// Calculator widget.
	let sendAmount = $state('1000');
	let calcDebounce: ReturnType<typeof setTimeout> | null = null;

	// Alert creation.
	let alertRate = $state('');
	let alertDirection = $state('above');
	let creatingAlert = $state(false);

	// Alert editing.
	let editingAlert = $state<RemittanceAlert | null>(null);
	let editAlertRate = $state('');
	let editAlertDirection = $state('above');
	let savingEdit = $state(false);

	const bestRate = $derived(rates.length > 0 ? rates.reduce((best, r) => (r.rate > best.rate ? r : best), rates[0]) : null);

	const chartData = $derived(
		history.map((h) => ({ date: new Date(h.day), avg: h.avg_rate, min: h.min_rate, max: h.max_rate }))
	);

	async function loadProviders() {
		const res = await api.get<RemittanceProvider[]>('/remittance/providers');
		if (!res.error) providers = res.data ?? [];
	}

	loadProviders();

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
		if (!sendCurrency || !receiveCurrency) return;
		loading = true;
		error = null;
		const [ratesRes, histRes, alertsRes] = await Promise.all([
			api.get<RemittanceRate[]>(`/remittance/rates?send=${sendCurrency}&receive=${receiveCurrency}`),
			api.get<RemittanceHistoryPoint[]>(`/remittance/history?send=${sendCurrency}&receive=${receiveCurrency}&days=30`),
			authed ? api.get<RemittanceAlert[]>(`/remittance/alerts`) : Promise.resolve({ data: null, error: null, status: 0 })
		]);
		loading = false;
		if (ratesRes.error) {
			error = ratesRes.error.message;
			return;
		}
		rates = ratesRes.data ?? [];
		history = histRes.data ?? [];
		if (!alertsRes.error) alerts = alertsRes.data ?? [];
		loadCalculation();
	}

	function onSelectCorridor() {
		const parts = `${sendCurrency}/${receiveCurrency}`.split('/');
		if (parts.length === 2) loadCorridor();
	}

	async function createAlert() {
		if (!authed || !alertRate || !sendCurrency || !receiveCurrency) return;
		creatingAlert = true;
		const res = await api.post<RemittanceAlert>('/remittance/alerts', {
			send_currency: sendCurrency,
			receive_currency: receiveCurrency,
			target_rate: parseFloat(alertRate),
			direction: alertDirection
		});
		creatingAlert = false;
		if (!res.error && res.data) {
			alerts = [...alerts, res.data];
			alertRate = '';
		}
	}

	async function deleteAlert(id: string) {
		const res = await api.delete(`/remittance/alerts/${id}`);
		if (!res.error) {
			alerts = alerts.filter((a) => a.id !== id);
		}
	}

	async function toggleAlertActive(a: RemittanceAlert) {
		const res = await api.patch<RemittanceAlert>(`/remittance/alerts/${a.id}`, {
			is_active: !a.is_active
		});
		if (!res.error && res.data) {
			alerts = alerts.map((alert) => (alert.id === res.data!.id ? res.data! : alert));
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
		editAlertDirection = 'above';
	}

	async function saveEditAlert() {
		if (!editingAlert) return;
		savingEdit = true;
		const res = await api.patch<RemittanceAlert>(`/remittance/alerts/${editingAlert.id}`, {
			target_rate: parseFloat(editAlertRate),
			direction: editAlertDirection
		});
		savingEdit = false;
		if (!res.error && res.data) {
			alerts = alerts.map((a) => (a.id === res.data!.id ? res.data! : a));
			editingAlert = null;
		}
	}

	function ratioVsBest(r: RemittanceRate): string {
		if (!bestRate || bestRate.rate === 0) return '—';
		const ratio = (r.rate / bestRate.rate) * 100;
		return ratio.toFixed(1) + '%';
	}

	function bestCalcProvider(calc: RemittanceCalculation | null): RemittanceProviderCalc | null {
		if (!calc || !calc.providers || calc.providers.length === 0) return null;
		return calc.providers.reduce((best, p) => (p.receive_amount > best.receive_amount ? p : best), calc.providers[0]);
	}

	// Autocomplete fetchers — filter the already-loaded corridors client-side.
	function fetchSendCurrencies(q: string): Promise<AutocompleteItem[]> {
		const codes = [...new Set(corridors.map((c) => c.send_currency))];
		const query = q.toLowerCase();
		const filtered = query === '' ? codes : codes.filter((c) => c.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((c) => ({ id: c, label: c })));
	}
	function fetchSendCurrenciesInitial(): Promise<AutocompleteItem[]> {
		return fetchSendCurrencies('');
	}
	function fetchReceiveCurrencies(q: string): Promise<AutocompleteItem[]> {
		const codes = [...new Set(corridors.map((c) => c.receive_currency))];
		const query = q.toLowerCase();
		const filtered = query === '' ? codes : codes.filter((c) => c.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((c) => ({ id: c, label: c })));
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
	<div class="border-b border-[var(--color-border)] px-4 py-3">
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
							onSelect={(item) => { sendCurrency = item.id; onSelectCorridor(); }}
							onClear={() => { sendCurrency = ''; onSelectCorridor(); }}
						/>
					</div>
				</div>
				<ArrowRight size={16} class="pb-2 text-[var(--color-muted)]" />
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
							onSelect={(item) => { receiveCurrency = item.id; onSelectCorridor(); }}
							onClear={() => { receiveCurrency = ''; onSelectCorridor(); }}
						/>
					</div>
				</div>
			</div>
		</div>

		{#if sendCurrency && receiveCurrency}
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
									<tr class="border-b border-[var(--color-border)] text-left text-xs text-[var(--color-muted)]">
										<th class="px-3 py-2 font-medium">{t('remittance.provider', 'Provider')}</th>
										<th class="px-3 py-2 text-right font-medium">{t('remittance.rate', 'Rate')}</th>
										<th class="px-3 py-2 text-right font-medium">{t('remittance.fee', 'Fee')}</th>
										<th class="px-3 py-2 text-right font-medium">{t('remittance.transfer_time', 'Time')}</th>
										<th class="px-3 py-2 text-right font-medium">{t('remittance.vs_best', 'vs Best')}</th>
									</tr>
								</thead>
								<tbody>
									{#each rates as r (r.id)}
										<tr class="border-b border-[var(--color-border)] last:border-0">
											<td class="px-3 py-2 text-[var(--color-text)]">
												{r.provider_name}
												{#if bestRate && r.id === bestRate.id}
													<span class="ml-1 rounded-full bg-[var(--color-primary)] px-1.5 py-0.5 text-xs text-white">{t('remittance.best', 'Best')}</span>
												{/if}
											</td>
											<td class="px-3 py-2 text-right font-medium text-[var(--color-text)]">{r.rate.toFixed(2)}</td>
											<td class="px-3 py-2 text-right text-[var(--color-muted)]">{r.fee_flat ? r.fee_flat.toFixed(2) : '—'}</td>
											<td class="px-3 py-2 text-right text-[var(--color-muted)]">{r.transfer_time || '—'}</td>
											<td class="px-3 py-2 text-right text-[var(--color-muted)]">{ratioVsBest(r)}</td>
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
							<span class="text-sm text-[var(--color-muted)]">{sendCurrency}</span>
						</div>

						{#if calcLoading}
							<p class="mt-2 text-sm text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
						{:else if calculation && calculation.providers && calculation.providers.length > 0}
							{@const best = bestCalcProvider(calculation)}
							<p class="mt-2 text-sm text-[var(--color-text)]">
								{t('remittance.best_rate_label', 'Best value')}:
								<strong class="font-semibold">{best?.receive_amount.toFixed(2)} {receiveCurrency}</strong>
								<span class="text-[var(--color-muted)]">({t('remittance.via', 'via')} {best?.provider_name})</span>
							</p>
							<div class="mt-3 overflow-x-auto rounded-[var(--radius)] border border-[var(--color-border)]">
								<table class="w-full text-sm">
									<thead>
										<tr class="border-b border-[var(--color-border)] text-left text-xs text-[var(--color-muted)]">
											<th class="px-3 py-2 font-medium">{t('remittance.provider', 'Provider')}</th>
											<th class="px-3 py-2 text-right font-medium">{t('remittance.rate', 'Rate')}</th>
											<th class="px-3 py-2 text-right font-medium">{t('remittance.fee', 'Fee')}</th>
											<th class="px-3 py-2 text-right font-medium">{t('remittance.receive_amount', 'Receive')}</th>
											<th class="px-3 py-2 text-right font-medium">{t('remittance.total_cost', 'Total cost')}</th>
										</tr>
									</thead>
									<tbody>
										{#each calculation.providers as p (p.provider_id)}
											<tr class="border-b border-[var(--color-border)] last:border-0">
												<td class="px-3 py-2 text-[var(--color-text)]">{p.provider_name}</td>
												<td class="px-3 py-2 text-right">{p.rate.toFixed(2)}</td>
												<td class="px-3 py-2 text-right text-[var(--color-muted)]">{p.fee_flat ? p.fee_flat.toFixed(2) : '—'}</td>
												<td class="px-3 py-2 text-right font-medium">{p.receive_amount.toFixed(2)} {receiveCurrency}</td>
												<td class="px-3 py-2 text-right text-[var(--color-muted)]">{p.total_cost.toFixed(2)} {sendCurrency}</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{:else if bestRate}
							<p class="mt-2 text-sm text-[var(--color-text)]">
								≈ {(parseFloat(sendAmount || '0') * bestRate.rate).toFixed(2)} {receiveCurrency}
								<span class="text-[var(--color-muted)]"> ({t('remittance.via', 'via')} {bestRate.provider_name})</span>
							</p>
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
							{#if providers.length === 0}
								<p class="text-sm text-[var(--color-muted)]">{t('remittance.no_providers', 'No providers configured.')}</p>
							{:else}
								<ul class="space-y-2">
									{#each providers as p (p.id)}
										<li class="text-sm text-[var(--color-text)]">
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
										</li>
									{/each}
								</ul>
							{/if}
						{/if}
					</div>

					<!-- Rate alerts -->
					{#if authed}
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<h2 class="mb-3 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
								<Bell size={16} /> {t('remittance.alerts', 'Rate alerts')}
							</h2>
							<form onsubmit={(e) => { e.preventDefault(); createAlert(); }} class="mb-3 flex flex-wrap items-end gap-2">
								<div>
									<label for="alert-rate" class="block text-xs text-[var(--color-muted)]">{t('remittance.target_rate', 'Target rate')}</label>
									<input
										id="alert-rate"
										type="number"
										bind:value={alertRate}
										step="0.01"
										class="mt-1 w-28 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
									/>
								</div>
								<div>
									<label for="alert-direction" class="block text-xs text-[var(--color-muted)]">{t('remittance.direction', 'Direction')}</label>
									<div class="mt-1">
										<SelectField
											id="alert-direction"
											value={alertDirection}
											options={[
												{ value: 'above', label: t('remittance.above', 'Above') },
												{ value: 'below', label: t('remittance.below', 'Below') }
											]}
											placeholder={t('remittance.direction', 'Direction')}
											onValueChange={(v) => (alertDirection = v)}
										/>
									</div>
								</div>
								<button
									type="submit"
									disabled={creatingAlert || !alertRate}
									class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
								>
									{t('remittance.create_alert', 'Create alert')}
								</button>
							</form>
							{#if alerts.length > 0}
								<ul class="space-y-2">
									{#each alerts as a (a.id)}
										<li class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
											{#if editingAlert?.id === a.id}
												<form onsubmit={(e) => { e.preventDefault(); saveEditAlert(); }} class="flex flex-wrap items-end gap-2">
													<input
														type="number"
														bind:value={editAlertRate}
														step="0.01"
														class="w-28 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
													/>
													<SelectField
														value={editAlertDirection}
														options={[
															{ value: 'above', label: t('remittance.above', 'Above') },
															{ value: 'below', label: t('remittance.below', 'Below') }
														]}
														placeholder={t('remittance.direction', 'Direction')}
														onValueChange={(v) => (editAlertDirection = v)}
													/>
													<button
														type="submit"
														disabled={savingEdit}
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
														{a.send_currency}→{a.receive_currency} @ {a.target_rate}
														<span class="text-[var(--color-muted)]">({a.direction})</span>
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
					{/if}
				{/if}
			</div>
		{/if}
	{/if}
</div>
