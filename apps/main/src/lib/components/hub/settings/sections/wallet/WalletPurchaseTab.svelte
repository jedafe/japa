<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { purchaseCredits, setCurrencyPreference } from '$lib/api/hub';
	import { Wallet, Sparkles } from 'lucide-svelte';
	import type { WalletData } from './types';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	const preferredCode = $derived(data.preferredCurrency?.preferred_currency ?? '');
	const sortedCurrencies = $derived([...data.currencies].sort((a, b) => (a.position ?? 0) - (b.position ?? 0)));
	// Display currency for price computation: the user's preference, else the
	// first active currency — without it every fiat price renders "—".
	const currentCurrency = $derived(
		data.currencies.find((c) => c.code === preferredCode) ?? sortedCurrencies[0] ?? null
	);
	// 0 = settings endpoint unavailable → no client-side minimum; the
	// backend validates the real configured minimum on purchase.
	const minPurchase = $derived(data.minPurchaseCredits ?? 0);

	const sortedPacks = $derived([...data.creditPacks].sort((a, b) => (a.position ?? 0) - (b.position ?? 0)));

	let selectedPackId = $state<string>('');
	let customAmount = $state<string>('');
	let purchasing = $state(false);
	let error = $state<string | null>(null);
	let success = $state(false);
	let currencySaving = $state(false);

	// Clicking a currency chip persists it as the preferred currency (same
	// PATCH /me/preferences/currency call the Details tab uses), so prices
	// re-render immediately after invalidateAll.
	async function pickCurrency(code: string) {
		if (currencySaving || code === preferredCode) return;
		currencySaving = true;
		error = null;
		const res = await setCurrencyPreference(code);
		currencySaving = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		await invalidateAll();
	}

	function fiatPrice(credits: number): string {
		if (!currentCurrency || !currentCurrency.credits_per_unit || currentCurrency.credits_per_unit <= 0) return '—';
		const major = (credits * 100) / (currentCurrency.credits_per_unit * 100);
		return `${currentCurrency.symbol}${major.toFixed(2)}`;
	}

	function packTotalCredits(pack: { credit_amount: number; bonus_credits?: number }): number {
		return pack.credit_amount + (pack.bonus_credits ?? 0);
	}

	async function purchase() {
		const amount = parseInt(customAmount, 10) || 0;
		if (!selectedPackId && (amount <= 0 || (minPurchase > 0 && amount < minPurchase))) {
			error = minPurchase > 0
				? t('wallet.purchase.minimum', 'Minimum {n} credits').replace('{n}', String(minPurchase))
				: t('wallet.withdraw.invalid_amount', 'Please enter a valid credit amount.');
			return;
		}

		purchasing = true;
		error = null;
		success = false;

		// provider omitted — the backend selects the configured default
		// (POST /wallet/purchase-credits treats provider as optional).
		const res = await purchaseCredits({
			pack_id: selectedPackId || undefined,
			credit_amount: selectedPackId ? 0 : amount,
			currency: preferredCode
		});

		purchasing = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data?.checkout_url) {
			window.location.href = res.data.checkout_url;
			return;
		}
		success = true;
		selectedPackId = '';
		customAmount = '';
		await invalidateAll();
	}
</script>

<div class="space-y-6">
	{#if data.currencies.length === 0}
		<p class="rounded-[var(--radius)] bg-[var(--color-warning)]/10 px-3 py-2 text-sm text-[var(--color-warning)]">
			{t('wallet.purchase.no_currencies', 'No currencies are currently configured. Please check back later.')}
		</p>
	{:else}
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-sm font-medium text-[var(--color-text)]">{t('wallet.purchase.your_currency', 'Your Currency')}</p>
			<div class="mt-2 flex flex-wrap gap-2">
				{#each sortedCurrencies as c (c.code)}
					<button
						type="button"
						disabled={currencySaving}
						onclick={() => pickCurrency(c.code)}
						class="rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors disabled:cursor-wait {c.code === currentCurrency?.code
							? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
							: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40 hover:text-[var(--color-text)]'}"
					>
						<span class="me-1 text-base">{c.symbol}</span>
						{c.code}
					</button>
				{/each}
			</div>
			{#if currentCurrency}
				<p class="mt-2 text-xs text-[var(--color-muted)]">
					{t('wallet.purchase.credits_per_unit', '1 {code} = {n} credits')
						.replace('{code}', currentCurrency.code)
						.replace('{n}', currentCurrency.credits_per_unit?.toLocaleString() ?? '0')}
				</p>
			{/if}
		</div>

		{#if sortedPacks.length > 0}
			<div class="space-y-3">
				<h2 class="text-sm font-medium text-[var(--color-text)]">{t('wallet.purchase.credit_packs', 'Credit Packs')}</h2>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each sortedPacks as pack (pack.id)}
						<button
							type="button"
							class="rounded-[var(--radius-card)] border p-4 text-start transition-all hover:border-[var(--color-primary)]/40 {selectedPackId === pack.id
								? 'border-[var(--color-primary)] ring-2 ring-[var(--color-primary)]/20'
								: 'border-[var(--color-border)] bg-[var(--color-surface)]'}"
							onclick={() => { selectedPackId = selectedPackId === pack.id ? '' : pack.id; customAmount = ''; }}
						>
							<div class="flex items-baseline justify-between">
								<h3 class="font-medium text-[var(--color-text)]">{pack.name}</h3>
								{#if pack.bonus_credits}
									<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-success)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-success)]">
										<Sparkles size={10} />
										+{pack.bonus_credits.toLocaleString()} {t('wallet.purchase.bonus', 'bonus')}
									</span>
								{/if}
							</div>
							{#if pack.description}
								<p class="mt-1 text-xs text-[var(--color-muted)]">{pack.description}</p>
							{/if}
							<div class="mt-3 flex items-baseline justify-between">
								<div>
									<p class="text-2xl font-bold text-[var(--color-text)]">{packTotalCredits(pack).toLocaleString()}</p>
									<p class="text-xs text-[var(--color-muted)]">{t('wallet.purchase.credits', 'credits')}</p>
								</div>
								<div class="text-end">
									<p class="text-lg font-semibold text-[var(--color-text)]">{fiatPrice(pack.credit_amount)}</p>
									<p class="text-xs text-[var(--color-muted)]">{preferredCode}</p>
								</div>
							</div>
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('wallet.purchase.custom_amount', 'Custom Amount')}</h2>
			<div class="flex items-end gap-3">
				<div class="flex-1">
					<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="custom-credits">
						{t('wallet.purchase.credits', 'credits')}
					</label>
					<input
						id="custom-credits"
						type="number"
						min={minPurchase || 1}
						bind:value={customAmount}
						oninput={() => (selectedPackId = '')}
						aria-describedby="custom-credits-hint"
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						placeholder={t('wallet.purchase.enter_amount', 'Enter credit amount')}
					/>
					<HintIcon id="custom-credits-hint" text={t('wallet.purchase.credits_hint', 'Number of wallet credits to buy — the price updates as you type.')} />
				</div>
				<div class="text-end">
					<p class="text-sm text-[var(--color-muted)]">{t('wallet.purchase.price', 'Price')}</p>
					<p class="text-lg font-semibold text-[var(--color-text)]">
						{#if parseInt(customAmount, 10) > 0}
							{fiatPrice(parseInt(customAmount, 10))}
						{:else}
							—
						{/if}
					</p>
				</div>
			</div>
			{#if minPurchase > 0}
				<p class="mt-2 text-xs text-[var(--color-muted)]">
					{t('wallet.purchase.minimum', 'Minimum {n} credits').replace('{n}', minPurchase.toLocaleString())}
				</p>
			{/if}
		</div>

		{#if error}
			<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{error}</p>
		{/if}
		{#if success}
			<p class="rounded-[var(--radius)] bg-[var(--color-success)]/10 px-3 py-2 text-sm text-[var(--color-success)]" role="status">
				{t('wallet.purchase.initiated', 'Purchase Initiated')}
			</p>
		{/if}

		<div class="flex justify-end">
			<button
				type="button"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-3 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
				onclick={purchase}
				disabled={purchasing || (selectedPackId === '' && ((parseInt(customAmount, 10) || 0) <= 0 || (minPurchase > 0 && (parseInt(customAmount, 10) || 0) < minPurchase)))}
			>
				<Wallet size={16} />
				{#if purchasing}
					{t('wallet.purchase.processing', 'Processing...')}
				{:else}
					{t('wallet.purchase.submit', 'Purchase Credits')}
				{/if}
			</button>
		</div>
	{/if}
</div>
