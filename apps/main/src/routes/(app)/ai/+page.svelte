<script lang="ts">
	// /ai — AI Agent chat page. Renders the AgentChat component inside the app
	// shell, with a header showing the agent identity + geo context. The
	// capabilities (tool catalog) come from the server load; the SSE stream
	// is opened client-side by the component.
	//
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
	// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, tasks 10.02-10.03)

	import { t } from '$lib/i18n';
	import { Sparkles, AlertCircle, MapPin, Wrench, Clock } from 'lucide-svelte';
	import AgentChat from '$lib/ai/AgentChat.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const capabilities = $derived(data.capabilities);
	const geo = $derived(capabilities?.geo_context);
	const toolCount = $derived(capabilities?.tools.length ?? 0);
</script>

<svelte:head>
	<title>{t('ai.agent.title', 'AI Agent')} — JapaConnect</title>
</svelte:head>

<div class="mx-auto flex h-[calc(100vh-8rem)] max-w-4xl flex-col">
	<!-- Header -->
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

	<!-- Chat (fills remaining height) -->
	<div class="flex-1 min-h-0">
		<AgentChat capabilities={capabilities} />
	</div>

	<!-- Footer link to session history -->
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
