<script lang="ts">
	// /tips — §125 admin tips ledger. Read-only list with filters, 30-day
	// stats strip, and a per-row Refund action (tips.moderate).
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '#lib/components/entity/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { tipsAdminApi } from '#lib/api.js';
	import { confirmDialog } from '@japaconnect/ui';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { RotateCcw } from 'lucide-svelte';

	type TipRow = {
		id: string;
		sender_name?: string;
		external_sender_name?: string | null;
		recipient_type: string;
		recipient_name?: string;
		method: string;
		credits_amount?: number;
		points_amount?: number;
		fiat_amount?: number | null;
		fiat_currency?: string | null;
		net_credits?: number;
		status: string;
		is_anonymous?: boolean;
		created_at: string;
	};

	type TipDetail = TipRow & Record<string, unknown>;

	let entityController = $state<EntityPageController | undefined>(undefined);
	let stats = $state<{ total?: number; credits_settled?: number; fiat_minor_settled?: number; oc_count?: number; fiat_by_currency?: Record<string, number> } | null>(null);
	let refunding = $state<string | null>(null);
	let pageError = $state('');
	let detailOpen = $state(false);
	let detail = $state<TipDetail | null>(null);
	let detailLoading = $state(false);

	onMount(async () => {
		const res = await tipsAdminApi.stats(30);
		if (res.ok && res.data) stats = res.data as typeof stats;
	});

	// ISO-4217 minor-unit exponent — mirrors internal/tips currencyExponent.
	function fiatExponent(currency: string | null | undefined): number {
		if (['JPY', 'KRW', 'VND', 'XOF', 'XAF'].includes(currency ?? '')) return 0;
		if (['BHD', 'JOD', 'KWD', 'OMR'].includes(currency ?? '')) return 3;
		return 2;
	}

	function fiatDisplay(minor: number, currency: string | null | undefined): string {
		const exp = fiatExponent(currency);
		return `${(minor / Math.pow(10, exp)).toFixed(exp)} ${currency ?? ''}`.trim();
	}

	function amount(r: Pick<TipRow, 'method' | 'credits_amount' | 'points_amount' | 'fiat_amount' | 'fiat_currency'>): string {
		if (r.method === 'credits') return `${r.credits_amount ?? 0} cr`;
		// ADR-136: historical points tips carry their amount in
		// tips_points_legacy — surfaced as points_amount by the API.
		if (r.method === 'points') return r.points_amount != null ? `${r.points_amount} pts` : '—';
		if (r.fiat_amount != null) return fiatDisplay(r.fiat_amount, r.fiat_currency);
		return '—';
	}

	async function refund(row: TipRow) {
		if (refunding) return;
		// Fiat tips now refund the donor provider-side too — confirm the
		// destructive financial action explicitly.
		if (!(await confirmDialog(t('tips.refund_confirm', 'Refund this tip? The recipient credits are clawed back and any fiat payment is refunded.')))) return;
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

	async function openDetail(id: string) {
		detailOpen = true;
		detail = null;
		detailLoading = true;
		const res = await tipsAdminApi.get(id);
		detailLoading = false;
		if (!res.ok) {
			pageError = res.error ?? t('tips.detail_failed', 'Failed to load tip');
			detailOpen = false;
			return;
		}
		const env = res.data as { data?: TipDetail };
		detail = (env?.data ?? (res.data as TipDetail)) ?? null;
	}

	// Curated detail rows — sender/recipient resolve to usernames (and org or
	// space names), never raw UUIDs. IDs of related objects stay visible for
	// audit tracing.
	function detailFields(d: TipDetail): { label: string; value: string }[] {
		const str = (k: string) => {
			const v = d[k];
			return v === null || v === undefined || v === '' ? '' : String(v);
		};
		const sender = str('sender_name') || str('external_sender_name');
		return [
			{ label: t('tips.field.id', 'Tip ID'), value: str('id') },
			{
				label: t('tips.sender', 'Sender'),
				value:
					(sender || '—') +
					(d.is_anonymous ? ` (${t('tips.anon_flag', 'anonymous')})` : '')
			},
			{ label: t('tips.recipient', 'Recipient'), value: str('recipient_name') || str('recipient_type') || '—' },
			{ label: t('tips.recipient_type', 'Recipient type'), value: str('recipient_type') || '—' },
			{ label: t('tips.method', 'Method'), value: str('method') || '—' },
			{ label: t('tips.amount', 'Amount'), value: amount(d) },
			{ label: t('tips.field.fee', 'Fee (credits)'), value: str('fee_amount') || '0' },
			{ label: t('tips.field.net', 'Net credits'), value: str('net_credits') || '0' },
			{ label: t('tips.status', 'Status'), value: str('status') || '—' },
			{
				label: t('tips.field.anonymous', 'Anonymous'),
				value: d.is_anonymous ? t('common.yes', 'Yes') : t('common.no', 'No')
			},
			{ label: t('tips.field.message', 'Message'), value: str('message') || '—' },
			{ label: t('tips.field.content', 'Content'), value: str('content_id') || '—' },
			{ label: t('tips.field.payment', 'Payment'), value: str('payment_id') || '—' },
			{ label: t('tips.field.oc', 'Open Collective contribution'), value: str('oc_contribution_id') || '—' },
			{
				label: t('tips.field.created', 'Created'),
				value: str('created_at') ? new Date(str('created_at')).toLocaleString() : '—'
			},
			{
				label: t('tips.field.updated', 'Updated'),
				value: str('updated_at') ? new Date(str('updated_at')).toLocaleString() : '—'
			}
		];
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
				// §125: admins always see the real sender — anonymity only
				// masks public surfaces. The flag is shown as a marker.
				render: (v: string, row: TipRow) =>
					(v || row.external_sender_name || '—') + (row.is_anonymous ? ` <em class="text-muted-foreground">(${t('tips.anon_flag', 'anonymous')})</em>` : '')
			},
			{ key: 'recipient_name', label: t('tips.recipient', 'Recipient'), render: (v: string, row: TipRow) => v || row.recipient_type },
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
					{ value: 'pending', label: t('tips.status.pending', 'Pending') },
					{ value: 'completed', label: t('tips.status.completed', 'Completed') },
					{ value: 'failed', label: t('tips.status.failed', 'Failed') },
					{ value: 'refunded', label: t('tips.status.refunded', 'Refunded') }
				]
			},
			{
				key: 'recipient_type',
				label: t('tips.recipient_type', 'Recipient type'),
				type: 'select',
				options: [
					{ value: 'platform', label: t('tips.recipient.platform', 'Platform') },
					{ value: 'user', label: t('tips.recipient.user', 'User') },
					{ value: 'organization', label: t('tips.recipient.organization', 'Organization') },
					{ value: 'space', label: t('tips.recipient.space', 'Space') }
				]
			},
			{
				key: 'method',
				label: t('tips.method', 'Method'),
				type: 'select',
				options: [
					{ value: 'points', label: t('tips.method.points', 'Points') },
					{ value: 'credits', label: t('tips.method.credits', 'Credits') },
					{ value: 'stripe', label: 'Stripe' },
					{ value: 'paystack', label: 'Paystack' },
					{ value: 'nowpayments', label: 'NOWPayments' },
					{ value: 'manual_transfer', label: t('tips.method.manual_transfer', 'Manual transfer') },
					{ value: 'opencollective', label: 'Open Collective' }
				]
			}
		],
		actions: [
			{
				label: t('tips.view', 'View'),
				onclick: (row: TipRow) => openDetail(row.id)
			},
			{
				label: t('tips.refund', 'Refund'),
				icon: RotateCcw,
				variant: 'destructive',
				show: (row: TipRow) => row.status === 'completed',
				onclick: (row: TipRow) => refund(row)
			}
		],
		fields: [],
		load: async ({ page, pageSize, filter, q }) => {
			const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
			if (q) params.set('q', String(q));
			if (filter?.status) params.set('status', String(filter.status));
			if (filter?.method) params.set('method', String(filter.method));
			if (filter?.recipient_type) params.set('recipient_type', String(filter.recipient_type));
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
	<div class="mb-4 flex flex-wrap gap-6 rounded-lg border border-border px-4 py-3 text-sm">
		<span><strong>{stats.total ?? 0}</strong> {t('tips.stats_30d', 'tips (30d)')}</span>
		<span><strong>{stats.credits_settled ?? 0}</strong> {t('tips.stats_credits', 'credits settled')}</span>
		<!-- Fiat minors can't be summed across currencies — show each
		     currency formatted at its own exponent. -->
		{#each Object.entries(stats.fiat_by_currency ?? {}) as [cur, minor]}
			<span><strong>{fiatDisplay(minor, cur)}</strong> {t('tips.stats_fiat', 'fiat settled')}</span>
		{/each}
		<span><strong>{stats.oc_count ?? 0}</strong> {t('tips.stats_oc', 'via Open Collective')}</span>
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<Modal bind:open={detailOpen} title={t('tips.detail_title', 'Tip detail')}>
	{#if detailLoading}
		<p class="text-sm text-muted-foreground">{t('common.loading', 'Loading…')}</p>
	{:else if detail}
		<dl class="space-y-2 text-sm">
			{#each detailFields(detail) as f}
				<div class="flex justify-between flex-wrap gap-4 border-b border-border/50 pb-1">
					<dt class="shrink-0 font-medium text-muted-foreground">{f.label}</dt>
					<dd class="min-w-0 break-all text-right">{f.value}</dd>
				</div>
			{/each}
		</dl>
	{/if}
</Modal>
