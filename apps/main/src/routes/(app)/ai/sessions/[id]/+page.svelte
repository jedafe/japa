<script lang="ts">
	// /ai/sessions/[id] — single agent session detail. Shows the session
	// metadata (prompt, status, cost, tokens, pre-search level) and the full
	// tool-call audit log with arguments, results, latency, and HITL links.
	//
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
	// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, task 10.04)

	import { t } from '$lib/i18n';
	import {
		Sparkles,
		Clock,
		Cpu,
		Database,
		Wrench,
		AlertCircle,
		CheckCircle2,
		XCircle,
		Loader2,
		Shield,
		Lock,
		ArrowLeft,
		BookOpen,
		Folder
	} from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { AgentSession, AgentToolCall } from '$lib/ai/agentTypes';

	let { data }: { data: PageData } = $props();

	const detail = $derived(data.detail);
	const session = $derived(detail?.session as AgentSession | undefined);
	const toolCalls = $derived(detail?.tool_calls as AgentToolCall[] | undefined);
	const projectName = $derived(data.projectName ?? '');

	function statusColor(status: string | undefined): string {
		switch (status) {
			case 'completed': return 'var(--color-success, #16a34a)';
			case 'failed': return 'var(--color-danger, #dc2626)';
			case 'cancelled': return 'var(--color-muted)';
			case 'running': return 'var(--color-info, #2563eb)';
			default: return 'var(--color-muted)';
		}
	}

	function statusIcon(status: string | undefined) {
		switch (status) {
			case 'completed': return CheckCircle2;
			case 'failed': return XCircle;
			case 'running': return Loader2;
			default: return Clock;
		}
	}

	function statusLabel(status: string | undefined): string {
		switch (status) {
			case 'completed': return t('ai.agent.status_completed', 'Completed');
			case 'failed': return t('ai.agent.status_failed', 'Failed');
			case 'cancelled': return t('ai.agent.status_cancelled', 'Cancelled');
			case 'running': return t('ai.agent.status_running', 'Running');
			case 'pending': return t('ai.agent.status_pending', 'Pending');
			default: return status ?? '';
		}
	}

	function toolStatusLabel(status: string): string {
		switch (status) {
			case 'success': return t('ai.agent.tool.success', 'Success');
			case 'error': return t('ai.agent.tool.error', 'Error');
			case 'unauthorized': return t('ai.agent.tool.unauthorized', 'Denied');
			case 'privacy_blocked': return t('ai.agent.tool.privacy_blocked', 'Privacy blocked');
			case 'hitl_queued': return t('ai.agent.tool.hitl_queued', 'Awaiting review');
			case 'pending': return t('ai.agent.tool.pending', 'Running');
			default: return status;
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

	function fmtCost(c: number | undefined): string {
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
</script>

<svelte:head>
	<title>{session ? truncate(session.prompt, 60) : 'Session'} — {t('ai.agent.session_history', 'Session History')} — JapaConnect</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto max-w-4xl">
	<!-- Back link -->
	<a
		href="/ai/sessions"
		class="mb-4 inline-flex items-center gap-1 text-sm transition-colors hover:opacity-80"
		style="color: var(--color-muted)"
	>
		<ArrowLeft size={14} />
		{t('ai.agent.back_to_history', 'Back to session history')}
	</a>

	{#if data.loadError}
		<div class="flex items-center gap-2 rounded-lg border p-4 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
			<AlertCircle class="h-5 w-5" />
			{t('ai.agent.load_failed', 'Failed to load session detail.')}
		</div>
	{:else if !session}
		<div class="flex items-center gap-2 rounded-lg border p-4 text-sm" style="border-color: var(--color-warning); color: var(--color-warning)">
			<AlertCircle class="h-5 w-5" />
			{t('ai.agent.session_not_found', 'Session not found.')}
		</div>
	{:else}
		{@const StatusIcon = statusIcon(session.status)}
		<!-- Session header -->
		<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="flex items-start justify-between gap-3">
				<div class="min-w-0 flex-1">
					<div class="mb-2 flex items-center gap-2">
						<Sparkles size={16} style="color: var(--color-primary)" />
						<span class="text-xs font-mono" style="color: var(--color-muted)">{session.id}</span>
					</div>
					<p class="text-base font-medium" style="color: var(--color-text)">{session.prompt}</p>
				</div>
				<span
					class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium"
					style="background-color: color-mix(in srgb, {statusColor(session.status)} 12%, transparent); color: {statusColor(session.status)}"
				>
					<StatusIcon size={12} class={session.status === 'running' ? 'animate-spin' : ''} />
					{statusLabel(session.status)}
				</span>
			</div>

			<!-- Metadata grid -->
			<div class="mt-4 grid grid-cols-2 gap-3 text-xs sm:grid-cols-4">
				<div>
					<p class="uppercase tracking-wider" style="color: var(--color-muted)">{t('ai.agent.started', 'Started')}</p>
					<p class="mt-0.5 font-medium" style="color: var(--color-text)">{fmtDate(session.started_at)}</p>
				</div>
				<div>
					<p class="uppercase tracking-wider" style="color: var(--color-muted)">{t('ai.agent.completed', 'Completed')}</p>
					<p class="mt-0.5 font-medium" style="color: var(--color-text)">{fmtDate(session.completed_at)}</p>
				</div>
				<div>
					<p class="uppercase tracking-wider" style="color: var(--color-muted)">{t('ai.agent.tokens', 'Tokens')}</p>
					<p class="mt-0.5 font-medium inline-flex items-center gap-1" style="color: var(--color-text)">
						<Cpu size={11} /> {session.total_tokens.toLocaleString()}
					</p>
				</div>
				<div>
					<p class="uppercase tracking-wider" style="color: var(--color-muted)">{t('ai.agent.cost', 'Cost')}</p>
					<p class="mt-0.5 font-medium inline-flex items-center gap-1" style="color: var(--color-text)">
						<Database size={11} /> {fmtCost(session.estimated_cost)}
					</p>
				</div>
			</div>

			<div class="mt-3 flex flex-wrap items-center gap-3 text-xs" style="color: var(--color-muted)">
				{#if projectName}
					<span
						class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium"
						style="background-color: var(--color-primary-subtle); color: var(--color-primary)"
					>
						<Folder size={10} />
						{projectName}
					</span>
				{/if}
				{#if session.presearch_level}
					<span class="inline-flex items-center gap-1"><BookOpen size={11} /> {t('ai.agent.presearch', 'Pre-search')}: {session.presearch_level}</span>
				{/if}
				{#if session.tool_call_count > 0}
					<span class="inline-flex items-center gap-1"><Wrench size={11} /> {session.tool_call_count} {t('ai.agent.tool_calls', 'tool calls')}</span>
				{/if}
				{#if session.result_summary}
					<span class="inline-flex items-center gap-1"><CheckCircle2 size={11} /> {truncate(session.result_summary, 80)}</span>
				{/if}
			</div>

			{#if session.error_message}
				<div class="mt-3 rounded-md border p-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">
					{session.error_message}
				</div>
			{/if}
		</div>

		<!-- Tool call audit log -->
		{#if toolCalls && toolCalls.length > 0}
			<div class="mt-6">
				<h2 class="mb-3 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">
					{t('ai.agent.tool_call_log', 'Tool Call Audit Log')}
				</h2>
				<div class="space-y-2">
					{#each toolCalls as tc (tc.id)}
						<div class="rounded-lg border p-3" style="border-color: var(--color-border); background-color: var(--color-surface)">
							<div class="flex items-center gap-2">
								<Wrench size={14} style="color: var(--color-muted)" />
								<span class="font-mono text-sm font-medium">{tc.tool_name}</span>
								<span
									class="ml-auto inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
									style="background-color: color-mix(in srgb, {toolStatusColor(tc.status)} 12%, transparent); color: {toolStatusColor(tc.status)}"
								>
									{toolStatusLabel(tc.status)}
								</span>
							</div>

							<div class="mt-2 flex flex-wrap items-center gap-3 text-xs" style="color: var(--color-muted)">
								<span class="inline-flex items-center gap-1"><Clock size={10} /> {fmtDate(tc.created_at)}</span>
								{#if tc.latency_ms != null}
									<span class="inline-flex items-center gap-1"><Clock size={10} /> {fmtLatency(tc.latency_ms)}</span>
								{/if}
								{#if tc.rbac_checked}
									<span class="inline-flex items-center gap-1"><Shield size={10} /> RBAC</span>
								{/if}
								{#if tc.privacy_checked}
									<span class="inline-flex items-center gap-1"><Lock size={10} /> {t('ai.agent.privacy', 'Privacy')}</span>
								{/if}
								{#if tc.hitl_review_id}
									<span class="inline-flex items-center gap-1"><AlertCircle size={10} /> HITL</span>
								{/if}
							</div>

							{#if tc.arguments != null}
								<details class="mt-2">
									<summary class="cursor-pointer text-xs" style="color: var(--color-muted)">{t('ai.agent.tool.arguments', 'Arguments')}</summary>
									<pre class="mt-1 max-h-48 overflow-auto rounded p-2 text-xs font-mono" style="background-color: var(--color-background)">{jsonPreview(tc.arguments)}</pre>
								</details>
							{/if}
							{#if tc.result != null}
								<details class="mt-1">
									<summary class="cursor-pointer text-xs" style="color: var(--color-muted)">{t('ai.agent.tool.result', 'Result')}</summary>
									<pre class="mt-1 max-h-64 overflow-auto rounded p-2 text-xs font-mono" style="background-color: var(--color-background)">{jsonPreview(tc.result)}</pre>
								</details>
							{/if}
							{#if tc.error_message}
								<p class="mt-1.5 text-xs" style="color: var(--color-danger)">{tc.error_message}</p>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{:else}
			<div class="mt-6 rounded-lg border p-6 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
				{t('ai.agent.no_tool_calls', 'No tool calls in this session.')}
			</div>
		{/if}
	{/if}
</div>
