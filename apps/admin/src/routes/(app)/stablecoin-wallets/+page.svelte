<script lang="ts">
	// /stablecoin-wallets — §128/ADR-140 member stablecoin wallet registry.
	// Read-only ops surface: provisioning state, delegated-signer status,
	// chain/address. Server-paginated DataTable with status/q filters.
	import { t } from '#lib/stores/i18n.svelte.js';
	import { stablecoinApi } from '#lib/api.js';
	import PageHeader from '#lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '#lib/components/ui/card/index.js';
	import { DataTable } from '#lib/components/ui/data-table/index.js';
	import { Search } from 'lucide-svelte';
	import { Button } from '#lib/components/ui/button/index.js';

	type SignerScope = {
		asset: string;
		daily_cap_minor: number;
	};

	type StablecoinWallet = {
		id: string;
		user_id: string;
		chain: string;
		address: string;
		locator: string;
		status: string;
		recovery_methods?: string[];
		delegated_signer?: {
			signer_ref: string;
			scopes?: SignerScope[];
			registered_at: string;
		} | null;
		created_at: string;
		updated_at: string;
	};

	let wallets = $state<StablecoinWallet[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let page = $state(0);
	let pageSize = $state(20);
	let totalRows = $state(0);
	let totalPages = $state(1);

	let statusFilter = $state('');
	let query = $state('');

	async function loadWallets() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			params.set('offset', String(page * pageSize));
			if (statusFilter) params.set('status', statusFilter);
			if (query.trim()) params.set('q', query.trim());
			const res = await stablecoinApi.listWallets(params.toString());
			if (res.ok) {
				wallets = res.data ?? [];
				// { data: [...] } without a count — a short page signals the end.
				totalRows = page * pageSize + wallets.length + (wallets.length === pageSize ? 1 : 0);
				totalPages = Math.max(page + (wallets.length === pageSize ? 2 : 1), 1);
			} else {
				errorMessage = res.error ?? t('app.stablecoin.error_load', 'Failed to load wallets');
			}
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : t('app.stablecoin.error_load', 'Failed to load wallets');
		} finally {
			loading = false;
		}
	}

	function shortAddr(addr: string): string {
		if (!addr) return '—';
		return addr.length > 16 ? `${addr.slice(0, 8)}…${addr.slice(-6)}` : addr;
	}

	function statusBadge(status: string): string {
		const cls =
			status === 'active' ? 'bg-success/15 text-success border-success'
			: status === 'suspended' ? 'bg-danger/15 text-danger border-danger'
			: 'bg-warning/15 text-warning border-warning';
		return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold">${status}</span>`;
	}

	function signerSummary(w: StablecoinWallet): string {
		if (!w.delegated_signer) return '—';
		const scopes = w.delegated_signer.scopes ?? [];
		if (scopes.length === 0) return '—';
		return scopes
			.map((s) => `${s.asset.toUpperCase()} ≤ ${(s.daily_cap_minor / 1_000_000).toLocaleString()}/day`)
			.join(', ');
	}

	const columns = [
		{ key: 'user_id', label: t('app.stablecoin.col_user', 'User'), render: (v: string) => `<span class="font-mono text-xs">${shortAddr(v)}</span>` },
		{ key: 'chain', label: t('app.stablecoin.col_chain', 'Chain'), render: (v: string) => v },
		{ key: 'address', label: t('app.stablecoin.col_address', 'Wallet Address'), render: (v: string) => `<span class="font-mono text-xs">${shortAddr(v)}</span>` },
		{ key: 'status', label: t('app.stablecoin.col_status', 'Status'), render: (v: string) => statusBadge(v) },
		{
			key: 'delegated_signer',
			label: t('app.stablecoin.col_signer', 'Delegated Signer'),
			render: (_v: unknown, row: StablecoinWallet) =>
				row.delegated_signer
					? `<span class="text-xs">${signerSummary(row)}</span>`
					: `<span class="text-xs text-muted-foreground">${t('app.stablecoin.no_signer', 'not installed')}</span>`
		},
		{ key: 'created_at', label: t('app.stablecoin.col_created', 'Provisioned'), render: (v: string) => new Date(v).toLocaleDateString() }
	];

	loadWallets();
</script>

<svelte:head>
	<title>{t('app.stablecoin.title', 'Stablecoin Wallets')} — Admin</title>
</svelte:head>

<PageHeader
	title={t('app.stablecoin.title', 'Stablecoin Wallets')}
	description={t('app.stablecoin.desc', 'Member non-custodial wallet registry (Crossmint, Solana USDC/USDT)')}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}

	<Card>
		<CardContent>
			<div class="flex flex-wrap items-end gap-4">
				<div>
					<label class="mb-1 block text-sm font-medium" for="filter-status">{t('app.stablecoin.filter_status', 'Status')}</label>
					<select
						id="filter-status"
						bind:value={statusFilter}
						onchange={() => { page = 0; loadWallets(); }}
						class="w-40 rounded-md border border-input bg-surface px-3 py-2 text-sm"
					>
						<option value="">{t('common.all', 'All')}</option>
						<option value="provisioned">{t('app.stablecoin.status_provisioned', 'Provisioned')}</option>
						<option value="active">{t('app.stablecoin.status_active', 'Active')}</option>
						<option value="suspended">{t('app.stablecoin.status_suspended', 'Suspended')}</option>
					</select>
				</div>
				<div>
					<label class="mb-1 block text-sm font-medium" for="filter-q">{t('app.stablecoin.filter_q', 'Search')}</label>
					<input
						id="filter-q"
						type="text"
						bind:value={query}
						onkeydown={(e) => { if (e.key === 'Enter') { page = 0; loadWallets(); } }}
						class="w-64 rounded-md border border-input bg-surface px-3 py-2 text-sm"
						placeholder={t('app.stablecoin.q_ph', 'User ID or wallet address…')}
					/>
				</div>
				<Button onclick={() => { page = 0; loadWallets(); }}>
					<Search class="size-4" />
					{t('common.filter', 'Filter')}
				</Button>
			</div>
		</CardContent>
	</Card>

	<Card>
		<CardContent>
			<DataTable
				{columns}
				data={wallets}
				serverSide
				bind:page
				bind:pageSize
				{totalRows}
				{totalPages}
				pageSizeOptions={[10, 20, 50]}
				onpagechange={(p) => { page = p; loadWallets(); }}
				onpagesizechange={() => { if (page === 0) loadWallets(); else page = 0; }}
				{loading}
				emptyMessage={t('app.stablecoin.empty', 'No member wallets provisioned')}
			/>
		</CardContent>
	</Card>
</div>
