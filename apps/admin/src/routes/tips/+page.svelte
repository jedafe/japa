<script lang="ts">
	// /tips — §125 admin tips ledger. Read-only list with filters, 30-day
	// stats strip, and a per-row Refund action (tips.moderate).
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '$lib/components/entity';
	import { tipsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RotateCcw } from 'lucide-svelte';

	type TipRow = {
		id: string;
		sender_name?: string;
		recipient_type: string;
		method: string;
		points_amount?: number;
		credits_amount?: number;
		fiat_amount?: number | null;
		fiat_currency?: string | null;
		net_credits?: number;
		status: string;
		is_anonymous?: boolean;
		created_at: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let stats = $state<{ total?: number; credits_settled?: number } | null>(null);
	let refunding = $state<string | null>(null);
	let pageError = $state('');

	onMount(async () => {
		const res = await tipsAdminApi.stats(30);
		if (res.ok && res.data) stats = res.data as typeof stats;
	});

	function amount(r: TipRow): string {
		if (r.method === 'points') return `${r.points_amount ?? 0} pts`;
		if (r.method === 'credits') return `${r.credits_amount ?? 0} cr`;
		if (r.fiat_amount != null) return `${(r.fiat_amount / 100).toFixed(2)} ${r.fiat_currency ?? ''}`;
		return '—';
	}

	async function refund(row: TipRow) {
		if (refunding) return;
		refunding = row.id;
		pageError = '';
		const res = await tipsAdminApi.refund(row.id);
		refunding = null;
		if (!res.ok) {
			pageError = res.error ?? t('tips.refund_failed', 'Refund failed');
			return;
		}
		entityController?.reload();
		const s = await tipsAdminApi.stats(30);
		if (s.ok && s.data) stats = s.data as typeof stats;
	}

	const config: EntityPageConfig = {
		title: t('tips.title', 'Tips & Donations'),
		description: t('tips.description', 'All tips and donations across the platform.'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'created_at',
				label: t('tips.date', 'Date'),
				render: (v: string) => `<span class="text-xs">${new Date(v).toLocaleString()}</span>`
			},
			{
				key: 'sender_name',
				label: t('tips.sender', 'Sender'),
				render: (v: string, row: TipRow) => (row.is_anonymous ? `<em>${t('tips.anonymous', 'Someone')}</em>` : v || '—')
			},
			{ key: 'recipient_type', label: t('tips.recipient', 'Recipient') },
			{
				key: 'amount',
				label: t('tips.amount', 'Amount'),
				render: (_v: unknown, row: TipRow) => amount(row)
			},
			{ key: 'method', label: t('tips.method', 'Method') },
			{
				key: 'status',
				label: t('tips.status', 'Status'),
				render: (v: string) =>
					`<span class="rounded px-1.5 py-0.5 text-xs ${v === 'completed' ? 'bg-green-500/10 text-green-600' : v === 'refunded' ? 'bg-red-500/10 text-red-600' : 'bg-amber-500/10 text-amber-600'}">${v}</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('tips.status', 'Status'),
				type: 'select',
				options: [
					{ value: 'pending', label: 'Pending' },
					{ value: 'completed', label: 'Completed' },
					{ value: 'failed', label: 'Failed' },
					{ value: 'refunded', label: 'Refunded' }
				]
			},
			{
				key: 'method',
				label: t('tips.method', 'Method'),
				type: 'select',
				options: [
					{ value: 'points', label: 'Points' },
					{ value: 'credits', label: 'Credits' },
					{ value: 'stripe', label: 'Stripe' },
					{ value: 'paystack', label: 'Paystack' },
					{ value: 'nowpayments', label: 'NOWPayments' },
					{ value: 'manual_transfer', label: 'Manual transfer' },
					{ value: 'opencollective', label: 'Open Collective' }
				]
			}
		],
		actions: [
			{
				label: t('tips.refund', 'Refund'),
				icon: RotateCcw,
				variant: 'destructive',
				show: (row: TipRow) => row.status === 'completed',
				onclick: (row: TipRow) => refund(row)
			}
		],
		fields: [],
		load: async ({ page, pageSize, filter }) => {
			const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
			if (filter?.status) params.set('status', String(filter.status));
			if (filter?.method) params.set('method', String(filter.method));
			const res = await tipsAdminApi.list(params.toString());
			if (!res.ok) return { data: [], total: 0, totalPages: 0 };
			const env = res.data as { data?: TipRow[]; meta?: { total?: number } };
			const rows = env?.data ?? (Array.isArray(env) ? env : []);
			const total = env?.meta?.total ?? rows.length;
			return { data: rows, total, totalPages: Math.ceil(total / pageSize) };
		}
	};
</script>

{#if pageError}
	<div class="mb-4 rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-600">{pageError}</div>
{/if}

{#if stats}
	<div class="mb-4 flex gap-6 rounded-lg border border-border px-4 py-3 text-sm">
		<span><strong>{stats.total ?? 0}</strong> {t('tips.stats_30d', 'tips (30d)')}</span>
		<span><strong>{stats.credits_settled ?? 0}</strong> {t('tips.stats_credits', 'credits settled')}</span>
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />
