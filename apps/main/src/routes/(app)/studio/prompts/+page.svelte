<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — Prompts tab:
	// prompt builder (member mode) + paste-generated-output → draft.
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import DefaultsBar from '$lib/components/studio/DefaultsBar.svelte';
	import PromptBuilder from '$lib/components/studio/PromptBuilder.svelte';
	import ImportPanel from '$lib/components/studio/ImportPanel.svelte';
	import { loadStudioDefaults, type StudioWebDefaults } from '$lib/studio/defaults';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let defaults = $state<StudioWebDefaults>({});
	onMount(() => {
		defaults = loadStudioDefaults();
	});
</script>

<svelte:head>
	<title>{t('studio.tabs.prompts', 'Prompts')} — {t('studio.title', 'Studio — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="p-4">
	<p class="mb-4 text-sm text-[var(--color-muted)]">
		{t('studio.prompts.description', 'Build a generation prompt for your own assistant, then paste the resulting markdown back to publish it as a draft.')}
	</p>

	<DefaultsBar bind:defaults contentTypes={data.contentTypes} spaces={data.spaces} />

	<div class="space-y-6">
		<PromptBuilder
			contentTypes={data.contentTypes}
			spaces={data.spaces}
			canPersona={data.canPersona}
			personas={data.personas}
			voices={data.voices}
		/>

		<ImportPanel
			contentTypes={data.contentTypes}
			spaces={data.spaces}
			drafts={data.drafts}
		/>
	</div>
</div>
