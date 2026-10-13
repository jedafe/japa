<script lang="ts">
	// /ai — unified AI surface. Two modes share this page behind a tab
	// switcher (?mode=chat|agent):
	//
	// - chat  → JapaBot (JapaBotChat component: conversations, RAG
	//           citations, model/voice pickers)
	// - agent → AI Agent (AgentChat component: SSE stream, tool log,
	//           cancel + session history)
	//
	// Each tab renders only when its feature flag is on; a single enabled
	// mode hides the switcher entirely. /ai/chat 308-redirects here.
	//
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
	// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, tasks 10.02-10.03)

	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { Sparkles, AlertCircle, MapPin, Wrench, Clock, MessageSquare } from 'lucide-svelte';
	import AgentChat from '$lib/ai/AgentChat.svelte';
	import JapaBotChat from '$lib/ai/JapaBotChat.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const capabilities = $derived(data.capabilities);
	const geo = $derived(capabilities?.geo_context);
	const toolCount = $derived(capabilities?.tools.length ?? 0);

	// Mode resolution: honor ?mode= when it names an enabled mode, otherwise
	// fall back to whichever mode is available (agent first — /ai was the
	// agent route before the merge).
	type Mode = 'chat' | 'agent';
	const requested = $derived(page.url.searchParams.get('mode'));
	const urlMode = $derived<Mode | null>(
		requested === 'chat' && data.chatEnabled
			? 'chat'
			: requested === 'agent' && data.agentEnabled
				? 'agent'
				: data.agentEnabled
					? 'agent'
					: data.chatEnabled
						? 'chat'
						: null
	);
	// Tab switches go through replaceState, which is shallow — it updates the
	// address bar and page.state but NOT page.url (that stays pinned to the
	// last real navigation). So page.state.mode is the source of truth once
	// set; urlMode covers SSR, direct links, and real navigations.
	const mode = $derived<Mode | null>(
		(page.state as { mode?: Mode } | undefined)?.mode ?? urlMode
	);
	const bothEnabled = $derived(data.chatEnabled && data.agentEnabled);

	// Each mode's component mounts lazily on first activation and then stays
	// mounted — hidden, not destroyed — so switching tabs preserves an
	// in-flight agent SSE stream, the chat draft, and transcript scroll.
	// The initial mode mounts eagerly so SSR renders it directly.
	const initialMode = untrack(() => mode);
	let mounted = $state<Record<Mode, boolean>>({
		chat: initialMode === 'chat',
		agent: initialMode === 'agent'
	});
	$effect(() => {
		if (mode) mounted[mode] = true;
	});

	// Tab switch: replaceState updates the URL (shareable, back/forward
	// work) without re-running load() — an <a href> navigation would
	// remount the page and drop in-flight agent sessions. The mode is
	// written into page.state because replaceState leaves page.url stale.
	function setMode(next: Mode) {
		if (next === mode) return;
		const url = new URL(page.url);
		url.searchParams.set('mode', next);
		replaceState(url, { ...page.state, mode: next });
	}
</script>

<svelte:head>
	<title>{t('ai.page.title', 'Japa AI')} — JapaConnect</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

{#if mode === null}
	<div class="mx-auto flex h-[calc(100vh-8rem)] max-w-5xl flex-col items-center justify-center gap-4 text-center">
		<Sparkles class="h-12 w-12" style="color: var(--color-muted)" />
		<h1 class="text-xl font-semibold" style="color: var(--color-text)">
			{t('ai.page.disabled', 'Japa AI is currently disabled')}
		</h1>
		<p class="text-sm" style="color: var(--color-muted)">
			{t('ai.page.disabled_help', 'This feature is turned off on this platform. Contact an administrator to enable it.')}
		</p>
	</div>
{:else}
<div class="mx-auto flex h-[calc(100vh-8rem)] max-w-5xl flex-col">
	<!-- Mode switcher — only rendered when both modes are available -->
	{#if bothEnabled}
		<div class="mb-4 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)" role="tablist">
			<button
				type="button"
				role="tab"
				aria-selected={mode === 'chat'}
				onclick={() => setMode('chat')}
				class="flex flex-1 items-center justify-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
				style={mode === 'chat'
					? 'background-color: var(--color-primary); color: white'
					: 'color: var(--color-muted)'}
			>
				<MessageSquare size={15} />
				{t('ai.mode.chat', 'JapaBot')}
			</button>
			<button
				type="button"
				role="tab"
				aria-selected={mode === 'agent'}
				onclick={() => setMode('agent')}
				class="flex flex-1 items-center justify-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
				style={mode === 'agent'
					? 'background-color: var(--color-primary); color: white'
					: 'color: var(--color-muted)'}
			>
				<Wrench size={15} />
				{t('ai.mode.agent', 'AI Agent')}
			</button>
		</div>
	{/if}

	{#if mounted.chat}
		<div class="flex min-h-0 flex-1 flex-col" class:hidden={mode !== 'chat'}>
			<JapaBotChat
				conversations={data.conversations}
				models={data.models}
				voices={data.voices}
			/>
		</div>
	{/if}

	{#if mounted.agent}
		<div class="flex min-h-0 flex-1 flex-col" class:hidden={mode !== 'agent'}>
			<!-- Agent mode: header + capabilities warning + SSE chat + footer -->
			<header class="mb-4 flex items-start justify-between gap-3">
				<div class="flex items-center gap-3">
					<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background-color: var(--color-primary)">
						<Sparkles class="h-5 w-5 text-white" />
					</div>
					<div>
						<h1 class="text-xl font-bold tracking-tight">{t('ai.agent.title', 'AI Agent')}</h1>
						<p class="text-xs" style="color: var(--color-muted)">
							{t('ai.agent.subtitle', 'Tool-augmented assistant with full audit trail')}
						</p>
					</div>
				</div>
				<div class="flex flex-col items-end gap-1.5 text-xs">
					{#if geo?.city || geo?.country}
						<span class="inline-flex items-center gap-1" style="color: var(--color-muted)">
							<MapPin size={12} />
							{[geo?.city, geo?.country].filter(Boolean).join(', ')}
						</span>
					{/if}
					{#if toolCount > 0}
						<span class="inline-flex items-center gap-1" style="color: var(--color-muted)">
							<Wrench size={12} />
							{toolCount} {t('ai.agent.tools_available', 'tools available')}
						</span>
					{/if}
				</div>
			</header>

			{#if !capabilities}
				<!-- Capabilities failed to load — the agent may be disabled or the user
				     may lack permission. The chat component will surface the specific
				     error when the user tries to send. We still render it so the user
				     sees the error message rather than a blank page. -->
				<div class="mb-3 flex items-center gap-2 rounded-md border p-2 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)">
					<AlertCircle class="h-4 w-4" />
					{t('ai.agent.capabilities_unavailable', 'Agent capabilities could not be loaded. The agent may be disabled or you may lack permission.')}
				</div>
			{/if}

			<div class="flex-1 min-h-0">
				<AgentChat capabilities={capabilities} />
			</div>

			<div class="mt-3 flex items-center justify-between text-xs">
				<a
					href="/ai/sessions"
					class="inline-flex items-center gap-1 transition-colors hover:opacity-80"
					style="color: var(--color-muted)"
				>
					<Clock size={12} />
					{t('ai.agent.session_history', 'Session history')}
				</a>
				<a
					href="/settings/ai-privacy"
					class="inline-flex items-center gap-1 transition-colors hover:opacity-80"
					style="color: var(--color-muted)"
				>
					{t('ai.agent.privacy_settings', 'AI privacy settings')}
				</a>
			</div>
		</div>
	{/if}
</div>
{/if}
