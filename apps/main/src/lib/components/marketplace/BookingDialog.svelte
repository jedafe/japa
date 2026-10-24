<script lang="ts">
	// Booking dialog (§114 marketplace, spec §3.3) — the booking form
	// extracted from services/[slug] + tier radio picker and coupon
	// price-preview on top.
	//
	// Ref: japaconnect-engineering/sections/111 service packages
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	//
	// Preserved from the inline form: departure autocomplete, party size,
	// preferred dates w/ flexible flag, provider + email for paid intent,
	// POST /services/{id}/bookings, checkout redirect on
	// payment.checkout_url, inquiry success state.
	//
	// Idempotency (Rule 9): the backend requires the Idempotency-Key header
	// on every booking (intent='booking' → 400 idempotency_key_required),
	// so every submit carries one — paid or inquiry. One key per dialog
	// open so a retry replays the cached response instead of
	// double-booking; regenerated after success.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Modal } from '$lib/components/ui';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import { formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import { safeSameOriginHref } from '$lib/utils/safeUrl';
	import { AlertCircle, CheckCircle2, Loader2, Send, TicketPercent } from 'lucide-svelte';
	import type {
		PackageTier,
		ServiceBookingResult,
		ServiceListing,
		ServicePricePreview
	} from '$lib/types';

	interface Props {
		service: ServiceListing;
		currencies: CurrencyInfo[];
		authed: boolean;
		/** Defaults to service.tiers — pass through so callers can inject a
		 *  subset. The API omits tiers entirely when the tiers flag is off,
		 *  so an empty list is also the flag-off state. */
		tiers?: PackageTier[];
		/** Tier preselected when the dialog opens (TierGrid "Choose X"). */
		initialTierId?: string;
		/** Prefill for the paid checkout receipt email. */
		defaultEmail?: string;
		open?: boolean;
	}
	let {
		service,
		currencies,
		authed,
		tiers,
		initialTierId = '',
		defaultEmail = '',
		open = $bindable(false)
	}: Props = $props();

	const isPaid = $derived(service.purchase_mode === 'paid');
	// Position order is server-side (ORDER BY position) — copy before sort
	// so the prop array is never mutated.
	const tierRows = $derived(
		[...(tiers ?? service.tiers ?? [])].sort((a, b) => a.position - b.position)
	);
	// Coupons ride the §114 flag; they apply to paid bookings only — the
	// backend rejects coupon_code on inquiry intent.
	const couponsEnabled = $derived(
		isPaid && !!page.data.features?.['service_coupons.enabled']
	);
	const openDepartures = $derived(
		(service.departures ?? []).filter((d) => d.status === 'open')
	);
	// Preserve the query string so a post-login return lands on the same
	// filtered/paginated URL (same pattern as ServiceCard's guest guard).
	const loginHref = $derived(
		`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`
	);

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString();
		} catch {
			return iso;
		}
	}

	let partySize = $state('1');
	let bookingDepartureId = $state('');
	// §114 tier selection — submitted as tier_id on paid bookings. The
	// backend rejects tier_id on inquiry intent, so inquiry submissions
	// carry the tier name inside the note instead.
	let selectedTierId = $state('');
	let tierError = $state('');
	let prefStartOn = $state('');
	let prefEndOn = $state('');
	let prefFlexible = $state(false);
	let bookingNote = $state('');
	let bookingProvider = $state('');
	let bookingEmail = $state(untrack(() => defaultEmail));
	let bookingBusy = $state(false);
	let submitError = $state('');
	let submitOk = $state(false);
	let bookingProviders = $state<{ id: string; name: string; enabled: boolean }[]>([]);
	let providersLoaded = $state(false);
	let bookingIdemKey = $state('');

	// Coupon state — couponCode is the raw input; appliedCode is the code
	// the last successful preview validated. Editing the input away from
	// the applied code clears the quote so stale totals never submit.
	let couponCode = $state('');
	let appliedCode = $state('');
	let preview = $state<ServicePricePreview | null>(null);
	let couponErrCode = $state('');
	let couponBusy = $state(false);

	const selectedTier = $derived(tierRows.find((x) => x.id === selectedTierId) ?? null);

	const inputCls =
		'rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	const departureItems = $derived<AutocompleteItem[]>(
		openDepartures.map((d) => ({
			id: d.id,
			label: formatDate(d.starts_on) + (d.ends_on ? ` → ${formatDate(d.ends_on)}` : ''),
			sublabel:
				d.capacity != null
					? t('services.pkg_seats_left', '{n} seats left', {
							n: Math.max(0, d.capacity - d.booked_count)
						})
					: t('services.pkg_unlimited', 'Unlimited seats')
		}))
	);
	const departureFetcher = $derived(staticAutocompleteFetcher(departureItems));
	const selectedDepartureLabel = $derived(
		bookingDepartureId
			? (departureItems.find((o) => o.id === bookingDepartureId)?.label ?? null)
			: null
	);

	const providerItems = $derived<AutocompleteItem[]>(
		bookingProviders.filter((p) => p.enabled).map((p) => ({ id: p.id, label: p.name }))
	);
	const providerFetcher = $derived(staticAutocompleteFetcher(providerItems));

	// One idempotency key + provider list per dialog open; the tier grid's
	// preselection is applied here so choosing a different tier while the
	// dialog is closed always wins over the previous session's pick.
	// The whole form resets per open (same contract as the mentorship
	// dialog) — a stale party size/note/coupon must never leak from one
	// listing's booking into the next.
	let prevOpen = false;
	$effect(() => {
		if (open && !prevOpen) {
			bookingIdemKey = crypto.randomUUID();
			submitError = '';
			submitOk = false;
			tierError = '';
			couponErrCode = '';
			selectedTierId = '';
			partySize = '1';
			bookingDepartureId = '';
			prefStartOn = '';
			prefEndOn = '';
			prefFlexible = false;
			bookingNote = '';
			couponCode = '';
			appliedCode = '';
			preview = null;
			if (initialTierId) selectedTierId = initialTierId;
			if (isPaid) loadProviders();
		}
		prevOpen = open;
	});

	async function loadProviders() {
		if (providersLoaded) return;
		const res = await api.get<{ id: string; name: string; enabled: boolean }[]>(
			'/payment-providers'
		);
		if (res.error) return; // leave providersLoaded false so reopening retries
		providersLoaded = true;
		bookingProviders = res.data ?? [];
	}

	// coupon_err_<code> keys are seeded for every backend error_code; an
	// unknown/missing code (network_error, rate_limited, …) falls through
	// to the generic line.
	function couponErrorMessage(code: string): string {
		const mapped = t(`services.coupon_err_${code}`, '');
		return mapped || t('services.coupon_err_generic', 'Could not apply coupon');
	}

	function onCouponInput() {
		if (couponCode.trim() !== appliedCode) {
			appliedCode = '';
			preview = null;
			couponErrCode = '';
		}
	}

	function chooseTier(id: string) {
		selectedTierId = id;
		tierError = '';
		// A validated coupon re-prices against the new tier automatically.
		if (appliedCode) void applyCoupon();
	}

	async function applyCoupon() {
		const code = couponCode.trim();
		if (!code || couponBusy) return;
		couponBusy = true;
		couponErrCode = '';
		preview = null;
		const params = new URLSearchParams();
		if (selectedTierId) params.set('tier_id', selectedTierId);
		params.set('coupon_code', code);
		const res = await api.get<ServicePricePreview>(
			`/services/${service.id}/price-preview?${params.toString()}`
		);
		if (res.error) {
			couponBusy = false;
			appliedCode = '';
			if (res.error.code === 'invalid_tier') {
				// Stale tier id — deselect and surface the error under the picker.
				selectedTierId = '';
				tierError = t(
					'services.tier_unavailable',
					'That package option is no longer available.'
				);
			} else {
				couponErrCode = res.error.code || 'invalid_coupon';
			}
			return;
		}
		couponBusy = false;
		if (!res.data?.coupon_valid) {
			appliedCode = '';
			couponErrCode = res.data?.error_code || 'invalid_coupon';
			return;
		}
		appliedCode = code;
		preview = res.data;
	}

	function fmt(v: number | null | undefined, currency: string | null | undefined): string {
		return formatMoney(v, currency, currencies, { minor: true });
	}

	async function submitBooking() {
		submitError = '';
		const size = Number(partySize);

		if (!Number.isInteger(size) || size < 1) {
			submitError = t('services.book_party_size_invalid', 'Enter a valid party size');
			return;
		}
		// Departure and preferred dates are mutually exclusive server-side
		// (400 invalid_departure) — catch it here so the booker fixes the
		// form instead of eating a server round-trip.
		if (bookingDepartureId && (prefStartOn || prefEndOn)) {
			submitError = t(
				'services.book_dates_conflict',
				'Choose either a departure or preferred dates, not both.'
			);
			return;
		}
		if (isPaid && (!bookingProvider || !bookingEmail.trim())) {
			submitError = t(
				'services.book_checkout_fields',
				'Payment provider and email are required'
			);
			return;
		}

		let note = bookingNote.trim();
		// Inquiry intent rejects tier_id (ErrInvalidCoupon) — carry the
		// selection in the note so the provider still sees it.
		if (!isPaid && selectedTier) {
			const tierLine = t('services.book_tier_note', 'Interested in the {name} tier.', {
				name: selectedTier.name
			});
			note = note ? `${note}\n${tierLine}` : tierLine;
		}

		const body: Record<string, unknown> = {
			intent: isPaid ? 'booking' : 'inquiry',
			party_size: size,
			note
		};
		if (isPaid && selectedTier) body.tier_id = selectedTier.id;
		if (bookingDepartureId) body.departure_id = bookingDepartureId;
		if (!bookingDepartureId && (prefStartOn || prefEndOn)) {
			body.preferred_dates = {
				start_on: prefStartOn,
				end_on: prefEndOn || prefStartOn,
				flexible: prefFlexible
			};
		}
		if (isPaid) {
			body.provider = bookingProvider;
			body.email = bookingEmail.trim();
			// M5 client allowlist — mirror the server same-origin gate: only
			// send return_url when it stays on this origin, else omit it.
			const returnUrl = safeSameOriginHref(page.url.href);
			if (returnUrl) body.return_url = returnUrl;
			// Pass the code through even without a preview — the backend
			// revalidates on submit and coupon_* errors map back onto the
			// coupon error line.
			const code = couponCode.trim();
			if (code) body.coupon_code = code;
		}

		bookingBusy = true;
		const res = await api.post<ServiceBookingResult>(`/services/${service.id}/bookings`, body, {
			'Idempotency-Key': bookingIdemKey || crypto.randomUUID()
		});

		if (res.data) {
			// Paid mode opens the §17 fee checkout — send the booker off-site.
			const checkout = res.data.payment?.checkout_url;
			if (isPaid && checkout) {
				window.location.href = checkout;
				return;
			}
			bookingBusy = false;
			submitOk = true;
			bookingIdemKey = crypto.randomUUID();
		} else {
			bookingBusy = false;
			const code = res.error?.code ?? '';
			if (code === 'invalid_tier') {
				selectedTierId = '';
				tierError = t(
					'services.tier_unavailable',
					'That package option is no longer available.'
				);
			} else if (code === 'invalid_coupon' || code.startsWith('coupon_')) {
				appliedCode = '';
				preview = null;
				couponErrCode = code;
			} else if (code === 'invalid_departure') {
				// Both date surfaces, a started/past departure, or a
				// now-full departure — the departure picker is the fix.
				bookingDepartureId = '';
				submitError = t(
					'services.book_departure_invalid',
					'That departure is no longer available — choose another date.'
				);
			} else if (code === 'departure_full' || code === 'invalid_party_size') {
				submitError = t(
					'services.book_capacity_exceeded',
					'That date cannot take your party size — try fewer travellers or another date.'
				);
			} else {
				submitError = res.error?.message ?? t('services.book_failed', 'Could not submit');
			}
		}
	}
</script>

<Modal
	bind:open
	title={isPaid
		? t('services.book_heading_paid', 'Book this package')
		: t('services.book_heading_inquiry', 'Booking & inquiries')}
	size="lg"
>
	{#if !authed}
		<!-- Guests get a login prompt — bookings require an account (the
		     price-preview endpoint itself accepts anonymous quotes). -->
		<p class="text-sm text-[var(--color-muted)]">
			<a href={loginHref} class="text-[var(--color-primary)] hover:underline">
				{isPaid
					? t('services.book_login_reserve', 'Log in to reserve')
					: t('services.book_login_inquiry', 'Log in to send an inquiry')}
			</a>
		</p>
	{:else if submitOk}
		<div
			class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-success)]/30 bg-[var(--color-success)]/10 px-4 py-3 text-sm text-[var(--color-success)]"
			role="status"
		>
			<CheckCircle2 size={18} aria-hidden="true" />
			{isPaid
				? t('services.book_reserved', 'Booking created — complete payment to confirm.')
				: t(
						'services.book_inquiry_sent',
						'Inquiry sent — the provider will respond through your bookings inbox.'
					)}
		</div>
		<div class="mt-4 flex justify-end">
			<button
				type="button"
				onclick={() => (open = false)}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.close', 'Close')}
			</button>
		</div>
	{:else}
		<div class="space-y-3">
			<!-- §114 tier picker — radio rows above the departure select.
			     tier_id submits on paid intent only; inquiry carries the
			     tier name inside the note (backend rejects tier_id there). -->
			{#if tierRows.length > 0}
				<fieldset>
					<legend class="mb-1 text-xs font-medium text-[var(--color-text-subtle)]">
						{t('services.tier_picker_label', 'Choose a package')}
					</legend>
					<div class="space-y-2">
						{#each tierRows as tier (tier.id)}
							{@const active = selectedTierId === tier.id}
							<label
								class="flex cursor-pointer items-center gap-3 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors {active
									? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5 ring-1 ring-[var(--color-primary)]/30'
									: 'border-[var(--color-border)] bg-[var(--color-surface)] hover:bg-[var(--color-surface-subtle)]'}"
							>
								<input
									type="radio"
									name="bk-tier"
									checked={active}
									onchange={() => chooseTier(tier.id)}
									class="h-3.5 w-3.5 shrink-0 accent-[var(--color-primary)]"
								/>
								<span class="flex min-w-0 flex-1 items-center gap-2">
									<span class="truncate font-medium text-[var(--color-text)]">{tier.name}</span>
									{#if tier.badge}
										<span
											class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[11px] text-[var(--color-text-subtle)]"
										>
											{tier.badge}
										</span>
									{/if}
								</span>
								<span class="shrink-0 font-semibold text-[var(--color-text)]">
									{fmt(tier.price_amount, tier.price_currency)}
								</span>
							</label>
						{/each}
					</div>
					{#if selectedTier}
						<button
							type="button"
							onclick={() => chooseTier('')}
							class="mt-1 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
						>
							{t('common.remove', 'Remove')}
						</button>
					{/if}
					{#if tierError}
						<p class="mt-1 flex items-center gap-1 text-sm text-red-500" role="alert">
							<AlertCircle size={14} aria-hidden="true" /> {tierError}
						</p>
					{/if}
				</fieldset>
			{/if}

			{#if openDepartures.length > 0}
				<div>
					<label
						for="bk-dep"
						class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
					>
						{t('services.book_departure', 'Departure')}
					</label>
					<Autocomplete
						id="bk-dep"
						placeholder={t('services.book_departure_ph', 'Choose a date')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={departureFetcher}
						initialFetcher={departureFetcher}
						selectedId={bookingDepartureId || null}
						selectedLabel={selectedDepartureLabel}
						onSelect={(item) => (bookingDepartureId = item.id)}
						onClear={() => (bookingDepartureId = '')}
					/>
				</div>
			{/if}

			{#if openDepartures.length === 0 || !bookingDepartureId}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					<div>
						<label
							for="bk-from"
							class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
							>{t('services.book_pref_start', 'Preferred start')}</label
						>
						<input id="bk-from" type="date" bind:value={prefStartOn} class="{inputCls} w-full" />
					</div>
					<div>
						<label
							for="bk-to"
							class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
							>{t('services.book_pref_end', 'Preferred end')}</label
						>
						<input id="bk-to" type="date" bind:value={prefEndOn} class="{inputCls} w-full" />
					</div>
				</div>
				<label class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
					<input
						type="checkbox"
						checked={prefFlexible}
						onchange={(e) => (prefFlexible = e.currentTarget.checked)}
						class="h-3.5 w-3.5 rounded border-[var(--color-border)]"
					/>
					{t('services.book_flexible', 'My dates are flexible')}
				</label>
			{/if}

			<div>
				<label
					for="bk-party"
					class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
					>{t('services.book_party_size', 'Party size')}</label
				>
				<input id="bk-party" type="number" min="1" bind:value={partySize} class="{inputCls} w-24" />
			</div>

			<!-- §114 coupon field — paid intent only; the backend rejects
			     coupon_code on inquiry. Apply hits the price-preview
			     endpoint; a valid code renders the totals breakdown. -->
			{#if couponsEnabled}
				<div>
					<label
						for="bk-coupon"
						class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
						>{t('services.coupon_code_label', 'Coupon code')}</label
					>
					<div class="flex gap-2">
						<input
							id="bk-coupon"
							bind:value={couponCode}
							oninput={onCouponInput}
							placeholder={t('services.coupon_code_ph', 'Enter a code')}
							autocomplete="off"
							class="{inputCls} flex-1"
						/>
						<button
							type="button"
							onclick={applyCoupon}
							disabled={couponBusy || !couponCode.trim()}
							class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{#if couponBusy}
								<Loader2 size={14} class="motion-safe:animate-spin" aria-hidden="true" />
							{:else}
								<TicketPercent size={14} aria-hidden="true" />
							{/if}
							{t('services.coupon_apply', 'Apply')}
						</button>
					</div>
					{#if couponErrCode}
						<p class="mt-1 flex items-center gap-1 text-sm text-red-500" role="alert">
							<AlertCircle size={14} aria-hidden="true" /> {couponErrorMessage(couponErrCode)}
						</p>
					{:else if preview?.coupon_valid}
						<div
							class="mt-2 rounded-[var(--radius)] border border-[var(--color-success)]/30 bg-[var(--color-success)]/5 px-3 py-2"
						>
							<p class="flex items-center gap-1.5 text-xs font-medium text-[var(--color-success)]">
								<CheckCircle2 size={14} aria-hidden="true" />
								{t('services.coupon_applied', 'Coupon applied')}
							</p>
							<dl class="mt-2 space-y-1 text-sm">
								<div class="flex items-center justify-between gap-2">
									<dt class="text-[var(--color-text-subtle)]">
										{t('services.preview_subtotal', 'Subtotal')}
									</dt>
									<dd class="text-[var(--color-text)]">
										{fmt(preview.base, preview.currency)}
									</dd>
								</div>
								<div class="flex items-center justify-between gap-2">
									<dt class="text-[var(--color-text-subtle)]">
										{t('services.preview_discount', 'Discount')}
									</dt>
									<dd class="text-[var(--color-success)]">
										−{fmt(preview.discount, preview.currency)}
									</dd>
								</div>
								<div class="flex items-center justify-between gap-2">
									<dt class="font-medium text-[var(--color-text)]">
										{t('services.preview_total', 'Total')}
									</dt>
									<dd class="font-semibold text-[var(--color-text)]">
										{fmt(preview.total, preview.currency)}
									</dd>
								</div>
								<div
									class="flex items-center justify-between gap-2 border-t border-[var(--color-border)] pt-1"
								>
									<dt class="text-[var(--color-text-subtle)]">
										{t('services.preview_fee', 'Fee due now')}
									</dt>
									<dd class="font-medium text-[var(--color-text)]">
										{fmt(preview.fee, preview.currency)}
									</dd>
								</div>
								<div class="flex items-center justify-between gap-2">
									<dt class="text-[var(--color-text-subtle)]">
										{t('services.preview_balance', 'Balance to provider')}
									</dt>
									<dd class="text-[var(--color-text)]">
										{fmt(preview.balance_due, preview.currency)}
									</dd>
								</div>
							</dl>
						</div>
					{/if}
				</div>
			{/if}

			<div>
				<label
					for="bk-note"
					class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
					>{t('services.book_note', 'Message to the provider')}</label
				>
				<textarea
					id="bk-note"
					bind:value={bookingNote}
					rows={3}
					placeholder={t('services.book_note_ph', 'Questions, requirements, context…')}
					class="{inputCls} w-full"
				></textarea>
			</div>

			{#if isPaid}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					<div>
						<label
							for="bk-prov"
							class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
							>{t('services.book_provider', 'Payment method')}</label
						>
						<Autocomplete
							id="bk-prov"
							placeholder={t('services.book_provider_ph', 'Choose a payment method')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={providerFetcher}
							initialFetcher={providerFetcher}
							selectedId={bookingProvider || null}
							selectedLabel={providerItems.find((o) => o.id === bookingProvider)?.label ?? null}
							onSelect={(item) => (bookingProvider = item.id)}
							onClear={() => (bookingProvider = '')}
						/>
					</div>
					<div>
						<label
							for="bk-email"
							class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]"
							>{t('services.book_email', 'Email for receipt')}</label
						>
						<input
							id="bk-email"
							type="email"
							bind:value={bookingEmail}
							placeholder={t('services.book_email_ph', 'you@example.com')}
							class="{inputCls} w-full"
						/>
					</div>
				</div>
			{/if}

			{#if submitError}
				<div
					class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500"
					role="alert"
				>
					<AlertCircle size={14} aria-hidden="true" /> {submitError}
				</div>
			{/if}
			<div class="flex gap-2">
				<button
					type="button"
					onclick={submitBooking}
					disabled={bookingBusy}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if bookingBusy}<Loader2
							size={16}
							class="motion-safe:animate-spin"
							aria-hidden="true"
						/>{:else}<Send class="rtl:-scale-x-100" size={16} aria-hidden="true" />{/if}
					{isPaid ? t('services.book_pay_reserve', 'Pay & reserve') : t('services.book_send', 'Send inquiry')}
				</button>
				<button
					type="button"
					onclick={() => (open = false)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.cancel', 'Cancel')}
				</button>
			</div>
		</div>
	{/if}
</Modal>
