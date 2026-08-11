<script lang="ts">
	// /ai/sessions — agent session history list. Displays the user's past
	// agent sessions with status, tool-call count, cost, and a link to the
	// detail view. Data comes from the server load (no client fetch needed
	// for the first page; pagination is via link navigation).
	//
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
	// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, task 10.04)

	import { t } from '$lib/i18n';
	import { Sparkles, Clock, Cpu, Database, Wrench, ChevronRight, AlertCircle, CheckCircle2, XCircle, Loader2 } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { AgentSession } from '$lib/ai/agentTypes';

	let { data }: { data: PageData } = $props();

	const sessions = $derived(data.sessions as AgentSession[]);
	const total = $derived(data.total);
	const pageSize = $derived(data.pageSize);
	const offset = $derived(data.offset);

	const currentPage = $derived(Math.floor(offset / pageSize) + 1);
	const totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)));

	function statusColor(status: string): string {
		switch (status) {
			case 'completed': return 'var(--color-success, #16a34a)';
			case 'failed': return 'var(--color-danger, #dc2626)';
			case 'cancelled': return 'var(--color-muted)';
			case 'running': return 'var(--color-info, #2563eb)';
			default: return 'var(--color-muted)';
		}
	}

	function statusIcon(status: string) {
		switch (status) {
			case 'completed': return CheckCircle2;
			case 'failed': return XCircle;
			case 'running': return Loader2;
			default: return Clock;
		}
	}

	function fmtDate(iso: string): string {
		if (!iso) return '';
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

	function truncate(s: string, n: number): string {
		if (s.length <= n) return s;
		return s.slice(0, n) + '…';
	}
</script>

<svelte:head>
	<title>{t('ai.agent.session_history', 'Session History')} — JapaConnect</title>
</svelte:head>

<div class="mx-auto max-w-4xl">
	<header class="mb-6 flex items-center justify-between gap-3">
		<div class="flex items-center gap-3">
			<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background-color: var(--color-primary)">
				<Sparkles class="h-5 w-5 text-white" />
			</div>
			<div>
				<h1 class="text-xl font-bold tracking-tight">{t('ai.agent.session_history', 'Session History')}</h1>
				<p class="text-xs" style="color: var(--color-muted)">
					{total} {t('ai.agent.sessions_total', 'sessions')}
				</p>
			</div>
		</div>
		<a
			href="/ai"
			class="inline-flex items-center gap-1 rounded-lg border px-3 py-1.5 text-sm transition-colors hover:bg-[var(--color-surface-subtle)]"
			style="border-color: var(--color-border); color: var(--color-text)"
		>
			{t('ai.agent.new_session', 'New session')}
		</a>
	</header>

	{#if sessions.length === 0}
		<div class="flex flex-col items-center justify-center gap-3 rounded-lg border p-12 text-center" style="border-color: var(--color-border)">
			<Clock class="h-8 w-8" style="color: var(--color-muted)" />
			<p class="text-sm" style="color: var(--color-muted)">
				{t('ai.agent.no_sessions', 'No agent sessions yet. Start one from the agent chat.')}
			</p>
			<a
				href="/ai"
				class="inline-flex items-center gap-1 rounded-lg px-3 py-1.5 text-sm text-white"
				style="background-color: var(--color-primary)"
			>
				<Sparkles size={14} />
				{t('ai.agent.go_to_chat', 'Go to agent chat')}
			</a>
		</div>
	{:else}
		<div class="space-y-2">
			{#each sessions as session (session.id)}
				{@const StatusIcon = statusIcon(session.status)}
				<a
					href={`/ai/sessions/${session.id}`}
					class="block rounded-lg border p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
				>
					<div class="flex items-start justify-between gap-3">
						<div class="min-w-0 flex-1">
							<p class="text-sm font-medium truncate" style="color: var(--color-text)">
								{truncate(session.prompt, 120)}
							</p>
							<div class="mt-2 flex flex-wrap items-center gap-3 text-xs" style="color: var(--color-muted)">
								<span class="inline-flex items-center gap-1">
									<StatusIcon size={11} class={session.status === 'running' ? 'animate-spin' : ''} style="color: {statusColor(session.status)}" />
									<span style="color: {statusColor(session.status)}">{session.status}</span>
								</span>
								{#if session.tool_call_count > 0}
									<span class="inline-flex items-center gap-1"><Wrench size={11} /> {session.tool_call_count}</span>
								{/if}
								{#if session.total_tokens > 0}
									<span class="inline-flex items-center gap-1"><Cpu size={11} /> {session.total_tokens.toLocaleString()}</span>
								{/if}
								<span class="inline-flex items-center gap-1"><Database size={11} /> {fmtCost(session.estimated_cost)}</span>
								<span class="inline-flex items-center gap-1"><Clock size={11} /> {fmtDate(session.started_at || session.created_at)}</span>
							</div>
							{#if session.presearch_level}
								<span class="mt-1 inline-block text-xs" style="color: var(--color-muted)">
									{t('ai.agent.presearch', 'Pre-search')}: {session.presearch_level}
								</span>
							{/if}
							{#if session.error_message}
								<p class="mt-1 text-xs" style="color: var(--color-danger)">{truncate(session.error_message, 100)}</p>
							{/if}
						</div>
						<ChevronRight size={16} style="color: var(--color-muted)" class="shrink-0" />
					</div>
				</a>
			{/each}
		</div>

		<!-- Pagination -->
		{#if totalPages > 1}
			<nav class="mt-6 flex items-center justify-between text-xs" style="color: var(--color-muted)">
				<span>
					{t('common.page', 'Page')} {currentPage} {t('common.of', 'of')} {totalPages}
				</span>
				<div class="flex gap-2">
					{#if currentPage > 1}
						<a
							href={`/ai/sessions?offset=${(currentPage - 2) * pageSize}`}
							class="rounded-md border px-2 py-1 transition-colors hover:bg-[var(--color-surface-subtle)]"
							style="border-color: var(--color-border)"
						>
							{t('common.prev', 'Previous')}
						</a>
					{/if}
					{#if currentPage < totalPages}
						<a
							href={`/ai/sessions?offset=${currentPage * pageSize}`}
							class="rounded-md border px-2 py-1 transition-colors hover:bg-[var(--color-surface-subtle)]"
							style="border-color: var(--color-border)"
						>
							{t('common.next', 'Next')}
						</a>
					{/if}
				</div>
			</nav>
		{/if}
	{/if}
</div>
