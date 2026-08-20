<script lang="ts">
	// CheckoutModal — course purchase dialog for paid tracks (ADR-063).
	// Shows the price, accepts a discount code, and initiates payment via
	// POST /payments/intent with purpose_type='course_access'. For Pro
	// subscribers, the parent skips this modal entirely.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import { formatPriceKobo } from '$lib/format';
	import { X, CreditCard, ShieldCheck, Tag } from 'lucide-svelte';
	import type { TrackDetail, SessionUser } from '$lib/types';

	interface Props {
		track: TrackDetail;
		user: SessionUser | null;
		open: boolean;
		onClose: () => void;
		onSuccess: () => void;
	}

	let { track, user, open, onClose, onSuccess }: Props = $props();

	let processing = $state(false);
	let discountCode = $state('');
	let discountApplied = $state<{ code: string; amount_kobo: number } | null>(null);
	let error = $state<string | null>(null);
	let applying = $state(false);

	const displayPrice = $derived(formatPriceKobo(track.price_kobo, track.currency, page.data.locale));
	const finalPrice = $derived(
		discountApplied
			? formatPriceKobo(track.price_kobo! - discountApplied.amount_kobo, track.currency, page.data.locale)
			: displayPrice
	);

	async function applyDiscount() {
		if (!discountCode.trim() || applying) return;
		applying = true;
		error = null;
		const r = await api.post<{ code: string; discount_kobo: number }>(
			`/payments/discount-codes/validate`,
			{ code: discountCode.trim(), purpose_type: 'course_access', purpose_id: track.id }
		);
		applying = false;
		if (r.data) {
			discountApplied = { code: r.data.code, amount_kobo: r.data.discount_kobo };
		} else if (r.error) {
			error = r.error.message;
		}
	}

	async function payWithWallet() {
		if (!user) return;
		processing = true;
		error = null;
		const r = await api.post<{ payment: { id: string }; status: string }>(
			`/payments/intent`,
			{
				provider: 'credit',
				purpose_type: 'course_access',
				purpose_id: track.id,
				amount: track.price_kobo,
				currency: track.currency,
				discount_code: discountApplied?.code ?? undefined
			},
			{ 'Idempotency-Key': crypto.randomUUID() }
		);
		processing = false;
		if (r.data?.status === 'succeeded' || r.data?.status === 'completed') {
			onSuccess();
		} else if (r.error) {
			error = r.error.message;
		}
	}

	async function payWithProvider(provider: string) {
		if (!user) return;
		processing = true;
		error = null;
		const r = await api.post<{ payment: { id: string }; checkout_url: string }>(
			`/payments/intent`,
			{
				provider,
				purpose_type: 'course_access',
				purpose_id: track.id,
				amount: track.price_kobo,
				currency: track.currency,
				discount_code: discountApplied?.code ?? undefined,
				return_url: `${window.location.origin}/tracks/${track.slug}`
			},
			{ 'Idempotency-Key': crypto.randomUUID() }
		);
		processing = false;
		if (r.data?.checkout_url) {
			window.location.href = r.data.checkout_url;
		} else if (r.error) {
			error = r.error.message;
		}
	}
</script>

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<!-- Backdrop -->
		<button
			type="button"
			class="absolute inset-0 bg-black/50 backdrop-blur-sm"
			onclick={onClose}
			aria-label={t('common.close', 'Close')}
		></button>

		<!-- Modal -->
		<div class="relative w-full max-w-md overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl">
			<!-- Header -->
			<div class="flex items-center justify-between border-b border-[var(--color-border)] p-4">
				<h2 class="text-lg font-semibold text-[var(--color-text)]">
					{t('university.checkout', 'Checkout')}
				</h2>
				<button
					type="button"
					class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
					onclick={onClose}
					aria-label={t('common.close', 'Close')}
				>
					<X size={18} />
				</button>
			</div>

			<!-- Body -->
			<div class="space-y-4 p-4">
				<!-- Course summary -->
				<div class="flex items-start gap-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
					<div class="flex-1">
						<p class="font-medium text-[var(--color-text)]">{track.title}</p>
						<p class="text-sm text-[var(--color-muted)]">{t('university.course_purchase', 'Course purchase')}</p>
					</div>
					<p class="text-lg font-bold text-[var(--color-text)]">{displayPrice}</p>
				</div>

				<!-- Discount code -->
				<div class="space-y-2">
					<label class="text-sm font-medium text-[var(--color-text)]" for="discount">
						<span class="inline-flex items-center gap-1"><Tag size={14} /> {t('university.discount_code', 'Discount code')}</span>
					</label>
					<div class="flex gap-2">
						<input
							id="discount"
							type="text"
							bind:value={discountCode}
							placeholder={t('university.enter_code', 'Enter code')}
							class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
						<button
							type="button"
							disabled={applying}
							onclick={applyDiscount}
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{applying ? t('common.processing', 'Processing...') : t('common.apply', 'Apply')}
						</button>
					</div>
					{#if discountApplied}
						<p class="text-xs text-[var(--color-progress)]">
							{t('university.discount_applied', 'Discount applied')}: -{formatPriceKobo(discountApplied.amount_kobo, track.currency, page.data.locale)}
						</p>
					{/if}
				</div>

				<!-- Total -->
				<div class="flex items-center justify-between border-t border-[var(--color-border)] pt-3">
					<span class="text-sm text-[var(--color-muted)]">{t('common.total', 'Total')}</span>
					<span class="text-xl font-bold text-[var(--color-text)]">{finalPrice}</span>
				</div>

				{#if error}
					<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 p-3 text-sm text-[var(--color-danger)]">{error}</p>
				{/if}

				<!-- Payment methods -->
				<div class="space-y-2">
					<!-- Wallet (credits) -->
					<button
						type="button"
						disabled={processing}
						onclick={payWithWallet}
						class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-3 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						<CreditCard size={18} />
						{processing ? t('common.processing', 'Processing...') : t('university.pay_with_wallet', 'Pay with Wallet')}
					</button>

					<!-- Paystack -->
					<button
						type="button"
						disabled={processing}
						onclick={() => payWithProvider('paystack')}
						class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-3 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{t('university.pay_with_card', 'Pay with Card (Paystack)')}
					</button>
				</div>

				<!-- Trust badge -->
				<p class="flex items-center justify-center gap-1 text-xs text-[var(--color-muted)]">
					<ShieldCheck size={14} />
					{t('university.secure_payment', 'Secure payment · 7-day money-back guarantee')}
				</p>
			</div>
		</div>
	</div>
{/if}
