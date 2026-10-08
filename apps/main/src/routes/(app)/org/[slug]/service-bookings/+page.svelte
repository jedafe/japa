<script lang="ts">
	// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { CalendarDays, Users, Inbox, AlertCircle, Layers, BadgePercent } from 'lucide-svelte';
	import { EmptyState, PaginationControls, RelativeTime } from '$lib/components/ui';
	import { formatMoney } from '$lib/utils/currency';
	import type { ServiceBooking } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('bookings.org_title', 'Bookings inbox — JapaConnect'));
	const bookings = $derived<ServiceBooking[]>(data.bookings);
	const meta = $derived(data.meta);
	const org = $derived(data.org);

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
		return `/org/${org.slug}/service-bookings?${sp.toString()}`;
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

	// §111 state machine — pending → confirmed|declined, confirmed →
	// fulfilled|cancelled. Paid rows surface only once the fee settled
	// (the API hides unsettled paid bookings from this inbox).
	const transitionsFor = (b: ServiceBooking): { status: string; label: string; danger?: boolean }[] => {
		if (b.status === 'pending') {
			return [
				{ status: 'confirmed', label: t('bookings.confirm', 'Confirm') },
				{ status: 'declined', label: t('bookings.decline', 'Decline'), danger: true }
			];
		}
		if (b.status === 'confirmed') {
			return [
				{ status: 'fulfilled', label: t('bookings.fulfill', 'Mark fulfilled') },
				{ status: 'cancelled', label: t('bookings.cancel', 'Cancel'), danger: true }
			];
		}
		return [];
	};

	let busyId = $state('');
	let actionError = $state('');
	let noteFor = $state<{ booking: ServiceBooking; status: string } | null>(null);
	let noteText = $state('');

	async function transition(b: ServiceBooking, status: string, note: string) {
		busyId = b.id;
		actionError = '';
		const res = await api.patch<ServiceBooking>(`/org/${org.id}/service-bookings/${b.id}`, {
			status,
			org_response_note: note.trim()
		});
		busyId = '';
		if (res.error) {
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		noteFor = null;
		noteText = '';
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
			<Inbox size={20} /> {t('bookings.org_title', 'Bookings inbox')}
		</h1>
		<p class="mt-0.5 text-xs text-[var(--color-muted)]">
			<a href="/org/{org.slug}" class="hover:underline">{org.name}</a>
		</p>
	</div>

	<div class="p-4">
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
				icon={Inbox}
				title={t('bookings.org_empty', 'No bookings yet')}
				body={t('bookings.org_empty_body', 'Inquiries and paid bookings for your packages land here.')}
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
									{#if b.booker_username}
										<span>@{b.booker_username}</span>
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
									{#if b.total_price != null && b.fee_currency}
										<span>{formatMoney(b.total_price, b.fee_currency, null, { minor: true })}</span>
									{/if}
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
								{#if b.preferred_dates}
									<p class="mt-1 text-xs text-[var(--color-muted)]">
										{t('bookings.preferred_dates', 'Preferred')}: {b.preferred_dates.start_on || '—'} → {b.preferred_dates.end_on || '—'}
										{#if b.preferred_dates.flexible} · {t('bookings.flexible', 'flexible')}{/if}
									</p>
								{/if}
								{#if b.status === 'disputed' && b.dispute_reason}
									<p class="mt-2 text-xs text-[var(--color-danger)]">{b.dispute_reason}</p>
								{/if}
							</div>
							<span class="shrink-0 text-xs font-medium capitalize {statusColor(b.status)}">
								{t(`bookings.status_${b.status}`, b.status)}
							</span>
						</div>

						{#if transitionsFor(b).length > 0}
							<div class="mt-3 flex flex-wrap gap-2 border-t border-[var(--color-border)] pt-3">
								{#each transitionsFor(b) as tr (tr.status)}
									<button
										type="button"
										disabled={busyId === b.id}
										onclick={() => { noteFor = { booking: b, status: tr.status }; noteText = ''; }}
										class="rounded-[var(--radius)] border px-3 py-1.5 text-xs font-medium disabled:opacity-50 {tr.danger
											? 'border-[var(--color-border)] text-[var(--color-danger)] hover:bg-red-500/10'
											: 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)] hover:bg-[var(--color-primary)]/20'}"
									>{tr.label}</button>
								{/each}
							</div>
						{/if}
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
							goto(`/org/${org.slug}/service-bookings?${params.toString()}`);
						}}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>

<!-- Transition note dialog -->
{#if noteFor}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
		<div class="w-full max-w-md rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-lg">
			<h2 class="text-sm font-semibold capitalize text-[var(--color-text)]">
				{t(`bookings.status_${noteFor.status}`, noteFor.status)} — {noteFor.booking.listing_title}
			</h2>
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{t('bookings.org_note_hint', 'Optional note shown to the traveller.')}
			</p>
			<textarea
				bind:value={noteText}
				rows="3"
				class="mt-3 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
				placeholder={t('bookings.org_note_ph', 'Message to the traveller (optional)')}
			></textarea>
			<div class="mt-3 flex justify-end gap-2">
				<button
					type="button"
					onclick={() => (noteFor = null)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>{t('common.cancel', 'Cancel')}</button>
				<button
					type="button"
					disabled={busyId === noteFor.booking.id}
					onclick={() => noteFor && transition(noteFor.booking, noteFor.status, noteText)}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>{t('bookings.transition_submit', 'Apply')}</button>
			</div>
		</div>
	</div>
{/if}
