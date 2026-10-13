<script lang="ts">
	// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md,
	// sections/102-mentorship-matching.md (000824 — mentorship bookings are
	// user-provider service_bookings rows)
	//
	// Right pane of the /messages/bookings + /messages/mentorship inboxes —
	// the selected booking's detail plus its actions:
	//   booker   → cancel (pending|confirmed), complete (confirmed
	//              mentorship via POST /me/service-bookings/:id/complete),
	//              dispute (confirmed|fulfilled), pay-resume (pending
	//              hosted-fee checkout) — ported from the
	//              /me/service-bookings pages.
	//   provider → accept/decline (pending), complete|cancel (confirmed) via
	//              POST /me/service-bookings/:id/respond — mentorship rows
	//              only; org-owned bookings never reach this pane under
	//              role=provider.
	// The BookingDTO carries no dm_conversation_id — "Open messages" falls
	// back to the /messages inbox (the accept path auto-opens the thread
	// server-side, ADR-110).
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { formatMoney } from '$lib/utils/currency';
	import { safeSameOriginHref } from '$lib/utils/safeUrl';
	import {
		CalendarDays,
		Users,
		AlertCircle,
		ArrowLeft,
		CheckCircle2,
		MapPin
	} from 'lucide-svelte';
	import { RelativeTime, Modal } from '$lib/components/ui';
	import type { ServiceBooking } from '$lib/types';
	import {
		bookingCounterpart,
		bookingStatus,
		bookingStatusColor,
		bookingStatusLabel,
		formatBookingDate
	} from './bookingDisplay';

	interface Props {
		booking: ServiceBooking;
		/** Which inbox the detail was opened from — drives counterpart labels. */
		role: 'sent' | 'received';
		/** The signed-in viewer — decides booker vs provider action sets. */
		viewerId: string;
		viewerEmail?: string;
		/** feature.dm — hides "Open messages" when DM is off (bookings still work). */
		dmEnabled: boolean;
		/** service_packages.enabled — the dispute route is packages-gated
		 *  (HandleMyBookingDispute), so mentorship-kind rows hide "Report a
		 *  problem" when packages is off. */
		packagesEnabled: boolean;
		/** Clear the ?b= selection (mobile back). */
		onClose: () => void;
	}

	let {
		booking,
		role,
		dmEnabled,
		packagesEnabled,
		viewerId,
		viewerEmail = '',
		onClose
	}: Props = $props();
	const b = $derived(booking);

	// The viewer's relationship to the row — derived from the booking, not
	// the list filter, so a ?b= deep link opened under the "wrong" role tab
	// still shows the right counterpart label and action set.
	const isBooker = $derived(!!b.booker_user_id && b.booker_user_id === viewerId);
	const isProvider = $derived(!!b.provider_user_id && b.provider_user_id === viewerId);
	const viewRole = $derived<'sent' | 'received'>(
		isProvider ? 'received' : isBooker ? 'sent' : role
	);

	const counterpart = $derived(bookingCounterpart(b, viewRole));
	const goals = $derived(b.goals || b.note || '');
	const canCancel = $derived(isBooker && (b.status === 'pending' || b.status === 'confirmed'));
	// The dispute route is packages-gated server-side — mentorship-kind rows
	// (the only kind that can exist while packages is off) hide the action.
	const canDispute = $derived(
		isBooker &&
			(b.status === 'confirmed' || b.status === 'fulfilled') &&
			(b.listing_kind !== 'mentorship' || packagesEnabled)
	);

	// Checkout resume — only pending paid bookings with an unsettled hosted
	// fee (manual_transfer/credit have no session to reopen).
	const HOSTED_PROVIDERS = new Set(['paystack', 'stripe', 'nowpayments']);
	const canResumePayment = $derived(
		isBooker &&
			b.intent === 'booking' &&
			b.status === 'pending' &&
			b.payment_id != null &&
			b.payment_status === 'pending' &&
			HOSTED_PROVIDERS.has(b.payment_provider ?? '')
	);
	// A failed fee checkout is terminal for this payment row — the booker
	// sees the failure with a support nudge rather than a dead resume
	// button (ResumePayment only accepts payment_status='pending').
	const feeFailed = $derived(
		isBooker &&
			b.intent === 'booking' &&
			b.status === 'pending' &&
			b.payment_id != null &&
			b.payment_status === 'failed'
	);

	// Provider actions (000824) — pending → accept|decline, confirmed →
	// complete|cancel. Only applies to user-provider (mentorship) rows.
	const canAcceptDecline = $derived(isProvider && b.status === 'pending');
	const canComplete = $derived(isProvider && b.status === 'confirmed');
	// Mentor cancel on a confirmed booking — the provider arm of "either
	// party cancels" via respond {action:'cancel'} (a pending row is
	// declined instead). The optional note maps to cancel_reason.
	const canProviderCancel = $derived(
		isProvider && b.status === 'confirmed' && b.listing_kind === 'mentorship'
	);
	// Mentee complete (000824) — POST /me/service-bookings/:id/complete,
	// confirmed → fulfilled. Mentorship-kind only: org-owned bookings leave
	// the fulfill decision to the org inbox.
	const canBookerComplete = $derived(
		isBooker && b.status === 'confirmed' && b.listing_kind === 'mentorship'
	);

	let busy = $state(false);
	let actionError = $state('');
	let disputeOpen = $state(false);
	let disputeReason = $state('');
	let declineOpen = $state(false);
	let declineNote = $state('');
	let cancelOpen = $state(false);
	let cancelNote = $state('');
	// Booker cancel (pending|confirmed) — confirm modal carrying the
	// cancel reason (the server records it; the provider is notified).
	let bookerCancelOpen = $state(false);
	let completeOpen = $state(false);
	let payOpen = $state(false);
	let payEmail = $state(untrack(() => viewerEmail));
	let payIdemKey = $state('');

	function openPayForm() {
		payIdemKey = crypto.randomUUID(); // Rule 9 — one key per attempt
		payOpen = true;
		actionError = '';
	}

	// Every booking mutation carries an idempotency key (Rule 9 — the
	// cancel/complete/respond/dispute routes replay on retry). One key per
	// attempt so a failure followed by a retry replays instead of
	// double-firing the transition + fee-settle tasks.
	function idemHeaders(): Record<string, string> {
		return { 'Idempotency-Key': crypto.randomUUID() };
	}

	// Svelte 5: $state must not be set in finally (no re-render) — set
	// busy explicitly on both branches instead.
	async function resumePayment() {
		if (!payEmail.trim()) {
			actionError = t('bookings.pay_email_required', 'Email is required for the payment receipt');
			return;
		}
		busy = true;
		actionError = '';
		const res = await api.post<{ id: string; checkout_url: string }>(
			`/me/service-bookings/${b.id}/pay`,
			// M5 client allowlist — only send a same-origin return_url.
			{
				email: payEmail.trim(),
				...(safeSameOriginHref(page.url.href)
					? { return_url: safeSameOriginHref(page.url.href) }
					: {})
			},
			{ 'Idempotency-Key': payIdemKey || crypto.randomUUID() }
		);
		if (res.error) {
			busy = false;
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		const url = res.data?.checkout_url;
		if (url) {
			window.location.href = url; // §17 hosted checkout — send booker off-site
			return;
		}
		busy = false;
		payOpen = false;
		await invalidateAll();
	}

	async function cancelBooking() {
		if (!cancelNote.trim()) {
			actionError = t('bookings.cancel_reason_required', 'Add a short note so the provider knows why.');
			return;
		}
		busy = true;
		actionError = '';
		const res = await api.post(`/me/service-bookings/${b.id}/cancel`, {
			reason: cancelNote.trim()
		}, idemHeaders());
		if (res.error) {
			busy = false;
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		busy = false;
		cancelOpen = false;
		cancelNote = '';
		bookerCancelOpen = false;
		await invalidateAll();
	}

	async function submitDispute() {
		busy = true;
		actionError = '';
		const res = await api.post(`/me/service-bookings/${b.id}/dispute`, {
			reason: disputeReason.trim()
		}, idemHeaders());
		if (res.error) {
			busy = false;
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		busy = false;
		disputeOpen = false;
		disputeReason = '';
		await invalidateAll();
	}

	// Provider transition — POST /me/service-bookings/:id/respond
	// {action: 'accept'|'decline'|'complete'|'cancel', note?}. The accept path
	// auto-opens the mentor↔mentee DM thread server-side (ADR-110); the
	// response has no conversation id, so the UI just refreshes.
	async function respond(action: 'accept' | 'decline' | 'complete' | 'cancel', note = '') {
		busy = true;
		actionError = '';
		const res = await api.post<ServiceBooking>(`/me/service-bookings/${b.id}/respond`, {
			action,
			...(note.trim() ? { note: note.trim() } : {})
		}, idemHeaders());
		if (res.error) {
			busy = false;
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		busy = false;
		declineOpen = false;
		declineNote = '';
		cancelOpen = false;
		cancelNote = '';
		await invalidateAll();
	}

	// Booker-side complete (000824) — the mentee arm of "either party
	// completes": POST /me/service-bookings/:id/complete moves a confirmed
	// user-provider booking to fulfilled and notifies the provider.
	async function completeBooking() {
		busy = true;
		actionError = '';
		const res = await api.post<ServiceBooking>(`/me/service-bookings/${b.id}/complete`, {}, idemHeaders());
		if (res.error) {
			busy = false;
			actionError = res.error.message ?? t('bookings.action_failed', 'Action failed');
			return;
		}
		busy = false;
		completeOpen = false;
		await invalidateAll();
	}
</script>

<div class="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-2 sm:hidden">
	<button
		type="button"
		onclick={onClose}
		class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-muted)] hover:text-[var(--color-text)]"
	>
		<ArrowLeft size={14} /> {t('messages.back', 'Back')}
	</button>
</div>

<div class="space-y-4 p-4">
	{#if actionError}
		<div
			class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500"
			role="alert"
		>
			<AlertCircle size={16} /> {actionError}
		</div>
	{/if}

	<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-start justify-between gap-3">
			{#if b.listing_slug}
				<a
					href="/services/{b.listing_slug}"
					class="min-w-0 flex-1 text-base font-medium text-[var(--color-text)] hover:underline"
				>
					{b.listing_title || t('bookings.untitled', 'Untitled listing')}
				</a>
			{:else}
				<span class="min-w-0 flex-1 text-base font-medium text-[var(--color-text)]">
					{b.listing_title || t('bookings.untitled', 'Untitled listing')}
				</span>
			{/if}
			<span class="shrink-0 text-sm font-medium capitalize {bookingStatusColor(bookingStatus(b))}">
				{bookingStatusLabel(b)}
			</span>
		</div>

		<dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-3">
			<div>
				<dt class="text-xs font-medium text-[var(--color-muted)]">
					{t('bookings.filter_type', 'Type')}
				</dt>
				<dd class="mt-0.5 capitalize text-[var(--color-text)]">
					{t(`bookings.intent_${b.intent}`, b.intent)}
				</dd>
			</div>
			<div>
				<dt class="text-xs font-medium text-[var(--color-muted)]">
					{viewRole === 'received'
						? t('bookings.booker', 'Booker')
						: t('bookings.provider', 'Provider')}
				</dt>
				<dd class="mt-0.5 truncate text-[var(--color-text)]">
					{#if counterpart.username}
						<a href="/u/{counterpart.username}" class="hover:text-[var(--color-primary)]">
							{counterpart.name || `@${counterpart.username}`}
						</a>
					{:else}
						{counterpart.name || '—'}
					{/if}
				</dd>
			</div>
			<div>
				<dt class="text-xs font-medium text-[var(--color-muted)]">
					{t('bookings.col_party_size', 'Party size')}
				</dt>
				<dd class="mt-0.5 inline-flex items-center gap-1 text-[var(--color-text)]">
					<Users size={13} /> {b.party_size}
				</dd>
			</div>
			{#if b.departure_starts_on}
				<div>
					<dt class="text-xs font-medium text-[var(--color-muted)]">
						{t('bookings.col_departure', 'Departure')}
					</dt>
					<dd class="mt-0.5 inline-flex items-center gap-1 text-[var(--color-text)]">
						<CalendarDays size={13} />
						{formatBookingDate(b.departure_starts_on)}{#if b.departure_ends_on}
							→ {formatBookingDate(b.departure_ends_on)}{/if}
					</dd>
				</div>
			{/if}
			{#if b.preferred_dates && (b.preferred_dates.start_on || b.preferred_dates.end_on)}
				<div>
					<dt class="text-xs font-medium text-[var(--color-muted)]">
						{t('bookings.preferred_dates', 'Preferred')}
					</dt>
					<dd class="mt-0.5 text-[var(--color-text)]">
						{b.preferred_dates.start_on || '—'} → {b.preferred_dates.end_on || '—'}
						{#if b.preferred_dates.flexible} · {t('bookings.flexible', 'flexible')}{/if}
					</dd>
				</div>
			{/if}
			{#if b.destination_space_name}
				<div>
					<dt class="text-xs font-medium text-[var(--color-muted)]">
						{t('bookings.destination', 'Destination')}
					</dt>
					<dd class="mt-0.5 inline-flex items-center gap-1 text-[var(--color-text)]">
						<MapPin size={13} /> {b.destination_space_name}
					</dd>
				</div>
			{/if}
			{#if b.status === 'pending' && b.expires_at}
				<div>
					<dt class="text-xs font-medium text-[var(--color-muted)]">
						{t('bookings.expires', 'Expires')}
					</dt>
					<dd class="mt-0.5 text-[var(--color-text)]"><RelativeTime iso={b.expires_at} /></dd>
				</div>
			{/if}
		</dl>

		{#if goals}
			<div class="mt-3 border-t border-[var(--color-border)] pt-3">
				<p class="text-xs font-medium text-[var(--color-muted)]">
					{b.listing_kind === 'mentorship'
						? t('bookings.goals', 'Goals')
						: viewRole === 'received'
							? t('bookings.their_note', 'Their note')
							: t('bookings.your_note', 'Your note')}
				</p>
				<p class="mt-1 whitespace-pre-line text-sm text-[var(--color-text)]">{goals}</p>
			</div>
		{/if}
	</div>

	{#if b.intent === 'booking' && (b.total_price != null || b.fee_amount != null)}
		<div
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<h2 class="text-sm font-semibold text-[var(--color-text)]">
				{t('bookings.payment_heading', 'Payment')}
			</h2>
			<dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-3">
				<div>
					<dt class="text-xs font-medium text-[var(--color-muted)]">
						{t('bookings.col_total', 'Package total')}
					</dt>
					<dd class="mt-0.5 text-[var(--color-text)]">
						{formatMoney(b.total_price, b.fee_currency, null, { minor: true }) || '—'}
					</dd>
				</div>
				<div>
					<dt class="text-xs font-medium text-[var(--color-muted)]">
						{t('bookings.col_fee', 'Platform fee paid')}
					</dt>
					<dd class="mt-0.5 text-[var(--color-text)]">
						{formatMoney(b.fee_amount, b.fee_currency, null, { minor: true }) || '—'}
					</dd>
				</div>
				{#if b.tier_name}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">
							{t('bookings.col_tier', 'Tier')}
						</dt>
						<dd class="mt-0.5 text-[var(--color-text)]">{b.tier_name}</dd>
					</div>
				{/if}
				{#if b.coupon_code}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">
							{t('bookings.col_coupon', 'Coupon')}
						</dt>
						<dd class="mt-0.5 font-mono text-[var(--color-text)]">{b.coupon_code}</dd>
					</div>
				{/if}
				{#if b.discount_amount != null && b.discount_amount > 0}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">
							{t('bookings.col_discount', 'Coupon discount')}
						</dt>
						<dd class="mt-0.5 text-[var(--color-success)]">
							−{formatMoney(b.discount_amount, b.fee_currency, null, { minor: true })}
						</dd>
					</div>
				{/if}
				{#if b.payment_status}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">
							{t('bookings.col_payment_status', 'Payment status')}
						</dt>
						<dd class="mt-0.5 capitalize text-[var(--color-text)]">
							{t(`bookings.pay_status_${b.payment_status}`, b.payment_status)}
							{#if b.payment_provider} · {b.payment_provider}{/if}
						</dd>
					</div>
				{/if}
				{#if b.agent_balance_due != null}
					<div>
						<dt class="text-xs font-medium text-[var(--color-muted)]">
							{t('bookings.col_balance', 'Balance due to provider')}
						</dt>
						<dd class="mt-0.5 text-[var(--color-text)]">
							{formatMoney(b.agent_balance_due, b.fee_currency, null, { minor: true })}
						</dd>
					</div>
				{/if}
			</dl>
		</div>
	{/if}

	{#if b.org_response_note || (b.status === 'disputed' && b.dispute_reason) || (b.status === 'cancelled' && b.cancel_reason)}
		<div
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<h2 class="text-sm font-semibold text-[var(--color-text)]">
				{t('bookings.activity_heading', 'Activity')}
			</h2>
			{#if b.org_response_note}
				<p
					class="mt-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1.5 text-sm text-[var(--color-text)]"
				>
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

	{#if canResumePayment || canCancel || canDispute || canAcceptDecline || canComplete || canProviderCancel || canBookerComplete || feeFailed || dmEnabled}
		<div class="flex flex-wrap gap-2">
			{#if canResumePayment}
				<button
					type="button"
					disabled={busy}
					onclick={openPayForm}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>{t('bookings.pay_resume', 'Complete payment')}</button>
			{/if}
			{#if feeFailed}
				<p class="w-full rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-xs text-red-500" role="alert">
					{t(
						'bookings.pay_failed',
						'The fee payment failed. Contact support to reopen this booking.'
					)}
				</p>
			{/if}
			{#if canAcceptDecline}
				<button
					type="button"
					disabled={busy}
					onclick={() => respond('accept')}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>{t('bookings.accept', 'Accept')}</button>
				<button
					type="button"
					disabled={busy}
					onclick={() => {
						declineOpen = true;
						declineNote = '';
					}}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-danger)] hover:bg-red-500/10 disabled:opacity-50"
				>{t('bookings.decline', 'Decline')}</button>
			{/if}
			{#if canComplete}
				<button
					type="button"
					disabled={busy}
					onclick={() => respond('complete')}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>{t('bookings.complete', 'Mark completed')}</button>
			{/if}
			{#if canProviderCancel}
				<button
					type="button"
					disabled={busy}
					onclick={() => {
						cancelOpen = true;
						cancelNote = '';
					}}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-danger)] hover:bg-red-500/10 disabled:opacity-50"
				>{t('bookings.cancel_mentorship', 'Cancel mentorship')}</button>
			{/if}
			{#if canBookerComplete}
				<button
					type="button"
					disabled={busy}
					onclick={() => (completeOpen = true)}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>{t('bookings.complete', 'Mark completed')}</button>
			{/if}
			{#if canCancel}
				<button
					type="button"
					disabled={busy}
					onclick={() => {
						bookerCancelOpen = true;
						cancelNote = '';
					}}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>{t('bookings.cancel', 'Cancel')}</button>
			{/if}
			{#if canDispute}
				<button
					type="button"
					disabled={busy}
					onclick={() => {
						disputeOpen = true;
						disputeReason = '';
					}}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-danger)] hover:bg-red-500/10 disabled:opacity-50"
				>{t('bookings.dispute', 'Report a problem')}</button>
			{/if}
			{#if dmEnabled}
				<a
					href="/messages"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>{t('bookings.open_messages', 'Open messages')}</a>
			{/if}
		</div>
	{/if}
</div>

<Modal bind:open={bookerCancelOpen} title={t('bookings.cancel_title', 'Cancel booking')} size="md">
	<p class="text-xs text-[var(--color-muted)]">
		{t(
			'bookings.cancel_hint',
			'Let the provider know why you are cancelling. Paid bookings cancelled late may forfeit the platform fee.'
		)}
	</p>
	<textarea
		bind:value={cancelNote}
		rows="3"
		class="mt-3 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
		placeholder={t('bookings.cancel_reason_ph', 'Reason for cancelling')}
	></textarea>
	<div class="mt-3 flex justify-end gap-2">
		<button
			type="button"
			onclick={() => (bookerCancelOpen = false)}
			class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>{t('common.cancel', 'Cancel')}</button>
		<button
			type="button"
			disabled={!cancelNote.trim() || busy}
			onclick={cancelBooking}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-danger)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
		>
			{#if busy}<CheckCircle2 size={14} />{/if}
			{t('bookings.cancel_submit', 'Cancel booking')}
		</button>
	</div>
</Modal>

<Modal bind:open={disputeOpen} title={t('bookings.dispute_title', 'Report a problem')} size="md">
	<p class="text-xs text-[var(--color-muted)]">
		{t(
			'bookings.dispute_hint',
			'Describe what went wrong. The platform team will review your dispute.'
		)}
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
</Modal>

<Modal bind:open={declineOpen} title={t('bookings.decline_title', 'Decline request')} size="md">
	<p class="text-xs text-[var(--color-muted)]">
		{t('bookings.respond_note_hint', 'Add an optional note for the other person.')}
	</p>
	<textarea
		bind:value={declineNote}
		rows="3"
		class="mt-3 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
		placeholder={t('bookings.respond_note_ph', 'Note (optional)')}
	></textarea>
	<div class="mt-3 flex justify-end gap-2">
		<button
			type="button"
			onclick={() => (declineOpen = false)}
			class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>{t('common.cancel', 'Cancel')}</button>
		<button
			type="button"
			disabled={busy}
			onclick={() => respond('decline', declineNote)}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-danger)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
		>
			{#if busy}<CheckCircle2 size={14} />{/if}
			{t('bookings.decline', 'Decline')}
		</button>
	</div>
</Modal>

<Modal
	bind:open={cancelOpen}
	title={t('bookings.cancel_mentorship', 'Cancel mentorship')}
	size="md"
>
	<p class="text-xs text-[var(--color-muted)]">
		{t('bookings.respond_note_hint', 'Add an optional note for the other person.')}
	</p>
	<textarea
		bind:value={cancelNote}
		rows="3"
		class="mt-3 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
		placeholder={t('bookings.respond_note_ph', 'Note (optional)')}
	></textarea>
	<div class="mt-3 flex justify-end gap-2">
		<button
			type="button"
			onclick={() => (cancelOpen = false)}
			class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>{t('common.cancel', 'Cancel')}</button>
		<button
			type="button"
			disabled={busy}
			onclick={() => respond('cancel', cancelNote)}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-danger)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
		>
			{#if busy}<CheckCircle2 size={14} />{/if}
			{t('bookings.cancel_mentorship', 'Cancel mentorship')}
		</button>
	</div>
</Modal>

<Modal bind:open={completeOpen} title={t('bookings.complete', 'Mark completed')} size="md">
	<p class="text-xs text-[var(--color-muted)]">
		{t(
			'bookings.complete_hint',
			'Confirm this mentorship is complete. The mentor will be notified.'
		)}
	</p>
	<div class="mt-3 flex justify-end gap-2">
		<button
			type="button"
			onclick={() => (completeOpen = false)}
			class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>{t('common.cancel', 'Cancel')}</button>
		<button
			type="button"
			disabled={busy}
			onclick={completeBooking}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
		>
			{#if busy}<CheckCircle2 size={14} />{/if}
			{t('bookings.complete', 'Mark completed')}
		</button>
	</div>
</Modal>

<Modal bind:open={payOpen} title={t('bookings.pay_title', 'Complete payment')} size="md">
	<p class="text-xs text-[var(--color-muted)]">
		{t(
			'bookings.pay_hint',
			'Reopen the secure checkout for the platform fee to keep this booking.'
		)}
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
			{t('bookings.pay_amount', 'Amount due now')}: {formatMoney(
				b.fee_amount,
				b.fee_currency,
				null,
				{ minor: true }
			)}
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
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
		>
			{#if busy}<CheckCircle2 size={14} />{/if}
			{t('bookings.pay_submit', 'Continue to checkout')}
		</button>
	</div>
</Modal>
