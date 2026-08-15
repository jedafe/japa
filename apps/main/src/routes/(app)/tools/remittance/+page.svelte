<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Banknote, Lock, Bell, ArrowRight, TrendingUp } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { LineChart } from 'layerchart';
	import type {
		RemittanceCorridor,
		RemittanceRate,
		RemittanceHistoryPoint,
		RemittanceAlert
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

	// Calculator widget.
	let sendAmount = $state('1000');

	// Alert creation.
	let alertRate = $state('');
	let alertDirection = $state('above');
	let creatingAlert = $state(false);

	const bestRate = $derived(rates.length > 0 ? rates.reduce((best, r) => (r.rate > best.rate ? r : best), rates[0]) : null);

	const chartData = $derived(
		history.map((h) => ({ date: new Date(h.day), avg: h.avg_rate, min: h.min_rate, max: h.max_rate }))
	);

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

	function ratioVsBest(r: RemittanceRate): string {
		if (!bestRate || bestRate.rate === 0) return '—';
		const ratio = (r.rate / bestRate.rate) * 100;
		return ratio.toFixed(1) + '%';
	}

	// Autocomplete fetchers — filter the already-loaded corridors client-side.
	// Send currencies are the distinct send_currency values; receive currencies
	// are the distinct receive_currency values from the corridors list.
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
								min="1"
								class="w-32 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
							/>
							<span class="text-sm text-[var(--color-muted)]">{sendCurrency}</span>
						</div>
						{#if bestRate}
							<p class="mt-2 text-sm text-[var(--color-text)]">
								≈ {(parseFloat(sendAmount || '0') * bestRate.rate).toFixed(2)} {receiveCurrency}
								<span class="text-[var(--color-muted)]"> ({t('remittance.via', 'via')} {bestRate.provider_name})</span>
							</p>
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
										<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
											<span class="text-[var(--color-text)]">
												{a.send_currency}→{a.receive_currency} @ {a.target_rate}
												<span class="text-[var(--color-muted)]">({a.direction})</span>
											</span>
											<button
												type="button"
												onclick={() => deleteAlert(a.id)}
												class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
											>
												{t('common.remove', 'Remove')}
											</button>
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
