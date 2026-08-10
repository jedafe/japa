<script lang="ts">
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	// Credit purchase page. Currencies + credit packs are fetched server-side
	// (Rule 16 — no hardcoded option lists). The purchase POST sends an
	// Idempotency-Key header so the backend idempotency middleware (Rule 9)
	// can deduplicate retries and prevent double-charges.
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { invalidate } from '$app/navigation';
	import { fly } from 'svelte/transition';
	import { Wallet, Sparkles, Check } from 'lucide-svelte';

	type Currency = {
		code: string;
		name: string;
		symbol: string;
		credits_per_unit: number;
		position: number;
	};

	type CreditPack = {
		id: string;
		name: string;
		description: string | null;
		credit_amount: number;
		bonus_credits: number;
		min_user_level: string | null;
		position: number;
	};

	type PurchaseResult = {
		payment_id: string;
		provider: string;
		currency: string;
		fiat_amount: number;
		credits_to_grant: number;
	};

	// §6.1 — do not destructure $derived(page.data); the destructured values
	// would capture the initial snapshot and never update. Access via deriveds.
	const currencies = $derived(page.data.currencies as Currency[]);
	const creditPacks = $derived(page.data.creditPacks as CreditPack[]);
	const minPurchaseCredits = $derived(page.data.minPurchaseCredits as number);

	let selectedCurrency = $state<string>('');
	let customAmount = $state<number>(0);
	let selectedPackId = $state<string>('');
	let purchasing = $state(false);
	let errorMessage = $state('');
	let successResult = $state<PurchaseResult | null>(null);

	// Initialize selected currency from the first currency in the list.
	$effect(() => {
		if (currencies.length > 0 && !selectedCurrency) {
			selectedCurrency = currencies[0].code;
		}
	});

	const sortedCurrencies = $derived(
		[...currencies].sort((a, b) => a.position - b.position)
	);

	const sortedPacks = $derived(
		[...creditPacks].sort((a, b) => a.position - b.position)
	);

	const currentCurrency = $derived(
		currencies.find((c: Currency) => c.code === selectedCurrency) ?? null
	);

	function fiatPrice(credits: number): string {
		if (!currentCurrency || currentCurrency.credits_per_unit <= 0) return '—';
		const major = (credits * 100) / (currentCurrency.credits_per_unit * 100);
		return `${currentCurrency.symbol}${major.toFixed(2)}`;
	}

	function packTotalCredits(pack: CreditPack): number {
		return pack.credit_amount + pack.bonus_credits;
	}

	async function selectCurrency(code: string) {
		selectedCurrency = code;
		// Persist preference to the backend.
		await api.patch('/me/preferences/currency', { currency_code: code });
	}

	async function purchase() {
		purchasing = true;
		errorMessage = '';
		successResult = null;

		const body: { credit_amount: number; pack_id?: string; currency?: string } = {
			credit_amount: selectedPackId ? 0 : customAmount,
			currency: selectedCurrency
		};
		if (selectedPackId) body.pack_id = selectedPackId;

		// Rule 9 — send an Idempotency-Key so the backend middleware can
		// deduplicate retries and prevent duplicate charges.
		const res = await api.post<PurchaseResult>('/wallet/purchase-credits', body, {
			'Idempotency-Key': crypto.randomUUID()
		});
		purchasing = false;

		if (res.error) {
			errorMessage = res.error.message;
		} else if (res.data) {
			successResult = res.data;
			// Invalidate wallet data so balances refresh.
			await invalidate('/me/wallet');
		}
	}
</script>

<svelte:head>
	<title>{t('wallet.purchase.title', 'Purchase Credits')}</title>
</svelte:head>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('wallet.purchase.title', 'Purchase Credits')}</h1>
	</header>

	{#if errorMessage}
		<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">
			{errorMessage}
		</p>
	{/if}

	{#if successResult}
		<div in:fly={{ y: -10, duration: 200 }} class="rounded-[var(--radius-card)] border border-[var(--color-success)]/30 bg-[var(--color-success)]/10 p-4">
			<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-success)]">
				<Check size={16} />
				{t('wallet.purchase.initiated', 'Purchase Initiated')}
			</h2>
			<p class="mt-1 text-sm text-[var(--color-text)]">
				{t('wallet.purchase.initiated_body', '{credits} credits for {currency} — {amount} {currency} via {provider}')
					.replace('{credits}', successResult.credits_to_grant.toLocaleString())
					.replace('{currency}', successResult.currency)
					.replace('{amount}', (successResult.fiat_amount / 100).toFixed(2))
					.replace('{provider}', successResult.provider)}
			</p>
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				{t('wallet.purchase.initiated_help', "Complete the payment to receive your credits. You'll be redirected to the payment provider.")}
			</p>
		</div>
	{/if}

	{#if currencies.length === 0}
		<p class="rounded-[var(--radius)] bg-[var(--color-warning)]/10 px-3 py-2 text-sm text-[var(--color-warning)]">
			{t('wallet.purchase.no_currencies', 'No currencies are currently configured. Please check back later.')}
		</p>
	{:else}
		<!-- Currency Selector -->
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<label class="mb-2 block text-sm font-medium text-[var(--color-text)]" for="currency-select">
				{t('wallet.purchase.your_currency', 'Your Currency')}
			</label>
			<div class="flex flex-wrap gap-2">
				{#each sortedCurrencies as c (c.code)}
					<button
						type="button"
						class="rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors hover:bg-[var(--color-surface-subtle)]
							{selectedCurrency === c.code
							? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
							: 'border-[var(--color-border)] text-[var(--color-text)]'}"
						onclick={() => selectCurrency(c.code)}
					>
						<span class="mr-1 text-base">{c.symbol}</span>
						{c.code}
					</button>
				{/each}
			</div>
			{#if currentCurrency}
				<p class="mt-2 text-xs text-[var(--color-muted)]">
					{t('wallet.purchase.credits_per_unit', '1 {code} = {n} credits')
						.replace('{code}', currentCurrency.code)
						.replace('{n}', currentCurrency.credits_per_unit.toLocaleString())}
				</p>
			{/if}
		</div>

		<!-- Credit Packs -->
		{#if sortedPacks.length > 0}
			<div class="space-y-3">
				<h2 class="text-sm font-medium text-[var(--color-text)]">{t('wallet.purchase.credit_packs', 'Credit Packs')}</h2>
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each sortedPacks as pack (pack.id)}
						<button
							type="button"
							class="rounded-[var(--radius-card)] border p-4 text-left transition-all hover:border-[var(--color-primary)]/40
								{selectedPackId === pack.id ? 'border-[var(--color-primary)] ring-2 ring-[var(--color-primary)]/20' : 'border-[var(--color-border)] bg-[var(--color-surface)]'}"
							onclick={() => { selectedPackId = selectedPackId === pack.id ? '' : pack.id; customAmount = 0; }}
						>
							<div class="flex items-baseline justify-between">
								<h3 class="font-medium text-[var(--color-text)]">{pack.name}</h3>
								{#if pack.bonus_credits > 0}
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
									<p class="text-xs text-[var(--color-muted)]">{selectedCurrency}</p>
								</div>
							</div>
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Custom Amount -->
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
						min={minPurchaseCredits}
						step={minPurchaseCredits}
						bind:value={customAmount}
						oninput={() => (selectedPackId = '')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						placeholder={t('wallet.purchase.enter_amount', 'Enter credit amount')}
					/>
				</div>
				<div class="text-right">
					<p class="text-sm text-[var(--color-muted)]">{t('wallet.purchase.price', 'Price')}</p>
					<p class="text-lg font-semibold text-[var(--color-text)]">
						{#if customAmount > 0}
							{fiatPrice(customAmount)}
						{:else}
							—
						{/if}
					</p>
				</div>
			</div>
			<p class="mt-2 text-xs text-[var(--color-muted)]">{t('wallet.purchase.minimum', 'Minimum {n} credits').replace('{n}', minPurchaseCredits.toLocaleString())}</p>
		</div>

		<!-- Purchase Button -->
		<div class="flex justify-end">
			<button
				type="button"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-3 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
				onclick={purchase}
				disabled={purchasing || (selectedPackId === '' && customAmount < minPurchaseCredits)}
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
</section>
