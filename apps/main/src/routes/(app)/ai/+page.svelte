<script lang="ts">
	// /ai — unified Japa AI surface (ADR-130): a single chat view whose
	// composer tools toggle switches between chat and agent turns.
	// /ai/chat 308-redirects here.
	//
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
	// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md

	import { t } from '$lib/i18n';
	import { Sparkles } from 'lucide-svelte';
	import JapaAIChat from '$lib/ai/JapaAIChat.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// content.max_tags caps the template panel's tag input (same setting
	// the composer enforces).
	const maxTags = $derived(
		typeof data.siteSettings?.['content.max_tags'] === 'number'
			? (data.siteSettings['content.max_tags'] as number)
			: 5
	);
</script>

<svelte:head>
	<title>{t('ai.page.title', 'Japa AI')} — JapaConnect</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

{#if !data.chatEnabled}
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
	<!-- Height hugs the viewport bottom: base reserves room for the
	     mobile top bar + bottom tab bar; md+ has neither, so the shell
	     runs to within a small margin of the viewport edge. -->
	<div
		class="mx-auto mt-2 flex h-[calc(100dvh-8rem)] w-full max-w-7xl flex-col sm:h-[calc(100dvh-4.5rem)] md:h-[calc(100dvh-1.5rem)]"
	>
		<JapaAIChat
			conversations={data.conversations.map((c) => ({ ...c, legacy: c.legacy ?? false }))}
			models={data.models}
			voices={data.voices}
			projects={data.projects}
			capabilities={data.capabilities}
			prefs={data.prefs}
			contentTypes={data.contentTypes}
			spaces={data.spaces}
			series={data.series}
			assessmentDimensions={data.assessmentDimensions}
			{maxTags}
			statusOptions={data.statusOptions}
			visibilityOptions={data.visibilityOptions}
		/>
	</div>
{/if}
