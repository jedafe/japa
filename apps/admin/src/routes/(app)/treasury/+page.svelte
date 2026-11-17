<script lang="ts">
	// /treasury — §128/ADR-140 platform treasury surface: hot/cold/revenue
	// wallet registry, Squads sweep proposals (approve/reject), and the
	// delegated-signer killswitch. Lists are server-paginated DataTables;
	// state actions go through confirm modals. Not an EntityPage — no CRUD.
	// Ref: japaconnect-engineering/sections/128 (treasury + killswitch)
	import { t } from '#lib/stores/i18n.svelte.js';
	import { treasuryApi, stablecoinApi } from '#lib/api.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import PageHeader from '#lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '#lib/components/ui/card/index.js';
	import { DataTable } from '#lib/components/ui/data-table/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { AlertTriangle, ShieldAlert } from 'lucide-svelte';

	type TreasuryWallet = {
		id: string;
		role: string;
		chain: string;
		address: string;
		locator?: string;
		float_cap_minor: number;
	};

	type TreasurySweep = {
		id: string;
		chain: string;
		asset: string;
		amount_minor: number;
		hot_wallet_id: string;
		cold_address: string;
		state: string;
		proposed_by: string;
		approved_by: string;
		squads_proposal_id: string;
		executed_sig: string;
	};

	const canDecide = $derived(hasPermission('treasury.sweep_approve'));
	const canKillswitch = $derived(hasPermission('treasury.killswitch'));

	let treasuryWallets = $state<TreasuryWallet[]>([]);
	let killswitchEngaged = $state(false);
	let killswitchBusy = $state(false);
	let killswitchConfirm = $state(false);
	let loading = $state(true);
	let errorMessage = $state('');
	let actionMessage = $state('');

	let sweeps = $state<TreasurySweep[]>([]);
	let sweepsLoading = $state(true);
	let page = $state(0);
	let pageSize = $state(20);
	let totalRows = $state(0);
	let totalPages = $state(1);
	let stateFilter = $state('');

	// Approve/reject modal state.
	let decisionSweep = $state<TreasurySweep | null>(null);
	let decisionModalOpen = $state(false);
	let decision = $state<'approve' | 'reject'>('approve');
	let squadsProposalId = $state('');
	let decisionBusy = $state(false);

	async function loadTreasury() {
		loading = true;
		errorMessage = '';
		try {
			const res = await treasuryApi.getTreasury();
			if (res.ok) {
				treasuryWallets = res.data?.wallets ?? [];
				killswitchEngaged = res.data?.killswitch_engaged ?? false;
			} else {
				errorMessage = res.error ?? t('app.treasury.error_load', 'Failed to load treasury wallets');
			}
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : t('app.treasury.error_load', 'Failed to load treasury wallets');
		} finally {
			loading = false;
		}
	}

	async function loadSweeps() {
		sweepsLoading = true;
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			params.set('offset', String(page * pageSize));
			if (stateFilter) params.set('state', stateFilter);
			const res = await treasuryApi.listSweeps(params.toString());
			if (res.ok) {
				sweeps = res.data ?? [];
				// The list endpoint returns { data: [...] } without a count —
				// a short page signals the end of the result set.
				totalRows = page * pageSize + sweeps.length + (sweeps.length === pageSize ? 1 : 0);
				totalPages = Math.max(page + (sweeps.length === pageSize ? 2 : 1), 1);
			} else {
				errorMessage = res.error ?? t('app.treasury.error_load_sweeps', 'Failed to load sweeps');
			}
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : t('app.treasury.error_load_sweeps', 'Failed to load sweeps');
		} finally {
			sweepsLoading = false;
		}
	}

	async function toggleKillswitch() {
		killswitchBusy = true;
		actionMessage = '';
		try {
			const res = await stablecoinApi.setKillswitch(!killswitchEngaged);
			if (res.ok) {
				killswitchEngaged = res.data?.engaged ?? !killswitchEngaged;
				killswitchConfirm = false;
			} else {
				actionMessage = res.error ?? t('app.treasury.error_killswitch', 'Failed to update killswitch');
			}
		} catch (e) {
			actionMessage = e instanceof Error ? e.message : t('app.treasury.error_killswitch', 'Failed to update killswitch');
		} finally {
			killswitchBusy = false;
		}
	}

	function openDecision(sweep: TreasurySweep, d: 'approve' | 'reject') {
		decisionSweep = sweep;
		decision = d;
		squadsProposalId = '';
		decisionModalOpen = true;
	}

	async function submitDecision() {
		if (!decisionSweep) return;
		decisionBusy = true;
		actionMessage = '';
		try {
			const body: { decision: 'approve' | 'reject'; squads_proposal_id?: string } = { decision };
			if (decision === 'approve' && squadsProposalId.trim()) {
				body.squads_proposal_id = squadsProposalId.trim();
			}
			const res = await treasuryApi.decideSweep(decisionSweep.id, body);
			if (res.ok) {
				decisionModalOpen = false;
				decisionSweep = null;
				loadSweeps();
			} else {
				actionMessage = res.error ?? t('app.treasury.error_decide', 'Failed to record decision');
			}
		} catch (e) {
			actionMessage = e instanceof Error ? e.message : t('app.treasury.error_decide', 'Failed to record decision');
		} finally {
			decisionBusy = false;
		}
	}

	function formatMinor(minor: number, asset: string): string {
		return `${(minor / 1_000_000).toLocaleString(undefined, { maximumFractionDigits: 6 })} ${asset?.toUpperCase() ?? ''}`;
	}

	function shortAddr(addr: string): string {
		if (!addr) return '—';
		return addr.length > 16 ? `${addr.slice(0, 8)}…${addr.slice(-6)}` : addr;
	}

	function stateBadge(state: string): string {
		const cls =
			state === 'executed' ? 'bg-success/15 text-success border-success'
			: state === 'rejected' || state === 'expired' ? 'bg-danger/15 text-danger border-danger'
			: state === 'approved' ? 'bg-info/15 text-info border-info'
			: 'bg-warning/15 text-warning border-warning';
		return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${state}</span>`;
	}

	const walletColumns = [
		{ key: 'role', label: t('app.treasury.col_role', 'Role'), render: (v: string) => `<span class="font-medium capitalize">${v}</span>` },
		{ key: 'chain', label: t('app.treasury.col_chain', 'Chain'), render: (v: string) => v },
		{ key: 'address', label: t('app.treasury.col_address', 'Address'), render: (v: string) => `<span class="font-mono text-xs">${shortAddr(v)}</span>` },
		{
			key: 'float_cap_minor',
			label: t('app.treasury.col_float_cap', 'Float Cap'),
			render: (v: number) => (v > 0 ? formatMinor(v, 'usdc') : '—')
		}
	];

	const sweepColumns = $derived([
		{ key: 'asset', label: t('app.treasury.col_asset', 'Asset'), render: (_v: string, row: TreasurySweep) => formatMinor(row.amount_minor, row.asset) },
		{ key: 'chain', label: t('app.treasury.col_chain', 'Chain'), render: (v: string) => v },
		{ key: 'cold_address', label: t('app.treasury.col_destination', 'Cold Vault'), render: (v: string) => `<span class="font-mono text-xs">${shortAddr(v)}</span>` },
		{ key: 'state', label: t('app.treasury.col_state', 'State'), render: (v: string) => stateBadge(v) },
		{
			key: 'squads_proposal_id',
			label: t('app.treasury.col_squads', 'Squads Proposal'),
			render: (v: string) => (v ? `<span class="font-mono text-xs">${shortAddr(v)}</span>` : '—')
		},
		{
			key: 'executed_sig',
			label: t('app.treasury.col_exec_sig', 'Executed Sig'),
			render: (v: string) => (v ? `<span class="font-mono text-xs">${shortAddr(v)}</span>` : '—')
		}
	]);

	const sweepActions = $derived(
		canDecide
			? [
					{
						label: t('app.treasury.action_approve', 'Approve'),
						onclick: (row: TreasurySweep) => openDecision(row, 'approve'),
						show: (row: TreasurySweep) => row.state === 'proposed'
					},
					{
						label: t('app.treasury.action_reject', 'Reject'),
						onclick: (row: TreasurySweep) => openDecision(row, 'reject'),
						variant: 'destructive' as const,
						show: (row: TreasurySweep) => row.state === 'proposed'
					}
				]
			: []
	);

	loadTreasury();
	loadSweeps();
</script>

<svelte:head>
	<title>{t('app.treasury.title', 'Treasury')} — Admin</title>
</svelte:head>

<PageHeader
	title={t('app.treasury.title', 'Treasury')}
	description={t('app.treasury.desc', 'Platform stablecoin treasury wallets, sweep proposals, and the delegated-signer killswitch')}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if actionMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{actionMessage}
		</div>
	{/if}

	<!-- Killswitch — halts every delegated-signer money movement platform-wide. -->
	<Card>
		<CardContent>
			<div class="flex flex-wrap items-center justify-between gap-4">
				<div class="flex items-center gap-3">
					<ShieldAlert class={killswitchEngaged ? 'size-5 text-danger' : 'size-5 text-muted-foreground'} />
					<div>
						<p class="text-sm font-medium">{t('app.treasury.killswitch', 'Delegated-signer killswitch')}</p>
						<p class="text-xs text-muted-foreground">
							{killswitchEngaged
								? t('app.treasury.killswitch_on', 'ENGAGED — all delegated-signer operations are halted')
								: t('app.treasury.killswitch_off', 'Disengaged — delegated operations are live')}
						</p>
					</div>
				</div>
				{#if canKillswitch}
					<Button
						variant={killswitchEngaged ? 'default' : 'destructive'}
						onclick={() => (killswitchConfirm = true)}
						disabled={loading}
					>
						{killswitchEngaged
							? t('app.treasury.killswitch_disengage', 'Disengage')
							: t('app.treasury.killswitch_engage', 'Engage killswitch')}
					</Button>
				{/if}
			</div>
		</CardContent>
	</Card>

	<Card>
		<CardContent>
			<h2 class="mb-4 text-sm font-semibold">{t('app.treasury.wallets', 'Treasury wallets')}</h2>
			<DataTable
				columns={walletColumns}
				data={treasuryWallets}
				{loading}
				emptyMessage={t('app.treasury.empty_wallets', 'No treasury wallets registered')}
			/>
		</CardContent>
	</Card>

	<Card>
		<CardContent>
			<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
				<h2 class="text-sm font-semibold">{t('app.treasury.sweeps', 'Sweep proposals')}</h2>
				<select
					bind:value={stateFilter}
					onchange={() => { page = 0; loadSweeps(); }}
					class="rounded-md border border-input bg-surface px-3 py-1.5 text-sm"
				>
					<option value="">{t('common.all', 'All states')}</option>
					<option value="proposed">{t('app.treasury.state_proposed', 'Proposed')}</option>
					<option value="approved">{t('app.treasury.state_approved', 'Approved')}</option>
					<option value="executed">{t('app.treasury.state_executed', 'Executed')}</option>
					<option value="rejected">{t('app.treasury.state_rejected', 'Rejected')}</option>
					<option value="expired">{t('app.treasury.state_expired', 'Expired')}</option>
				</select>
			</div>
			<DataTable
				columns={sweepColumns}
				data={sweeps}
				serverSide
				bind:page
				bind:pageSize
				{totalRows}
				{totalPages}
				pageSizeOptions={[10, 20, 50]}
				onpagechange={(p) => { page = p; loadSweeps(); }}
				onpagesizechange={() => { if (page === 0) loadSweeps(); else page = 0; }}
				actions={sweepActions}
				loading={sweepsLoading}
				emptyMessage={t('app.treasury.empty_sweeps', 'No sweep proposals')}
			/>
		</CardContent>
	</Card>
</div>

<!-- Killswitch confirm -->
<Modal bind:open={killswitchConfirm} title={killswitchEngaged ? t('app.treasury.killswitch_disengage_title', 'Disengage killswitch') : t('app.treasury.killswitch_engage_title', 'Engage killswitch')}>
	<div class="flex items-start gap-3">
		<AlertTriangle class="mt-0.5 size-5 shrink-0 text-warning" />
		<p class="text-sm">
			{killswitchEngaged
				? t('app.treasury.killswitch_disengage_body', 'Delegated-signer operations (tips, withdrawals, fee collection) will resume platform-wide.')
				: t('app.treasury.killswitch_engage_body', 'This halts ALL delegated-signer money movement platform-wide — member tips, withdrawals, and fee collection stop immediately. Member-signed operations are unaffected.')}
		</p>
	</div>
	{#snippet footer()}
		<Button variant="ghost" onclick={() => (killswitchConfirm = false)} disabled={killswitchBusy}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant={killswitchEngaged ? 'default' : 'destructive'} onclick={toggleKillswitch} disabled={killswitchBusy}>
			{killswitchBusy ? t('common.saving', 'Saving…') : killswitchEngaged ? t('app.treasury.killswitch_disengage', 'Disengage') : t('app.treasury.killswitch_engage', 'Engage killswitch')}
		</Button>
	{/snippet}
</Modal>

<!-- Sweep decision modal -->
<Modal
	bind:open={decisionModalOpen}
	onclose={() => { decisionSweep = null; }}
	title={decision === 'approve' ? t('app.treasury.approve_title', 'Approve sweep') : t('app.treasury.reject_title', 'Reject sweep')}
>
	{#if decisionSweep}
		<p class="text-sm">
			{decision === 'approve'
				? t('app.treasury.approve_body', 'Approve this hot→cold sweep proposal? Execution still requires the Squads multisig ceremony.')
				: t('app.treasury.reject_body', 'Reject this sweep proposal? It cannot be re-approved afterwards.')}
		</p>
		<p class="mt-2 text-sm font-medium">{formatMinor(decisionSweep.amount_minor, decisionSweep.asset)} → {shortAddr(decisionSweep.cold_address)}</p>
		{#if decision === 'approve'}
			<div class="mt-3">
				<label class="mb-1 block text-sm font-medium" for="squads-id">{t('app.treasury.squads_id', 'Squads proposal ID (optional)')}</label>
				<input
					id="squads-id"
					type="text"
					bind:value={squadsProposalId}
					class="w-full rounded-md border border-input bg-surface px-3 py-2 font-mono text-sm"
					placeholder={t('app.treasury.squads_id_ph', 'Squads multisig proposal reference')}
				/>
			</div>
		{/if}
	{/if}
	{#snippet footer()}
		<Button variant="ghost" onclick={() => (decisionModalOpen = false)} disabled={decisionBusy}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant={decision === 'reject' ? 'destructive' : 'default'} onclick={submitDecision} disabled={decisionBusy}>
			{decisionBusy ? t('common.saving', 'Saving…') : decision === 'approve' ? t('app.treasury.action_approve', 'Approve') : t('app.treasury.action_reject', 'Reject')}
		</Button>
	{/snippet}
</Modal>
