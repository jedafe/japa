<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — Prompts tab:
	// prompt builder (member mode) + paste-generated-output → draft. Uses
	// the composer-grid layout (builder in the main column, import panel in
	// the right rail) so the tab renders in the same surface as Create.
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

<div class="composer-grid">
	<div class="composer-main">
		<DefaultsBar bind:defaults contentTypes={data.contentTypes} spaces={data.spaces} />
		<PromptBuilder
			contentTypes={data.contentTypes}
			spaces={data.spaces}
			canPersona={data.canPersona}
			personas={data.personas}
			voices={data.voices}
		/>
	</div>

	<aside class="composer-right">
		<ImportPanel
			contentTypes={data.contentTypes}
			spaces={data.spaces}
			drafts={data.drafts}
		/>
	</aside>
</div>

<style>
	/* Same grid contract as CreateShell's .composer-grid. */
	.composer-grid {
		display: grid;
		grid-template-columns: 1fr 320px;
		gap: 1.5rem;
	}

	.composer-main {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		min-width: 0;
	}

	.composer-right {
		min-width: 0;
	}

	@media (max-width: 1023px) {
		.composer-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
