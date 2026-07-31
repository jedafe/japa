<script lang="ts">
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { invalidate } from '$app/navigation';
	import { fly } from 'svelte/transition';

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

	let { currencies, creditPacks } = $derived(page.data);

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

		const res = await api.post<PurchaseResult>('/wallet/purchase-credits', body);
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
	<title>Purchase Credits — Wallet</title>
</svelte:head>

<div class="mx-auto max-w-4xl space-y-6 p-4 sm:p-6">
	<h1 class="text-2xl font-bold">Purchase Credits</h1>

	{#if errorMessage}
		<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
			{errorMessage}
		</div>
	{/if}

	{#if successResult}
		<div in:fly={{ y: -10, duration: 200 }} class="rounded-lg border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-900 dark:bg-emerald-950">
			<h2 class="text-lg font-semibold text-emerald-800 dark:text-emerald-200">Purchase Initiated</h2>
			<p class="mt-1 text-sm text-emerald-700 dark:text-emerald-300">
				{successResult.credits_to_grant.toLocaleString()} credits for {successResult.currency} —
				{(successResult.fiat_amount / 100).toFixed(2)} {successResult.currency}
				via {successResult.provider}
			</p>
			<p class="mt-2 text-xs text-emerald-600 dark:text-emerald-400">
				Complete the payment to receive your credits. You'll be redirected to the payment provider.
			</p>
		</div>
	{/if}

	{#if currencies.length === 0}
		<div class="rounded-lg border border-yellow-200 bg-yellow-50 p-4 text-sm text-yellow-800 dark:border-yellow-900 dark:bg-yellow-950 dark:text-yellow-300">
			No currencies are currently configured. Please check back later.
		</div>
	{:else}
		<!-- Currency Selector -->
		<div class="rounded-lg border bg-card p-4">
			<label class="mb-2 block text-sm font-medium" for="currency-select">Your Currency</label>
			<div class="flex flex-wrap gap-2">
				{#each sortedCurrencies as c (c.code)}
					<button
						class="rounded-lg border px-3 py-2 text-sm transition-colors hover:bg-accent
							${selectedCurrency === c.code ? 'border-primary bg-primary text-primary-foreground' : ''}"
						onclick={() => selectCurrency(c.code)}
					>
						<span class="mr-1 text-base">{c.symbol}</span>
						{c.code}
					</button>
				{/each}
			</div>
			{#if currentCurrency}
				<p class="mt-2 text-xs text-muted-foreground">
					1 {currentCurrency.code} = {currentCurrency.credits_per_unit.toLocaleString()} credits
				</p>
			{/if}
		</div>

		<!-- Credit Packs -->
		{#if sortedPacks.length > 0}
			<div class="space-y-3">
				<h2 class="text-lg font-semibold">Credit Packs</h2>
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each sortedPacks as pack (pack.id)}
						<button
							class="rounded-xl border p-4 text-left transition-all hover:shadow-md
								${selectedPackId === pack.id ? 'border-primary ring-2 ring-primary' : ''}"
							onclick={() => { selectedPackId = selectedPackId === pack.id ? '' : pack.id; customAmount = 0; }}
						>
							<div class="flex items-baseline justify-between">
								<h3 class="font-semibold">{pack.name}</h3>
								{#if pack.bonus_credits > 0}
									<span class="rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-semibold text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">
										+{pack.bonus_credits.toLocaleString()} bonus
									</span>
								{/if}
							</div>
							{#if pack.description}
								<p class="mt-1 text-xs text-muted-foreground">{pack.description}</p>
							{/if}
							<div class="mt-3 flex items-baseline justify-between">
								<div>
									<p class="text-2xl font-bold">{packTotalCredits(pack).toLocaleString()}</p>
									<p class="text-xs text-muted-foreground">credits</p>
								</div>
								<div class="text-right">
									<p class="text-lg font-semibold">{fiatPrice(pack.credit_amount)}</p>
									<p class="text-xs text-muted-foreground">{selectedCurrency}</p>
								</div>
							</div>
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Custom Amount -->
		<div class="rounded-lg border bg-card p-4">
			<h2 class="mb-3 text-lg font-semibold">Custom Amount</h2>
			<div class="flex items-end gap-3">
				<div class="flex-1">
					<label class="mb-1 block text-sm font-medium" for="custom-credits">Credits</label>
					<input
						id="custom-credits"
						type="number"
						min="100"
						step="100"
						bind:value={customAmount}
						oninput={() => (selectedPackId = '')}
						class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
						placeholder="Enter credit amount"
					/>
				</div>
				<div class="text-right">
					<p class="text-sm text-muted-foreground">Price</p>
					<p class="text-lg font-semibold">
						{#if customAmount > 0}
							{fiatPrice(customAmount)}
						{:else}
							—
						{/if}
					</p>
				</div>
			</div>
			<p class="mt-2 text-xs text-muted-foreground">Minimum 100 credits</p>
		</div>

		<!-- Purchase Button -->
		<div class="flex justify-end">
			<button
				class="rounded-lg bg-primary px-6 py-3 font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
				onclick={purchase}
				disabled={purchasing || (selectedPackId === '' && customAmount < 100)}
			>
				{#if purchasing}
					Processing...
				{:else}
					Purchase Credits
				{/if}
			</button>
		</div>
	{/if}
</div>
