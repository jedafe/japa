<script lang="ts">
	// Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Settings hub)
	// Billing / Payments section. Displays the user's payment history fetched
	// from GET /me/payments and lets the user view a single payment's details
	// via GET /me/payments/{id}.
	import { Modal } from '$lib/components/ui';
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Receipt } from 'lucide-svelte';
	import { loadCurrencies, formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import type { Payment } from '$lib/types';

	interface PaymentsData {
		payments: Payment[];
		pageSize?: number;
	}

	let { data }: { data: unknown } = $props();

	const pageSize = $derived((data as PaymentsData | null)?.pageSize ?? 20);
	let payments = $state<Payment[]>([]);
	let selected = $state<Payment | null>(null);
	let detailLoading = $state(false);
	let loadingMore = $state(false);
	let loadMoreError = $state('');
	// §58 — managed currency list for amount display.
	let currencies = $state<CurrencyInfo[]>([]);
	// The list endpoint returns no total — "more" is inferred from a full page.
	let hasMore = $state(false);
	let currentPage = $state(0);

	// The resync reads only the loader payload (not `payments`) so appended
	// load-more pages don't retrigger a reset.
	$effect(() => {
		loadCurrencies().then((list) => (currencies = list));
		const incoming = (data as PaymentsData | null)?.payments ?? [];
		payments = incoming;
		currentPage = incoming.length > 0 ? 1 : 0;
		hasMore = incoming.length === pageSize;
	});

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		loadMoreError = '';
		const res = await api.get<Payment[]>(
			`/me/payments?page_size=${pageSize}&page_offset=${payments.length}`
		);
		if (res.error) {
			loadMoreError = res.error.message;
		} else {
			const batch = res.data ?? [];
			payments = [...payments, ...batch];
			currentPage = Math.ceil(payments.length / pageSize);
			hasMore = batch.length === pageSize;
		}
		loadingMore = false;
	}

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
		return formatMoney(num, currency, currencies, { locale: i18n.locale }) || `${amount} ${currency}`;
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
		<div class="overflow-x-auto">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-[var(--color-border)] text-start text-xs text-[var(--color-muted)]">
					<th class="py-2 pe-4">{t('settings.payments.provider')}</th>
					<th class="py-2 pe-4">{t('settings.payments.amount')}</th>
					<th class="py-2 pe-4">{t('settings.payments.status')}</th>
					<th class="py-2 pe-4">{t('settings.payments.date')}</th>
					<th class="py-2"></th>
				</tr>
			</thead>
			<tbody>
				{#each payments as p (p.id)}
					<tr class="border-b border-[var(--color-border)]">
						<td class="py-2 pe-4 text-[var(--color-text)]">{p.provider}</td>
						<td class="py-2 pe-4 text-[var(--color-text)]">{formatCurrency(p.amount, p.currency)}</td>
						<td class="py-2 pe-4 capitalize text-[var(--color-muted)]">{statusLabel(p.status)}</td>
						<td class="py-2 pe-4 text-[var(--color-muted)]">{formatDate(p.created_at)}</td>
						<td class="py-2 text-end">
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
		</div>
		{#if currentPage > 0}
			<p class="pt-2 text-xs text-[var(--color-muted)]">
				{t('settings.common.page', 'Page {n}').replace('{n}', String(currentPage))} · {payments.length}
			</p>
		{/if}
		{#if loadMoreError}
			<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{loadMoreError}</p>
		{/if}
		{#if hasMore}
			<button
				type="button"
				onclick={loadMore}
				disabled={loadingMore}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-hover)] disabled:opacity-50"
			>
				{loadingMore ? t('settings.common.loading', 'Loading…') : t('settings.common.load_more', 'Load more')}
			</button>
		{/if}
	{/if}

	<Modal open={selected !== null} onOpenChange={(v) => { if (!v) selected = null; }} title={t('settings.payments.detail_title')} closeLabel={t('settings.payments.close')} size="md">
				{#if detailLoading}
					<p class="text-sm text-[var(--color-muted)]">{t('settings.payments.loading')}</p>
				{:else if selected}
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
	</Modal>
</section>
