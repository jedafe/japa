<script lang="ts">
	// /atproto — AT Protocol mini-PDS ops dashboard (§127, ADR-139): account
	// stats + hosted account inventory + the A3 conformance readback that
	// cmd/atproto-conformance runs as the release gate. Runs persist
	// server-side; the history table shows when appview indexing broke.
	// Backend: GET|POST /control/atproto/conformance, GET /control/atproto/{stats,accounts}
	// (control.panel.access + atproto.accounts.view, feature.atproto gated)
	import PageHeader from '#lib/components/PageHeader.svelte';
	import MetricCard from '#lib/components/MetricCard.svelte';
	import DataTable from '#lib/components/DataTable.svelte';
	import { api, apiList } from '#lib/api.js';
	import type { DataTableColumn } from '#lib/api/types.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { CircleCheck, CircleX, Play, Search } from 'lucide-svelte';
	import { untrack } from 'svelte';

	type AtprotoAccount = {
		id: string;
		username: string;
		handle: string;
		did: string;
		status: string;
		repo_seq: number;
		created_at: string;
		disabled_at?: string;
	};

	type Check = { name: string; pass: boolean; detail: string };
	type ConformanceRun = {
		id: string;
		actor: string;
		did: string;
		checks: Check[];
		passed: boolean;
		created_at: string;
	};
	type Stats = {
		active_accounts: number;
		disabled_accounts: number;
		total_accounts: number;
		total_records: number;
		total_commits: number;
		total_repo_events: number;
	};

	let stats = $state<Stats | null>(null);
	let pageError = $state('');

	// ── Accounts table (server-paginated + searchable) ────────────────────
	let accounts = $state<AtprotoAccount[]>([]);
	let accountsLoading = $state(true);
	let acctPage = $state(0);
	let acctPageSize = $state(10);
	let acctTotalRows = $state(0);
	let acctSearch = $state('');

	async function loadAccounts(signal?: AbortSignal) {
		accountsLoading = true;
		try {
			const params = new URLSearchParams({
				limit: String(acctPageSize),
				offset: String(acctPage * acctPageSize)
			});
			if (acctSearch) params.set('q', acctSearch);
			const res = await apiList<AtprotoAccount[]>(`/control/atproto/accounts?${params}`, { signal });
			if (res.ok) {
				accounts = res.data ?? [];
				acctTotalRows = res.meta.total ?? accounts.length;
			} else {
				pageError = res.error;
			}
		} finally {
			accountsLoading = false;
		}
	}

	// Reload when page or pageSize changes; initial load included.
	$effect(() => {
		void acctPage;
		void acctPageSize;
		const controller = new AbortController();
		untrack(() => loadAccounts(controller.signal));
		return () => controller.abort();
	});

	function searchAccounts() {
		acctPage = 0;
		loadAccounts();
	}

	const accountColumns: DataTableColumn[] = [
		{ key: 'username', label: t('atproto.col_member', 'Member') },
		{ key: 'handle', label: t('atproto.col_handle', 'Handle') },
		{
			key: 'did',
			label: 'DID',
			render: (v) => `<span class="font-mono text-xs">${v}</span>`
		},
		{
			key: 'status',
			label: t('atproto.col_status', 'Status'),
			render: (v) => {
				const cls =
					v === 'active'
						? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20'
						: v === 'tombstone'
							? 'bg-destructive/10 text-destructive border-destructive/20'
							: 'bg-muted text-muted-foreground border-border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${v}</span>`;
			}
		},
		{ key: 'repo_seq', label: t('atproto.col_commits', 'Commits') },
		{
			key: 'created_at',
			label: t('atproto.col_created', 'Created'),
			render: (v) => new Date(v as string).toLocaleString()
		}
	];

	// ── Conformance run ────────────────────────────────────────────────────
	let target = $state('');
	let running = $state(false);
	let runError = $state('');
	let lastResult = $state<{ actor: string; did: string; checks: Check[]; passed: boolean } | null>(
		null
	);

	async function runCheck() {
		if (running) return;
		running = true;
		runError = '';
		lastResult = null;
		try {
			const input = target.trim();
			const body = input.startsWith('did:') ? { did: input } : input ? { handle: input } : {};
			const res = await api<{ result?: { actor: string; did: string; checks: Check[]; passed: boolean } }>(
				'/control/atproto/conformance',
				{ method: 'POST', body }
			);
			if (res.ok) {
				lastResult = res.data.result ?? null;
				loadRuns();
			} else {
				runError = res.error;
			}
		} finally {
			running = false;
		}
	}

	// ── Run history (server-paginated) ─────────────────────────────────────
	let runs = $state<ConformanceRun[]>([]);
	let runsLoading = $state(true);
	let runPage = $state(0);
	let runPageSize = $state(10);
	let runTotalRows = $state(0);

	async function loadRuns(signal?: AbortSignal) {
		runsLoading = true;
		try {
			const params = new URLSearchParams({
				limit: String(runPageSize),
				offset: String(runPage * runPageSize)
			});
			const res = await apiList<ConformanceRun[]>(`/control/atproto/conformance?${params}`, { signal });
			if (res.ok) {
				runs = (res.data ?? []).map((r) => ({
					...r,
					checks: typeof r.checks === 'string' ? JSON.parse(r.checks) : (r.checks ?? [])
				}));
				runTotalRows = res.meta.total ?? runs.length;
			}
		} finally {
			runsLoading = false;
		}
	}

	$effect(() => {
		void runPage;
		void runPageSize;
		const controller = new AbortController();
		untrack(() => loadRuns(controller.signal));
		return () => controller.abort();
	});

	const runColumns: DataTableColumn[] = [
		{
			key: 'actor',
			label: t('atproto.col_target', 'Target'),
			render: (v) => `<span class="font-mono text-xs">${v}</span>`
		},
		{
			key: 'did',
			label: 'DID',
			render: (v) => `<span class="font-mono text-xs">${v || '—'}</span>`
		},
		{
			key: 'passed',
			label: t('atproto.col_result', 'Result'),
			render: (v, row) => {
				const r = row as ConformanceRun;
				const failed = (r.checks ?? [])
					.filter((c) => !c.pass)
					.map((c) => c.name)
					.join(', ');
				const cls = v
					? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20'
					: 'bg-destructive/10 text-destructive border-destructive/20';
				const label = v ? 'PASS' : `FAIL${failed ? ` — ${failed}` : ''}`;
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${label}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('atproto.col_when', 'When'),
			render: (v) => new Date(v as string).toLocaleString()
		}
	];

	$effect(() => {
		const controller = new AbortController();
		api<Stats>('/control/atproto/stats', { signal: controller.signal }).then((res) => {
			if (res.ok) stats = res.data;
		});
		return () => controller.abort();
	});
</script>

<svelte:head>
	<title>{t('atproto.title', 'AT Protocol')} — Control</title>
</svelte:head>

<PageHeader
	title={t('atproto.title', 'AT Protocol')}
	description={t(
		'atproto.description',
		'Mini-PDS accounts, network status, and the appview conformance gate'
	)}
/>

<div class="space-y-6">
	{#if pageError}
		<div
			class="bg-destructive/10 border-destructive/20 text-destructive rounded-lg border px-4 py-3 text-sm"
		>
			{pageError}
		</div>
	{/if}

	<!-- Stats -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		<MetricCard
			label={t('atproto.stat_active', 'Active accounts')}
			value={(stats?.active_accounts ?? 0).toLocaleString()}
		/>
		<MetricCard
			label={t('atproto.stat_disabled', 'Disabled')}
			value={(stats?.disabled_accounts ?? 0).toLocaleString()}
		/>
		<MetricCard
			label={t('atproto.stat_records', 'Records projected')}
			value={(stats?.total_records ?? 0).toLocaleString()}
		/>
		<MetricCard
			label={t('atproto.stat_events', 'Repo events')}
			value={(stats?.total_repo_events ?? 0).toLocaleString()}
		/>
	</div>

	<!-- Conformance check -->
	<div class="border-border-card bg-background-alt rounded-9px border">
		<div class="border-border-card border-b px-5 py-4">
			<h3 class="text-sm font-medium">{t('atproto.conformance_title', 'Conformance check')}</h3>
			<p class="text-muted-foreground mt-1 text-xs">
				{t(
					'atproto.conformance_desc',
					'Reads the account back through the appview — the A3 release gate. Leave blank to auto-pick an active account.'
				)}
			</p>
		</div>
		<div class="px-5 py-4">
			<div class="flex flex-wrap items-end gap-3">
				<div class="min-w-64 flex-1">
					<label class="mb-1 block text-sm font-medium" for="conf-target"
						>{t('atproto.target', 'Handle or DID')}</label
					>
					<input
						id="conf-target"
						type="text"
						bind:value={target}
						placeholder={t('atproto.target_ph', 'alice.japa.to or did:web:…')}
						class="border-border-input bg-background text-foreground w-full rounded-md border px-3 py-2 text-sm"
						disabled={running}
					/>
				</div>
				<button
					onclick={runCheck}
					disabled={running}
					class="border-border-input bg-primary text-primary-foreground hover:bg-primary/90 flex items-center gap-1.5 rounded-md border px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
				>
					<Play class="size-4" />
					{running ? t('atproto.running', 'Running…') : t('atproto.run', 'Run check')}
				</button>
			</div>

			{#if runError}
				<div
					class="border-destructive/30 text-destructive mt-3 rounded-md border px-3 py-2 text-sm"
				>
					{runError}
				</div>
			{/if}

			{#if lastResult}
				<div class="mt-4 space-y-1">
					{#each lastResult.checks as check (check.name)}
						<div class="flex items-start gap-2 text-sm">
							{#if check.pass}
								<CircleCheck class="mt-0.5 size-4 shrink-0 text-emerald-500" />
							{:else}
								<CircleX class="text-destructive mt-0.5 size-4 shrink-0" />
							{/if}
							<span class="font-medium">{check.name}</span>
							{#if check.detail}
								<span class="text-muted-foreground font-mono text-xs">{check.detail}</span>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>

	<!-- Accounts -->
	<div class="border-border-card bg-background-alt rounded-9px border">
		<div class="border-border-card flex flex-wrap items-center gap-3 border-b px-5 py-4">
			<h3 class="text-sm font-medium">{t('atproto.accounts_title', 'Accounts')}</h3>
			<div class="flex-1"></div>
			<div class="relative">
				<Search class="text-muted-foreground absolute top-2.5 left-2.5 size-4" />
				<input
					type="text"
					bind:value={acctSearch}
					onkeydown={(e) => e.key === 'Enter' && searchAccounts()}
					placeholder={t('atproto.search_ph', 'Search handle or DID…')}
					class="border-border-input bg-background text-foreground w-64 rounded-md border py-2 pr-3 pl-9 text-sm"
				/>
			</div>
			<button
				onclick={searchAccounts}
				class="border-border-input bg-background hover:bg-muted text-foreground rounded-md border px-3 py-2 text-sm font-medium transition-colors"
			>
				{t('common.search', 'Search')}
			</button>
		</div>
		<DataTable
			columns={accountColumns}
			rows={accounts}
			totalRows={acctTotalRows}
			bind:page={acctPage}
			bind:pageSize={acctPageSize}
			loading={accountsLoading}
			selectable={false}
			emptyMessage={t('atproto.no_accounts', 'No AT Protocol accounts yet')}
			getRowId={(row) => (row as AtprotoAccount).id}
		/>
	</div>

	<!-- Run history -->
	<div class="border-border-card bg-background-alt rounded-9px border">
		<div class="border-border-card border-b px-5 py-4">
			<h3 class="text-sm font-medium">{t('atproto.history_title', 'Conformance history')}</h3>
		</div>
		<DataTable
			columns={runColumns}
			rows={runs}
			totalRows={runTotalRows}
			bind:page={runPage}
			bind:pageSize={runPageSize}
			loading={runsLoading}
			selectable={false}
			emptyMessage={t('atproto.no_runs', 'No conformance runs yet')}
			getRowId={(row) => (row as ConformanceRun).id}
		/>
	</div>
</div>
