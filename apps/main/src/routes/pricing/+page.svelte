<script lang="ts">
	import {
		Check,
		X as XIcon,
		Loader2,
		CreditCard,
		Zap,
		Building2,
		Tag,
		ShieldCheck,
		ArrowRight
	} from 'lucide-svelte';
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';

	type PlanId = 'free' | 'pro' | 'business';
	type BillingCycle = 'monthly' | 'yearly';

	interface PlanFeature {
		text: string;
		included: boolean;
	}

	interface Plan {
		id: PlanId;
		name: string;
		description: string;
		monthlyPrice: number;
		yearlyPrice: number;
		icon: typeof Zap;
		features: PlanFeature[];
		highlighted: boolean;
	}

	let { data } = $props();

	const pageData = data as typeof data & { plans: any[] };

	// Map backend plans to frontend Plan type with icons
	const planIcons: Record<string, typeof ShieldCheck> = {
		free: ShieldCheck,
		pro: Zap,
		business: Building2
	};

	const plans: Plan[] = (pageData.plans ?? []).map((p: any) => ({
		id: p.id as PlanId,
		name: p.name,
		description: p.description,
		monthlyPrice: p.monthly.amount,
		yearlyPrice: p.yearly.amount,
		icon: planIcons[p.id] ?? Zap,
		features: p.features,
		highlighted: p.highlighted
	}));

	let billingCycle = $state<BillingCycle>('monthly');
	let discountCode = $state('');
	let discountValid = $state(false);
	let discountInfo = $state<{ type: string; value: number } | null>(null);
	let validatingDiscount = $state(false);
	let discountError = $state('');
	let subscribing = $state<PlanId | null>(null);
	let error = $state('');
	let showCreditConfirm = $state(false);
	let pendingPlanId = $state<PlanId | null>(null);

	const yearlySavings = $derived('Save 2 months');

	function formatPrice(amountKobo: number): string {
		if (amountKobo === 0) return 'Free';
		return '₦' + new Intl.NumberFormat('en-US').format(amountKobo / 100);
	}

	function getDisplayPrice(plan: Plan): number {
		return billingCycle === 'monthly' ? plan.monthlyPrice : plan.yearlyPrice;
	}

	function getDiscountedPrice(plan: Plan): number {
		const price = getDisplayPrice(plan);
		if (!discountValid || !discountInfo || price === 0) return price;
		if (discountInfo.type === 'percentage') {
			return Math.round(price * (1 - discountInfo.value / 100));
		}
		return Math.max(0, price - discountInfo.value * 100);
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

	async function handleSubscribe(plan: Plan) {
		if (plan.id === 'free') {
			await goto('/dashboard');
			return;
		}

		error = '';
		subscribing = plan.id;

		try {
			const result = await api.createSubscription({
				plan_id: plan.id,
				discount_code: discountValid ? discountCode.trim() : undefined,
				payment_provider: 'paystack'
			});

			if (result.checkout_url) {
				window.location.href = result.checkout_url;
			} else {
				await goto('/payments');
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create subscription';
		} finally {
			subscribing = null;
		}
	}

	function handleCreditSubscribe(plan: Plan) {
		pendingPlanId = plan.id;
		showCreditConfirm = true;
	}

	async function confirmCreditPayment() {
		if (!pendingPlanId) return;
		showCreditConfirm = false;
		subscribing = pendingPlanId;
		error = '';

		try {
			await api.createSubscription({
				plan_id: pendingPlanId,
				discount_code: discountValid ? discountCode.trim() : undefined,
				payment_provider: 'credit'
			});
			await goto('/payments');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Payment failed';
		} finally {
			subscribing = null;
			pendingPlanId = null;
		}
	}
</script>

<svelte:head>
	<title>Pricing | JapaConnect</title>
	<meta name="description" content="Choose the plan that fits your needs. Upgrade anytime." />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-12">
	<div class="text-center">
		<h1 class="text-3xl font-bold tracking-tight sm:text-4xl" style="color: var(--color-foreground)">
			Simple, transparent pricing
		</h1>
		<p class="mt-3 text-lg" style="color: var(--color-muted)">
			Choose the plan that fits your journey. Upgrade or downgrade anytime.
		</p>
	</div>

	<!-- Billing Toggle -->
	<div class="mt-8 flex items-center justify-center gap-2">
		<div
			class="inline-flex rounded-xl border p-1"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<button
				class="rounded-lg px-5 py-2 text-sm font-medium transition-all"
				style="background-color: {billingCycle === 'monthly' ? 'var(--color-primary)' : 'transparent'}; color: {billingCycle === 'monthly' ? '#fff' : 'var(--color-muted)'}; {billingCycle === 'monthly' ? 'box-shadow: var(--shadow-sm)' : ''}"
				onclick={() => (billingCycle = 'monthly')}
			>
				Monthly
			</button>
			<button
				class="rounded-lg px-5 py-2 text-sm font-medium transition-all"
				style="background-color: {billingCycle === 'yearly' ? 'var(--color-primary)' : 'transparent'}; color: {billingCycle === 'yearly' ? '#fff' : 'var(--color-muted)'}; {billingCycle === 'yearly' ? 'box-shadow: var(--shadow-sm)' : ''}"
				onclick={() => (billingCycle = 'yearly')}
			>
				Yearly
			</button>
		</div>
		{#if billingCycle === 'yearly'}
			<span
				class="animate-scale-in inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-semibold"
				style="background: color-mix(in srgb, var(--color-success) 12%, transparent); color: var(--color-success)"
			>
				{yearlySavings}
			</span>
		{/if}
	</div>

	{#if error}
		<div
			class="mx-auto mt-6 max-w-xl rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
		>
			{error}
		</div>
	{/if}

	<!-- Plan Cards -->
	<div class="mt-10 grid gap-6 lg:grid-cols-3">
		{#each plans as plan, i (plan.id)}
			{@const PlanIcon = plan.icon}
			{@const displayPrice = getDisplayPrice(plan)}
			{@const discountedPrice = getDiscountedPrice(plan)}
			{@const hasDiscount = discountValid && discountedPrice < displayPrice}

			<div
				class="card-interactive relative flex flex-col overflow-hidden rounded-2xl border p-6"
				style="border-color: {plan.highlighted ? 'var(--color-primary)' : 'var(--color-border)'}; background: var(--color-surface); {plan.highlighted ? 'box-shadow: 0 0 0 2px var(--color-primary), var(--shadow-lg)' : ''}; animation: slide-up 0.4s ease-out {i * 100}ms both"
			>
				{#if plan.highlighted}
					<div
						class="absolute inset-x-0 top-0 h-1"
						style="background: var(--gradient-primary)"
					></div>
					<div
						class="absolute -top-0 right-4 rounded-b-lg px-3 py-1 text-xs font-semibold text-white shadow-sm"
						style="background: var(--gradient-primary)"
					>
						Most Popular
					</div>
				{/if}

				<div class="flex items-center gap-3">
					<div
						class="flex h-12 w-12 items-center justify-center rounded-xl shadow-sm"
						style="background: {plan.highlighted ? 'var(--gradient-primary)' : 'color-mix(in srgb, var(--color-primary) 12%, transparent)'}"
					>
						<PlanIcon class="h-6 w-6" style="color: {plan.highlighted ? 'white' : 'var(--color-primary)'}" />
					</div>
					<div>
						<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">
							{plan.name}
						</h2>
					</div>
				</div>

				<p class="mt-3 text-sm leading-relaxed" style="color: var(--color-muted)">
					{plan.description}
				</p>

				<div class="mt-5">
					{#if hasDiscount}
						<span class="text-sm line-through" style="color: var(--color-muted)">
							{formatPrice(displayPrice)}
						</span>
					{/if}
					<span class="text-4xl font-bold tracking-tight" style="color: var(--color-foreground)">
						{formatPrice(hasDiscount ? discountedPrice : displayPrice)}
					</span>
					{#if displayPrice > 0}
						<span class="text-sm font-medium" style="color: var(--color-muted)">
							/{billingCycle === 'monthly' ? 'mo' : 'yr'}
						</span>
					{/if}
				</div>

				<ul class="mt-6 flex-1 space-y-3">
					{#each plan.features as feature}
						<li class="flex items-start gap-2.5">
							{#if feature.included}
								<div
									class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full"
									style="background: color-mix(in srgb, var(--color-success) 12%, transparent)"
								>
									<Check class="h-3 w-3" style="color: var(--color-success)" />
								</div>
								<span class="text-sm" style="color: var(--color-foreground)">{feature.text}</span>
							{:else}
								<div
									class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full"
									style="background: color-mix(in srgb, var(--color-muted) 10%, transparent)"
								>
									<XIcon class="h-3 w-3" style="color: var(--color-muted)" />
								</div>
								<span class="text-sm" style="color: var(--color-muted)">{feature.text}</span>
							{/if}
						</li>
					{/each}
				</ul>

				<div class="mt-6 space-y-2">
					{#if plan.id === 'free'}
						<button
							class="btn btn-secondary w-full"
							onclick={() => goto('/dashboard')}
						>
							Get Started
							<ArrowRight class="h-4 w-4" />
						</button>
					{:else}
						<button
							class="{plan.highlighted ? 'btn btn-primary' : 'btn btn-secondary'} w-full"
							disabled={subscribing !== null}
							onclick={() => handleSubscribe(plan)}
						>
							{#if subscribing === plan.id}
								<Loader2 class="h-4 w-4 animate-spin" />
								Processing...
							{:else}
								<CreditCard class="h-4 w-4" />
								Subscribe
							{/if}
						</button>
						<button
							class="btn btn-ghost w-full text-xs"
							disabled={subscribing !== null}
							onclick={() => handleCreditSubscribe(plan)}
						>
							Pay with wallet credits
						</button>
					{/if}
				</div>
			</div>
		{/each}
	</div>

	<!-- Discount Code -->
	<div class="mx-auto mt-10 max-w-md">
		<div
			class="rounded-2xl border p-6"
			style="border-color: var(--color-border); background: var(--gradient-surface)"
		>
			<div class="flex items-center gap-3">
				<div
					class="flex h-10 w-10 items-center justify-center rounded-xl"
					style="background: color-mix(in srgb, var(--color-primary) 10%, transparent)"
				>
					<Tag class="h-5 w-5" style="color: var(--color-primary)" />
				</div>
				<div>
					<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
						{t('pricing.promo_code')}
					</h3>
					<p class="text-xs" style="color: var(--color-muted)">{t('pricing.promo_hint')}</p>
				</div>
			</div>
			<div class="mt-4 flex gap-2">
				<input
					type="text"
					bind:value={discountCode}
					placeholder="Enter code"
					class="flex-1 rounded-xl border px-4 py-2.5 text-sm outline-none transition-all focus:border-[var(--color-primary)] focus:ring-2 focus:ring-offset-1"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: color-mix(in srgb, var(--color-primary) 30%, transparent)"
					onkeydown={(e) => { if (e.key === 'Enter') validateDiscount(); }}
				/>
				<button
					class="btn btn-primary px-5 disabled:opacity-50"
					disabled={validatingDiscount || !discountCode.trim()}
					onclick={validateDiscount}
				>
					{#if validatingDiscount}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						Apply
					{/if}
				</button>
			</div>
			{#if discountError}
				<p class="mt-2 flex items-center gap-1 text-xs" style="color: var(--color-danger)">
					{discountError}
				</p>
			{/if}
			{#if discountValid && discountInfo}
				<p class="mt-2 flex items-center gap-1 text-xs" style="color: var(--color-success)">
					✓ {t('pricing.code_applied')}
					{discountInfo.type === 'percentage'
						? `${discountInfo.value}% off`
						: `₦${discountInfo.value} off`}
				</p>
			{/if}
		</div>
	</div>
</main>

<!-- Credit Confirmation Modal -->
{#if showCreditConfirm}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-4 animate-fade-in"
		style="background: rgb(0 0 0 / 0.5); backdrop-filter: blur(4px)"
		onclick={() => (showCreditConfirm = false)}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-sm animate-scale-in rounded-2xl border p-6 shadow-2xl"
			style="border-color: var(--color-border); background: var(--color-background)"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">
				Confirm Payment
			</h3>
			<p class="mt-2 text-sm leading-relaxed" style="color: var(--color-muted)">
				This will deduct credits from your wallet balance. Are you sure?
			</p>
			{#if pendingPlanId}
				{@const plan = plans.find((p) => p.id === pendingPlanId)}
				{#if plan}
					<div class="mt-4 rounded-xl border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
						<p class="text-sm font-medium" style="color: var(--color-foreground)">
							{plan.name} Plan — {billingCycle}
						</p>
						<p class="mt-1 text-2xl font-bold" style="color: var(--color-foreground)">
							{formatPrice(getDiscountedPrice(plan))}
						</p>
					</div>
				{/if}
			{/if}
			<div class="mt-6 flex gap-3">
				<button
					class="btn btn-secondary flex-1"
					onclick={() => (showCreditConfirm = false)}
				>
					Cancel
				</button>
				<button
					class="btn btn-primary flex-1"
					onclick={confirmCreditPayment}
				>
					Confirm
				</button>
			</div>
		</div>
	</div>
{/if}
