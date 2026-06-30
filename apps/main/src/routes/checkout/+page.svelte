<script lang="ts">
	import { page } from '$app/state';
	import {
		CreditCard,
		Wallet,
		Bitcoin,
		Loader2,
		CheckCircle2,
		XCircle,
		ArrowRight,
		Tag,
		ShieldCheck,
		RefreshCw
	} from 'lucide-svelte';
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';

	type PaymentProvider = 'paystack' | 'stripe' | 'nowpayments' | 'credit';
	type CheckoutState = 'idle' | 'processing' | 'success' | 'error';

	interface ProviderOption {
		id: PaymentProvider;
		name: string;
		description: string;
		icon: typeof CreditCard;
	}

	const providers: ProviderOption[] = [
		{
			id: 'paystack',
			name: 'Paystack',
			description: 'Card, bank transfer, USSD',
			icon: CreditCard
		},
		{
			id: 'stripe',
			name: 'Stripe',
			description: 'International cards',
			icon: ShieldCheck
		},
		{
			id: 'nowpayments',
			name: 'Crypto',
			description: 'Bitcoin, USDT, ETH',
			icon: Bitcoin
		},
		{
			id: 'credit',
			name: 'Wallet Credits',
			description: 'Pay from your balance',
			icon: Wallet
		}
	];

	let amount = $derived(Number(page.url.searchParams.get('amount') ?? '0'));
	let purpose = $derived(page.url.searchParams.get('purpose') ?? 'general');
	let currency = $derived(page.url.searchParams.get('currency') ?? 'NGN');

	let selectedProvider = $state<PaymentProvider>('paystack');
	let discountCode = $state('');
	let discountValid = $state(false);
	let discountInfo = $state<{ type: string; value: number } | null>(null);
	let validatingDiscount = $state(false);
	let discountError = $state('');
	let checkoutState = $state<CheckoutState>('idle');
	let errorMessage = $state('');
	let paymentResult = $state<{ payment_id: string; reference: string } | null>(null);
	let walletBalance = $state<number>(0);

	$effect(() => {
		if (selectedProvider === 'credit') {
			api.getWalletBalance()
				.then((res) => { walletBalance = res.credits; })
				.catch(() => { walletBalance = 0; });
		}
	});

	const displayAmount = $derived(() => {
		if (amount === 0) return '₦0';
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: currency.toUpperCase(),
			minimumFractionDigits: 0
		}).format(amount / 100);
	});

	const discountedAmount = $derived(() => {
		const raw = amount;
		if (!discountValid || !discountInfo || raw === 0) return raw;
		if (discountInfo.type === 'percentage') {
			return Math.round(raw * (1 - discountInfo.value / 100));
		}
		return Math.max(0, raw - discountInfo.value * 100);
	});

	const hasDiscount = $derived(discountValid && discountedAmount() < amount);

	const insufficientCredits = $derived(
		selectedProvider === 'credit' && discountedAmount() > walletBalance
	);

	function formatKobo(kobo: number): string {
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: currency.toUpperCase(),
			minimumFractionDigits: 0
		}).format(kobo / 100);
	}

	async function validateDiscount() {
		const code = discountCode.trim();
		if (!code) return;

		validatingDiscount = true;
		discountError = '';
		discountValid = false;
		discountInfo = null;

		try {
			const result = await api.validateDiscountCode(code);
			if (result.valid) {
				discountValid = true;
				discountInfo = { type: result.discount_type, value: result.discount_value };
			} else {
				discountError = 'Invalid discount code';
			}
		} catch (err) {
			discountError = err instanceof Error ? err.message : 'Failed to validate code';
		} finally {
			validatingDiscount = false;
		}
	}

	async function handlePay() {
		if (insufficientCredits) {
			errorMessage = 'Insufficient wallet balance';
			return;
		}

		checkoutState = 'processing';
		errorMessage = '';

		try {
			const result = await api.initiatePayment({
				amount: discountedAmount(),
				currency: currency.toLowerCase(),
				provider: selectedProvider,
				purpose,
				metadata: discountValid ? { discount_code: discountCode.trim() } : undefined
			});

			if (selectedProvider === 'credit') {
				checkoutState = 'success';
				paymentResult = { payment_id: result.payment_id, reference: result.reference };
			} else if (result.checkout_url) {
				window.location.href = result.checkout_url;
			} else {
				checkoutState = 'success';
				paymentResult = { payment_id: result.payment_id, reference: result.reference };
			}
		} catch (err) {
			checkoutState = 'error';
			errorMessage = err instanceof Error ? err.message : 'Payment failed. Please try again.';
		}
	}

	function retry() {
		checkoutState = 'idle';
		errorMessage = '';
	}
</script>

<svelte:head>
	<title>Checkout | JapaConnect</title>
	<meta name="description" content="Complete your payment securely." />
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-12">
	{#if checkoutState === 'success' && paymentResult}
		<!-- Success State -->
		<div class="text-center">
			<div
				class="mx-auto flex h-16 w-16 items-center justify-center rounded-full"
				style="background-color: color-mix(in srgb, var(--color-success) 12%, transparent)"
			>
				<CheckCircle2 class="h-8 w-8" style="color: var(--color-success)" />
			</div>
			<h1 class="mt-4 text-2xl font-bold" style="color: var(--color-foreground)">
				{t('checkout.payment_successful')}
			</h1>
			<p class="mt-2" style="color: var(--color-muted)">
				{t('checkout.payment_processed')}
			</p>

			<div
				class="mx-auto mt-6 max-w-sm rounded-xl border p-4"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<div class="space-y-2 text-sm">
					<div class="flex justify-between">
						<span style="color: var(--color-muted)">Payment ID</span>
						<span class="font-mono text-xs" style="color: var(--color-foreground)">
							{paymentResult.payment_id.slice(0, 12)}...
						</span>
					</div>
					<div class="flex justify-between">
						<span style="color: var(--color-muted)">Reference</span>
						<span class="font-mono text-xs" style="color: var(--color-foreground)">
							{paymentResult.reference}
						</span>
					</div>
					<div class="flex justify-between">
						<span style="color: var(--color-muted)">Amount</span>
						<span class="font-semibold" style="color: var(--color-foreground)">
							{formatKobo(discountedAmount())}
						</span>
					</div>
				</div>
			</div>

			<div class="mt-8 flex flex-col gap-3 sm:flex-row sm:justify-center">
				<button
					class="rounded-lg px-6 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
					style="background-color: var(--color-primary)"
					onclick={() => goto('/payments')}
				>
					{t('checkout.view_payments')}
				</button>
				<button
					class="rounded-lg border px-6 py-2.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/5"
					style="border-color: var(--color-border); color: var(--color-foreground)"
					onclick={() => goto('/dashboard')}
				>
					{t('checkout.back_dashboard')}
				</button>
			</div>
		</div>

	{:else if checkoutState === 'error'}
		<!-- Error State -->
		<div class="text-center">
			<div
				class="mx-auto flex h-16 w-16 items-center justify-center rounded-full"
				style="background-color: color-mix(in srgb, var(--color-danger) 12%, transparent)"
			>
				<XCircle class="h-8 w-8" style="color: var(--color-danger)" />
			</div>
			<h1 class="mt-4 text-2xl font-bold" style="color: var(--color-foreground)">
				{t('checkout.payment_failed')}
			</h1>
			<p class="mt-2" style="color: var(--color-muted)">
				{errorMessage}
			</p>
			<div class="mt-8 flex flex-col gap-3 sm:flex-row sm:justify-center">
				<button
					class="flex items-center justify-center gap-2 rounded-lg px-6 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
					style="background-color: var(--color-primary)"
					onclick={retry}
				>
					<RefreshCw class="h-4 w-4" />
					{t('common.retry')}
				</button>
				<button
					class="rounded-lg border px-6 py-2.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/5"
					style="border-color: var(--color-border); color: var(--color-foreground)"
					onclick={() => goto('/payments')}
				>
					{t('checkout.back_payments')}
				</button>
			</div>
		</div>

	{:else}
		<!-- Checkout Form -->
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
			Checkout
		</h1>

		<div class="mt-8 grid gap-6 lg:grid-cols-5">
			<!-- Order Summary -->
			<div
				class="rounded-xl border p-6 lg:col-span-2"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<h2 class="text-sm font-semibold uppercase tracking-wide" style="color: var(--color-muted)">
					{t('checkout.order_summary')}
				</h2>
				<div class="mt-4 space-y-3">
					<div class="flex justify-between text-sm">
						<span style="color: var(--color-muted)">{t('checkout.purpose')}</span>
						<span class="capitalize" style="color: var(--color-foreground)">
							{purpose.replace(/_/g, ' ')}
						</span>
					</div>
					<div class="flex justify-between text-sm">
						<span style="color: var(--color-muted)">{t('checkout.amount')}</span>
						<span style="color: var(--color-foreground)">{displayAmount()}</span>
					</div>
					{#if hasDiscount}
						<div class="flex justify-between text-sm">
							<span style="color: var(--color-success)">{t('checkout.discount')}</span>
							<span style="color: var(--color-success)">
								-{formatKobo(amount - discountedAmount())}
							</span>
						</div>
					{/if}
					<div
						class="border-t pt-3"
						style="border-color: var(--color-border)"
					>
						<div class="flex justify-between">
							<span class="font-semibold" style="color: var(--color-foreground)">{t('checkout.total')}</span>
							<span class="text-lg font-bold" style="color: var(--color-foreground)">
								{formatKobo(discountedAmount())}
							</span>
						</div>
					</div>
				</div>
			</div>

			<!-- Payment Details -->
			<div class="space-y-6 lg:col-span-3">
				<!-- Payment Method -->
				<div
					class="rounded-xl border p-6"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<h2 class="text-sm font-semibold uppercase tracking-wide" style="color: var(--color-muted)">
						{t('checkout.payment_method')}
					</h2>
					<div class="mt-4 space-y-2">
						{#each providers as provider (provider.id)}
							{@const PIcon = provider.icon}
							<!-- svelte-ignore a11y_click_events_have_key_events -->
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<button
								class="flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors"
								style="border-color: {selectedProvider === provider.id ? 'var(--color-primary)' : 'var(--color-border)'}; background: {selectedProvider === provider.id ? 'color-mix(in srgb, var(--color-primary) 5%, transparent)' : 'transparent'}"
								onclick={() => (selectedProvider = provider.id)}
							>
								<PIcon class="h-5 w-5 shrink-0" style="color: {selectedProvider === provider.id ? 'var(--color-primary)' : 'var(--color-muted)'}" />
								<div class="flex-1">
									<p class="text-sm font-medium" style="color: var(--color-foreground)">
										{provider.name}
									</p>
									<p class="text-xs" style="color: var(--color-muted)">
										{provider.description}
									</p>
								</div>
								{#if selectedProvider === provider.id}
									<div
										class="h-4 w-4 rounded-full"
										style="background-color: var(--color-primary)"
									></div>
								{/if}
							</button>
						{/each}
					</div>

					{#if selectedProvider === 'credit'}
						<div class="mt-3 rounded-lg p-3" style="background: var(--color-background)">
							<p class="text-xs" style="color: var(--color-muted)">
								{t('checkout.wallet_balance')} <span class="font-semibold" style="color: var(--color-foreground)">{formatKobo(walletBalance)}</span>
							</p>
							{#if insufficientCredits}
								<p class="mt-1 text-xs" style="color: var(--color-danger)">
									{t('checkout.insufficient_balance')} <a href="/wallet" class="underline">{t('checkout.top_up')}</a>
								</p>
							{/if}
						</div>
					{/if}
				</div>

				<!-- Discount Code -->
				<div
					class="rounded-xl border p-6"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-center gap-2">
						<Tag class="h-4 w-4" style="color: var(--color-muted)" />
					<h2 class="text-sm font-semibold uppercase tracking-wide" style="color: var(--color-muted)">
						{t('checkout.discount_code')}
					</h2>
				</div>
				<div class="mt-3 flex gap-2">
					<input
						type="text"
						bind:value={discountCode}
						placeholder={t('checkout.enter_code')}
							class="flex-1 rounded-lg border px-3 py-2 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
							onkeydown={(e) => { if (e.key === 'Enter') validateDiscount(); }}
						/>
						<button
							class="rounded-lg px-4 py-2 text-sm font-medium transition-opacity hover:opacity-90 disabled:opacity-50"
							style="background-color: var(--color-primary); color: #fff"
							disabled={validatingDiscount || !discountCode.trim()}
							onclick={validateDiscount}
						>
							{#if validatingDiscount}
								<Loader2 class="h-4 w-4 animate-spin" />
							{:else}
								{t('checkout.apply')}
							{/if}
						</button>
					</div>
					{#if discountError}
						<p class="mt-2 text-xs" style="color: var(--color-danger)">{discountError}</p>
					{/if}
					{#if discountValid && discountInfo}
						<p class="mt-2 text-xs" style="color: var(--color-success)">
							✓ Applied —
							{discountInfo.type === 'percentage'
								? `${discountInfo.value}% off`
								: `₦${discountInfo.value} off`}
						</p>
					{/if}
				</div>

				{#if errorMessage && checkoutState === 'idle'}
					<div
						class="rounded-lg border px-4 py-3 text-sm"
						style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
					>
						{errorMessage}
					</div>
				{/if}

				<!-- Pay Button -->
				<button
					class="flex w-full items-center justify-center gap-2 rounded-lg px-6 py-3 text-base font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={checkoutState === 'processing' || insufficientCredits || amount === 0}
					onclick={handlePay}
				>
					{#if checkoutState === 'processing'}
						<Loader2 class="h-5 w-5 animate-spin" />
						{t('wallet.processing')}
					{:else}
						<ArrowRight class="h-5 w-5" />
						{t('wallet.pay')} {formatKobo(discountedAmount())}
					{/if}
				</button>
			</div>
		</div>
	{/if}
</main>
