<script lang="ts">
	// /agent — Admin AI Agent session viewer. Read-only audit surface with
	// three tabs: Sessions (list + detail slide-over), Tool Stats, Cost Report.
	//
	// All data from GET /admin/agent/sessions, /admin/agent/sessions/{id},
	// /admin/agent/tool-stats, /admin/agent/cost-report. No mutations — the
	// admin can view but not modify agent sessions (cancellation is done by
	// the user from the main app).
	//
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Admin Routes)
	// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, task 10.06)

	import { onMount } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { apiFetch } from '$lib/api';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import {
		Sparkles, Wrench, DollarSign, BarChart3, Eye, Loader2, AlertCircle,
		Search, Clock, Cpu, Database, CheckCircle2, XCircle, Shield, Lock,
		ChevronLeft, ChevronRight, Calendar
	} from 'lucide-svelte';

	// ─── Types ────────────────────────────────────────────────────────
	interface AgentSession {
		id: string;
		user_id: string;
		prompt: string;
		presearch_level: string;
		status: string;
		tool_call_count: number;
		total_tokens: number;
		started_at: string;
		created_at: string;
		geo_space_id?: string;
		result_summary?: string;
		error_message?: string;
		completed_at?: string;
		estimated_cost: number;
	}

	interface AgentToolCall {
		id: string;
		session_id: string;
		tool_name: string;
		status: string;
		rbac_checked: boolean;
		privacy_checked: boolean;
		created_at: string;
		arguments?: unknown;
		result?: unknown;
		error_message?: string;
		latency_ms?: number;
		hitl_review_id?: string;
	}

	interface AgentToolStat {
		tool_name: string;
		total_calls: number;
		success_count: number;
		error_count: number;
		unauthorized_count: number;
		privacy_blocked_count: number;
		hitl_queued_count: number;
		success_rate: number;
		avg_latency_ms: number;
		max_latency_ms: number;
	}

	interface AgentCostReportRow {
		user_id: string;
		username?: string;
		session_count: number;
		total_tokens: number;
		total_tool_calls: number;
		total_cost_usd: number;
	}

	type Tab = 'sessions' | 'tool-stats' | 'cost-report';

	// ─── State ────────────────────────────────────────────────────────
	let activeTab = $state<Tab>('sessions');
	let loading = $state(false);
	let error = $state('');

	// Sessions list state
	let sessions = $state<AgentSession[]>([]);
	let sessionsTotal = $state(0);
	let sessionsPage = $state(1);
	let sessionsPageSize = $state(25);
	let sessionsStatusFilter = $state('');
	let sessionsSearch = $state('');

	// Session detail state
	let detailOpen = $state(false);
	let detailLoading = $state(false);
	let detailSession = $state<AgentSession | null>(null);
	let detailToolCalls = $state<AgentToolCall[]>([]);

	// Tool stats state
	let toolStats = $state<AgentToolStat[]>([]);
	let statsFromDate = $state('');
	let statsToDate = $state('');

	// Cost report state
	let costReport = $state<AgentCostReportRow[]>([]);
	let costFromDate = $state('');
	let costToDate = $state('');

	const canView = $derived(hasPermission('admin.ai.view') || hasPermission('admin.agent.view'));

	// ─── Data loading ─────────────────────────────────────────────────
	async function loadSessions() {
		loading = true;
		error = '';
		const params = new URLSearchParams();
		params.set('page_size', String(sessionsPageSize));
		params.set('page_offset', String((sessionsPage - 1) * sessionsPageSize));
		if (sessionsStatusFilter) params.set('status', sessionsStatusFilter);
		// apiFetch unwraps body.data into res.data; res.body is the full
		// response envelope (including meta).
		const res = await apiFetch<AgentSession[]>(`/admin/agent/sessions?${params}`);
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		sessions = res.data ?? [];
		sessionsTotal = (res.body as { meta?: { total?: number } } | null)?.meta?.total ?? 0;
	}

	async function loadDetail(id: string) {
		detailOpen = true;
		detailLoading = true;
		const res = await apiFetch<{ session: AgentSession; tool_calls: AgentToolCall[] }>(`/admin/agent/sessions/${id}`);
		detailLoading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		detailSession = res.data?.session ?? null;
		detailToolCalls = res.data?.tool_calls ?? [];
	}

	async function loadToolStats() {
		loading = true;
		error = '';
		const params = new URLSearchParams();
		if (statsFromDate) params.set('from', statsFromDate);
		if (statsToDate) params.set('to', statsToDate);
		const res = await apiFetch<AgentToolStat[]>(`/admin/agent/tool-stats?${params}`);
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		toolStats = res.data ?? [];
	}

	async function loadCostReport() {
		loading = true;
		error = '';
		const params = new URLSearchParams();
		if (costFromDate) params.set('from', costFromDate);
		if (costToDate) params.set('to', costToDate);
		const res = await apiFetch<AgentCostReportRow[]>(`/admin/agent/cost-report?${params}`);
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		costReport = res.data ?? [];
	}

	function switchTab(tab: Tab) {
		activeTab = tab;
		error = '';
		if (tab === 'sessions' && sessions.length === 0) loadSessions();
		if (tab === 'tool-stats' && toolStats.length === 0) loadToolStats();
		if (tab === 'cost-report' && costReport.length === 0) loadCostReport();
	}

	function applyStatusFilter() {
		sessionsPage = 1;
		loadSessions();
	}

	function applyStatsDateRange() {
		loadToolStats();
	}

	function applyCostDateRange() {
		loadCostReport();
	}

	function nextPage() {
		const totalPages = Math.ceil(sessionsTotal / sessionsPageSize);
		if (sessionsPage < totalPages) {
			sessionsPage++;
			loadSessions();
		}
	}

	function prevPage() {
		if (sessionsPage > 1) {
			sessionsPage--;
			loadSessions();
		}
	}

	// ─── Helpers ──────────────────────────────────────────────────────
	function statusColor(status: string): string {
		switch (status) {
			case 'completed': return 'var(--color-success, #16a34a)';
			case 'failed': return 'var(--color-danger, #dc2626)';
			case 'cancelled': return 'var(--color-muted)';
			case 'running': return 'var(--color-info, #2563eb)';
			default: return 'var(--color-muted)';
		}
	}

	function fmtDate(iso: string | undefined): string {
		if (!iso) return '—';
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}

	function fmtCost(c: number): string {
		if (!c) return '$0.0000';
		return `$${c.toFixed(4)}`;
	}

	function fmtLatency(ms: number | undefined): string {
		if (ms == null) return '—';
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(2)}s`;
	}

	function toolStatusColor(status: string): string {
		switch (status) {
			case 'success': return 'var(--color-success, #16a34a)';
			case 'error': return 'var(--color-danger, #dc2626)';
			case 'unauthorized': return 'var(--color-warning, #d97706)';
			case 'privacy_blocked': return 'var(--color-warning, #d97706)';
			case 'hitl_queued': return 'var(--color-info, #2563eb)';
			default: return 'var(--color-muted)';
		}
	}

	function jsonPreview(v: unknown): string {
		if (v == null) return '';
		if (typeof v === 'string') return v;
		try {
			return JSON.stringify(v, null, 2);
		} catch {
			return String(v);
		}
	}

	function truncate(s: string, n: number): string {
		if (s.length <= n) return s;
		return s.slice(0, n) + '…';
	}

	const totalPages = $derived(Math.max(1, Math.ceil(sessionsTotal / sessionsPageSize)));

	onMount(() => {
		if (canView) loadSessions();
	});
</script>

<svelte:head>
	<title>{t('admin.agent.title', 'AI Agent Sessions')} — JapaConnect Admin</title>
</svelte:head>

{#if !canView}
	<div class="flex items-center gap-2 rounded-lg border p-4 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
		<AlertCircle class="h-5 w-5" />
		{t('admin.agent.no_permission', 'You do not have permission to view agent sessions.')}
	</div>
{:else}
	<div class="space-y-4">
		<!-- Header -->
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-3">
				<div class="flex h-10 w-10 items-center justify-center rounded-lg" style="background-color: var(--color-primary)">
					<Sparkles class="h-5 w-5 text-white" />
				</div>
				<div>
					<h1 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('admin.agent.title', 'AI Agent Sessions')}</h1>
					<p class="text-xs" style="color: var(--color-muted)">{t('admin.agent.subtitle', 'Audit trail of agent sessions, tool calls, and costs')}</p>
				</div>
			</div>
		</div>

		<!-- Tabs -->
		<div class="flex gap-1 border-b" style="border-color: var(--color-border)">
			<button
				type="button"
				onclick={() => switchTab('sessions')}
				class="inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors {activeTab === 'sessions' ? '' : 'border-transparent'}"
				style={activeTab === 'sessions' ? 'border-color: var(--color-primary); color: var(--color-primary)' : 'color: var(--color-muted)'}
			>
				<Clock size={14} /> {t('admin.agent.tab.sessions', 'Sessions')}
			</button>
			<button
				type="button"
				onclick={() => switchTab('tool-stats')}
				class="inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors {activeTab === 'tool-stats' ? '' : 'border-transparent'}"
				style={activeTab === 'tool-stats' ? 'border-color: var(--color-primary); color: var(--color-primary)' : 'color: var(--color-muted)'}
			>
				<BarChart3 size={14} /> {t('admin.agent.tab.tool_stats', 'Tool Stats')}
			</button>
			<button
				type="button"
				onclick={() => switchTab('cost-report')}
				class="inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors {activeTab === 'cost-report' ? '' : 'border-transparent'}"
				style={activeTab === 'cost-report' ? 'border-color: var(--color-primary); color: var(--color-primary)' : 'color: var(--color-muted)'}
			>
				<DollarSign size={14} /> {t('admin.agent.tab.cost_report', 'Cost Report')}
			</button>
		</div>

		{#if error}
			<div class="flex items-center gap-2 rounded-lg border p-3 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
				<AlertCircle class="h-4 w-4" /> {error}
			</div>
		{/if}

		<!-- ─── Sessions tab ─────────────────────────────────────────── -->
		{#if activeTab === 'sessions'}
			<!-- Filters -->
			<div class="flex items-center gap-3">
				<select
					bind:value={sessionsStatusFilter}
					onchange={applyStatusFilter}
					class="rounded-md border px-2 py-1.5 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				>
					<option value="">{t('admin.agent.filter.all_statuses', 'All statuses')}</option>
					<option value="running">{t('admin.agent.status.running', 'Running')}</option>
					<option value="completed">{t('admin.agent.status.completed', 'Completed')}</option>
					<option value="failed">{t('admin.agent.status.failed', 'Failed')}</option>
					<option value="cancelled">{t('admin.agent.status.cancelled', 'Cancelled')}</option>
				</select>
				<span class="text-xs" style="color: var(--color-muted)">
					{sessionsTotal} {t('admin.agent.sessions_total', 'sessions')}
				</span>
			</div>

			{#if loading}
				<div class="flex items-center justify-center py-12">
					<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
				</div>
			{:else if sessions.length === 0}
				<div class="rounded-lg border p-8 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
					{t('admin.agent.no_sessions', 'No agent sessions found.')}
				</div>
			{:else}
				<!-- Sessions table -->
				<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
					<table class="w-full text-sm">
						<thead style="background-color: var(--color-muted-15, color-mix(in srgb, var(--color-muted) 15%, transparent))">
							<tr>
								<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('admin.agent.col.prompt', 'Prompt')}</th>
								<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('admin.agent.col.status', 'Status')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.tools', 'Tools')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.tokens', 'Tokens')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.cost', 'Cost')}</th>
								<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('admin.agent.col.started', 'Started')}</th>
								<th class="px-3 py-2"></th>
							</tr>
						</thead>
						<tbody>
							{#each sessions as s (s.id)}
								<tr class="border-t transition-colors hover:bg-[var(--color-surface-subtle)]" style="border-color: var(--color-border)">
									<td class="px-3 py-2 max-w-xs truncate" style="color: var(--color-foreground)">{s.prompt}</td>
									<td class="px-3 py-2">
										<span
											class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
											style="background-color: color-mix(in srgb, {statusColor(s.status)} 12%, transparent); color: {statusColor(s.status)}"
										>
											{s.status}
										</span>
									</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{s.tool_call_count}</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{s.total_tokens.toLocaleString()}</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{fmtCost(s.estimated_cost)}</td>
									<td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{fmtDate(s.started_at)}</td>
									<td class="px-3 py-2 text-right">
										<Button variant="ghost" size="sm" onclick={() => loadDetail(s.id)}>
											<Eye size={14} />
										</Button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Pagination -->
				{#if totalPages > 1}
					<div class="flex items-center justify-between text-xs" style="color: var(--color-muted)">
						<div class="flex items-center gap-3">
							<span>{t('common.page', 'Page')} {sessionsPage} {t('common.of', 'of')} {totalPages}</span>
							<select
								value={sessionsPageSize}
								onchange={(e) => { sessionsPageSize = Number((e.currentTarget as HTMLSelectElement).value); sessionsPage = 1; void loadSessions(); }}
								class="rounded border px-2 py-1 text-xs"
								style="border-color: var(--color-border); color: var(--color-foreground); background: var(--color-surface)"
								aria-label={t('common.pageSize', 'Page size')}
							>
								<option value={10}>10</option>
								<option value={25}>25</option>
								<option value={50}>50</option>
								<option value={100}>100</option>
							</select>
						</div>
						<div class="flex gap-2">
							<Button variant="outline" size="sm" disabled={sessionsPage <= 1} onclick={prevPage}>
								<ChevronLeft size={14} />
							</Button>
							<Button variant="outline" size="sm" disabled={sessionsPage >= totalPages} onclick={nextPage}>
								<ChevronRight size={14} />
							</Button>
						</div>
					</div>
				{/if}
			{/if}

		<!-- ─── Tool Stats tab ───────────────────────────────────────── -->
		{:else if activeTab === 'tool-stats'}
			<!-- Date range filter -->
			<div class="flex items-end gap-3">
				<label class="block">
					<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('admin.agent.from', 'From')}</span>
					<input
						type="date"
						bind:value={statsFromDate}
						class="rounded-md border px-2 py-1.5 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</label>
				<label class="block">
					<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('admin.agent.to', 'To')}</span>
					<input
						type="date"
						bind:value={statsToDate}
						class="rounded-md border px-2 py-1.5 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</label>
				<Button variant="default" size="sm" onclick={applyStatsDateRange}>
					<Calendar size={14} /> {t('common.apply', 'Apply')}
				</Button>
			</div>

			{#if loading}
				<div class="flex items-center justify-center py-12">
					<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
				</div>
			{:else if toolStats.length === 0}
				<div class="rounded-lg border p-8 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
					{t('admin.agent.no_tool_stats', 'No tool usage in the selected period.')}
				</div>
			{:else}
				<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
					<table class="w-full text-sm">
						<thead style="background-color: var(--color-muted-15, color-mix(in srgb, var(--color-muted) 15%, transparent))">
							<tr>
								<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('admin.agent.col.tool', 'Tool')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.calls', 'Calls')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.success_rate', 'Success')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.errors', 'Errors')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.unauthorized', 'Denied')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.privacy_blocked', 'Privacy')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.hitl', 'HITL')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.avg_latency', 'Avg')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.max_latency', 'Max')}</th>
							</tr>
						</thead>
						<tbody>
							{#each toolStats as stat (stat.tool_name)}
								<tr class="border-t" style="border-color: var(--color-border)">
									<td class="px-3 py-2 font-mono text-xs font-medium" style="color: var(--color-foreground)">{stat.tool_name}</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-foreground)">{stat.total_calls}</td>
									<td class="px-3 py-2 text-right">
										<span style="color: {stat.success_rate >= 0.9 ? 'var(--color-success, #16a34a)' : stat.success_rate >= 0.5 ? 'var(--color-warning, #d97706)' : 'var(--color-danger, #dc2626)'}">
											{(stat.success_rate * 100).toFixed(1)}%
										</span>
									</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{stat.error_count}</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{stat.unauthorized_count}</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{stat.privacy_blocked_count}</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{stat.hitl_queued_count}</td>
									<td class="px-3 py-2 text-right text-xs" style="color: var(--color-muted)">{fmtLatency(stat.avg_latency_ms)}</td>
									<td class="px-3 py-2 text-right text-xs" style="color: var(--color-muted)">{fmtLatency(stat.max_latency_ms)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

		<!-- ─── Cost Report tab ──────────────────────────────────────── -->
		{:else if activeTab === 'cost-report'}
			<!-- Date range filter -->
			<div class="flex items-end gap-3">
				<label class="block">
					<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('admin.agent.from', 'From')}</span>
					<input
						type="date"
						bind:value={costFromDate}
						class="rounded-md border px-2 py-1.5 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</label>
				<label class="block">
					<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('admin.agent.to', 'To')}</span>
					<input
						type="date"
						bind:value={costToDate}
						class="rounded-md border px-2 py-1.5 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</label>
				<Button variant="default" size="sm" onclick={applyCostDateRange}>
					<Calendar size={14} /> {t('common.apply', 'Apply')}
				</Button>
			</div>

			{#if loading}
				<div class="flex items-center justify-center py-12">
					<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
				</div>
			{:else if costReport.length === 0}
				<div class="rounded-lg border p-8 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
					{t('admin.agent.no_cost_data', 'No cost data in the selected period.')}
				</div>
			{:else}
				<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
					<table class="w-full text-sm">
						<thead style="background-color: var(--color-muted-15, color-mix(in srgb, var(--color-muted) 15%, transparent))">
							<tr>
								<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('admin.agent.col.user', 'User')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.sessions', 'Sessions')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.tokens', 'Tokens')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.tool_calls', 'Tool Calls')}</th>
								<th class="px-3 py-2 text-right font-medium" style="color: var(--color-muted)">{t('admin.agent.col.total_cost', 'Total Cost')}</th>
							</tr>
						</thead>
						<tbody>
							{#each costReport as row (row.user_id)}
								<tr class="border-t" style="border-color: var(--color-border)">
									<td class="px-3 py-2" style="color: var(--color-foreground)">
										{row.username ?? row.user_id.slice(0, 8)}
									</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{row.session_count}</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{row.total_tokens.toLocaleString()}</td>
									<td class="px-3 py-2 text-right" style="color: var(--color-muted)">{row.total_tool_calls}</td>
									<td class="px-3 py-2 text-right font-medium" style="color: var(--color-foreground)">{fmtCost(row.total_cost_usd)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		{/if}
	</div>

	<!-- ─── Session detail slide-over ─────────────────────────────────── -->
	<SlideOver bind:open={detailOpen} title={t('admin.agent.session_detail', 'Session Detail')}>
		{#if detailLoading}
			<div class="flex items-center justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
			</div>
		{:else if detailSession}
			<div class="space-y-4">
				<!-- Session metadata -->
				<div class="rounded-lg border p-3" style="border-color: var(--color-border); background-color: var(--color-surface)">
					<div class="mb-2 flex items-center gap-2">
						<span class="text-xs font-mono" style="color: var(--color-muted)">{detailSession.id}</span>
						<span
							class="ml-auto inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
							style="background-color: color-mix(in srgb, {statusColor(detailSession.status)} 12%, transparent); color: {statusColor(detailSession.status)}"
						>
							{detailSession.status}
						</span>
					</div>
					<p class="text-sm font-medium" style="color: var(--color-foreground)">{detailSession.prompt}</p>
					<div class="mt-3 grid grid-cols-2 gap-2 text-xs" style="color: var(--color-muted)">
						<div><Clock size={10} class="inline" /> {fmtDate(detailSession.started_at)}</div>
						<div><Cpu size={10} class="inline" /> {detailSession.total_tokens.toLocaleString()} tokens</div>
						<div><Database size={10} class="inline" /> {fmtCost(detailSession.estimated_cost)}</div>
						<div><Wrench size={10} class="inline" /> {detailSession.tool_call_count} tools</div>
					</div>
					<div class="mt-2 text-xs" style="color: var(--color-muted)">
						User: <span class="font-mono">{detailSession.user_id.slice(0, 8)}</span>
					</div>
					{#if detailSession.error_message}
						<div class="mt-2 rounded border p-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">
							{detailSession.error_message}
						</div>
					{/if}
				</div>

				<!-- Tool calls -->
				{#if detailToolCalls.length > 0}
					<div>
						<h3 class="mb-2 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">
							{t('admin.agent.tool_call_log', 'Tool Call Log')}
						</h3>
						<div class="space-y-2">
							{#each detailToolCalls as tc (tc.id)}
								<div class="rounded-md border p-2.5 text-xs" style="border-color: var(--color-border); background-color: var(--color-surface)">
									<div class="flex items-center gap-2">
										<Wrench size={12} style="color: var(--color-muted)" />
										<span class="font-mono font-medium">{tc.tool_name}</span>
										<span
											class="ml-auto rounded-full px-1.5 py-0.5 text-xs font-medium"
											style="background-color: color-mix(in srgb, {toolStatusColor(tc.status)} 12%, transparent); color: {toolStatusColor(tc.status)}"
										>
											{tc.status}
										</span>
									</div>
									<div class="mt-1.5 flex flex-wrap items-center gap-2" style="color: var(--color-muted)">
										<span><Clock size={9} class="inline" /> {fmtLatency(tc.latency_ms)}</span>
										{#if tc.rbac_checked}<span><Shield size={9} class="inline" /> RBAC</span>{/if}
										{#if tc.privacy_checked}<span><Lock size={9} class="inline" /> Privacy</span>{/if}
										{#if tc.hitl_review_id}<span><AlertCircle size={9} class="inline" /> HITL</span>{/if}
									</div>
									{#if tc.arguments != null}
										<details class="mt-1.5">
											<summary class="cursor-pointer" style="color: var(--color-muted)">Arguments</summary>
											<pre class="mt-1 max-h-32 overflow-auto rounded p-1.5 font-mono text-xs" style="background-color: var(--color-background)">{jsonPreview(tc.arguments)}</pre>
										</details>
									{/if}
									{#if tc.result != null}
										<details class="mt-1">
											<summary class="cursor-pointer" style="color: var(--color-muted)">Result</summary>
											<pre class="mt-1 max-h-40 overflow-auto rounded p-1.5 font-mono text-xs" style="background-color: var(--color-background)">{jsonPreview(tc.result)}</pre>
										</details>
									{/if}
									{#if tc.error_message}
										<p class="mt-1" style="color: var(--color-danger)">{tc.error_message}</p>
									{/if}
								</div>
							{/each}
						</div>
					</div>
				{:else}
					<p class="text-xs text-center" style="color: var(--color-muted)">{t('admin.agent.no_tool_calls', 'No tool calls in this session.')}</p>
				{/if}
			</div>
		{:else}
			<div class="text-center text-sm" style="color: var(--color-muted)">
				{t('admin.agent.session_not_found', 'Session not found.')}
			</div>
		{/if}
	</SlideOver>
{/if}
