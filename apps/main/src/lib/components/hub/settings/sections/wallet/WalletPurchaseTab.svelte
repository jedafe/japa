<script lang="ts">
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { purchaseCredits } from '$lib/api/hub';
	import { Wallet, Sparkles } from 'lucide-svelte';
	import type { WalletData } from './types';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	const preferredCode = $derived(data.preferredCurrency?.preferred_currency ?? '');
	const currentCurrency = $derived(data.currencies.find((c) => c.code === preferredCode) ?? null);
	const minPurchase = $derived(data.minPurchaseCredits ?? 100);

	const sortedCurrencies = $derived([...data.currencies].sort((a, b) => (a.position ?? 0) - (b.position ?? 0)));
	const sortedPacks = $derived([...data.creditPacks].sort((a, b) => (a.position ?? 0) - (b.position ?? 0)));

	let selectedPackId = $state<string>('');
	let customAmount = $state<string>('');
	let purchasing = $state(false);
	let error = $state<string | null>(null);
	let success = $state(false);

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
		if (!selectedPackId && amount < minPurchase) {
			error = t('wallet.purchase.minimum', 'Minimum {n} credits').replace('{n}', String(minPurchase));
			return;
		}

		purchasing = true;
		error = null;
		success = false;

		const res = await purchaseCredits({
			pack_id: selectedPackId || undefined,
			credit_amount: selectedPackId ? 0 : amount,
			currency: preferredCode,
			provider: 'paystack'
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
					<span
						class="rounded-[var(--radius)] border px-3 py-2 text-sm {c.code === preferredCode
							? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
							: 'border-[var(--color-border)] text-[var(--color-muted)]'}"
					>
						<span class="mr-1 text-base">{c.symbol}</span>
						{c.code}
					</span>
				{/each}
			</div>
			{#if currentCurrency}
				<p class="mt-2 text-xs text-[var(--color-muted)]">
					{t('wallet.purchase.credits_per_unit', '1 {code} = {n} credits')
						.replace('{code}', currentCurrency.code)
						.replace('{n}', currentCurrency.credits_per_unit?.toLocaleString() ?? '0')}
				</p>
			{/if}
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{t('settings.wallet.purchase_currency_hint', 'Change your preferred currency on the Details tab.')}
			</p>
		</div>

		{#if sortedPacks.length > 0}
			<div class="space-y-3">
				<h2 class="text-sm font-medium text-[var(--color-text)]">{t('wallet.purchase.credit_packs', 'Credit Packs')}</h2>
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each sortedPacks as pack (pack.id)}
						<button
							type="button"
							class="rounded-[var(--radius-card)] border p-4 text-left transition-all hover:border-[var(--color-primary)]/40 {selectedPackId === pack.id
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
								<div class="text-right">
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
						min={minPurchase}
						step={minPurchase}
						bind:value={customAmount}
						oninput={() => (selectedPackId = '')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						placeholder={t('wallet.purchase.enter_amount', 'Enter credit amount')}
					/>
				</div>
				<div class="text-right">
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
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				{t('wallet.purchase.minimum', 'Minimum {n} credits').replace('{n}', minPurchase.toLocaleString())}
			</p>
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
				disabled={purchasing || (selectedPackId === '' && (parseInt(customAmount, 10) || 0) < minPurchase)}
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
