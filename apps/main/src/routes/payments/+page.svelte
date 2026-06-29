<script lang="ts">
	import { CreditCard, Receipt, RefreshCw, Loader2, ExternalLink } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { getToken } from '$lib/api';
	import { onMount } from 'svelte';

	interface Payment {
		id: string;
		amount: number;
		currency: string;
		status: string;
		description: string;
		provider: string;
		created_at: string;
	}

	interface Subscription {
		id: string;
		plan: string;
		status: string;
		current_period_end: string;
		cancel_at: string | null;
	}

	let payments = $state<Payment[]>([]);
	let subscription = $state<Subscription | null>(null);
	let loading = $state(true);
	let cancelling = $state(false);
	let errorMessage = $state('');

	function authHeaders(extra?: Record<string, string>): Record<string, string> {
		const h: Record<string, string> = { 'Content-Type': 'application/json', ...extra };
		const token = getToken();
		if (token) h['Authorization'] = `Bearer ${token}`;
		return h;
	}

	onMount(async () => {
		try {
			const [paymentsRes, subRes] = await Promise.allSettled([
				fetch('/api/v1/payments?limit=50', { headers: authHeaders() }),
				fetch('/api/v1/subscriptions/active', { headers: authHeaders() })
			]);

			if (paymentsRes.status === 'fulfilled') {
				const body = await paymentsRes.value.json();
				if (body.data) payments = body.data;
			}

			if (subRes.status === 'fulfilled') {
				const body = await subRes.value.json();
				if (body.data) subscription = body.data;
			}
		} catch {
			payments = [];
		} finally {
			loading = false;
		}
	});

	function formatAmount(amount: number, currency: string): string {
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: currency.toUpperCase()
		}).format(amount / 100);
	}

	import { formatTimestamp } from '$lib/utils/timezone';

	function formatDate(dateStr: string): string {
		return formatTimestamp(dateStr);
	}

	function statusColor(status: string): string {
		switch (status) {
			case 'completed':
			case 'active':
				return 'var(--color-success, #22c55e)';
			case 'pending':
				return 'var(--color-warning, #f59e0b)';
			case 'failed':
			case 'cancelled':
			case 'expired':
				return 'var(--color-danger)';
			default:
				return 'var(--color-muted)';
		}
	}

	async function cancelSubscription() {
		if (!subscription) return;
		cancelling = true;
		errorMessage = '';
		try {
			await fetch(`/api/v1/subscriptions/${subscription.id}/cancel`, {
				method: 'PATCH',
				headers: authHeaders()
			});
			subscription = { ...subscription, status: 'cancelled' };
		} catch {
			errorMessage = 'Failed to cancel subscription. Please try again.';
		} finally {
			cancelling = false;
		}
	}
</script>

<svelte:head>
	<title>Payments | JapaConnect</title>
	<meta name="description" content="View your payment history and active subscriptions." />
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<h1 class="mb-8 text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
		Payments
	</h1>

	{#if loading}
		<div class="space-y-4">
			<div class="h-32 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
			<div class="h-64 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
		</div>
	{:else}
		<!-- Active Subscription -->
		{#if subscription}
			<div
				class="mb-8 rounded-xl border p-6"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<div class="flex items-start justify-between">
					<div>
						<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">
							Active Subscription
						</h2>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							Plan: <span class="font-medium capitalize" style="color: var(--color-foreground)">{subscription.plan}</span>
						</p>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							Status:
							<span class="font-medium capitalize" style="color: {statusColor(subscription.status)}">
								{subscription.status}
							</span>
						</p>
						{#if subscription.current_period_end}
							<p class="mt-1 text-sm" style="color: var(--color-muted)">
								Next billing: {formatDate(subscription.current_period_end)}
							</p>
						{/if}
						{#if subscription.cancel_at}
							<p class="mt-1 text-sm" style="color: var(--color-warning, #f59e0b)">
								Cancels on: {formatDate(subscription.cancel_at)}
							</p>
						{/if}
					</div>
					{#if subscription.status === 'active' && !subscription.cancel_at}
						<button
							onclick={cancelSubscription}
							disabled={cancelling}
							class="flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							{#if cancelling}
								<Loader2 class="h-4 w-4 animate-spin" />
							{:else}
								<RefreshCw class="h-4 w-4" />
							{/if}
							Cancel
						</button>
					{/if}
				</div>
			</div>
		{/if}

		{#if errorMessage}
			<div
				class="mb-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
			>
				{errorMessage}
			</div>
		{/if}

		<!-- Payment History -->
		<div
			class="rounded-xl border"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<div class="border-b px-6 py-4" style="border-color: var(--color-border)">
				<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">
					Payment History
				</h2>
			</div>

			{#if payments.length === 0}
				<EmptyState
					icon={Receipt}
					title="No payments yet"
					message="Your payment history will appear here."
				/>
			{:else}
				<div class="divide-y" style="divide-color: var(--color-border)">
					{#each payments as payment (payment.id)}
						<div class="flex items-center justify-between px-6 py-4">
							<div class="flex items-center gap-4">
								<div
									class="flex h-10 w-10 items-center justify-center rounded-lg"
									style="background-color: var(--color-background)"
								>
									<CreditCard class="h-5 w-5" style="color: var(--color-muted)" />
								</div>
								<div>
									<p class="text-sm font-medium" style="color: var(--color-foreground)">
										{payment.description || 'Payment'}
									</p>
									<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
										{formatDate(payment.created_at)}
										{#if payment.provider}
											&middot; {payment.provider}
										{/if}
									</p>
								</div>
							</div>
							<div class="text-right">
								<p class="text-sm font-semibold" style="color: var(--color-foreground)">
									{formatAmount(payment.amount, payment.currency)}
								</p>
								<p class="mt-0.5 text-xs capitalize" style="color: {statusColor(payment.status)}">
									{payment.status}
								</p>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</main>
