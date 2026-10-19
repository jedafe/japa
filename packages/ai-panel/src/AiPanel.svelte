<script lang="ts">
	// AiPanel — the §115 page-aware quick-chat drawer. Lightweight sibling
	// of the /ai unified surface (ADR-130): same POST /ai/chat SSE contract,
	// trimmed to transcript + composer + a context chip. Threads are
	// anchored per page (server-side ai_conversations.anchor) so reopening
	// the panel on the same page resumes the same conversation.
	//
	// Ref: japaconnect-engineering/sections/115-ai-quick-panel.md

	import { onDestroy, tick } from 'svelte';
	import {
		Sparkles,
		X,
		ArrowUp,
		Square,
		ExternalLink,
		Loader2,
		CheckCircle2,
		XCircle,
		AlertCircle,
		Clock
	} from 'lucide-svelte';
	import { apiGet, apiPost, getAiPanelDeps, pt } from './deps';
	import { createStreamChat } from './stream';
	import { aiPageContext } from './context.svelte';
	import {
		AgentRequestError,
		type AgentEvent,
		type AiCitationSource,
		type ConversationMessage
	} from './types';

	interface PanelToolCall {
		name: string;
		status: string;
	}
	interface PanelTurn {
		id: number;
		role: 'user' | 'assistant';
		content: string;
		in_progress?: boolean;
		progress?: string;
		tool_calls?: PanelToolCall[];
		sources?: AiCitationSource[];
		error?: string;
		session_id?: string | null;
	}

	interface Props {
		open: boolean;
		/** null → guest → sign-in CTA. */
		user: { permissions?: string[] } | null;
		onclose: () => void;
	}

	let { open, user, onclose }: Props = $props();

	let turns = $state<PanelTurn[]>([]);
	let conversationId = $state('');
	let sending = $state(false);
	let composer = $state('');
	let loadedAnchor = $state('');
	let agentCapable = $state<boolean | null>(null);
	let mode = $state<'chat' | 'agent'>('agent'); // the panel's purpose is agent ops
	let loadingThread = $state(false);
	let turnSeq = 0;
	let activeAbort: AbortController | null = null;
	let transcriptEl = $state<HTMLElement | null>(null);
	let composerEl = $state<HTMLTextAreaElement | null>(null);

	const ctxNow = $derived(aiPageContext.current);
	const anchor = $derived(ctxNow?.anchor ?? 'panel:general');
	const streamChat = createStreamChat();

	// --- Detached-turn polling (trimmed from JapaAIChat) ------------------
	let bgSessionId = $state<string | null>(null);
	let bgPollTimer: ReturnType<typeof setInterval> | null = null;
	let bgCancelling = $state(false);
	const BG_POLL_MS = 3000;
	const BG_POLL_CAP_MS = 10 * 60 * 1000;

	function stopSessionPoll() {
		if (bgPollTimer) clearInterval(bgPollTimer);
		bgPollTimer = null;
		bgSessionId = null;
		bgCancelling = false;
	}

	async function findRunningSessionId(convId: string): Promise<string> {
		const res = await apiGet<{ running_session_id?: string }>(`/ai/conversations/${convId}`);
		return res?.running_session_id ?? '';
	}

	async function cancelBackgroundSession() {
		const sid = bgSessionId;
		if (!sid || bgCancelling) return;
		bgCancelling = true;
		try {
			await apiPost(`/ai/agent/sessions/${sid}/cancel`);
		} finally {
			bgCancelling = false;
		}
	}

	function startSessionPoll(sessionId: string) {
		if (!sessionId || bgSessionId === sessionId) return;
		stopSessionPoll();
		bgSessionId = sessionId;
		const sid = sessionId;
		const startedAt = Date.now();
		let failures = 0;
		let inFlight = false;
		bgPollTimer = setInterval(() => {
			if (bgSessionId !== sid || inFlight) return;
			if (Date.now() - startedAt > BG_POLL_CAP_MS) {
				stopSessionPoll();
				return;
			}
			inFlight = true;
			void (async () => {
				try {
					const res = await apiGet<{
						session: { status: string; conversation_id?: string };
					}>(`/ai/agent/sessions/${sid}`);
					if (bgSessionId !== sid) return;
					if (res === null) {
						if (++failures >= 2) stopSessionPoll();
						return;
					}
					failures = 0;
					const status = res?.session?.status;
					if (status && status !== 'running') {
						stopSessionPoll();
						const stuck = turns.find((tn) => tn.in_progress && tn.session_id === sid);
						if (stuck) {
							stuck.in_progress = false;
							stuck.progress = '';
						}
						const convId = res?.session?.conversation_id || conversationId || '';
						if (!sending && convId) await loadAnchoredThread(loadedAnchor);
					}
				} finally {
					inFlight = false;
				}
			})();
		}, BG_POLL_MS);
	}

	onDestroy(() => {
		stopSessionPoll();
		activeAbort?.abort();
	});

	// --- Thread resume ---------------------------------------------------

	async function probeCapabilities() {
		const caps = await apiGet<{ categories?: unknown[] }>('/ai/agent/capabilities');
		agentCapable = Array.isArray(caps?.categories) && caps.categories.length > 0;
	}

	async function loadAnchoredThread(a: string) {
		loadingThread = true;
		loadedAnchor = a;
		const list = await apiGet<{ conversation_id: string }[]>(
			`/ai/conversations?anchor=${encodeURIComponent(a)}`
		);
		if (list === null) {
			// Fetch failed — keep whatever is already displayed rather than
			// blanking the transcript on a transient error.
			if (loadedAnchor === a) loadingThread = false;
			return;
		}
		const conv = list[0];
		if (!conv) {
			if (loadedAnchor === a) {
				conversationId = '';
				turns = [];
				loadingThread = false;
			}
			return;
		}
		const detail = await apiGet<{
			conversation_id: string;
			running_session_id?: string;
			messages?: ConversationMessage[];
		}>(`/ai/conversations/${conv.conversation_id}`);
		// A navigation may have started a newer load — don't clobber it.
		if (loadedAnchor !== a) return;
		conversationId = detail?.conversation_id ?? conv.conversation_id;
		turns = (detail?.messages ?? []).map((m) => ({
			id: ++turnSeq,
			role: m.role,
			content: m.content,
			session_id: m.agent_session_id,
			tool_calls: m.tool_calls?.map((tc) => ({ name: tc.tool_name, status: tc.status }))
		}));
		loadingThread = false;
		scrollToBottom();
		if (detail?.running_session_id && !sending) startSessionPoll(detail.running_session_id);
	}

	// Load on open → anchor-change while open → reopen (fresh copy each
	// time, so another device's appends show up).
	let prevOpen = false;
	$effect(() => {
		if (open && user && (!prevOpen || anchor !== loadedAnchor)) {
			void loadAnchoredThread(anchor);
		}
		prevOpen = open;
	});
	$effect(() => {
		if (open && user && agentCapable === null) void probeCapabilities();
	});
	// Move focus into the composer when the panel opens (a11y: the dialog
	// is modal, focus should land inside it).
	$effect(() => {
		if (open && user) void tick().then(() => composerEl?.focus());
	});

	function scrollToBottom() {
		void tick().then(() => {
			if (transcriptEl) transcriptEl.scrollTop = transcriptEl.scrollHeight;
		});
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			void send();
		}
	}
	function onEsc(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}

	// --- Send ------------------------------------------------------------

	function handleEvent(ev: AgentEvent, turn: PanelTurn) {
		switch (ev.type) {
			case 'session_start':
				turn.session_id = ev.session_id ?? null;
				break;
			case 'progress':
				turn.progress = ev.message ?? '';
				break;
			case 'tool_call_start':
				turn.tool_calls = [
					...(turn.tool_calls ?? []),
					{ name: ev.tool_name ?? '', status: 'running' }
				];
				break;
			case 'tool_call_result': {
				const tc = turn.tool_calls?.findLast(
					(t) => t.name === ev.tool_name && t.status === 'running'
				);
				if (tc) tc.status = ev.status ?? 'done';
				break;
			}
			case 'delta':
				turn.content += ev.content ?? '';
				break;
			case 'sources':
				turn.sources = ev.sources ?? [];
				break;
			case 'conversation':
				if (ev.conversation_id) conversationId = ev.conversation_id;
				break;
			case 'final_response':
				turn.in_progress = false;
				if (ev.content) turn.content = ev.content;
				if (ev.conversation_id) conversationId = ev.conversation_id;
				break;
			case 'error':
				turn.in_progress = false;
				turn.error = ev.error ?? pt('ai.panel.error', 'Something went wrong.');
				break;
		}
		turns = [...turns];
		scrollToBottom();
	}

	async function send() {
		const message = composer.trim();
		if (!message || sending) return;
		composer = '';
		sending = true;
		turns = [
			...turns,
			{ id: ++turnSeq, role: 'user', content: message },
			{ id: ++turnSeq, role: 'assistant', content: '', in_progress: true, tool_calls: [] }
		];
		const live = turns[turns.length - 1];
		const abort = new AbortController();
		activeAbort = abort;
		scrollToBottom();
		try {
			for await (const ev of streamChat({
				message,
				conversationId: conversationId || undefined,
				anchor: conversationId ? undefined : anchor,
				context: ctxNow?.context,
				tools: agentCapable && mode === 'agent' ? { enabled: true } : { enabled: false },
				signal: abort.signal
			})) {
				handleEvent(ev, live);
			}
			if (live.in_progress) {
				const sid =
					live.session_id ?? (conversationId ? await findRunningSessionId(conversationId) : '');
				if (sid) {
					live.progress = pt('ai.panel.bg_running', 'Working in the background…');
					live.session_id = sid;
					startSessionPoll(sid);
				} else {
					live.in_progress = false;
					if (!live.content && !live.error)
						live.content = pt('ai.panel.stream_closed', 'Stream closed.');
				}
				turns = [...turns];
			}
		} catch (e) {
			live.in_progress = false;
			if (!abort.signal.aborted) {
				// User-stopped turns keep their partial content silently.
				live.error =
					e instanceof AgentRequestError
						? e.message
						: pt('ai.panel.error', 'Something went wrong.');
				if (!composer) composer = message; // don't eat the user's draft
			}
			turns = [...turns];
		} finally {
			sending = false;
			activeAbort = null;
		}
	}

	function stop() {
		activeAbort?.abort();
	}
</script>

<svelte:window onkeydown={open ? onEsc : undefined} />

{#if open}
	<!-- §115: scrim — click outside the panel closes it. A <button> keeps
	     the a11y linter happy (interactive element, labeled). -->
	<button
		type="button"
		class="ai-panel-backdrop"
		aria-label={pt('ai.panel.close', 'Close')}
		tabindex="-1"
		onclick={onclose}
	></button>
	<div
		class="ai-panel"
		role="dialog"
		aria-modal="true"
		aria-label={pt('ai.panel.title', 'Japa AI')}
	>
		<header class="ai-panel-head">
			<span class="ai-panel-title">
				<Sparkles size={16} />
				{pt('ai.panel.title', 'Japa AI')}
			</span>
			{#if ctxNow?.label}
				<span class="ai-panel-chip" title={ctxNow.label}>{ctxNow.label}</span>
			{/if}
			<span class="ai-panel-head-spacer"></span>
			{#if user}
				<a
					href={getAiPanelDeps().fullChatUrl(conversationId || undefined)}
					class="ai-panel-iconbtn"
					aria-label={pt('ai.panel.open_full', 'Open in Japa AI')}
					title={pt('ai.panel.open_full', 'Open in Japa AI')}
				>
					<ExternalLink size={15} />
				</a>
			{/if}
			<button
				type="button"
				class="ai-panel-iconbtn"
				aria-label={pt('ai.panel.close', 'Close')}
				onclick={onclose}
			>
				<X size={16} />
			</button>
		</header>

		{#if !user}
			<div class="ai-panel-guest">
				<Sparkles size={28} />
				<p>{pt('ai.panel.sign_in_help', 'Japa AI is available to signed-in members.')}</p>
				<a href={getAiPanelDeps().fullChatUrl()} class="ai-panel-signin">
					{pt('ai.panel.sign_in', 'Sign in to chat')}
				</a>
			</div>
		{:else}
			<div class="ai-panel-transcript" bind:this={transcriptEl}>
				{#if loadingThread}
					<div class="ai-panel-loading">
						<Loader2 size={18} class="ai-spin" />
					</div>
				{:else if turns.length === 0}
					<p class="ai-panel-welcome">
						{pt('ai.panel.welcome', "Ask me anything — I can see what page you're on.")}
					</p>
				{/if}
				{#each turns as turn (turn.id)}
					<div class="ai-turn" class:ai-turn-user={turn.role === 'user'}>
						{#if turn.tool_calls?.length}
							<div class="ai-toolcalls">
								{#each turn.tool_calls as tc, i (i)}
									<span class="ai-toolpill" data-status={tc.status}>
										{#if tc.status === 'running'}
											<Loader2 size={11} class="ai-spin" />
										{:else if tc.status === 'success' || tc.status === 'done'}
											<CheckCircle2 size={11} />
										{:else if tc.status === 'hitl_queued' || tc.status === 'pending'}
											<Clock size={11} />
										{:else}
											<XCircle size={11} />
										{/if}
										{tc.name}
									</span>
								{/each}
							</div>
						{/if}
						{#if turn.content}
							<div class="ai-bubble">{turn.content}</div>
						{/if}
						{#if turn.in_progress}
							<div class="ai-progress">
								<Loader2 size={12} class="ai-spin" />
								{turn.progress || pt('ai.panel.thinking', 'Thinking…')}
								{#if turn.session_id && turn.session_id === bgSessionId}
									<button type="button" class="ai-linkbtn" onclick={cancelBackgroundSession}>
										{pt('ai.panel.cancel', 'Cancel')}
									</button>
								{/if}
							</div>
						{/if}
						{#if turn.error}
							<div class="ai-error"><AlertCircle size={12} /> {turn.error}</div>
						{/if}
						{#if turn.sources?.length}
							<div class="ai-sources">
								{#each turn.sources as s, i (i)}
									{#if s.url}
										<a class="ai-source" href={s.url}>{s.title ?? s.slug ?? s.url}</a>
									{:else}
										<span class="ai-source">{s.title ?? s.slug ?? ''}</span>
									{/if}
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>

			<footer class="ai-panel-composer">
				{#if agentCapable}
					<!-- Chat vs Agent mode: agent turns get the tool catalog and
					     can act on the current page; chat turns are read-only. -->
					<div
						class="ai-mode-switch"
						role="radiogroup"
						aria-label={pt('ai.panel.mode', 'Mode')}
					>
						<button
							type="button"
							role="radio"
							aria-checked={mode === 'chat'}
							class:active={mode === 'chat'}
							onclick={() => (mode = 'chat')}
						>
							{pt('ai.panel.mode_chat', 'Chat')}
						</button>
						<button
							type="button"
							role="radio"
							aria-checked={mode === 'agent'}
							class:active={mode === 'agent'}
							onclick={() => (mode = 'agent')}
						>
							{pt('ai.panel.mode_agent', 'Agent')}
						</button>
					</div>
				{/if}
				<div class="ai-panel-inputrow">
					<textarea
						bind:this={composerEl}
						bind:value={composer}
						onkeydown={onKeydown}
						rows="2"
						placeholder={pt('ai.panel.placeholder', 'Ask about this page…')}
						aria-label={pt('ai.panel.placeholder', 'Ask about this page…')}
					></textarea>
					{#if sending}
						<button
							type="button"
							class="ai-panel-send"
							aria-label={pt('ai.panel.stop', 'Stop')}
							onclick={stop}
						>
							<Square size={15} />
						</button>
					{:else}
						<button
							type="button"
							class="ai-panel-send"
							aria-label={pt('ai.panel.send', 'Send')}
							disabled={!composer.trim()}
							onclick={() => void send()}
						>
							<ArrowUp size={16} />
						</button>
					{/if}
				</div>
			</footer>
		{/if}
	</div>
{/if}

<style>
	.ai-panel-backdrop {
		position: fixed;
		inset: 0;
		background: rgb(0 0 0 / 0.25);
		border: 0;
		padding: 0;
		z-index: 55;
		animation: ai-fade-in 0.15s ease-out;
	}
	@keyframes ai-fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
	.ai-panel {
		position: fixed;
		top: 0;
		right: 0;
		bottom: 0;
		width: 400px;
		max-width: 100vw;
		display: flex;
		flex-direction: column;
		background: var(--color-surface);
		border-left: 1px solid var(--color-border);
		box-shadow: -8px 0 24px rgb(0 0 0 / 0.18);
		z-index: 60;
		animation: ai-panel-in 0.18s ease-out;
	}
	@keyframes ai-panel-in {
		from {
			transform: translateX(24px);
			opacity: 0;
		}
		to {
			transform: translateX(0);
			opacity: 1;
		}
	}
	@media (max-width: 640px) {
		.ai-panel {
			width: 100vw;
			border-left: none;
		}
	}
	.ai-panel-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.625rem 0.75rem;
		border-bottom: 1px solid var(--color-border);
	}
	.ai-panel-title {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.ai-panel-chip {
		max-width: 45%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 0.6875rem;
		padding: 0.125rem 0.5rem;
		border-radius: 999px;
		border: 1px solid var(--color-border);
		color: var(--color-muted);
	}
	.ai-panel-head-spacer {
		flex: 1;
	}
	.ai-panel-iconbtn {
		display: grid;
		place-items: center;
		width: 1.75rem;
		height: 1.75rem;
		border-radius: 999px;
		color: var(--color-muted);
		transition:
			color 0.15s,
			background 0.15s;
	}
	.ai-panel-iconbtn:hover {
		color: var(--color-text);
		background: var(--color-surface-subtle);
	}
	.ai-mode-switch {
		display: inline-flex;
		align-items: center;
		align-self: flex-start;
		gap: 2px;
		padding: 2px;
		border: 1px solid var(--color-border);
		border-radius: 999px;
		background: var(--color-surface-subtle);
		flex-shrink: 0;
	}
	.ai-mode-switch button {
		font-size: 0.6875rem;
		font-weight: 500;
		line-height: 1;
		padding: 0.3125rem 0.5rem;
		border-radius: 999px;
		color: var(--color-muted);
		transition:
			background 0.15s,
			color 0.15s;
	}
	.ai-mode-switch button.active {
		background: var(--color-primary);
		color: var(--color-primary-contrast);
	}
	.ai-panel-guest {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		padding: 1.5rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.875rem;
	}
	.ai-panel-signin {
		padding: 0.5rem 1rem;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.875rem;
		font-weight: 500;
	}
	.ai-panel-transcript {
		flex: 1;
		overflow-y: auto;
		padding: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.625rem;
	}
	.ai-panel-loading {
		display: grid;
		place-items: center;
		padding: 2rem 0;
		color: var(--color-muted);
	}
	.ai-panel-welcome {
		font-size: 0.8125rem;
		color: var(--color-muted);
		text-align: center;
		padding: 1.5rem 0.5rem;
	}
	.ai-turn {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		align-items: flex-start;
	}
	.ai-turn-user {
		align-items: flex-end;
	}
	.ai-bubble {
		max-width: 90%;
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius-card);
		background: var(--color-surface-subtle);
		color: var(--color-text);
		font-size: 0.8125rem;
		line-height: 1.45;
		white-space: pre-wrap;
		word-break: break-word;
	}
	.ai-turn-user .ai-bubble {
		background: var(--color-primary);
		color: var(--color-primary-contrast);
	}
	.ai-toolcalls {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
	}
	.ai-toolpill {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.6875rem;
		padding: 0.125rem 0.5rem;
		border-radius: 999px;
		border: 1px solid var(--color-border);
		color: var(--color-muted);
	}
	.ai-progress {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.75rem;
		color: var(--color-muted);
	}
	.ai-linkbtn {
		font-size: 0.75rem;
		color: var(--color-primary);
		text-decoration: underline;
	}
	.ai-error {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.75rem;
		color: var(--color-danger, #c0392b);
	}
	.ai-sources {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
	}
	.ai-source {
		font-size: 0.6875rem;
		padding: 0.125rem 0.5rem;
		border-radius: 999px;
		background: var(--color-surface-subtle);
		color: var(--color-text);
		max-width: 12rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	a.ai-source:hover {
		color: var(--color-primary);
	}
	.ai-panel-composer {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.625rem 0.75rem;
		border-top: 1px solid var(--color-border);
	}
	.ai-panel-inputrow {
		display: flex;
		align-items: flex-end;
		gap: 0.5rem;
	}
	.ai-panel-composer textarea {
		flex: 1;
		resize: none;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--color-text);
		font-size: 0.8125rem;
		line-height: 1.4;
		padding: 0.5rem 0.625rem;
		max-height: 7rem;
	}
	.ai-panel-composer textarea:focus {
		outline: none;
		border-color: var(--color-primary);
	}
	.ai-panel-send {
		display: grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		border-radius: 999px;
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		transition: opacity 0.15s;
	}
	.ai-panel-send:disabled {
		opacity: 0.4;
	}
	:global(.ai-spin) {
		animation: ai-rotate 1s linear infinite;
	}
	@keyframes ai-rotate {
		to {
			transform: rotate(360deg);
		}
	}
</style>
