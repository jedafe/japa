<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — Prompts tab in
	// the composer idiom: builder + import panel in the main column, the
	// Guide|Journey|Settings|SEO inspector in the right rail — same layout
	// contract as /studio/create.
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import DefaultsBar from '$lib/components/studio/DefaultsBar.svelte';
	import PromptBuilder from '$lib/components/studio/PromptBuilder.svelte';
	import PromptRightPanel from '$lib/components/studio/PromptRightPanel.svelte';
	import ImportPanel from '$lib/components/studio/ImportPanel.svelte';
	import { loadStudioDefaults, type StudioWebDefaults } from '$lib/studio/defaults';
	import {
		DEFAULT_PROMPT_VALUES,
		promptFromDefaults,
		type PromptFormValues
	} from '$lib/studio/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let values = $state<PromptFormValues>(structuredClone(DEFAULT_PROMPT_VALUES));
	let defaults = $state<StudioWebDefaults>({});

	// Project defaults apply once on mount — before the user can interact —
	// which satisfies "pre-fill only untouched fields" in practice.
	onMount(() => {
		defaults = loadStudioDefaults();
		if (data.canPersona) {
			values = promptFromDefaults(defaults);
		} else {
			// Member mode: persona/voice defaults are meaningless.
			const { default_persona_id: _p, default_voice_id: _v, ...memberDefaults } = defaults;
			values = promptFromDefaults(memberDefaults);
		}
	});

	const spaceName = $derived(
		data.spaces.find((s) => s.id === values.space_id)?.name ?? ''
	);
	const maxTags = $derived(
		typeof data.siteSettings?.['content.max_tags'] === 'number'
			? (data.siteSettings['content.max_tags'] as number)
			: 5
	);
</script>

<svelte:head>
	<title>{t('studio.tabs.prompts', 'Prompts')} — {t('studio.title', 'Studio — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="composer-grid">
	<div class="composer-main">
		<DefaultsBar bind:defaults contentTypes={data.contentTypes} spaces={data.spaces} />
		<PromptBuilder
			{values}
			contentTypes={data.contentTypes}
			canPersona={data.canPersona}
			personas={data.personas}
			voices={data.voices}
			{spaceName}
		/>
		<ImportPanel
			contentTypes={data.contentTypes}
			spaces={data.spaces}
			drafts={data.drafts}
		/>
	</div>

	<aside class="composer-right">
		<PromptRightPanel
			{values}
			bind:spaces={data.spaces}
			visibilityOptions={data.visibilityOptions}
			{maxTags}
			canPersona={data.canPersona}
			personas={data.personas}
			voices={data.voices}
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
