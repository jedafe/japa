<script lang="ts">
	// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { CalendarDays, Users, Package, AlertCircle, CheckCircle2, Layers, BadgePercent } from 'lucide-svelte';
	import { EmptyState, PaginationControls, RelativeTime } from '$lib/components/ui';
	import { formatMoney } from '$lib/utils/currency';
	import type { ServiceBooking } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('bookings.my_title', 'Service Bookings — JapaConnect'));
	const bookings = $derived<ServiceBooking[]>(data.bookings);
	const meta = $derived(data.meta);

	// §111 booking statuses — pending, confirmed, fulfilled, cancelled,
	// declined, disputed, refunded.
	const statusFilters = [
		{ id: '', label: t('bookings.filter_all', 'All') },
		{ id: 'pending', label: t('bookings.status_pending', 'Pending') },
		{ id: 'confirmed', label: t('bookings.status_confirmed', 'Confirmed') },
		{ id: 'fulfilled', label: t('bookings.status_fulfilled', 'Fulfilled') },
		{ id: 'cancelled', label: t('bookings.status_cancelled', 'Cancelled') },
		{ id: 'declined', label: t('bookings.status_declined', 'Declined') },
		{ id: 'disputed', label: t('bookings.status_disputed', 'Disputed') },
		{ id: 'refunded', label: t('bookings.status_refunded', 'Refunded') }
	];
	const intentFilters = [
		{ id: '', label: t('bookings.filter_all', 'All') },
		{ id: 'inquiry', label: t('bookings.intent_inquiry', 'Inquiries') },
		{ id: 'booking', label: t('bookings.intent_booking', 'Bookings') }
	];

	function filterHref(params: Record<string, string>): string {
		const sp = new URLSearchParams(page.url.searchParams);
		for (const [k, v] of Object.entries(params)) {
			if (v) sp.set(k, v);
			else sp.delete(k);
		}
		sp.delete('page');
		return `/me/service-bookings?${sp.toString()}`;
	}

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
		if (!iso) return '';
		return new Date(iso).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	// Booker actions per §111: cancel pending|confirmed, dispute
	// confirmed|fulfilled.
	const canCancel = (b: ServiceBooking) => b.status === 'pending' || b.status === 'confirmed';
	const canDispute = (b: ServiceBooking) => b.status === 'confirmed' || b.status === 'fulfilled';
	// Pending paid booking with an unsettled hosted fee → checkout resume on
	// the detail page (manual_transfer/credit have no session to reopen).
	const HOSTED_PROVIDERS = new Set(['paystack', 'stripe', 'nowpayments']);
	const canPay = (b: ServiceBooking) =>
		b.intent === 'booking' && b.status === 'pending' && b.payment_id != null &&
		b.payment_status === 'pending' && HOSTED_PROVIDERS.has(b.payment_provider ?? '');

	let busyId = $state('');
	let actionError = $state('');
	let disputeFor = $state<ServiceBooking | null>(null);
	let disputeReason = $state('');

	async function cancelBooking(b: ServiceBooking) {
		busyId = b.id;
		actionError = '';
		const res = await api.post<ServiceBooking>(`/me/service-bookings/${b.id}/cancel`, {});
		busyId = '';
		if (res.error) {
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		await invalidateAll();
	}

	async function submitDispute() {
		if (!disputeFor) return;
		busyId = disputeFor.id;
		actionError = '';
		const res = await api.post<ServiceBooking>(`/me/service-bookings/${disputeFor.id}/dispute`, {
			reason: disputeReason.trim()
		});
		busyId = '';
		if (res.error) {
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		disputeFor = null;
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
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Package size={20} /> {t('bookings.my_title', 'Service Bookings')}
		</h1>
	</div>

	<div class="p-4">
		<!-- Filters -->
		<div class="mb-4 flex flex-wrap gap-4">
			<div class="flex flex-wrap items-center gap-1.5">
				<span class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.filter_status', 'Status')}:</span>
				{#each statusFilters as f (f.id)}
					<a
						href={filterHref({ status: f.id })}
						class="rounded-full border px-2.5 py-1 text-xs font-medium transition-colors {data.status === f.id
							? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>{f.label}</a>
				{/each}
			</div>
			<div class="flex flex-wrap items-center gap-1.5">
				<span class="text-xs font-medium text-[var(--color-muted)]">{t('bookings.filter_type', 'Type')}:</span>
				{#each intentFilters as f (f.id)}
					<a
						href={filterHref({ intent: f.id })}
						class="rounded-full border px-2.5 py-1 text-xs font-medium transition-colors {data.intent === f.id
							? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>{f.label}</a>
				{/each}
			</div>
		</div>

		{#if actionError}
			<div class="mb-4 flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				<AlertCircle size={16} /> {actionError}
			</div>
		{/if}

		{#if bookings.length === 0}
			<EmptyState
				icon={Package}
				title={t('bookings.empty', 'No bookings yet')}
				body={t('bookings.empty_body', 'Browse service packages to send an inquiry or book a departure.')}
				action={{ label: t('bookings.browse', 'Browse services'), href: '/services?kind=package' }}
			/>
		{:else}
			<div class="space-y-3">
				{#each bookings as b (b.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<a href="/services/{b.listing_slug}" class="text-sm font-medium text-[var(--color-text)] hover:underline">
									{b.listing_title || t('bookings.untitled', 'Untitled listing')}
								</a>
								<div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
									<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 font-medium">
										{t(`bookings.intent_${b.intent}`, b.intent)}
									</span>
									{#if b.org_name}
										<span>{b.org_name}</span>
									{/if}
									{#if b.departure_starts_on}
										<span class="inline-flex items-center gap-1">
											<CalendarDays size={12} />
											{formatDate(b.departure_starts_on)}{#if b.departure_ends_on} – {formatDate(b.departure_ends_on)}{/if}
										</span>
									{/if}
									<span class="inline-flex items-center gap-1">
										<Users size={12} /> {b.party_size}
									</span>
									{#if b.tier_name}
										<span class="inline-flex items-center gap-1"><Layers size={12} /> {b.tier_name}</span>
									{/if}
									{#if b.coupon_code}
										<span class="inline-flex items-center gap-1 text-[var(--color-primary)]">
											<BadgePercent size={12} /> {b.coupon_code}{#if b.discount_amount != null}
												&nbsp;−{formatMoney(b.discount_amount, b.fee_currency, null, { minor: true })}{/if}
										</span>
									{/if}
									<span><RelativeTime iso={b.created_at} /></span>
								</div>
								{#if b.note}
									<p class="mt-2 text-xs text-[var(--color-muted)]">{b.note}</p>
								{/if}
								{#if b.org_response_note}
									<p class="mt-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1.5 text-xs text-[var(--color-text)]">
										{b.org_response_note}
									</p>
								{/if}
								{#if b.status === 'disputed' && b.dispute_reason}
									<p class="mt-2 text-xs text-[var(--color-danger)]">{b.dispute_reason}</p>
								{/if}
								{#if b.status === 'cancelled' && b.cancel_reason}
									<p class="mt-2 text-xs text-[var(--color-muted)]">{b.cancel_reason}</p>
								{/if}
							</div>
							<span class="shrink-0 text-xs font-medium capitalize {statusColor(b.status)}">
								{t(`bookings.status_${b.status}`, b.status)}
							</span>
						</div>

						<div class="mt-3 flex gap-2 border-t border-[var(--color-border)] pt-3">
							{#if canPay(b)}
								<a
									href="/me/service-bookings/{b.id}"
									class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90"
								>{t('bookings.pay_resume', 'Complete payment')}</a>
							{/if}
							<a
								href="/me/service-bookings/{b.id}"
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>{t('bookings.details', 'Details')}</a>
							{#if canCancel(b)}
								<button
									type="button"
									disabled={busyId === b.id}
									onclick={() => cancelBooking(b)}
									class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
								>{t('bookings.cancel', 'Cancel')}</button>
							{/if}
							{#if canDispute(b)}
								<button
									type="button"
									disabled={busyId === b.id}
									onclick={() => { disputeFor = b; disputeReason = ''; }}
									class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-danger)] hover:bg-red-500/10 disabled:opacity-50"
								>{t('bookings.dispute', 'Report a problem')}</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>

			{#if meta?.total && meta.total > (meta.limit ?? 20)}
				<div class="mt-4">
					<PaginationControls
						page={data.currentPage}
						totalPages={Math.ceil((meta.total ?? 0) / (meta.limit ?? 20))}
						onPageChange={(p) => {
							const params = new URLSearchParams(page.url.searchParams);
							params.set('page', String(p));
							goto(`/me/service-bookings?${params.toString()}`);
						}}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>

<!-- Dispute dialog -->
{#if disputeFor}
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
					onclick={() => (disputeFor = null)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>{t('common.cancel', 'Cancel')}</button>
				<button
					type="button"
					disabled={!disputeReason.trim() || busyId === disputeFor.id}
					onclick={submitDispute}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-danger)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
				>
					{#if busyId === disputeFor.id}<CheckCircle2 size={14} />{/if}
					{t('bookings.dispute_submit', 'Submit dispute')}
				</button>
			</div>
		</div>
	</div>
{/if}
