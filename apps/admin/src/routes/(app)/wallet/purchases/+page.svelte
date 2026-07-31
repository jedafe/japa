<script lang="ts">
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	import { t } from '$lib/stores/i18n.svelte';
	import { walletApi } from '$lib/api';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Search } from 'lucide-svelte';

	type CreditPurchase = {
		id: string;
		user_id: string;
		provider: string;
		amount: number;
		currency: string;
		status: string;
		created_at: string;
	};

	let purchases = $state<CreditPurchase[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let filters = $state({
		user_id: '',
		currency: '',
		from: '',
		to: ''
	});

	async function loadPurchases() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (filters.user_id) params.set('user_id', filters.user_id);
			if (filters.currency) params.set('currency', filters.currency);
			if (filters.from) params.set('from', filters.from);
			if (filters.to) params.set('to', filters.to);
			const res = await walletApi.listCreditPurchases(params.toString());
			purchases = (res as { data: CreditPurchase[] }).data ?? [];
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : 'Failed to load purchases';
		} finally {
			loading = false;
		}
	}

	function applyFilters() {
		loadPurchases();
	}

	function formatAmount(minor: number, currency: string): string {
		const major = minor / 100;
		return `${major.toFixed(2)} ${currency}`;
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	const purchaseColumns = [
		{ key: 'created_at', label: 'Date', sortable: true,
			render: (v: string) => formatDate(v) },
		{ key: 'user_id', label: t('app.wallet.filter_user', 'User ID'),
			render: (v: string) => `<span class="font-mono text-xs">${v.slice(0, 8)}...</span>` },
		{ key: 'amount', label: t('app.wallet.fiat_amount', 'Fiat Amount'), sortable: true,
			render: (_: number, row: CreditPurchase) => formatAmount(row.amount, row.currency) },
		{ key: 'currency', label: t('app.wallet.filter_currency', 'Currency'),
			render: (v: string) => v },
		{ key: 'provider', label: t('app.wallet.provider', 'Provider'),
			render: (v: string) => v },
		{ key: 'status', label: t('app.wallet.purchase_status', 'Status'),
			render: (v: string) => {
				const cls =
					v === 'succeeded'
						? 'bg-success/15 text-success border-success'
						: v === 'failed'
							? 'bg-danger/15 text-danger border-danger'
							: 'bg-surface text-foreground border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${v}</span>`;
			} }
	];

	loadPurchases();
</script>

<svelte:head>
	<title>{t('app.wallet.purchases_title', 'Credit Purchase History')} — Admin</title>
</svelte:head>

<PageHeader
	title={t('app.wallet.purchases_title', 'Credit Purchase History')}
	description={t('app.wallet.purchases_desc', 'View all credit purchase transactions')}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}

	<!-- Filters -->
	<Card>
		<CardContent>
			<div class="flex flex-wrap items-end gap-4">
				<div>
					<label class="mb-1 block text-sm font-medium" for="filter-user">
						{t('app.wallet.filter_user', 'User ID')}
					</label>
					<input
						id="filter-user"
						type="text"
						bind:value={filters.user_id}
						class="w-48 rounded-md border border-input bg-surface px-3 py-2 text-sm"
						placeholder="UUID..."
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="filter-currency">
						{t('app.wallet.filter_currency', 'Currency')}
					</label>
					<input
						id="filter-currency"
						type="text"
						bind:value={filters.currency}
						class="w-24 rounded-md border border-input bg-surface px-3 py-2 text-sm"
						placeholder="USD"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="filter-from">
						{t('app.wallet.filter_date_from', 'From Date')}
					</label>
					<input
						id="filter-from"
						type="date"
						bind:value={filters.from}
						class="rounded-md border border-input bg-surface px-3 py-2 text-sm"
					/>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="filter-to">
						{t('app.wallet.filter_date_to', 'To Date')}
					</label>
					<input
						id="filter-to"
						type="date"
						bind:value={filters.to}
						class="rounded-md border border-input bg-surface px-3 py-2 text-sm"
					/>
				</div>
				<Button onclick={applyFilters}>
					<Search class="size-4" />
					Filter
				</Button>
			</div>
		</CardContent>
	</Card>

	<Card>
		<CardContent>
			{#if loading}
				<div class="flex items-center justify-center py-12 text-muted">
					Loading...
				</div>
			{:else if purchases.length === 0}
				<div class="flex flex-col items-center justify-center py-12 text-muted">
					<p>No credit purchases found.</p>
				</div>
			{:else}
				<DataTable columns={purchaseColumns} data={purchases} pageSize={20} searchable
					emptyMessage="No credit purchases found" />
			{/if}
		</CardContent>
	</Card>
</div>
