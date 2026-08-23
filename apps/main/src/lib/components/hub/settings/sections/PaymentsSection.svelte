<script lang="ts">
	// Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Settings hub)
	// Billing / Payments section. Displays the user's payment history fetched
	// from GET /me/payments and lets the user view a single payment's details
	// via GET /me/payments/{id}.
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Receipt, X } from 'lucide-svelte';
	import type { Payment } from '$lib/types';

	interface PaymentsData {
		payments: Payment[];
	}

	let { data }: { data: unknown } = $props();

	let payments = $derived((data as PaymentsData | null)?.payments ?? []);
	let selected = $state<Payment | null>(null);
	let detailLoading = $state(false);

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString(i18n.locale, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	function formatCurrency(amount: string, currency: string): string {
		const num = Number(amount);
		if (Number.isNaN(num)) return `${amount} ${currency}`;
		try {
			return new Intl.NumberFormat(i18n.locale, { style: 'currency', currency }).format(num);
		} catch {
			return `${amount} ${currency}`;
		}
	}

	function statusLabel(status: string): string {
		return t(`settings.payments.status_${status}`, status);
	}

	async function viewDetail(payment: Payment) {
		if (selected?.id === payment.id) {
			selected = payment;
			return;
		}
		selected = payment;
		detailLoading = true;
		try {
			const result = await api.get<Payment>(`/me/payments/${payment.id}`);
			if (result.data) selected = result.data;
		} catch {
			// Fall back to the list row data if the detail fetch fails.
		} finally {
			detailLoading = false;
		}
	}
</script>

<section class="max-w-3xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.payments')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.payments.desc')}</p>
	</header>

	{#if payments.length === 0}
		<EmptyState icon={Receipt} title={t('settings.payments.empty_title')} body={t('settings.payments.empty_body')} />
	{:else}
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-[var(--color-border)] text-left text-xs text-[var(--color-muted)]">
					<th class="py-2 pr-4">{t('settings.payments.provider')}</th>
					<th class="py-2 pr-4">{t('settings.payments.amount')}</th>
					<th class="py-2 pr-4">{t('settings.payments.status')}</th>
					<th class="py-2 pr-4">{t('settings.payments.date')}</th>
					<th class="py-2"></th>
				</tr>
			</thead>
			<tbody>
				{#each payments as p (p.id)}
					<tr class="border-b border-[var(--color-border)]">
						<td class="py-2 pr-4 text-[var(--color-text)]">{p.provider}</td>
						<td class="py-2 pr-4 text-[var(--color-text)]">{formatCurrency(p.amount, p.currency)}</td>
						<td class="py-2 pr-4 capitalize text-[var(--color-muted)]">{statusLabel(p.status)}</td>
						<td class="py-2 pr-4 text-[var(--color-muted)]">{formatDate(p.created_at)}</td>
						<td class="py-2 text-right">
							<button
								type="button"
								onclick={() => viewDetail(p)}
								class="text-xs text-[var(--color-primary)] hover:underline"
							>
								{t('settings.payments.view')}
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}

	{#if selected}
		<div
			class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
			aria-label={t('settings.payments.detail_title')}
		>
			<div class="w-full max-w-md rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-base font-medium text-[var(--color-text)]">{t('settings.payments.detail_title')}</h2>
					<button
						type="button"
						onclick={() => selected = null}
						class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
						aria-label={t('settings.payments.close')}
					>
						<X class="h-4 w-4" />
					</button>
				</div>

				{#if detailLoading}
					<p class="text-sm text-[var(--color-muted)]">{t('settings.payments.loading')}</p>
				{:else}
					<dl class="space-y-2 text-sm">
						<div class="flex justify-between">
							<dt class="text-[var(--color-muted)]">{t('settings.payments.provider')}</dt>
							<dd class="text-[var(--color-text)]">{selected.provider}</dd>
						</div>
						<div class="flex justify-between">
							<dt class="text-[var(--color-muted)]">{t('settings.payments.amount')}</dt>
							<dd class="text-[var(--color-text)]">{formatCurrency(selected.amount, selected.currency)}</dd>
						</div>
						<div class="flex justify-between">
							<dt class="text-[var(--color-muted)]">{t('settings.payments.status')}</dt>
							<dd class="text-[var(--color-text)]">{statusLabel(selected.status)}</dd>
						</div>
						{#if selected.purpose_type}
							<div class="flex justify-between">
								<dt class="text-[var(--color-muted)]">{t('settings.payments.purpose')}</dt>
								<dd class="text-[var(--color-text)]">{selected.purpose_type}</dd>
							</div>
						{/if}
						{#if selected.provider_ref}
							<div class="flex justify-between">
								<dt class="text-[var(--color-muted)]">{t('settings.payments.provider_ref')}</dt>
								<dd class="text-[var(--color-text)] break-all">{selected.provider_ref}</dd>
							</div>
						{/if}
						<div class="flex justify-between">
							<dt class="text-[var(--color-muted)]">{t('settings.payments.date')}</dt>
							<dd class="text-[var(--color-text)]">{formatDate(selected.created_at)}</dd>
						</div>
						{#if selected.checkout_url}
							<div class="pt-2">
								<a
									href={selected.checkout_url}
									target="_blank"
									rel="noopener noreferrer"
									class="text-xs text-[var(--color-primary)] hover:underline"
								>
									{t('settings.payments.checkout_link')}
								</a>
							</div>
						{/if}
					</dl>
				{/if}
			</div>
		</div>
	{/if}
</section>
