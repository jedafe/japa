<script lang="ts">
	// Agent chat component — SSE consumer for POST /api/v1/ai/agent.
	// Displays the live agent loop: session start, tool calls (with status),
	// progress messages, the final response, and errors. Supports cancellation
	// via POST /api/v1/ai/agent/sessions/{id}/cancel + an AbortController.
	//
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Agent Loop, SSE Streaming)
	// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, task 10.01)

	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import {
		Send,
		Sparkles,
		Loader2,
		AlertCircle,
		Wrench,
		CheckCircle2,
		XCircle,
		ShieldCheck,
		Square,
		Clock,
		Cpu,
		Database,
		Search,
		BookOpen,
		Globe,
		FileText,
		Tag,
		Users as UsersIcon,
		GraduationCap,
		LayoutGrid
	} from 'lucide-svelte';
	import { streamAgent, AgentRequestError } from '$lib/ai/agentStream';
	import type { AgentEvent, AgentCapabilities } from '$lib/ai/agentTypes';

	type ToolStatus = 'pending' | 'success' | 'error' | 'unauthorized' | 'privacy_blocked' | 'hitl_queued';

	interface ToolCallEntry {
		id: number;
		tool_name: string;
		arguments?: string;
		status: ToolStatus;
		result?: string;
		error?: string;
		iteration: number;
	}

	interface ChatTurn {
		id: number;
		role: 'user' | 'assistant';
		content: string;
		// ADR-130 Phase 2: accumulated `delta` fragments for the in-flight
		// reply — rendered in place of content while non-empty; cleared on
		// final_response which carries the authoritative text.
		streamed?: string;
		tool_calls?: ToolCallEntry[];
		session_id?: string;
		presearch_level?: string;
		tokens_used?: number;
		estimated_cost?: number;
		error?: string;
		in_progress?: boolean;
	}

	let { capabilities }: { capabilities: AgentCapabilities | null } = $props();

	let turns = $state<ChatTurn[]>([]);
	let input = $state('');
	let inputEl: HTMLTextAreaElement | undefined = $state();
	let sending = $state(false);
	let error = $state('');
	let scrollEl: HTMLElement | undefined = $state();
	let turnSeq = 0;
	let toolSeq = 0;

	// Cancellation state — the AbortController cancels the fetch (and the
	// backend loop detects ctx cancellation). The cancel endpoint also sets
	// the jc:agent:session:{id} Valkey flag so a stuck loop breaks.
	let activeAbort: AbortController | null = $state(null);
	let activeSessionId: string | null = $state(null);
	let cancelling = $state(false);

	// Auto-scroll to the latest content whenever the transcript grows or a
	// tool call updates.
	$effect(() => {
		void turns.length;
		void turns[turns.length - 1]?.tool_calls?.length;
		if (scrollEl) {
			scrollEl.scrollTop = scrollEl.scrollHeight;
		}
	});

	// Icon per tool namespace — helps the user scan the tool-call log.
	const toolIcon: Record<string, typeof Wrench> = {
		'content': FileText,
		'rag': BookOpen,
		'external': Globe,
		'custom_field': Tag,
		'comparison': LayoutGrid,
		'spaces': Globe,
		'academy': GraduationCap,
		'docs': FileText,
		'tools': Wrench,
		'menu': LayoutGrid,
		'user': UsersIcon
	};

	function iconForTool(name: string): typeof Wrench {
		const ns = name.split('.')[0] ?? '';
		return toolIcon[ns] ?? Wrench;
	}

	function statusToToolStatus(s: string | undefined): ToolStatus {
		switch (s) {
			case 'success': return 'success';
			case 'error': return 'error';
			case 'unauthorized': return 'unauthorized';
			case 'privacy_blocked': return 'privacy_blocked';
			case 'hitl_queued': return 'hitl_queued';
			default: return 'pending';
		}
	}

	function statusColor(status: ToolStatus): string {
		switch (status) {
			case 'success': return 'var(--color-success, #16a34a)';
			case 'error': return 'var(--color-danger, #dc2626)';
			case 'unauthorized': return 'var(--color-warning, #d97706)';
			case 'privacy_blocked': return 'var(--color-warning, #d97706)';
			case 'hitl_queued': return 'var(--color-info, #2563eb)';
			default: return 'var(--color-muted)';
		}
	}

	function statusLabel(status: ToolStatus): string {
		switch (status) {
			case 'success': return t('ai.agent.tool.success', 'Success');
			case 'error': return t('ai.agent.tool.error', 'Error');
			case 'unauthorized': return t('ai.agent.tool.unauthorized', 'Denied');
			case 'privacy_blocked': return t('ai.agent.tool.privacy_blocked', 'Privacy blocked');
			case 'hitl_queued': return t('ai.agent.tool.hitl_queued', 'Awaiting review');
			case 'pending': return t('ai.agent.tool.pending', 'Running');
		}
	}

	async function send() {
		const prompt = input.trim();
		if (!prompt || sending) return;

		error = '';
		input = '';
		sending = true;

		const userTurn: ChatTurn = { id: ++turnSeq, role: 'user', content: prompt };
		const assistantTurn: ChatTurn = {
			id: ++turnSeq,
			role: 'assistant',
			content: '',
			in_progress: true
		};
		turns = [...turns, userTurn, assistantTurn];
		// Mutations must go through the $state proxy — writes to the raw
		// assistantTurn object bypass reactivity and leave the row frozen at
		// its initial render. Indexing turns returns the reactive proxy.
		const liveTurn = turns[turns.length - 1];

		const abort = new AbortController();
		activeAbort = abort;
		activeSessionId = null;
		cancelling = false;

		try {
			for await (const ev of streamAgent({ prompt, signal: abort.signal })) {
				handleEvent(ev, liveTurn);
			}
			// Stream closed — if the assistant turn never got a final_response,
			// mark it complete with whatever content we have.
			if (liveTurn.in_progress) {
				liveTurn.in_progress = false;
				if (!liveTurn.content && !liveTurn.error) {
					liveTurn.content = t('ai.agent.stream_closed', 'Stream closed.');
				}
				turns = [...turns];
			}
		} catch (e) {
			liveTurn.in_progress = false;
			if (e instanceof AgentRequestError) {
				liveTurn.error = e.message;
				error = e.message;
				// Remove the empty assistant turn if it never produced any content.
				if (!liveTurn.content && !liveTurn.tool_calls?.length) {
					turns = turns.filter((tn) => tn.id !== liveTurn.id);
				}
			} else if (e instanceof DOMException && e.name === 'AbortError') {
				liveTurn.error = t('ai.agent.cancelled', 'Cancelled.');
			} else {
				liveTurn.error = e instanceof Error ? e.message : String(e);
				error = liveTurn.error;
			}
			turns = [...turns];
		} finally {
			sending = false;
			activeAbort = null;
			activeSessionId = null;
			cancelling = false;
		}
	}

	function handleEvent(ev: AgentEvent, turn: ChatTurn) {
		switch (ev.type) {
			case 'session_start': {
				turn.session_id = ev.session_id;
				turn.presearch_level = ev.presearch_level;
				activeSessionId = ev.session_id ?? null;
				turns = [...turns];
				break;
			}
			case 'tool_call_start': {
				const entry: ToolCallEntry = {
					id: ++toolSeq,
					tool_name: ev.tool_name ?? '',
					arguments: ev.arguments,
					status: 'pending',
					iteration: ev.iteration ?? 0
				};
				turn.tool_calls = [...(turn.tool_calls ?? []), entry];
				// Any deltas accumulated before this call were preamble text
				// from an intermediate iteration — transient, never persisted.
				// Clear so the final iteration streams into a clean bubble.
				turn.streamed = '';
				turns = [...turns];
				break;
			}
			case 'tool_call_result': {
				const calls = turn.tool_calls ?? [];
				// Match the tool call by name + pending status. If the result
				// event doesn't carry a tool_name (omitempty), fall back to the
				// last pending tool call — the agent loop is sequential so only
				// one tool call is pending at a time.
				let matched = false;
				for (let i = calls.length - 1; i >= 0; i--) {
					if (calls[i].status === 'pending' && (!ev.tool_name || calls[i].tool_name === ev.tool_name)) {
						calls[i].status = statusToToolStatus(ev.status);
						calls[i].result = ev.content;
						calls[i].error = ev.error;
						matched = true;
						break;
					}
				}
				if (!matched) {
					// No pending call matched — update the last call as fallback.
					const last = calls[calls.length - 1];
					if (last) {
						last.status = statusToToolStatus(ev.status);
						last.result = ev.content;
						last.error = ev.error;
					}
				}
				turn.tool_calls = [...calls];
				turns = [...turns];
				break;
			}
			case 'delta': {
				// Token-level fragment (ADR-130 Phase 2) — accumulate; the
				// template renders streamed text over the progress placeholder.
				turn.streamed = (turn.streamed ?? '') + (ev.content ?? '');
				turns = [...turns];
				break;
			}
			case 'progress': {
				// Progress events carry a transient status message. Update the
				// assistant content to the latest progress message so the user
				// sees current activity. The final_response event overwrites
				// this with the actual answer.
				if (ev.message) {
					turn.content = ev.message;
					turns = [...turns];
				}
				break;
			}
			case 'final_response': {
				turn.content = ev.content ?? '';
				turn.streamed = '';
				turn.tokens_used = ev.tokens_used;
				turn.estimated_cost = ev.estimated_cost;
				turn.in_progress = false;
				turns = [...turns];
				break;
			}
			case 'error': {
				// Promote any streamed partial into committed content so the
				// bubble shows what arrived before the failure, then the error.
				// The partial wins over the transient progress placeholder
				// that progress events write into content.
				if (turn.streamed) {
					turn.content = turn.streamed;
				}
				turn.streamed = '';
				turn.error = ev.error ?? 'Unknown error';
				turn.in_progress = false;
				turns = [...turns];
				break;
			}
		}
	}

	async function cancel() {
		if (!activeAbort || cancelling) return;
		cancelling = true;
		activeAbort.abort();
		if (activeSessionId) {
			// Best-effort server-side cancel — sets the jc:agent:session:{id}
			// flag so a stuck loop breaks on its next iteration check. Errors
			// are suppressed because the AbortController already cancelled the
			// fetch; the backend detects ctx cancellation independently.
			try {
				await api.post(`/ai/agent/sessions/${activeSessionId}/cancel`);
			} catch {
				// Suppress — the stream is already aborted.
			}
		}
	}

	function resizeInput() {
		if (!inputEl) return;
		inputEl.style.height = 'auto';
		inputEl.style.height = Math.min(inputEl.scrollHeight, 160) + 'px';
	}

	$effect(() => {
		// Grow/shrink the composer with its content (incl. reset after send).
		// Measure after paint — on mount the flex width may not be resolved.
		input;
		const id = requestAnimationFrame(resizeInput);
		window.addEventListener('resize', resizeInput);
		return () => {
			cancelAnimationFrame(id);
			window.removeEventListener('resize', resizeInput);
		};
	});

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	function fmtCost(c: number | undefined): string {
		if (!c) return '';
		return `$${c.toFixed(4)}`;
	}

	function fmtTokens(n: number | undefined): string {
		if (!n) return '';
		return n.toLocaleString();
	}
</script>

<div class="flex h-full flex-col">
	<!-- Transcript -->
	<div
		bind:this={scrollEl}
		class="flex-1 overflow-y-auto rounded-lg border p-4"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
	>
		{#if turns.length === 0}
			<div class="flex h-full flex-col items-center justify-center gap-3 text-center">
				<div class="flex h-12 w-12 items-center justify-center rounded-full" style="background-color: var(--color-primary)">
					<Sparkles class="h-6 w-6 text-white" />
				</div>
				<p class="text-sm font-medium">{t('ai.agent.empty_title', 'Ask the agent')}</p>
				<p class="text-xs max-w-sm" style="color: var(--color-muted)">
					{t('ai.agent.empty_desc', 'The agent can search content, read custom fields, run RAG lookups, and draft posts — then show you every tool call it made.')}
				</p>
				{#if capabilities?.tools.length}
					<div class="mt-2 flex flex-wrap items-center justify-center gap-1.5 max-w-lg">
						{#each capabilities.tools.slice(0, 12) as tool (tool.function_name)}
							<span
								class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
								style="border-color: var(--color-border); color: var(--color-muted)"
								title={tool.description}
							>
								<Wrench size={10} />
								{tool.function_name}
							</span>
						{/each}
						{#if capabilities.tools.length > 12}
							<span class="text-xs" style="color: var(--color-muted)">
								+{capabilities.tools.length - 12}
							</span>
						{/if}
					</div>
				{/if}
			</div>
		{:else}
			<div class="space-y-4">
				{#each turns as turn (turn.id)}
					<div class="flex {turn.role === 'user' ? 'justify-end' : 'justify-start'}">
						<div class="max-w-[85%] w-full">
							<div
								class="rounded-2xl px-4 py-2 text-sm whitespace-pre-wrap"
								style={turn.role === 'user'
									? 'background-color: var(--color-primary); color: white'
									: 'background-color: var(--color-muted-15, color-mix(in srgb, var(--color-muted) 15%, transparent)); color: var(--color-text)'}
							>
								{turn.streamed || turn.content}
								{#if turn.in_progress && !turn.content && !turn.streamed}
									<span class="inline-flex items-center gap-1 text-xs" style="color: var(--color-muted)">
										<Loader2 size={12} class="animate-spin" />
										{t('ai.agent.thinking', 'Working...')}
									</span>
								{/if}
							</div>

							<!-- Pre-search level badge -->
							{#if turn.role === 'assistant' && turn.presearch_level}
								<div class="mt-1.5 flex items-center gap-1.5 text-xs" style="color: var(--color-muted)">
									<Search size={11} />
									{t('ai.agent.presearch', 'Pre-search')}: <span class="font-medium">{turn.presearch_level}</span>
								</div>
							{/if}

							<!-- Tool call log -->
							{#if turn.tool_calls && turn.tool_calls.length > 0}
								<div class="mt-2 space-y-1.5">
									{#each turn.tool_calls as tc (tc.id)}
									{@const Icon = iconForTool(tc.tool_name)}
										<div
											class="rounded-md border px-2.5 py-1.5 text-xs"
											style="border-color: var(--color-border); background-color: var(--color-background)"
										>
											<div class="flex items-center gap-2">
												<Icon size={12} style="color: var(--color-muted)" />
												<span class="font-mono font-medium">{tc.tool_name}</span>
												<span
													class="ml-auto inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-xs font-medium"
													style="background-color: color-mix(in srgb, {statusColor(tc.status)} 12%, transparent); color: {statusColor(tc.status)}"
												>
													{#if tc.status === 'pending'}
														<Loader2 size={10} class="animate-spin" />
													{:else if tc.status === 'success'}
														<CheckCircle2 size={10} />
													{:else if tc.status === 'error'}
														<XCircle size={10} />
													{/if}
													{statusLabel(tc.status)}
												</span>
											</div>
											{#if tc.arguments}
												<details class="mt-1">
													<summary class="cursor-pointer text-xs" style="color: var(--color-muted)">
														{t('ai.agent.tool.arguments', 'Arguments')}
													</summary>
													<pre class="mt-1 overflow-x-auto rounded p-1.5 text-xs font-mono" style="background-color: var(--color-surface)">{tc.arguments}</pre>
												</details>
											{/if}
											{#if tc.result}
												<details class="mt-1">
													<summary class="cursor-pointer text-xs" style="color: var(--color-muted)">
														{t('ai.agent.tool.result', 'Result')}
													</summary>
													<pre class="mt-1 max-h-48 overflow-auto rounded p-1.5 text-xs font-mono" style="background-color: var(--color-surface)">{tc.result}</pre>
												</details>
											{/if}
											{#if tc.error}
												<p class="mt-1 text-xs" style="color: var(--color-danger)">{tc.error}</p>
											{/if}
										</div>
									{/each}
								</div>
							{/if}

							<!-- Token + cost footer -->
							{#if turn.role === 'assistant' && (turn.tokens_used || turn.estimated_cost) && !turn.in_progress}
								<div class="mt-1.5 flex items-center gap-3 text-xs" style="color: var(--color-muted)">
									{#if turn.tokens_used}
										<span class="inline-flex items-center gap-1"><Cpu size={11} /> {fmtTokens(turn.tokens_used)} {t('ai.agent.tokens', 'tokens')}</span>
									{/if}
									{#if turn.estimated_cost}
										<span class="inline-flex items-center gap-1"><Database size={11} /> {fmtCost(turn.estimated_cost)}</span>
									{/if}
								</div>
							{/if}

							<!-- Error -->
							{#if turn.error}
								<div class="mt-1.5 flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">
									<AlertCircle size={12} />
									{turn.error}
								</div>
							{/if}

							<!-- Disclaimer -->
							{#if turn.role === 'assistant' && turn.content && !turn.in_progress}
								<p class="mt-1 flex items-center gap-1 text-xs" style="color: var(--color-muted)">
									<ShieldCheck size={11} />
									{t('ai.chat.disclaimer', 'AI-generated. Verify immigration/financial details with official sources.')}
								</p>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	{#if error}
		<div class="mt-3 flex items-center gap-2 rounded-md border p-2 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			<AlertCircle class="h-4 w-4" />
			{error}
		</div>
	{/if}

	<!-- Composer -->
	<div class="mt-3 flex items-end gap-2">
		<textarea
			bind:this={inputEl}
			bind:value={input}
			rows="1"
			placeholder={t('ai.agent.placeholder', 'Ask the agent to search, draft, or analyze...')}
			oninput={resizeInput}
			class="flex-1 resize-none rounded-lg border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-background)"
			onkeydown={onKeydown}
			disabled={sending}
		></textarea>
		{#if sending}
			<button
				type="button"
				onclick={cancel}
				disabled={cancelling}
				class="flex h-10 w-10 items-center justify-center rounded-lg border transition-colors disabled:opacity-50"
				style="border-color: var(--color-danger); color: var(--color-danger)"
				aria-label={t('ai.agent.cancel', 'Cancel')}
				title={t('ai.agent.cancel', 'Cancel')}
			>
				{#if cancelling}
					<Loader2 class="h-5 w-5 animate-spin" />
				{:else}
					<Square class="h-4 w-4" />
				{/if}
			</button>
		{:else}
			<button
				type="button"
				onclick={send}
				disabled={!input.trim()}
				class="flex h-10 w-10 items-center justify-center rounded-lg text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				aria-label={t('ai.agent.send', 'Send')}
			>
				<Send class="h-5 w-5" />
			</button>
		{/if}
	</div>
</div>
