<script lang="ts">
	// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { formatMoney } from '$lib/utils/currency';
	import { CalendarDays, Users, Package, AlertCircle, ArrowLeft, CheckCircle2 } from 'lucide-svelte';
	import { RelativeTime } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const b = $derived(data.booking);
	const title = $derived(t('bookings.detail_title', 'Booking — JapaConnect'));

	function statusColor(status: string): string {
		switch (status) {
			case 'confirmed':
			case 'fulfilled':
				return 'text-[var(--color-success)]';
			case 'pending':
				return 'text-[var(--color-info)]';
			case 'disputed':
				return 'text-[var(--color-warning,var(--color-info))]';
			case 'cancelled':
			case 'declined':
				return 'text-[var(--color-danger)]';
			default:
				return 'text-[var(--color-muted)]';
		}
	}

	function formatDate(iso?: string): string {
		if (!iso) return '—';
		return new Date(iso).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	// §111 booker actions — cancel pending|confirmed, dispute
	// confirmed|fulfilled. Same rules as the inbox rows.
	const canCancel = $derived(b.status === 'pending' || b.status === 'confirmed');
	const canDispute = $derived(b.status === 'confirmed' || b.status === 'fulfilled');

	// Checkout resume — only pending paid bookings with an unsettled hosted
	// fee (manual_transfer/credit have no session to reopen).
	const HOSTED_PROVIDERS = new Set(['paystack', 'stripe', 'nowpayments']);
	const canResumePayment = $derived(
		b.intent === 'booking' &&
			b.status === 'pending' &&
			b.payment_id != null &&
			b.payment_status === 'pending' &&
			HOSTED_PROVIDERS.has(b.payment_provider ?? '')
	);

	let busy = $state(false);
	let actionError = $state('');
	let disputeOpen = $state(false);
	let disputeReason = $state('');
	let payOpen = $state(false);
	let payEmail = $state(untrack(() => data.user?.email ?? ''));
	let payIdemKey = $state('');

	function openPayForm() {
		payIdemKey = crypto.randomUUID(); // Rule 9 — one key per attempt
		payOpen = true;
		actionError = '';
	}

	async function resumePayment() {
		if (!payEmail.trim()) {
			actionError = t('bookings.pay_email_required', 'Email is required for the payment receipt');
			return;
		}
		busy = true;
		actionError = '';
		const res = await api.post<{ id: string; checkout_url: string }>(
			`/me/service-bookings/${b.id}/pay`,
			{ email: payEmail.trim(), return_url: page.url.href },
			{ 'Idempotency-Key': payIdemKey || crypto.randomUUID() }
		);
		busy = false;
		if (res.error) {
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		const url = res.data?.checkout_url;
		if (url) {
			window.location.href = url; // §17 hosted checkout — send booker off-site
			return;
		}
		payOpen = false;
		await invalidateAll();
	}

	async function cancelBooking() {
		busy = true;
		actionError = '';
		const res = await api.post(`/me/service-bookings/${b.id}/cancel`, {});
		busy = false;
		if (res.error) {
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		await invalidateAll();
	}

	async function submitDispute() {
		busy = true;
		actionError = '';
		const res = await api.post(`/me/service-bookings/${b.id}/dispute`, {
			reason: disputeReason.trim()
		});
		busy = false;
		if (res.error) {
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		disputeOpen = false;
		disputeReason = '';
		await invalidateAll();
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/me/service-bookings"
			class="mb-1 inline-flex items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={14} /> {t('bookings.back', 'All bookings')}
		</a>
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Package size={20} />
			{t('bookings.detail_heading', 'Booking')}
			<span class="text-sm font-medium capitalize {statusColor(b.status)}">
				{t(`bookings.status_${b.status}`, b.status)}
			</span>
		</h1>
	</div>

	<div class="space-y-4 p-4">
		{#if actionError}
			<div class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				<AlertCircle size={16} /> {actionError}
			</div>
		{/if}

		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<a href="/services/{b.listing_slug}" class="text-base font-medium text-[var(--color-text)] hover:underline">
				{b.listing_title || t('bookings.untitled', 'Untitled listing')}
			</a>

			<dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-3">
				<div>
					<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.filter_type', 'Type')}</dt>
					<dd class="mt-0.5 capitalize text-[var(--color-text)]">{t(`bookings.intent_${b.intent}`, b.intent)}</dd>
				</div>
				{#if b.org_name}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.provider', 'Provider')}</dt>
						<dd class="mt-0.5 text-[var(--color-text)]">{b.org_name}</dd>
					</div>
				{/if}
				<div>
					<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.col_party_size', 'Party size')}</dt>
					<dd class="mt-0.5 inline-flex items-center gap-1 text-[var(--color-text)]"><Users size={13} /> {b.party_size}</dd>
				</div>
				{#if b.departure_starts_on}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.col_departure', 'Departure')}</dt>
						<dd class="mt-0.5 inline-flex items-center gap-1 text-[var(--color-text)]">
							<CalendarDays size={13} />
							{formatDate(b.departure_starts_on)}{#if b.departure_ends_on} → {formatDate(b.departure_ends_on)}{/if}
						</dd>
					</div>
				{/if}
				{#if b.preferred_dates && (b.preferred_dates.start_on || b.preferred_dates.end_on)}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.preferred_dates', 'Preferred')}</dt>
						<dd class="mt-0.5 text-[var(--color-text)]">
							{b.preferred_dates.start_on || '—'} → {b.preferred_dates.end_on || '—'}
							{#if b.preferred_dates.flexible} · {t('bookings.flexible', 'flexible')}{/if}
						</dd>
					</div>
				{/if}
				{#if b.status === 'pending' && b.expires_at}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.expires', 'Expires')}</dt>
						<dd class="mt-0.5 text-[var(--color-text)]"><RelativeTime iso={b.expires_at} /></dd>
					</div>
				{/if}
			</dl>

			{#if b.note}
				<div class="mt-3 border-t border-[var(--color-border)] pt-3">
					<p class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.your_note', 'Your note')}</p>
					<p class="mt-1 text-sm text-[var(--color-text)]">{b.note}</p>
				</div>
			{/if}
		</div>

		{#if b.intent === 'booking' && (b.total_price != null || b.fee_amount != null)}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('bookings.payment_heading', 'Payment')}</h2>
				<dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-3">
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.col_total', 'Package total')}</dt>
						<dd class="mt-0.5 text-[var(--color-text)]">{formatMoney(b.total_price, b.fee_currency, null, { minor: true }) || '—'}</dd>
					</div>
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.col_fee', 'Platform fee paid')}</dt>
						<dd class="mt-0.5 text-[var(--color-text)]">{formatMoney(b.fee_amount, b.fee_currency, null, { minor: true }) || '—'}</dd>
					</div>
					{#if b.payment_status}
						<div>
							<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.col_payment_status', 'Payment status')}</dt>
							<dd class="mt-0.5 capitalize text-[var(--color-text)]">
								{t(`bookings.pay_status_${b.payment_status}`, b.payment_status)}
								{#if b.payment_provider} · {b.payment_provider}{/if}
							</dd>
						</div>
					{/if}
					{#if b.agent_balance_due != null}
						<div>
							<dt class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.col_balance', 'Balance due to provider')}</dt>
							<dd class="mt-0.5 text-[var(--color-text)]">{formatMoney(b.agent_balance_due, b.fee_currency, null, { minor: true })}</dd>
						</div>
					{/if}
				</dl>
			</div>
		{/if}

		{#if b.org_response_note || (b.status === 'disputed' && b.dispute_reason) || (b.status === 'cancelled' && b.cancel_reason)}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('bookings.activity_heading', 'Activity')}</h2>
				{#if b.org_response_note}
					<p class="mt-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1.5 text-sm text-[var(--color-text)]">
						{b.org_response_note}
					</p>
				{/if}
				{#if b.status === 'disputed' && b.dispute_reason}
					<p class="mt-2 text-sm text-[var(--color-danger)]">{b.dispute_reason}</p>
				{/if}
				{#if b.status === 'cancelled' && b.cancel_reason}
					<p class="mt-2 text-sm text-[var(--color-muted)]">{b.cancel_reason}</p>
				{/if}
			</div>
		{/if}

		{#if canResumePayment || canCancel || canDispute}
			<div class="flex gap-2">
				{#if canResumePayment}
					<button
						type="button"
						disabled={busy}
						onclick={openPayForm}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
					>{t('bookings.pay_resume', 'Complete payment')}</button>
				{/if}
				{#if canCancel}
					<button
						type="button"
						disabled={busy}
						onclick={cancelBooking}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>{t('bookings.cancel', 'Cancel booking')}</button>
				{/if}
				{#if canDispute}
					<button
						type="button"
						disabled={busy}
						onclick={() => { disputeOpen = true; disputeReason = ''; }}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-danger)] hover:bg-red-500/10 disabled:opacity-50"
					>{t('bookings.dispute', 'Report a problem')}</button>
				{/if}
			</div>
		{/if}
	</div>
</div>

{#if disputeOpen}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
		<div class="w-full max-w-md rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-lg">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('bookings.dispute_title', 'Report a problem')}</h2>
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{t('bookings.dispute_hint', 'Describe what went wrong. The platform team will review your dispute.')}
			</p>
			<textarea
				bind:value={disputeReason}
				rows="4"
				class="mt-3 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
				placeholder={t('bookings.dispute_reason_ph', 'What happened?')}
			></textarea>
			<div class="mt-3 flex justify-end gap-2">
				<button
					type="button"
					onclick={() => (disputeOpen = false)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>{t('common.cancel', 'Cancel')}</button>
				<button
					type="button"
					disabled={!disputeReason.trim() || busy}
					onclick={submitDispute}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-danger)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
				>
					{#if busy}<CheckCircle2 size={14} />{/if}
					{t('bookings.dispute_submit', 'Submit dispute')}
				</button>
			</div>
		</div>
	</div>
{/if}

{#if payOpen}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
		<div class="w-full max-w-md rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-lg">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('bookings.pay_title', 'Complete payment')}</h2>
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{t('bookings.pay_hint', 'Reopen the secure checkout for the platform fee to keep this booking.')}
			</p>
			<label for="pay-email" class="mb-1 mt-3 block text-xs font-medium text-[var(--color-muted)]">
				{t('bookings.pay_email', 'Email for receipt')}
			</label>
			<input
				id="pay-email"
				type="email"
				bind:value={payEmail}
				placeholder="you@example.com"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
			/>
			{#if b.fee_amount != null}
				<p class="mt-2 text-xs text-[var(--color-muted)]">
					{t('bookings.pay_amount', 'Amount due now')}: {formatMoney(b.fee_amount, b.fee_currency, null, { minor: true })}
				</p>
			{/if}
			<div class="mt-3 flex justify-end gap-2">
				<button
					type="button"
					onclick={() => (payOpen = false)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>{t('common.cancel', 'Cancel')}</button>
				<button
					type="button"
					disabled={!payEmail.trim() || busy}
					onclick={resumePayment}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
				>
					{#if busy}<CheckCircle2 size={14} />{/if}
					{t('bookings.pay_submit', 'Continue to checkout')}
				</button>
			</div>
		</div>
	</div>
{/if}
