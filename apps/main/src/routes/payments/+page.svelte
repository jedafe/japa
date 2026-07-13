<script lang="ts">
	import {
		CreditCard,
		Receipt,
		RefreshCw,
		Loader2,
		ExternalLink,
		Plus,
		X,
		Undo2,
		CheckCircle2,
		AlertCircle
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type PaymentDetail } from '$lib/api';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	let payments = $state<Array<{ id: string; amount: number; currency: string; status: string; description: string; provider: string; created_at: string; refundable?: boolean; refunded_at?: string }>>([]);
	let subscription = $state<{ id: string; plan: string; status: string; current_period_end: string; cancel_at: string | null } | null>(null);
	let loading = $state(true);
	let cancelling = $state(false);
	let errorMessage = $state('');

	// Detail modal
	let selectedPaymentId = $state<string | null>(null);
	let paymentDetail = $state<PaymentDetail | null>(null);
	let detailLoading = $state(false);

	// Refund modal
	let showRefundModal = $state(false);
	let refundPaymentId = $state<string | null>(null);
	let refundReason = $state('');
	let refundSubmitting = $state(false);
	let refundError = $state('');

	onMount(async () => {
		await loadData();
	});

	async function loadData() {
		loading = true;
		try {
			const [paymentsResult, subResult] = await Promise.allSettled([
				api.listPayments({ page_size: 50 }),
				api.getActiveSubscription()
			]);

			if (paymentsResult.status === 'fulfilled') {
				payments = paymentsResult.value;
			}

			if (subResult.status === 'fulfilled') {
				subscription = subResult.value;
			}
		} catch {
			payments = [];
		} finally {
			loading = false;
		}
	}

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
			case 'refunded':
				return 'var(--color-info, #2563eb)';
			default:
				return 'var(--color-muted)';
		}
	}

	async function cancelSubscription() {
		if (!subscription) return;
		cancelling = true;
		errorMessage = '';
		try {
			await api.cancelSubscription();
			subscription = { ...subscription, status: 'cancelled' };
		} catch {
			errorMessage = 'Failed to cancel subscription. Please try again.';
		} finally {
			cancelling = false;
		}
	}

	async function openPaymentDetail(paymentId: string) {
		selectedPaymentId = paymentId;
		detailLoading = true;
		paymentDetail = null;
		try {
			paymentDetail = await api.getPayment(paymentId);
		} catch {
			paymentDetail = null;
		} finally {
			detailLoading = false;
		}
	}

	function closeDetailModal() {
		selectedPaymentId = null;
		paymentDetail = null;
	}

	function openRefundModal(paymentId: string) {
		refundPaymentId = paymentId;
		refundReason = '';
		refundError = '';
		showRefundModal = true;
	}

	function closeRefundModal() {
		showRefundModal = false;
		refundPaymentId = null;
		refundReason = '';
		refundError = '';
	}

	async function submitRefund() {
		if (!refundPaymentId || !refundReason.trim()) return;
		refundSubmitting = true;
		refundError = '';

		try {
			await api.requestRefund(refundPaymentId, { reason: refundReason.trim() });
			// Update local state
			payments = payments.map((p) =>
				p.id === refundPaymentId ? { ...p, status: 'refunded', refundable: false } : p
			);
			closeRefundModal();
			if (selectedPaymentId === refundPaymentId) {
				closeDetailModal();
			}
		} catch (err) {
			refundError = err instanceof Error ? err.message : 'Failed to request refund';
		} finally {
			refundSubmitting = false;
		}
	}
</script>

<svelte:head>
	<title>Payments | JapaConnect</title>
	<meta name="description" content="View your payment history and active subscriptions." />
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
			Payments
		</h1>
		<button
			class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
			onclick={() => goto('/checkout?amount=0&purpose=general')}
		>
			<Plus class="h-4 w-4" />
			Make a Payment
		</button>
	</div>

	{#if loading}
		<div class="mt-8 space-y-4">
			<div class="h-32 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
			<div class="h-64 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
		</div>
	{:else}
		{#if errorMessage}
			<div
				class="mb-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
			>
				{errorMessage}
			</div>
		{/if}

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
					actionLabel="Make a Payment"
					onaction={() => goto('/checkout?amount=0&purpose=general')}
				/>
			{:else}
				<div class="divide-y" style="divide-color: var(--color-border)">
					{#each payments as payment (payment.id)}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div
							class="flex items-center justify-between px-6 py-4 transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02] cursor-pointer"
							onclick={() => openPaymentDetail(payment.id)}
						>
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
							<div class="flex items-center gap-3">
								<div class="text-right">
									<p class="text-sm font-semibold" style="color: var(--color-foreground)">
										{formatAmount(payment.amount, payment.currency)}
									</p>
									<p class="mt-0.5 text-xs capitalize" style="color: {statusColor(payment.status)}">
										{payment.status}
									</p>
								</div>
								{#if payment.refundable}
									<button
										class="rounded-md border px-2 py-1 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/5"
										style="border-color: var(--color-border); color: var(--color-muted)"
										onclick={(e) => { e.stopPropagation(); openRefundModal(payment.id); }}
									>
										Refund
									</button>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</main>

<!-- Payment Detail Modal -->
{#if selectedPaymentId}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		onclick={closeDetailModal}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-md rounded-xl border p-6"
			style="border-color: var(--color-border); background: var(--color-surface)"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="flex items-center justify-between">
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">
					Payment Detail
				</h3>
				<button
					class="rounded-md p-1 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
					onclick={closeDetailModal}
				>
					<X class="h-5 w-5" style="color: var(--color-muted)" />
				</button>
			</div>

			{#if detailLoading}
				<div class="mt-6 flex items-center justify-center py-8">
					<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
				</div>
			{:else if paymentDetail}
				<div class="mt-4 space-y-3">
					<div class="flex justify-between text-sm">
						<span style="color: var(--color-muted)">Amount</span>
						<span class="font-semibold" style="color: var(--color-foreground)">
							{formatAmount(paymentDetail.amount, paymentDetail.currency)}
						</span>
					</div>
					<div class="flex justify-between text-sm">
						<span style="color: var(--color-muted)">Status</span>
						<span class="capitalize font-medium" style="color: {statusColor(paymentDetail.status)}">
							{paymentDetail.status}
						</span>
					</div>
					<div class="flex justify-between text-sm">
						<span style="color: var(--color-muted)">Provider</span>
						<span class="capitalize" style="color: var(--color-foreground)">
							{paymentDetail.provider}
						</span>
					</div>
					<div class="flex justify-between text-sm">
						<span style="color: var(--color-muted)">Purpose</span>
						<span class="capitalize" style="color: var(--color-foreground)">
							{paymentDetail.purpose?.replace(/_/g, ' ') ?? '—'}
						</span>
					</div>
					<div class="flex justify-between text-sm">
						<span style="color: var(--color-muted)">Reference</span>
						<span class="font-mono text-xs" style="color: var(--color-foreground)">
							{paymentDetail.reference}
						</span>
					</div>
					<div class="flex justify-between text-sm">
						<span style="color: var(--color-muted)">Date</span>
						<span style="color: var(--color-foreground)">
							{formatDate(paymentDetail.created_at)}
						</span>
					</div>
					{#if paymentDetail.refunded_at}
						<div class="flex items-center gap-2 rounded-lg p-3" style="background-color: color-mix(in srgb, var(--color-info, #2563eb) 8%, transparent)">
							<Undo2 class="h-4 w-4" style="color: var(--color-info, #2563eb)" />
							<span class="text-sm" style="color: var(--color-info, #2563eb)">
								Refunded on {formatDate(paymentDetail.refunded_at)}
							</span>
						</div>
					{/if}
				</div>

				{#if paymentDetail.refundable}
					<div class="mt-6">
						<button
							class="flex w-full items-center justify-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/5"
							style="border-color: var(--color-danger); color: var(--color-danger)"
							onclick={() => openRefundModal(paymentDetail!.id)}
						>
							<Undo2 class="h-4 w-4" />
							Request Refund
						</button>
					</div>
				{/if}
			{:else}
				<div class="mt-6 text-center">
					<AlertCircle class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
					<p class="mt-2 text-sm" style="color: var(--color-muted)">
						Failed to load payment details
					</p>
				</div>
			{/if}
		</div>
	</div>
{/if}

<!-- Refund Modal -->
{#if showRefundModal}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		onclick={closeRefundModal}
	>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-sm rounded-xl border p-6"
			style="border-color: var(--color-border); background: var(--color-surface)"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">
				Request Refund
			</h3>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Please provide a reason for your refund request.
			</p>

			{#if refundError}
				<div
					class="mt-3 rounded-lg border px-3 py-2 text-sm"
					style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
				>
					{refundError}
				</div>
			{/if}

			<div class="mt-4">
				<label class="text-sm font-medium" style="color: var(--color-foreground)" for="refund-reason">
					Reason
				</label>
				<textarea
					id="refund-reason"
					bind:value={refundReason}
					rows="3"
					placeholder="Tell us why you need a refund..."
					class="mt-1 w-full rounded-lg border px-3 py-2 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
				></textarea>
			</div>

			<div class="mt-6 flex gap-3">
				<button
					class="flex-1 rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/5"
					style="border-color: var(--color-border); color: var(--color-foreground)"
					onclick={closeRefundModal}
				>
					Cancel
				</button>
				<button
					class="flex-1 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-danger)"
					disabled={refundSubmitting || !refundReason.trim()}
					onclick={submitRefund}
				>
					{#if refundSubmitting}
						<Loader2 class="inline h-4 w-4 animate-spin" />
					{:else}
						Submit
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}
