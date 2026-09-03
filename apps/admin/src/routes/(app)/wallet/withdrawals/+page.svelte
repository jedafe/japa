<script lang="ts">
	// /wallet/withdrawals — admin withdrawal management.
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import type { ActionItem } from '$lib/components/ui/data-table/types';
	import { walletApi, api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Check, Eye, X } from 'lucide-svelte';

	type Withdrawal = {
		id: string;
		user_id?: string;
		credit_amount: number;
		currency_code: string;
		credits_per_unit: number;
		fiat_amount: number;
		fee_amount: number;
		net_amount: number;
		status: string;
		payout_provider?: string | null;
		payout_method_id?: string | null;
		provider_ref?: string | null;
		admin_note?: string | null;
		created_at?: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let processing = $state(false);
	let currencies = $state<Array<{ code: string; symbol: string }>>([]);
	let withdrawEnabled = $state<boolean | null>(null);

	$effect(() => {
		walletApi.getCurrencies().then((res) => {
			if (res.ok && Array.isArray(res.data)) currencies = res.data;
		});
		api<Record<string, boolean>>('/feature-flags').then((res) => {
			if (res.ok) {
				withdrawEnabled = res.data['feature.withdraw'] === true;
			} else {
				withdrawEnabled = false;
			}
		});
	});

	function statusClass(status: string): string {
		switch (status) {
			case 'completed':
				return 'bg-success/15 text-success border-success';
			case 'failed':
			case 'rejected':
			case 'cancelled':
				return 'bg-danger/15 text-danger border-danger';
			case 'pending_approval':
			case 'pending':
				return 'bg-warning/15 text-warning border-warning';
			case 'approved':
			case 'processing':
				return 'bg-primary/15 text-primary border-primary';
			default:
				return 'bg-surface text-foreground border';
		}
	}

	function formatFiat(minor: number, code: string): string {
		const c = currencies.find((x) => x.code === code);
		return `${c?.symbol ?? ''}${(minor / 100).toFixed(2)} ${code}`;
	}

	async function processAction(row: Withdrawal, action: string) {
		if (processing) return;
		processing = true;
		pageError = '';
		const res = await walletApi.processWithdrawal(row.id, { action });
		processing = false;
		if (res.ok) {
			entityController?.reload();
		} else {
			pageError = res.error ?? t('app.wallet.error_process', 'Failed to process withdrawal');
		}
	}

	const viewAction: ActionItem = {
		label: t('common.view', 'View'),
		icon: Eye,
		onclick: (_row: any, ctx: any) => ctx?.openView?.(_row)
	};

	const approveAction: ActionItem = {
		label: t('app.wallet.approve', 'Approve & Payout'),
		icon: Check,
		variant: 'default',
		show: (row: Withdrawal) => row.status === 'pending_approval' || row.status === 'pending',
		onclick: (row: Withdrawal) => processAction(row, 'approve')
	};

	const rejectAction: ActionItem = {
		label: t('app.wallet.reject', 'Reject & Refund'),
		icon: X,
		variant: 'destructive',
		show: (row: Withdrawal) => row.status === 'pending_approval' || row.status === 'pending',
		onclick: (row: Withdrawal) => processAction(row, 'reject')
	};

	const fields: FieldDef[] = [
		{ key: 'admin_note', label: t('app.wallet.admin_note', 'Admin Note'), type: 'textarea' }
	];

	const config: EntityPageConfig = {
		title: t('app.wallet.withdrawals_title', 'Withdrawal Management'),
		description: t('app.wallet.withdrawals_desc', 'Review and process user credit withdrawal requests'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'id', label: t('common.id', 'ID'), render: (v: string) => `<span class="font-mono text-xs">${(v ?? '').slice(0, 8)}...</span>` },
			{ key: 'credit_amount', label: t('app.wallet.credits', 'Credits'), render: (v: number) => (v ?? 0).toLocaleString() },
			{ key: 'currency_code', label: t('app.wallet.currency', 'Currency') },
			{ key: 'fiat_amount', label: t('app.wallet.fiat_amount', 'Fiat Amount'), render: (_: unknown, row: Withdrawal) => formatFiat(row.fiat_amount, row.currency_code) },
			{ key: 'fee_amount', label: t('app.wallet.fee', 'Fee'), render: (_: unknown, row: Withdrawal) => formatFiat(row.fee_amount, row.currency_code) },
			{ key: 'net_amount', label: t('app.wallet.net_amount', 'Net Amount'), render: (_: unknown, row: Withdrawal) => formatFiat(row.net_amount, row.currency_code) },
			{
				key: 'status',
				label: t('common.status', 'Status'),
				render: (_: unknown, row: Withdrawal) => {
					const label = row.status;
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${statusClass(row.status)}">${label}</span>`;
				}
			},
			{ key: 'payout_provider', label: t('app.wallet.payout_provider', 'Provider'), render: (v: string | null) => v ?? '—' },
			{ key: 'created_at', label: t('common.created', 'Created'), render: (v: string) => (v ? new Date(v).toLocaleString() : '—') }
		],
		filters: [
			{
				key: 'status',
				label: t('common.status', 'Status'),
				options: [
					{ value: 'pending', label: 'Pending' },
					{ value: 'pending_approval', label: 'Pending Approval' },
					{ value: 'approved', label: 'Approved' },
					{ value: 'processing', label: 'Processing' },
					{ value: 'completed', label: 'Completed' },
					{ value: 'failed', label: 'Failed' },
					{ value: 'rejected', label: 'Rejected' },
					{ value: 'cancelled', label: 'Cancelled' }
				]
			}
		],
		actions: [viewAction, approveAction, rejectAction],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(page * pageSize));
			if (filter.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			const res = await walletApi.listWithdrawals(params.toString());
			if (res.ok) {
				const payload = res.data as { data: Withdrawal[]; count: number };
				const data = payload.data ?? [];
				const total = payload.count ?? data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				return { data, total, totalPages };
			}
			throw new Error(res.error ?? t('app.wallet.error_load_withdrawals', 'Failed to load withdrawals'));
		},
		viewSnippet: WithdrawalView
	};
</script>

<svelte:head>
	<title>{t('app.wallet.withdrawals_title', 'Withdrawal Management')} — Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

{#if withdrawEnabled === null}
	<div class="flex items-center justify-center py-20 text-sm" style="color: var(--color-muted)">
		{t('app.wallet.loading_feature', 'Loading feature status…')}
	</div>
{:else if !withdrawEnabled}
	<div class="rounded-lg p-4 text-sm" style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)">
		{t('app.wallet.withdraw_feature_disabled', 'Credit withdrawals are currently disabled. Enable the feature.withdraw flag to manage withdrawals.')}
	</div>
{:else}
	<EntityPage {config} bind:controller={entityController} />
{/if}

{#snippet WithdrawalView({ row }: { row: Withdrawal })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.id', 'ID')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.id}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.user_id', 'User ID')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.user_id ?? '—'}</p>
			</div>
		</div>
		<div class="grid grid-cols-3 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.credits', 'Credits')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.credit_amount.toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.rate', 'Rate')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">1 {row.currency_code} = {row.credits_per_unit.toLocaleString()} {t('app.wallet.credits', 'credits')}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
		</div>
		<div class="grid grid-cols-3 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.fiat_amount', 'Fiat Amount')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatFiat(row.fiat_amount, row.currency_code)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.fee', 'Fee')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatFiat(row.fee_amount, row.currency_code)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.net_amount', 'Net Amount')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatFiat(row.net_amount, row.currency_code)}</p>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.payout_provider', 'Payout Provider')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.payout_provider ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.provider_ref', 'Provider Reference')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.provider_ref ?? '—'}</p>
			</div>
		</div>
		{#if row.admin_note}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.admin_note', 'Admin Note')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.admin_note}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created', 'Created')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
