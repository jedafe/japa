<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/i18n';
	import {
		cancelSubscription,
		reactivateSubscription,
		createSubscription,
		type PaymentProvider
	} from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidateAll } from '$app/navigation';

	// Contract: GET /me/subscription returns gen.Subscription with fields
	// plan, status, current_period_end (RFC 3339), cancel_at_period_end.
	// The loader returns null when the user has no active subscription (404
	// from the backend). The loader also returns pricing plans and payment
	// providers for the subscribe flow (Rule 16: all data from API).
	interface Subscription {
		plan?: string;
		status?: string;
		current_period_end?: string;
		cancel_at_period_end?: boolean;
	}

	interface PlanFeature {
		text: string;
		included: boolean;
	}

	interface PlanPricing {
		amount: number;
		currency: string;
	}

	interface PricingPlan {
		id: string;
		name: string;
		description: string;
		features: PlanFeature[];
		monthly: PlanPricing;
		yearly: PlanPricing;
		highlighted: boolean;
	}

	interface Currency {
		code: string;
		symbol: string;
	}

	interface SubscriptionData {
		subscription: Subscription | null;
		plans: PricingPlan[];
		providers: PaymentProvider[];
		currencies?: Currency[];
	}

	let { data }: { data: unknown } = $props();

	// Sync local state from `data` whenever it changes (BUG-P014). The
	// previous code captured a snapshot via untrack() once at init and never
	// updated, so invalidateAll() re-fetched on the server but the component
	// kept showing stale state.
	let sub = $state<Subscription | null>(null);
	let plans = $state<PricingPlan[]>([]);
	let providers = $state<PaymentProvider[]>([]);
	let currencies = $state<Currency[]>([]);

	$effect(() => {
		const d = data as SubscriptionData | null;
		sub = d?.subscription ?? null;
		plans = d?.plans ?? [];
		providers = d?.providers ?? [];
		currencies = d?.currencies ?? [];
	});

	let cancelOpen = $state(false);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let success = $state<string | null>(null);

	// Subscribe flow state
	let subscribeOpen = $state(false);
	let selectedPlan = $state<string>('');
	let billingCycle = $state<'monthly' | 'yearly'>('monthly');
	let selectedProvider = $state<string>('');

	// BUG-P013: the backend CancelSubscription sets cancel_at_period_end =
	// true but leaves status = 'active' (the user keeps access until period
	// end). Checking only status === 'canceled' misses the scheduled-cancel
	// state, so the Reactivate button never appears and the Cancel button
	// stays visible after cancelling.
	const isCancelled = $derived(
		sub?.status === 'canceled' || sub?.cancel_at_period_end === true
	);

	const statusLabel = $derived(
		sub?.status ? t(`settings.subscription.status_${sub.status}`, sub.status) : '—'
	);

	const renewsLabel = $derived(
		sub?.current_period_end
			? new Date(sub.current_period_end).toLocaleDateString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric'
				})
			: '—'
	);

	// Only paid plans are subscribable (Free is the default tier).
	const subscribablePlans = $derived(plans.filter((p) => p.monthly.amount > 0));
	const enabledProviders = $derived(providers.filter((p) => p.enabled));

	// Currency symbols from the API (Rule 16 — no hardcoded data).
	// Falls back to the currency code if the symbol is not found.
	const currencySymbolMap = $derived(
		new Map(currencies.map((c) => [c.code, c.symbol]))
	);

	function formatPrice(amount: number, currency: string): string {
		const symbol = currencySymbolMap.get(currency) ?? currency;
		return `${symbol}${(amount / 100).toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 2 })}`;
	}

	function openSubscribe() {
		selectedPlan = subscribablePlans[0]?.id ?? '';
		billingCycle = 'monthly';
		selectedProvider = enabledProviders[0]?.id ?? '';
		error = null;
		success = null;
		subscribeOpen = true;
	}

	async function doSubscribe() {
		if (!selectedPlan || !selectedProvider) return;
		subscribeOpen = false;
		error = null;
		success = null;
		loading = true;
		// Map plan slug + billing cycle to the backend plan constant.
		// Backend expects "pro_monthly" or "pro_yearly" (PlanProMonthly/PlanProYearly).
		const planCode = `${selectedPlan}_${billingCycle}`;
		try {
			const res = await createSubscription(planCode, selectedProvider);
			loading = false;
			if (res.error) {
				// BUG-P012: the backend CreateSubscription returns
				// not_implemented for all providers (recurring billing is
				// not yet built). Show a user-friendly message instead of a
				// generic error so the user understands the feature is
				// unavailable rather than broken.
				if (res.error.code === 'not_implemented') {
					error = t(
						'settings.subscription.recurring_not_available',
						'Recurring subscription billing is not yet available. You can still purchase Pro membership as a one-time payment.'
					);
				} else {
					error = t('settings.subscription.subscribe_error');
				}
			} else {
				success = t('settings.subscription.subscribe_success');
				await invalidateAll();
			}
		} catch {
			loading = false;
			error = t('settings.subscription.subscribe_error');
		}
	}

	async function doCancel() {
		cancelOpen = false;
		error = null;
		success = null;
		loading = true;
		// Optimistic: reflect the scheduled-cancel state (cancel_at_period_end
		// = true, status stays 'active') so the UI updates immediately.
		const prevSub = sub;
		const res = await optimisticAction({
			apply: () => (sub = { ...sub, cancel_at_period_end: true }),
			rollback: () => (sub = prevSub),
			call: () => cancelSubscription()
		});
		loading = false;
		if (!res.ok) error = res.error?.message ?? null;
		await invalidateAll();
	}

	async function doReactivate() {
		error = null;
		success = null;
		loading = true;
		const prevSub = sub;
		const res = await optimisticAction({
			apply: () => (sub = { ...sub, status: 'active', cancel_at_period_end: false }),
			rollback: () => (sub = prevSub),
			call: () => reactivateSubscription()
		});
		loading = false;
		if (!res.ok) error = res.error?.message ?? null;
		await invalidateAll();
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.subscription')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.subscription.desc')}</p>
	</header>

	{#if sub}
		<div class="mb-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<dl class="space-y-2">
				<div class="flex justify-between">
					<dt class="text-sm text-[var(--color-muted)]">{t('settings.subscription.plan')}</dt>
					<dd class="text-sm font-medium text-[var(--color-text)]">{sub.plan ?? '—'}</dd>
				</div>
				<div class="flex justify-between">
					<dt class="text-sm text-[var(--color-muted)]">{t('settings.subscription.status')}</dt>
					<dd class="text-sm font-medium text-[var(--color-text)]">{statusLabel}</dd>
				</div>
				<div class="flex justify-between">
					<dt class="text-sm text-[var(--color-muted)]">{t('settings.subscription.renews')}</dt>
					<dd class="text-sm font-medium text-[var(--color-text)]">{renewsLabel}</dd>
				</div>
			</dl>
		</div>

		{#if error}<p class="mb-4 text-xs text-[var(--color-danger)]">{error}</p>{/if}
		{#if success}<p class="mb-4 text-xs text-[var(--color-success)]">{success}</p>{/if}

		{#if isCancelled}
			<button
				type="button"
				disabled={loading}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] disabled:opacity-50"
				onclick={doReactivate}
			>
				{loading ? t('settings.subscription.loading') : t('settings.subscription.reactivate')}
			</button>
		{:else}
			<Dialog.Root bind:open={cancelOpen}>
				<Dialog.Trigger
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('settings.subscription.cancel')}
				</Dialog.Trigger>
				<Dialog.Portal>
					<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
						<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
							{t('settings.subscription.cancel_confirm_title')}
						</Dialog.Title>
						<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">
							{t('settings.subscription.cancel_confirm_body')}
						</Dialog.Description>
						<div class="flex justify-end gap-2">
							<Dialog.Close
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]"
							>
								{t('common.cancel')}
							</Dialog.Close>
							<button
								type="button"
								disabled={loading}
								class="rounded-[var(--radius)] bg-[var(--color-danger)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
								onclick={doCancel}
							>
								{loading ? t('settings.subscription.loading') : t('settings.subscription.cancel_confirm')}
							</button>
						</div>
					</Dialog.Content>
				</Dialog.Portal>
			</Dialog.Root>
		{/if}
	{:else}
		<div class="mb-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-sm text-[var(--color-muted)]">{t('settings.subscription.empty')}</p>
		</div>

		{#if error}<p class="mb-4 text-xs text-[var(--color-danger)]">{error}</p>{/if}
		{#if success}<p class="mb-4 text-xs text-[var(--color-success)]">{success}</p>{/if}

		<!-- Subscribe flow: plan cards + billing cycle + payment provider dialog -->
		{#if subscribablePlans.length > 0}
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-sm font-medium text-[var(--color-text)]">{t('settings.subscription.choose_plan')}</h2>
				<a href="/pricing" class="text-xs text-[var(--color-primary)] hover:underline">
					{t('settings.subscription.subscribe')}
				</a>
			</div>

			<div class="mb-4 grid gap-3 sm:grid-cols-2">
				{#each subscribablePlans as plan (plan.id)}
					<button
						type="button"
						class="rounded-[var(--radius)] border p-4 text-left transition-colors {selectedPlan === plan.id
							? 'border-[var(--color-primary)] bg-[var(--color-surface)] ring-1 ring-[var(--color-primary)]'
							: 'border-[var(--color-border)] bg-[var(--color-surface)] hover:bg-[var(--color-surface-subtle)]'}"
						onclick={() => (selectedPlan = plan.id)}
					>
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium text-[var(--color-text)]">{plan.name}</span>
							{#if plan.highlighted}
								<span class="rounded-full bg-[var(--color-primary)] px-2 py-0.5 text-xs text-[var(--color-primary-contrast)]">
									{t('pricing.popular', 'Most popular')}
								</span>
							{/if}
						</div>
						<p class="mt-1 text-xs text-[var(--color-muted)]">{plan.description}</p>
						<p class="mt-2 text-lg font-bold text-[var(--color-text)]">
							{formatPrice(billingCycle === 'monthly' ? plan.monthly.amount : plan.yearly.amount, plan.monthly.currency)}
							<span class="text-xs font-normal text-[var(--color-muted)]">
								{billingCycle === 'monthly' ? t('settings.subscription.per_month') : t('settings.subscription.per_year')}
							</span>
						</p>
					</button>
				{/each}
			</div>

			<!-- Billing cycle toggle -->
			<div class="mb-4 flex items-center gap-2">
				<span class="text-xs text-[var(--color-muted)]">{t('settings.subscription.billing_cycle')}:</span>
				<button
					type="button"
					class="rounded-[var(--radius)] px-3 py-1 text-xs font-medium {billingCycle === 'monthly'
						? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
						: 'border border-[var(--color-border)] text-[var(--color-text)]'}"
					onclick={() => (billingCycle = 'monthly')}
				>
					{t('settings.subscription.monthly')}
				</button>
				<button
					type="button"
					class="rounded-[var(--radius)] px-3 py-1 text-xs font-medium {billingCycle === 'yearly'
						? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
						: 'border border-[var(--color-border)] text-[var(--color-text)]'}"
					onclick={() => (billingCycle = 'yearly')}
				>
					{t('settings.subscription.yearly')}
				</button>
			</div>

			{#if enabledProviders.length > 0}
				<!-- Payment provider selector -->
				<div class="mb-4">
					<span class="mb-2 block text-xs text-[var(--color-muted)]">{t('settings.subscription.payment_method')}</span>
					<div class="flex flex-wrap gap-2">
						{#each enabledProviders as provider (provider.id)}
							<button
								type="button"
								class="rounded-[var(--radius)] border px-3 py-2 text-xs font-medium {selectedProvider === provider.id
									? 'border-[var(--color-primary)] bg-[var(--color-surface)] ring-1 ring-[var(--color-primary)]'
									: 'border-[var(--color-border)] text-[var(--color-text)]'}"
								onclick={() => (selectedProvider = provider.id)}
							>
								{provider.name}
							</button>
						{/each}
					</div>
				</div>

				<button
					type="button"
					disabled={loading || !selectedPlan || !selectedProvider}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] disabled:opacity-50"
					onclick={doSubscribe}
				>
					{loading ? t('settings.subscription.loading') : t('settings.subscription.confirm_subscribe')}
				</button>
			{:else}
				<p class="text-xs text-[var(--color-muted)]">{t('settings.subscription.no_providers')}</p>
			{/if}
		{:else}
			<a
				href="/pricing"
				class="inline-block rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)]"
			>
				{t('settings.subscription.subscribe')}
			</a>
		{/if}
	{/if}
</section>
