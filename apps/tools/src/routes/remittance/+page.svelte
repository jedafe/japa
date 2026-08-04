<script lang="ts">
	import { api } from '$lib/api/client';
	import { mainAppUrl } from '$lib/seo';
	import { buildSeoMeta } from '$lib/seo';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import { LineChart } from 'layerchart';
	import type {
		RemittanceCorridor,
		RemittanceProvider,
		RemittanceRate,
		RemittanceAlert,
		RemittanceHistoryPoint,
		RemittanceCalculation
	} from '$lib/types';

	/**
	 * Remittance Rate Tracker tool — §74, ADR-075.
	 * Corridor selector → provider rate table → trend chart (LayerChart) →
	 * calculator widget → rate alerts (auth only).
	 */
	let { data } = $props();

	let corridors = $state<RemittanceCorridor[]>(data.corridors);
	let providers = $state<RemittanceProvider[]>(data.providers);
	let rates = $state<RemittanceRate[]>(data.rates);
	let alerts = $state<RemittanceAlert[]>(data.alerts);
	let user = $state(data.user);

	let selectedCorridor = $state<RemittanceCorridor>(data.defaultCorridor);
	let history = $state<RemittanceHistoryPoint[]>([]);
	let loadingRates = $state(false);
	let loadingHistory = $state(false);

	// Calculator state
	let calcSendAmount = $state(1000);
	let calcResult = $state<RemittanceCalculation | null>(null);
	let calcLoading = $state(false);

	// Alert creation state
	let alertTargetRate = $state('');
	let alertDirection = $state<'above' | 'below'>('above');
	let showRegisterPrompt = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: 'Remittance Rate Tracker — JapaConnect Tools',
			description:
				'Compare live remittance rates across providers, view historical trends, calculate transfer costs, and set rate alerts.',
			path: '/remittance',
			origin: page.url.origin
		})
	);

	// Best rate
	const bestRate = $derived.by(() => {
		if (rates.length === 0) return null;
		return rates.reduce((best, r) => (r.rate > best.rate ? r : best));
	});

	async function loadRates() {
		loadingRates = true;
		const res = await api.get<RemittanceRate[]>(
			`/remittance/rates?send=${selectedCorridor.send_currency}&receive=${selectedCorridor.receive_currency}`
		);
		loadingRates = false;
		if (!res.error) rates = res.data ?? [];
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
		calcLoading = true;
		const res = await api.get<RemittanceCalculation>(
			`/remittance/calculate?send_amount=${calcSendAmount}&send=${selectedCorridor.send_currency}&receive=${selectedCorridor.receive_currency}`
		);
		calcLoading = false;
		if (!res.error) calcResult = res.data;
	}

	async function createAlert() {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const res = await api.post<RemittanceAlert>('/remittance/alerts', {
			send_currency: selectedCorridor.send_currency,
			receive_currency: selectedCorridor.receive_currency,
			target_rate: parseFloat(alertTargetRate),
			direction: alertDirection
		});
		if (!res.error && res.data) {
			alerts = [...alerts, res.data];
			alertTargetRate = '';
		}
	}

	async function deleteAlert(id: string) {
		const res = await api.delete(`/remittance/alerts/${id}`);
		if (!res.error) alerts = alerts.filter((a) => a.id !== id);
	}

	function formatRate(rate: number): string {
		return rate.toFixed(2);
	}

	function formatMoney(amount: number, currency: string): string {
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: currency.toUpperCase(),
			maximumFractionDigits: 2
		}).format(amount);
	}

	function onCorridorChange(e: Event) {
		const value = (e.currentTarget as HTMLSelectElement).value;
		const corridor = corridors.find(
			(c) => `${c.send_currency}-${c.receive_currency}` === value
		);
		if (corridor) {
			selectedCorridor = corridor;
			loadRates();
		}
	}

	// Prepare history data for LayerChart — group by provider for multi-line
	const chartData = $derived.by(() => {
		return history.map((p) => ({
			timestamp: new Date(p.timestamp),
			rate: p.rate,
			provider: p.provider_name
		}));
	});
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-5xl px-4 py-8">
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">Remittance Rate Tracker</h1>
		<p class="mt-2 text-text-subtle">
			Compare live rates across providers, view trends, and set alerts.
		</p>
	</div>

	{#if corridors.length === 0}
		<EmptyState
			icon="arrow-left-right"
			title="No corridors available"
			description="Remittance data hasn't been configured yet. Please check back later."
		/>
	{:else}
		<!-- Corridor selector -->
		<div class="mb-6 flex items-center gap-3">
			<label for="corridor" class="text-sm font-medium text-text">Corridor:</label>
			<select
				id="corridor"
				onchange={onCorridorChange}
				class="px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
			>
				{#each corridors as c (c.label)}
					<option value={`${c.send_currency}-${c.receive_currency}`}>{c.label}</option>
				{/each}
			</select>
		</div>

		<div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
			<!-- Rate table -->
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					Current rates
				</h2>
				{#if loadingRates}
					<div class="space-y-2">
						<LoadingSkeleton height="60px" />
						<LoadingSkeleton height="60px" />
						<LoadingSkeleton height="60px" />
					</div>
				{:else if rates.length === 0}
					<EmptyState
						icon="trending-down"
						title="No rates available"
						description="No providers have rates for this corridor right now."
					/>
				{:else}
					<div class="space-y-2">
						{#if bestRate}
							<div class="p-3 bg-[var(--tool-accent)]/5 border border-[var(--tool-accent)]/30 rounded-lg flex items-center justify-between">
								<div>
									<span class="text-xs font-medium text-[var(--tool-accent)]">Best rate</span>
									<p class="text-sm font-semibold text-text">{bestRate.provider_name}</p>
								</div>
								<span class="text-lg font-bold text-[var(--tool-accent)]">
									{formatRate(bestRate.rate)}
								</span>
							</div>
						{/if}
						{#each rates as rate (rate.id)}
							<div class="p-3 bg-surface border border-border rounded-lg flex items-center justify-between">
								<div>
									<p class="text-sm font-medium text-text">{rate.provider_name}</p>
									<div class="flex items-center gap-2 text-xs text-muted mt-0.5">
										{#if rate.fee_flat}
											<span>Fee: {formatMoney(rate.fee_flat, rate.send_currency)}</span>
										{/if}
										{#if rate.transfer_time}
											<span>· {rate.transfer_time}</span>
										{/if}
										{#if rate.is_manual}
											<span class="text-yellow-600">· manual</span>
										{/if}
									</div>
								</div>
								<div class="text-right">
									<span class="text-base font-semibold text-text">{formatRate(rate.rate)}</span>
									<p class="text-xs text-muted">{rate.receive_currency} per {rate.send_currency}</p>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Calculator -->
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
					Calculator
				</h2>
				<div class="p-4 bg-surface border border-border rounded-lg">
					<div class="flex items-center gap-2 mb-3">
						<input
							type="number"
							bind:value={calcSendAmount}
							oninput={() => calculate()}
							min="1"
							class="w-32 px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
						/>
						<span class="text-sm text-muted">{selectedCorridor.send_currency}</span>
						<Icon name="arrow-right" size={16} class="text-muted" />
						<span class="text-sm text-muted">{selectedCorridor.receive_currency}</span>
					</div>

					{#if calcLoading}
						<LoadingSkeleton height="40px" />
					{:else if calcResult?.results?.length}
						<div class="space-y-2">
							{#each calcResult.results as row (row.provider_id)}
								<div class="flex items-center justify-between text-sm">
									<span class="text-text">{row.provider_name}</span>
									<span class="font-semibold text-text">
										{formatMoney(row.receive_amount, calcResult.receive_currency)}
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
				30-day rate history
			</h2>
			{#if loadingHistory}
				<LoadingSkeleton height="240px" />
			{:else if history.length > 0}
				<div class="p-4 bg-surface border border-border rounded-lg">
					<LineChart
						data={chartData}
						x="timestamp"
						y="rate"
						color="provider"
						{...{ width: 800, height: 240 }}
					/>
				</div>
			{:else}
				<EmptyState
					icon="chart-line"
					title="No history available"
					description="Historical rate data will appear here once enough data is collected."
				/>
			{/if}
		</div>

		<!-- Rate alerts -->
		<div class="mt-8">
			<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
				Rate alerts
			</h2>
			{#if !user}
				<div class="p-4 bg-surface border border-border rounded-lg text-center">
					<p class="text-sm text-text-subtle mb-3">
						Sign in to create rate alerts and get notified when your target rate is hit.
					</p>
					<a
						href={mainAppUrl('/login')}
						class="inline-flex items-center gap-1.5 px-4 py-2 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
					>
						Sign in
					</a>
				</div>
			{:else}
				<div class="space-y-4">
					<!-- Create alert -->
					<div class="p-4 bg-surface border border-border rounded-lg flex flex-wrap items-center gap-3">
						<span class="text-sm text-text">Notify me when</span>
						<select
							bind:value={alertDirection}
							class="px-3 py-1.5 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						>
							<option value="above">rate goes above</option>
							<option value="below">rate drops below</option>
						</select>
						<input
							type="number"
							bind:value={alertTargetRate}
							placeholder="target rate"
							step="0.01"
							class="w-28 px-3 py-1.5 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
						<span class="text-sm text-muted">
							{selectedCorridor.receive_currency}/{selectedCorridor.send_currency}
						</span>
						<button
							onclick={createAlert}
							disabled={!alertTargetRate}
							class="px-4 py-1.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
						>
							Create alert
						</button>
					</div>

					<!-- Existing alerts -->
					{#if alerts.length > 0}
						<div class="space-y-2">
							{#each alerts.filter((a) => a.send_currency === selectedCorridor.send_currency && a.receive_currency === selectedCorridor.receive_currency) as alert (alert.id)}
								<div class="flex items-center justify-between p-3 bg-surface border border-border rounded-lg">
									<div class="text-sm">
										<span class="text-text">Rate {alert.direction} </span>
										<span class="font-semibold text-text">{formatRate(alert.target_rate)}</span>
										<span class="text-muted"> ({alert.send_currency}→{alert.receive_currency})</span>
										{#if !alert.is_active}
											<span class="ml-2 text-xs text-muted">(inactive)</span>
										{/if}
									</div>
									<button
										onclick={() => deleteAlert(alert.id)}
										class="p-1.5 text-muted hover:text-red-500 transition-colors"
										aria-label="Delete alert"
									>
										<Icon name="trash-2" size={16} />
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{/if}

	{#if showRegisterPrompt}
		<div
			class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
			onclick={() => (showRegisterPrompt = false)}
			onkeydown={(e) => e.key === 'Escape' && (showRegisterPrompt = false)}
			role="button"
			tabindex="0"
		>
			<div
				class="bg-surface rounded-[var(--radius-card)] p-6 max-w-sm w-full text-center"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
				role="button"
				tabindex="0"
			>
				<div class="flex justify-center mb-4">
					<div class="w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center">
						<Icon name="bell" size={24} class="text-primary" />
					</div>
				</div>
				<h3 class="text-lg font-semibold text-text">Create rate alerts</h3>
				<p class="mt-2 text-sm text-text-subtle">
					Create a free account to set rate alerts and get notified when your target rate is hit.
				</p>
				<div class="mt-5 flex flex-col gap-2">
					<a
						href={mainAppUrl('/signup')}
						class="px-4 py-2.5 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
					>
						Create free account
					</a>
					<a
						href={mainAppUrl('/login')}
						class="px-4 py-2.5 text-sm font-medium text-text hover:text-primary transition-colors"
					>
						Sign in
					</a>
				</div>
			</div>
		</div>
	{/if}
</div>
