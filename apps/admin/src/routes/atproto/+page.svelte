<script lang="ts">
	// /atproto — AT Protocol mini-PDS dashboard (§127, ADR-139): account
	// stats + opt-in account list + the A3 conformance readback that
	// cmd/atproto-conformance runs as the release gate. Runs persist
	// server-side; the history table shows when appview indexing broke.
	// Backend: GET /admin/atproto/stats|accounts|conformance,
	//          POST /admin/atproto/conformance (RBAC: atproto.accounts.view)
	import { siteName } from '#lib/stores/site.svelte.js';
	import PageHeader from '#lib/components/layout/page-header.svelte';
	import StatCard from '#lib/components/ui/StatCard.svelte';
	import { Card, CardContent, CardHeader } from '#lib/components/ui/card/index.js';
	import { DataTable } from '#lib/components/ui/data-table/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { atprotoAdminApi } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { onMount } from 'svelte';
	import { CircleCheck, CircleX, Play } from 'lucide-svelte';

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

	async function loadAccounts() {
		accountsLoading = true;
		try {
			const params = new URLSearchParams({ limit: String(acctPageSize), offset: String(acctPage * acctPageSize) });
			if (acctSearch) params.set('q', acctSearch);
			const res = await atprotoAdminApi.listAccounts(params.toString());
			if (res.ok) {
				const body = res.data as { data?: AtprotoAccount[]; meta?: { total?: number } };
				accounts = body.data ?? [];
				acctTotalRows = body.meta?.total ?? accounts.length;
			} else {
				pageError = res.error ?? t('app.atproto.error', 'Failed to load accounts');
			}
		} finally {
			accountsLoading = false;
		}
	}

	const acctTotalPages = $derived(Math.max(1, Math.ceil(acctTotalRows / acctPageSize)));

	const accountColumns = [
		{ key: 'username', label: t('app.atproto.col_member', 'Member') },
		{ key: 'handle', label: t('app.atproto.col_handle', 'Handle') },
		{ key: 'did', label: 'DID', render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{
			key: 'status',
			label: t('app.atproto.col_status', 'Status'),
			render: (v: string) => {
				const cls = v === 'active' ? 'bg-success/15 text-success border-success' : v === 'tombstone' ? 'bg-danger/15 text-danger border-danger' : 'bg-surface text-foreground border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${v}</span>`;
			}
		},
		{ key: 'repo_seq', label: t('app.atproto.col_commits', 'Commits') },
		{ key: 'created_at', label: t('app.atproto.col_created', 'Created'), render: (v: string) => new Date(v).toLocaleString() }
	];

	// ── Conformance run ────────────────────────────────────────────────────
	let target = $state('');
	let running = $state(false);
	let runError = $state('');
	let lastResult = $state<{ actor: string; did: string; checks: Check[]; passed: boolean } | null>(null);

	async function runCheck() {
		if (running) return;
		running = true;
		runError = '';
		lastResult = null;
		try {
			const input = target.trim();
			const body = input.startsWith('did:') ? { did: input } : input ? { handle: input } : {};
			const res = await atprotoAdminApi.runConformance(body);
			if (res.ok) {
				const payload = res.data as { result?: { actor: string; did: string; checks: Check[]; passed: boolean } };
				lastResult = payload.result ?? null;
				loadRuns();
			} else {
				runError = res.error ?? t('app.atproto.conformance_error', 'Conformance run failed');
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

	async function loadRuns() {
		runsLoading = true;
		try {
			const params = new URLSearchParams({ limit: String(runPageSize), offset: String(runPage * runPageSize) });
			const res = await atprotoAdminApi.listConformanceRuns(params.toString());
			if (res.ok) {
				const body = res.data as { data?: ConformanceRun[]; meta?: { total?: number } };
				runs = (body.data ?? []).map((r) => ({ ...r, checks: typeof r.checks === 'string' ? JSON.parse(r.checks) : (r.checks ?? []) }));
				runTotalRows = body.meta?.total ?? runs.length;
			}
		} finally {
			runsLoading = false;
		}
	}

	const runTotalPages = $derived(Math.max(1, Math.ceil(runTotalRows / runPageSize)));

	const runColumns = [
		{ key: 'actor', label: t('app.atproto.col_target', 'Target'), render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{ key: 'did', label: 'DID', render: (v: string) => `<span class="font-mono text-xs">${v || '—'}</span>` },
		{
			key: 'passed',
			label: t('app.atproto.col_result', 'Result'),
			render: (v: boolean, row: ConformanceRun) => {
				const failed = (row.checks ?? []).filter((c) => !c.pass).map((c) => c.name).join(', ');
				const cls = v ? 'bg-success/15 text-success border-success' : 'bg-danger/15 text-danger border-danger';
				const label = v ? 'PASS' : `FAIL${failed ? ` — ${failed}` : ''}`;
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${label}</span>`;
			}
		},
		{ key: 'created_at', label: t('app.atproto.col_when', 'When'), render: (v: string) => new Date(v).toLocaleString() }
	];

	onMount(async () => {
		try {
			const res = await atprotoAdminApi.getStats();
			if (res.ok) stats = res.data as Stats;
		} catch { /* stats are best-effort */ }
		loadAccounts();
		loadRuns();
	});
</script>

<svelte:head>
	<title>{t('app.atproto.title', 'AT Protocol')} — {siteName()}</title>
</svelte:head>

<PageHeader
	title={t('app.atproto.title', 'AT Protocol')}
	description={t('app.atproto.description', 'Mini-PDS accounts, network status, and the appview conformance gate')}
/>

<div class="space-y-6">
	{#if pageError}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">{pageError}</div>
	{/if}

	<!-- Stats -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
		<StatCard label={t('app.atproto.stat_active', 'Active accounts')} value={(stats?.active_accounts ?? 0).toLocaleString()} />
		<StatCard label={t('app.atproto.stat_disabled', 'Disabled')} value={(stats?.disabled_accounts ?? 0).toLocaleString()} />
		<StatCard label={t('app.atproto.stat_records', 'Records projected')} value={(stats?.total_records ?? 0).toLocaleString()} />
		<StatCard label={t('app.atproto.stat_events', 'Repo events')} value={(stats?.total_repo_events ?? 0).toLocaleString()} />
	</div>

	<!-- Conformance check -->
	<Card>
		<CardHeader>
			<h3 class="text-sm font-medium">{t('app.atproto.conformance_title', 'Conformance check')}</h3>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('app.atproto.conformance_desc', 'Reads the account back through the appview — the A3 release gate. Leave blank to auto-pick an active account.')}
			</p>
		</CardHeader>
		<CardContent>
			<div class="flex flex-wrap items-end gap-3">
				<div class="min-w-64 flex-1">
					<label class="mb-1 block text-sm font-medium" for="conf-target">{t('app.atproto.target', 'Handle or DID')}</label>
					<input
						id="conf-target"
						type="text"
						bind:value={target}
						placeholder={t('app.atproto.target_ph', 'alice.japa.to or did:web:…')}
						class="w-full rounded-md border border-input bg-surface px-3 py-2 text-sm"
						disabled={running}
					/>
				</div>
				<Button onclick={runCheck} disabled={running}>
					<Play class="size-4" />
					{running ? t('app.atproto.running', 'Running…') : t('app.atproto.run', 'Run check')}
				</Button>
			</div>

			{#if runError}
				<div class="mt-3 rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">{runError}</div>
			{/if}

			{#if lastResult}
				<div class="mt-4 space-y-1">
					{#each lastResult.checks as check (check.name)}
						<div class="flex items-start gap-2 text-sm">
							{#if check.pass}
								<CircleCheck class="mt-0.5 size-4 shrink-0" style="color: var(--color-success)" />
							{:else}
								<CircleX class="mt-0.5 size-4 shrink-0" style="color: var(--color-danger)" />
							{/if}
							<span class="font-medium">{check.name}</span>
							{#if check.detail}
								<span class="font-mono text-xs" style="color: var(--color-muted)">{check.detail}</span>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</CardContent>
	</Card>

	<!-- Accounts -->
	<Card>
		<CardHeader>
			<h3 class="text-sm font-medium">{t('app.atproto.accounts_title', 'Accounts')}</h3>
		</CardHeader>
		<CardContent>
			<DataTable
				columns={accountColumns}
				data={accounts}
				serverSide
				bind:page={acctPage}
				bind:pageSize={acctPageSize}
				totalRows={acctTotalRows}
				totalPages={acctTotalPages}
				searchable
				onsearch={(q: string) => { acctSearch = q; acctPage = 0; loadAccounts(); }}
				onpagechange={loadAccounts}
				onpagesizechange={() => { if (acctPage === 0) loadAccounts(); else acctPage = 0; }}
				loading={accountsLoading}
				emptyMessage={t('app.atproto.no_accounts', 'No AT Protocol accounts yet')}
			/>
		</CardContent>
	</Card>

	<!-- Run history -->
	<Card>
		<CardHeader>
			<h3 class="text-sm font-medium">{t('app.atproto.history_title', 'Conformance history')}</h3>
		</CardHeader>
		<CardContent>
			<DataTable
				columns={runColumns}
				data={runs}
				serverSide
				bind:page={runPage}
				bind:pageSize={runPageSize}
				totalRows={runTotalRows}
				totalPages={runTotalPages}
				onpagechange={loadRuns}
				onpagesizechange={() => { if (runPage === 0) loadRuns(); else runPage = 0; }}
				loading={runsLoading}
				emptyMessage={t('app.atproto.no_runs', 'No conformance runs yet')}
			/>
		</CardContent>
	</Card>
</div>
