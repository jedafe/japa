<script lang="ts">
	// JapaAIChat — the ADR-130 unified chat surface. One transcript, one
	// composer, one history: the resolved tool catalog is what differs
	// between "chat" and "agent" turns (an empty catalog degenerates the
	// backend loop to JapaBot-class chat).
	//
	// Eligible users (agent.use) get a composer tools toggle + a per-tool/
	// category popover; everyone else gets plain chat. Streams over
	// POST /api/v1/ai/chat with Accept: text/event-stream (fetch SSE —
	// form actions cannot stream). Tool calls, progress, citations, and
	// token/cost footers render inline per turn.
	//
	// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md
	// Ref: japaconnect-engineering/sections/20-ai-core.md
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md

	import { t } from '$lib/i18n';
	import { tick } from 'svelte';
	import {
		Send,
		Sparkles,
		MessageSquare,
		Loader2,
		AlertCircle,
		ShieldCheck,
		Menu,
		X,
		Plus,
		Trash2,
		Wrench,
		SlidersHorizontal,
		CheckCircle2,
		XCircle,
		Search,
		Cpu,
		Database,
		BookOpen,
		Globe,
		FileText,
		Tag,
		Users as UsersIcon,
		GraduationCap,
		LayoutGrid,
		Square
	} from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { AgentRequestError } from './agentStream';
	import { streamChat, type ToolSelectionOverride } from './unifiedStream';
	import AiModelVoicePicker from './AiModelVoicePicker.svelte';
	import AiSourceCitations from './AiSourceCitations.svelte';
	import type { AiCitationSource } from './types';
	import type {
		AgentCapabilities,
		AgentEvent,
		AIPrefs,
		ConversationMessage,
		ConversationToolCall,
		UnifiedConversationSummary
	} from './agentTypes';

	interface Props {
		conversations: UnifiedConversationSummary[];
		models: { id: string; label: string }[];
		voices: { id: string; name: string }[];
		capabilities: AgentCapabilities | null;
		prefs: AIPrefs | null;
	}

	let { conversations, models, voices, capabilities, prefs }: Props = $props();

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

	interface UnifiedTurn {
		id: number;
		role: 'user' | 'assistant';
		content: string;
		sources?: AiCitationSource[];
		tool_calls?: ToolCallEntry[];
		session_id?: string;
		presearch_level?: string;
		tokens_used?: number;
		estimated_cost?: number;
		error?: string;
		in_progress?: boolean;
	}

	// Local copy of the sidebar list — a conversation created mid-session is
	// prepended when its `conversation` event arrives.
	let convos = $state<UnifiedConversationSummary[]>([]);
	$effect(() => {
		convos = [...conversations];
	});

	let turns = $state<UnifiedTurn[]>([]);
	let input = $state('');
	let conversationId = $state('');
	let sending = $state(false);
	let error = $state('');
	let scrollEl: HTMLElement | undefined = $state();
	let loadingConversation = $state(false);
	let sidebarOpen = $state(false);
	let turnSeq = 0;
	let toolSeq = 0;

	let activeAbort: AbortController | null = $state(null);
	let activeSessionId: string | null = $state(null);
	let cancelling = $state(false);

	// --- Tools state -------------------------------------------------------
	// Categories/tools come from GET /ai/agent/capabilities (already
	// platform+RBAC filtered). The composer can only narrow further — tools
	// the user's prefs disable render locked.
	const categories = $derived(capabilities?.categories ?? []);
	const toolsAvailable = $derived(categories.length > 0);

	// Per-tool enabled state keyed by function_name. Initialized from prefs
	// (disabled_tools / disabled_categories with enabled_tools exceptions —
	// mirrors AIPrefs.AllowsTool in internal/ai/prefs.go).
	function prefsAllows(name: string, category: string): boolean {
		if (!prefs) return true;
		if (prefs.disabled_tools?.includes(name)) return false;
		if (prefs.disabled_categories?.includes(category)) {
			return prefs.enabled_tools?.includes(name) ?? false;
		}
		return true;
	}

	let toolEnabled = $state<Record<string, boolean>>({});
	let toolsOn = $state(false);
	let toolsOpen = $state(false);
	let prefsSaved = $state(false);

	$effect(() => {
		// Seed once capabilities/prefs arrive (first non-empty categories).
		if (categories.length > 0 && Object.keys(toolEnabled).length === 0) {
			const next: Record<string, boolean> = {};
			for (const cat of categories) {
				for (const tool of cat.tools) {
					next[tool.name] = prefsAllows(tool.name, cat.name);
				}
			}
			toolEnabled = next;
			toolsOn = prefs?.default_mode === 'agent';
		}
	});

	const enabledToolCount = $derived(
		Object.values(toolEnabled).filter(Boolean).length
	);
	const totalToolCount = $derived(
		categories.reduce((n, c) => n + c.tools.length, 0)
	);
	// Tools the composer may toggle on — prefs-disabled tools are locked.
	const selectableCount = $derived(
		categories.reduce(
			(n, c) => n + c.tools.filter((tl) => prefsAllows(tl.name, c.name)).length,
			0
		)
	);

	function toolCategory(name: string): string {
		for (const cat of categories) {
			if (cat.tools.some((tl) => tl.name === name)) return cat.name;
		}
		return '';
	}

	function setCategory(name: string, on: boolean) {
		const cat = categories.find((c) => c.name === name);
		if (!cat) return;
		const next = { ...toolEnabled };
		for (const tool of cat.tools) {
			// Narrowing only — a prefs-disabled tool stays off.
			next[tool.name] = on && prefsAllows(tool.name, cat.name);
		}
		toolEnabled = next;
	}

	function categoryState(cat: { name: string; tools: { name: string }[] }): 'all' | 'some' | 'none' {
		const enabled = cat.tools.filter((tl) => toolEnabled[tl.name]).length;
		if (enabled === 0) return 'none';
		return enabled === cat.tools.length ? 'all' : 'some';
	}

	// Build the per-message `tools` override. Returns undefined when the
	// composer state matches prefs defaults (inherit), {enabled:false} when
	// the master switch is off, or a tool whitelist when narrowed.
	function toolsOverride(): ToolSelectionOverride | undefined {
		if (!toolsAvailable) return undefined;
		if (!toolsOn) return { enabled: false };
		const selected = Object.entries(toolEnabled)
			.filter(([, on]) => on)
			.map(([name]) => name);
		// FilterCatalog treats an empty whitelist as "no whitelist" — an
		// all-unchecked composer must send enabled:false, not tools:[].
		if (selected.length === 0) return { enabled: false };
		if (selected.length === selectableCount) return undefined;
		return { enabled: true, tools: selected };
	}

	// Serialize composer state back to AIPrefs without flattening category
	// semantics: a fully-off category stays a category (so future tools added
	// to it stay off); a partially-off category keeps the minority side as
	// exceptions — mostly-off → disabled_categories + enabled_tools,
	// mostly-on → disabled_tools.
	async function savePrefsAsDefault() {
		const disabledCats: string[] = [];
		const disabledTools: string[] = [];
		const enabledTools: string[] = [];
		for (const cat of categories) {
			const on = cat.tools.filter((tl) => toolEnabled[tl.name]).map((tl) => tl.name);
			const off = cat.tools.filter((tl) => !toolEnabled[tl.name]).map((tl) => tl.name);
			if (on.length === 0) {
				disabledCats.push(cat.name);
			} else if (off.length === 0) {
				// fully on — no pref entries
			} else if (on.length <= off.length) {
				disabledCats.push(cat.name);
				enabledTools.push(...on);
			} else {
				disabledTools.push(...off);
			}
		}
		const res = await api.put('/me/ai-preferences', {
			default_mode: toolsOn ? 'agent' : 'chat',
			disabled_categories: disabledCats,
			disabled_tools: disabledTools,
			enabled_tools: enabledTools
		});
		if (!res.error) {
			prefsSaved = true;
			setTimeout(() => (prefsSaved = false), 2000);
		}
	}

	// --- Composer pickers ---------------------------------------------------
	let selectedModel = $state('');
	let selectedVoice = $state('');
	const modelOptions = $derived([
		{ value: '', label: t('ai.chat.model_default', 'Default model') },
		...models.map((m) => ({ value: m.id, label: m.label }))
	]);
	const voiceOptions = $derived([
		{ value: '', label: t('ai.chat.voice_default', 'Default voice') },
		...voices.map((v) => ({ value: v.id, label: v.name }))
	]);

	const suggestions = $derived([
		t('ai.chat.suggestion_1', 'What documents do I need for a UK student visa?'),
		t('ai.chat.suggestion_2', 'How do I find scholarships for masters?'),
		t('ai.chat.suggestion_3', "What's the cost of living in Toronto vs Lagos?"),
		t('ai.chat.suggestion_4', 'How do I send money home cheaply?')
	]);

	// Auto-scroll on transcript growth or tool-call updates.
	$effect(() => {
		void turns.length;
		void turns[turns.length - 1]?.tool_calls?.length;
		if (scrollEl) {
			scrollEl.scrollTop = scrollEl.scrollHeight;
		}
	});

	// --- Conversation load/delete ------------------------------------------
	function turnFromToolCall(tc: ConversationToolCall): ToolCallEntry {
		return {
			id: ++toolSeq,
			tool_name: tc.tool_name,
			arguments: tc.arguments !== undefined ? JSON.stringify(tc.arguments) : undefined,
			status: statusToToolStatus(tc.status),
			result: tc.result !== undefined ? JSON.stringify(tc.result) : undefined,
			error: tc.error_message,
			iteration: 0
		};
	}

	async function loadConversation(id: string) {
		loadingConversation = true;
		const res = await api.get<{
			conversation_id: string;
			messages?: ConversationMessage[];
			entries?: { id: string; content: string; metadata?: Record<string, unknown>; created_at: string }[];
		}>(`/ai/conversations/${id}`);
		loadingConversation = false;
		if (res.error || !res.data) {
			error = res.error?.message ?? t('ai.chat.error', 'Failed to load conversation');
			return;
		}
		conversationId = res.data.conversation_id;
		error = '';
		if (res.data.messages) {
			// Unified thread — assistant messages may carry tool_calls.
			turns = res.data.messages.map((m) => ({
				id: ++turnSeq,
				role: m.role === 'user' ? 'user' : 'assistant',
				content: m.content,
				session_id: m.agent_session_id,
				tool_calls: m.tool_calls?.map(turnFromToolCall)
			}));
		} else {
			// Legacy Honcho thread — entries carry role in metadata.
			turns = (res.data.entries ?? []).map((e) => ({
				id: ++turnSeq,
				role: (e.metadata?.role === 'user' ? 'user' : 'assistant') as 'user' | 'assistant',
				content: e.content
			}));
		}
		sidebarOpen = false;
	}

	async function deleteConversation(id: string) {
		const res = await api.delete(`/ai/conversations/${id}`);
		if (!res.error) {
			convos = convos.filter((c) => c.conversation_id !== id);
			if (conversationId === id) startNewChat();
		}
	}

	function startNewChat() {
		conversationId = '';
		turns = [];
		sidebarOpen = false;
	}

	// --- Send / stream -------------------------------------------------------
	async function send() {
		const message = input.trim();
		if (!message || sending) return;

		error = '';
		input = '';
		sending = true;

		const userTurn: UnifiedTurn = { id: ++turnSeq, role: 'user', content: message };
		const assistantTurn: UnifiedTurn = {
			id: ++turnSeq,
			role: 'assistant',
			content: '',
			in_progress: true
		};
		turns = [...turns, userTurn, assistantTurn];
		// Mutate via the $state proxy — writes to the raw object bypass reactivity.
		const liveTurn = turns[turns.length - 1];

		const abort = new AbortController();
		activeAbort = abort;
		activeSessionId = null;
		cancelling = false;

		try {
			for await (const ev of streamChat({
				message,
				conversationId: conversationId || undefined,
				model: selectedModel || undefined,
				voiceId: selectedVoice || undefined,
				tools: toolsOverride(),
				signal: abort.signal
			})) {
				handleEvent(ev, liveTurn);
			}
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

	function handleEvent(ev: AgentEvent, turn: UnifiedTurn) {
		switch (ev.type) {
			case 'session_start': {
				turn.session_id = ev.session_id;
				turn.presearch_level = ev.presearch_level;
				activeSessionId = ev.session_id ?? null;
				turns = [...turns];
				break;
			}
			case 'conversation': {
				// ADR-130: carries the thread id — adopt it so follow-ups and the
				// sidebar resolve to the persisted conversation.
				if (ev.conversation_id) {
					conversationId = ev.conversation_id;
					if (!convos.some((c) => c.conversation_id === ev.conversation_id)) {
						convos = [
							{
								conversation_id: ev.conversation_id,
								turn_count: 0,
								legacy: false,
								preview: turns.find((tn) => tn.role === 'user')?.content?.slice(0, 80) ?? ''
							},
							...convos
						];
					}
				}
				break;
			}
			case 'sources': {
				turn.sources = ev.sources;
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
				turns = [...turns];
				break;
			}
			case 'tool_call_result': {
				const calls = turn.tool_calls ?? [];
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
			case 'progress': {
				if (ev.message) {
					turn.content = ev.message;
					turns = [...turns];
				}
				break;
			}
			case 'final_response': {
				turn.content = ev.content ?? '';
				turn.tokens_used = ev.tokens_used;
				turn.estimated_cost = ev.estimated_cost;
				if (ev.conversation_id) conversationId = ev.conversation_id;
				turn.in_progress = false;
				turns = [...turns];
				break;
			}
			case 'error': {
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
			try {
				await api.post(`/ai/agent/sessions/${activeSessionId}/cancel`);
			} catch {
				// Suppress — the stream is already aborted.
			}
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	// --- Presentation helpers ------------------------------------------------
	const toolIcon: Record<string, typeof Wrench> = {
		content: FileText,
		rag: BookOpen,
		external: Globe,
		custom_field: Tag,
		comparison: LayoutGrid,
		spaces: Globe,
		academy: GraduationCap,
		docs: FileText,
		tools: Wrench,
		menu: LayoutGrid,
		user: UsersIcon
	};

	function iconForTool(name: string): typeof Wrench {
		return toolIcon[name.split('.')[0] ?? ''] ?? Wrench;
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

	function fmtCost(c: number | undefined): string {
		if (!c) return '';
		return `$${c.toFixed(4)}`;
	}

	function fmtTokens(n: number | undefined): string {
		if (!n) return '';
		return n.toLocaleString();
	}
</script>

<div class="mx-auto flex min-h-0 w-full max-w-5xl flex-1 flex-col gap-4 lg:flex-row">
	<!-- Conversation sidebar -->
	<aside
		class="w-full lg:w-64 lg:shrink-0 {sidebarOpen
			? 'flex'
			: 'hidden lg:flex'} max-h-[50vh] lg:max-h-full flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
	>
		<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('ai.chat.history', 'Conversations')}</h2>
			<button
				type="button"
				onclick={startNewChat}
				class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
			>
				<Plus size={14} />
				{t('ai.chat.new', 'New')}
			</button>
		</div>
		<div class="flex-1 overflow-y-auto p-2">
			{#if convos.length === 0}
				<p class="p-2 text-center text-xs text-[var(--color-muted)]">{t('ai.chat.no_history', 'No previous conversations.')}</p>
			{:else}
				<ul class="space-y-1">
					{#each convos as conv (conv.conversation_id)}
						<li class="group relative">
							<button
								type="button"
								onclick={() => loadConversation(conv.conversation_id)}
								class="w-full rounded-[var(--radius)] px-3 py-2 text-left text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] {conversationId === conv.conversation_id ? 'bg-[var(--color-primary)]/10 font-medium' : ''}"
							>
								<div class="flex items-center gap-1.5">
									<span class="truncate">{conv.title || conv.preview || t('ai.chat.unnamed', 'Conversation')}</span>
									{#if conv.legacy}
										<span class="shrink-0 rounded-full border px-1.5 py-px text-[10px]" style="border-color: var(--color-border); color: var(--color-muted)">
											{t('ai.chat.legacy', 'legacy')}
										</span>
									{/if}
								</div>
								<div class="mt-0.5 text-xs text-[var(--color-muted)]">
									{conv.turn_count} {conv.turn_count === 1 ? t('ai.chat.turn', 'turn') : t('ai.chat.turns', 'turns')}
								</div>
							</button>
							{#if !conv.legacy}
								<button
									type="button"
									onclick={() => deleteConversation(conv.conversation_id)}
									class="absolute right-1.5 top-1.5 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-danger)] focus:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100"
									aria-label={t('ai.chat.delete', 'Delete conversation')}
									title={t('ai.chat.delete', 'Delete conversation')}
								>
									<Trash2 size={13} />
								</button>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</aside>

	<!-- Main chat -->
	<div class="flex flex-1 flex-col min-h-0">
		<header class="mb-4 flex items-center gap-3">
			<button
				type="button"
				onclick={() => (sidebarOpen = !sidebarOpen)}
				class="mr-2 inline-flex items-center gap-1 text-sm text-[var(--color-muted)] lg:hidden"
				aria-label={t('ai.chat.toggle_history', 'Toggle conversation history')}
			>
				{#if sidebarOpen}
					<X size={18} />
				{:else}
					<Menu size={18} />
				{/if}
			</button>
			<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background-color: var(--color-primary)">
				<Sparkles class="h-5 w-5 text-white" />
			</div>
			<div>
				<h1 class="text-xl font-bold tracking-tight">{t('ai.page.title', 'Japa AI')}</h1>
				<p class="text-xs" style="color: var(--color-muted)">
					{toolsAvailable
						? t('ai.page.subtitle_agent', 'Chat or use tools — switch anytime')
						: t('ai.chat.subtitle', 'Your AI assistant')}
				</p>
			</div>
		</header>

		<!-- Transcript -->
		<div
			bind:this={scrollEl}
			class="relative flex-1 overflow-y-auto rounded-lg border p-4"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			{#if turns.length === 0 && !loadingConversation}
				<div class="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
					<MessageSquare class="h-8 w-8" style="color: var(--color-muted)" />
					<p class="text-sm" style="color: var(--color-muted)">{t('ai.chat.empty', 'Start a conversation with Japa AI')}</p>
					<div class="mt-3 grid w-full max-w-md grid-cols-1 gap-2 sm:grid-cols-2">
						{#each suggestions as suggestion (suggestion)}
							<button
								type="button"
								onclick={async () => {
									input = suggestion;
									await tick();
									send();
								}}
								class="rounded-lg border p-3 text-left text-sm transition-colors hover:border-[var(--color-primary)]"
								style="border-color: var(--color-border); background-color: var(--color-surface-subtle); color: var(--color-muted)"
							>
								{suggestion}
							</button>
						{/each}
					</div>
				</div>
			{:else}
				{#if loadingConversation}
					<div class="absolute inset-0 z-10 flex items-center justify-center bg-[var(--color-surface)]/80">
						<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-primary)" />
					</div>
				{/if}
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
									{turn.content}
									{#if turn.in_progress && !turn.content}
										<span class="inline-flex items-center gap-1 text-xs" style="color: var(--color-muted)">
											<Loader2 size={12} class="animate-spin" />
											{t('ai.agent.thinking', 'Working...')}
										</span>
									{/if}
								</div>

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

								<!-- RAG source citations (Section 69, ADR-069) -->
								{#if turn.role === 'assistant' && turn.sources && turn.sources.length > 0}
									<AiSourceCitations sources={turn.sources} />
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

								{#if turn.error}
									<div class="mt-1.5 flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">
										<AlertCircle size={12} />
										{turn.error}
									</div>
								{/if}

								<!-- Immigration/financial content disclaimer (Section 69) -->
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

		<div class="mt-3">
			<AiModelVoicePicker
				bind:model={selectedModel}
				bind:voice={selectedVoice}
				{modelOptions}
				{voiceOptions}
				idPrefix="japaai"
			/>
		</div>

		<!-- Composer -->
		<div class="relative mt-2 flex items-end gap-2">
			<!-- Tools controls — only for users whose catalog is non-empty -->
			{#if toolsAvailable}
				<div class="relative">
					<button
						type="button"
						onclick={() => (toolsOpen = !toolsOpen)}
						class="flex h-10 items-center gap-1.5 rounded-lg border px-3 text-xs transition-colors {toolsOn ? 'font-medium' : ''}"
						style={toolsOn
							? 'border-color: var(--color-primary); color: var(--color-primary); background-color: color-mix(in srgb, var(--color-primary) 8%, transparent)'
							: 'border-color: var(--color-border); color: var(--color-muted)'}
						title={t('ai.tools.configure', 'Configure tools')}
					>
						<Wrench size={14} />
						{#if toolsOn}
							{enabledToolCount}
						{:else}
							{t('ai.tools.off', 'Off')}
						{/if}
					</button>

					{#if toolsOpen}
						<div
							class="absolute bottom-12 left-0 z-20 w-72 rounded-lg border p-3 shadow-lg"
							style="border-color: var(--color-border); background-color: var(--color-surface)"
						>
							<div class="mb-2 flex items-center justify-between">
								<span class="text-xs font-semibold" style="color: var(--color-text)">
									{t('ai.tools.title', 'Tools')}
								</span>
								<!-- Master per-conversation switch -->
								<button
									type="button"
									role="switch"
									aria-checked={toolsOn}
									onclick={() => (toolsOn = !toolsOn)}
									class="relative h-5 w-9 rounded-full transition-colors"
									style="background-color: {toolsOn ? 'var(--color-primary)' : 'var(--color-border)'}"
									aria-label={t('ai.tools.toggle', 'Toggle tools')}
								>
									<span
										class="absolute top-0.5 h-4 w-4 rounded-full bg-white transition-transform"
										style="left: 2px; transform: translateX({toolsOn ? '16px' : '0'})"
									></span>
								</button>
							</div>

							{#if toolsOn}
								<div class="max-h-56 space-y-2 overflow-y-auto">
									{#each categories as cat (cat.name)}
										<div>
											<button
												type="button"
												onclick={() => setCategory(cat.name, categoryState(cat) !== 'all')}
												class="flex w-full items-center gap-1.5 text-xs font-medium"
												style="color: var(--color-text)"
											>
												<SlidersHorizontal size={11} style="color: var(--color-muted)" />
												{cat.name}
												<span class="ml-auto" style="color: var(--color-muted)">
													{cat.tools.filter((tl) => toolEnabled[tl.name]).length}/{cat.tools.length}
												</span>
											</button>
											<div class="mt-1 space-y-0.5 pl-4">
												{#each cat.tools as tool (tool.name)}
													{@const locked = !prefsAllows(tool.name, cat.name)}
													<label class="flex items-center gap-1.5 text-xs {locked ? 'opacity-50' : 'cursor-pointer'}" style="color: var(--color-muted)">
														<input
															type="checkbox"
															checked={toolEnabled[tool.name] ?? false}
															disabled={locked}
															onchange={(e) => (toolEnabled = { ...toolEnabled, [tool.name]: e.currentTarget.checked })}
															class="h-3 w-3"
														/>
														<span class="font-mono">{tool.name}</span>
														{#if locked}
															<span class="text-[10px]">({t('ai.tools.disabled_in_settings', 'off in settings')})</span>
														{/if}
													</label>
												{/each}
											</div>
										</div>
									{/each}
								</div>

								<button
									type="button"
									onclick={savePrefsAsDefault}
									class="mt-2 w-full rounded-md border px-2 py-1 text-xs transition-colors hover:opacity-80"
									style="border-color: var(--color-border); color: var(--color-muted)"
								>
									{prefsSaved ? t('ai.tools.saved', 'Saved') : t('ai.tools.save_default', 'Use these as my defaults')}
								</button>
							{:else}
								<p class="text-xs" style="color: var(--color-muted)">
									{t('ai.tools.off_hint', 'Tools are off — messages are answered as plain chat.')}
								</p>
							{/if}
						</div>
					{/if}
				</div>
			{/if}

			<textarea
				bind:value={input}
				rows="1"
				placeholder={toolsOn
					? t('ai.agent.placeholder', 'Ask the agent to search, draft, or analyze...')
					: t('ai.chat.placeholder', 'Ask Japa AI anything...')}
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
					aria-label={t('ai.chat.send', 'Send')}
				>
					<Send class="h-5 w-5" />
				</button>
			{/if}
		</div>
	</div>
</div>
