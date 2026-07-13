<script lang="ts">
	import {
		Wallet,
		ArrowRightLeft,
		History,
		Loader2,
		Coins,
		TrendingUp,
		TrendingDown,
		Plus,
		X,
		CreditCard
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { goto } from '$app/navigation';

	// TODO: Move to system_settings table via GET /api/v1/system-settings/wallet.preset_amounts
	// Ref: AGENTS.md Rule 3 — configurable values must come from system_settings
	const presetAmounts = [100000, 500000, 1000000]; // in kobo: ₦1,000, ₦5,000, ₦10,000

	let { data } = $props();

	let balance = $derived(data.balance);
	let ledger = $derived(data.ledger);
	let converting = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');

	// Top-up modal
	let showTopUpModal = $state(false);
	let selectedAmount = $state<number>(presetAmounts[0]);
	let customAmount = $state('');
	let useCustom = $state(false);
	let topUpProcessing = $state(false);
	let topUpError = $state('');

	function formatNumber(n: number): string {
		return new Intl.NumberFormat('en-US').format(n);
	}

	function formatKobo(kobo: number): string {
		return '₦' + new Intl.NumberFormat('en-US').format(kobo / 100).replace(/\.00$/, '');
	}

	import { formatTimestampWithTime } from '$lib/utils/timezone';

	function formatDate(dateStr: string): string {
		return formatTimestampWithTime(dateStr);
	}

	function isPositiveEntry(type: string): boolean {
		return type === 'credit' || type === 'earn' || type === 'convert_in';
	}

	function entryColor(type: string): string {
		if (type === 'credit' || type === 'earn' || type === 'convert_in') return 'var(--color-success, #22c55e)';
		return 'var(--color-danger)';
	}

	async function convertPointsToCredits() {
		if (balance.points <= 0) return;
		converting = true;
		errorMessage = '';
		successMessage = '';

		try {
			const result = await api.convertWallet({ points: balance.points });
			balance = result;
			successMessage = 'Points converted to credits successfully!';
			const ledgerResult = await api.getWalletLedger({ page_size: 30 });
			ledger = ledgerResult;
		} catch {
			errorMessage = 'Failed to convert points. Please try again.';
		} finally {
			converting = false;
		}
	}

	function openTopUpModal() {
		selectedAmount = presetAmounts[0];
		customAmount = '';
		useCustom = false;
		topUpError = '';
		showTopUpModal = true;
	}

	function closeTopUpModal() {
		showTopUpModal = false;
	}

	function selectPreset(amount: number) {
		selectedAmount = amount;
		useCustom = false;
		customAmount = '';
	}

	function handleCustomInput() {
		useCustom = true;
		const parsed = parseInt(customAmount.replace(/[^0-9]/g, ''), 10);
		if (!isNaN(parsed) && parsed > 0) {
			selectedAmount = parsed * 100; // Convert naira to kobo
		}
	}

	const topUpAmount = $derived(useCustom ? selectedAmount : selectedAmount);

	async function processTopUp() {
		if (topUpAmount <= 0) return;
		topUpProcessing = true;
		topUpError = '';

		try {
			const result = await api.initiatePayment({
				amount: topUpAmount,
				currency: 'ngn',
				provider: 'paystack',
				purpose: 'wallet_topup',
				metadata: { source: 'wallet_topup' }
			});

			if (result.checkout_url) {
				window.location.href = result.checkout_url;
			} else {
				closeTopUpModal();
				// Re-fetch wallet data
				try {
					const [balanceRes, ledgerRes] = await Promise.allSettled([
						api.getWalletBalance(),
						api.getWalletLedger({ page_size: 30 })
					]);
					if (balanceRes.status === 'fulfilled') balance = balanceRes.value;
					if (ledgerRes.status === 'fulfilled') ledger = ledgerRes.value;
				} catch {
					// use defaults
				}
				successMessage = 'Wallet topped up successfully!';
			}
		} catch (err) {
			topUpError = err instanceof Error ? err.message : 'Failed to initiate top-up';
		} finally {
			topUpProcessing = false;
		}
	}
</script>

<svelte:head>
	<title>Wallet | JapaConnect</title>
	<meta name="description" content="Manage your points and credits." />
</svelte:head>

{#if !isFeatureEnabled('feature.wallet')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('wallet.not_available')}</h1>
			<p class="mt-2" style="color: var(--color-muted)">{t('forum.feature_disabled')}</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">{t('forum.go_home')}</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-4xl px-4 py-8">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
			{t('wallet.title')}
		</h1>
		<button
			class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
			onclick={openTopUpModal}
		>
			<Plus class="h-4 w-4" />
			{t('wallet.add_credits')}
		</button>
	</div>

		<!-- Balance Cards -->
		<div class="mt-8 grid gap-4 sm:grid-cols-2">
			<div
				class="rounded-xl border p-6"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<div class="flex items-center gap-3">
					<div
						class="flex h-12 w-12 items-center justify-center rounded-xl"
						style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)"
					>
						<Coins class="h-6 w-6" style="color: var(--color-primary)" />
					</div>
					<div>
						<p class="text-sm" style="color: var(--color-muted)">{t('wallet.points')}</p>
						<p class="text-2xl font-bold" style="color: var(--color-foreground)">
							{formatNumber(balance.points)}
						</p>
					</div>
				</div>
			</div>

			<div
				class="rounded-xl border p-6"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<div class="flex items-center gap-3">
					<div
						class="flex h-12 w-12 items-center justify-center rounded-xl"
						style="background-color: color-mix(in srgb, var(--color-success, #22c55e) 12%, transparent)"
					>
						<Wallet class="h-6 w-6" style="color: var(--color-success, #22c55e)" />
					</div>
					<div>
						<p class="text-sm" style="color: var(--color-muted)">{t('wallet.credits')}</p>
						<p class="text-2xl font-bold" style="color: var(--color-foreground)">
							{formatNumber(balance.credits)}
						</p>
					</div>
				</div>
			</div>
		</div>

		<!-- Convert Button -->
		{#if balance.points > 0}
			<div class="mt-8">
				<button
					onclick={convertPointsToCredits}
					disabled={converting}
					class="flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
				>
					{#if converting}
						<Loader2 class="h-4 w-4 animate-spin" />
						{t('wallet.converting')}
					{:else}
						<ArrowRightLeft class="h-4 w-4" />
						{t('wallet.convert_points').replace('{count}', formatNumber(balance.points))}
					{/if}
				</button>
				<p class="mt-2 text-xs" style="color: var(--color-muted)">
					{t('wallet.convert_ratio')}
				</p>
			</div>
		{/if}

		{#if errorMessage}
			<div
				class="mt-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
			>
				{errorMessage}
			</div>
		{/if}

		{#if successMessage}
			<div
				class="mt-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-success, #22c55e); background-color: color-mix(in srgb, var(--color-success, #22c55e) 8%, transparent); color: var(--color-success, #22c55e)"
			>
				{successMessage}
			</div>
		{/if}

		<!-- Transaction Ledger -->
		<div
			class="mt-8 rounded-xl border"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<div class="flex items-center gap-2 border-b px-6 py-4" style="border-color: var(--color-border)">
				<History class="h-5 w-5" style="color: var(--color-muted)" />
				<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">
					{t('wallet.recent_activity')}
				</h2>
			</div>

			{#if ledger.length === 0}
				<EmptyState
					icon={History}
					title={t('wallet.no_transactions')}
					message={t('wallet.no_transactions_msg')}
				/>
			{:else}
				<div class="divide-y" style="divide-color: var(--color-border)">
					{#each ledger as entry (entry.id)}
						<div class="flex items-center justify-between px-6 py-4">
							<div class="flex items-center gap-3">
								<div
									class="flex h-9 w-9 items-center justify-center rounded-full"
									style="background-color: color-mix(in srgb, {entryColor(entry.type)} 12%, transparent)"
								>
									{#if isPositiveEntry(entry.type)}
										<TrendingUp class="h-4 w-4" style="color: {entryColor(entry.type)}" />
									{:else}
										<TrendingDown class="h-4 w-4" style="color: {entryColor(entry.type)}" />
									{/if}
								</div>
								<div>
									<p class="text-sm font-medium" style="color: var(--color-foreground)">
										{entry.description}
									</p>
									<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
										{formatDate(entry.created_at)}
									</p>
								</div>
							</div>
							<div class="text-right">
								<p class="text-sm font-semibold" style="color: {entryColor(entry.type)}">
									{entry.amount > 0 ? '+' : ''}{formatNumber(entry.amount)}
								</p>
								<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
									Balance: {formatNumber(entry.balance_after)}
								</p>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</main>
{/if}

<!-- Top-Up Modal -->
{#if showTopUpModal}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		onclick={closeTopUpModal}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-sm rounded-xl border p-6"
			style="border-color: var(--color-border); background: var(--color-surface)"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="flex items-center justify-between">
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">
					{t('wallet.add_credits')}
				</h3>
				<button
					class="rounded-md p-1 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
					onclick={closeTopUpModal}
				>
					<X class="h-5 w-5" style="color: var(--color-muted)" />
				</button>
			</div>

			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('wallet.choose_amount')}
			</p>

			{#if topUpError}
				<div
					class="mt-3 rounded-lg border px-3 py-2 text-sm"
					style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
				>
					{topUpError}
				</div>
			{/if}

			<!-- Preset Amounts -->
			<div class="mt-4 grid grid-cols-3 gap-2">
				{#each presetAmounts as amount (amount)}
					<button
						class="rounded-lg border p-3 text-center transition-colors"
						style="border-color: {!useCustom && selectedAmount === amount ? 'var(--color-primary)' : 'var(--color-border)'}; background: {!useCustom && selectedAmount === amount ? 'color-mix(in srgb, var(--color-primary) 5%, transparent)' : 'transparent'}"
						onclick={() => selectPreset(amount)}
					>
						<p class="text-sm font-semibold" style="color: var(--color-foreground)">
							{formatKobo(amount)}
						</p>
					</button>
				{/each}
			</div>

			<!-- Custom Amount -->
			<div class="mt-4">
				<label class="text-sm font-medium" style="color: var(--color-foreground)" for="custom-amount">
					{t('wallet.custom_amount')} (₦)
				</label>
				<input
					id="custom-amount"
					type="text"
					bind:value={customAmount}
					oninput={handleCustomInput}
					placeholder={t('wallet.enter_amount')}
					class="mt-1 w-full rounded-lg border px-3 py-2 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
				/>
			</div>

			<!-- Summary -->
			<div class="mt-4 rounded-lg p-3" style="background: var(--color-background)">
				<div class="flex justify-between text-sm">
					<span style="color: var(--color-muted)">{t('wallet.amount')}</span>
					<span class="font-semibold" style="color: var(--color-foreground)">
						{formatKobo(topUpAmount)}
					</span>
				</div>
			</div>

			<div class="mt-6 flex gap-3">
				<button
					class="flex-1 rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/5"
					style="border-color: var(--color-border); color: var(--color-foreground)"
					onclick={closeTopUpModal}
				>
					{t('common.cancel')}
				</button>
				<button
					class="flex flex-1 items-center justify-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={topUpProcessing || topUpAmount <= 0}
					onclick={processTopUp}
				>
					{#if topUpProcessing}
						<Loader2 class="h-4 w-4 animate-spin" />
						{t('wallet.processing')}
					{:else}
						<CreditCard class="h-4 w-4" />
						{t('wallet.pay')} {formatKobo(topUpAmount)}
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}
