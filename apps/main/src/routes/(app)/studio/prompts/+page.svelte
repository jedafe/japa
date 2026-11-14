<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — Prompts tab in
	// the composer idiom: content type / guide category / title / generated
	// prompt / import panel in the main column, the Prompt|Structure|Journey|
	// Settings inspector in the right rail — same contract as /studio/create.
	import { onMount, untrack } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import PromptBuilder from '#lib/components/studio/PromptBuilder.svelte';
	import PromptRightPanel from '#lib/components/studio/PromptRightPanel.svelte';
	import ImportPanel from '#lib/components/studio/ImportPanel.svelte';
	import { loadStudioDefaults } from '#lib/studio/defaults.js';
	import { studioMobile, resetStudioMobileBridge } from '#lib/studio/mobileView.svelte.js';
	import {
		DEFAULT_PROMPT_VALUES,
		promptFromDefaults,
		type PromptFormValues
	} from '#lib/studio/types.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let values = $state<PromptFormValues>(structuredClone(DEFAULT_PROMPT_VALUES));

	// Local reactive copy of the spaces list — SpaceSelector appends a newly
	// created space via bind:, and load data is not a valid binding target.
	let spaces = $state(untrack(() => data.spaces));
	let series = $state(untrack(() => data.series));

	// Saved project defaults pre-fill the form once on mount — before the
	// user can interact — which satisfies "pre-fill only untouched fields"
	// in practice.
	onMount(() => {
		const defaults = loadStudioDefaults();
		if (data.canPersona) {
			values = promptFromDefaults(defaults);
		} else {
			// Member mode: persona/voice defaults are meaningless.
			const { default_persona_id: _p, default_voice_id: _v, ...memberDefaults } = defaults;
			values = promptFromDefaults(memberDefaults);
		}
	});

	// Mobile type bridge — the studio layout's content-type FAB modal reads
	// this registration; selecting a type mirrors the builder's own picker
	// (type change clears guide_category). Also resets the editor/config
	// column to the editor on (re)entry.
	onMount(() => {
		studioMobile.section = 'editor';
	});
	$effect(() => {
		studioMobile.typeItems = data.contentTypes.map((ct) => ({ id: ct.value, label: ct.label }));
		studioMobile.selectedTypeId = values.content_type || null;
		studioMobile.typeLocked = false;
		studioMobile.selectType = (id: string) => {
			values.content_type = id;
			values.guide_category = '';
		};
		return resetStudioMobileBridge;
	});

	const spaceName = $derived(
		spaces.find((s) => s.id === values.space_id)?.name ?? ''
	);
	// agent.skip_hitl (§103) — shared by the builder, the import flow, and
	// the right rail so all three apply the same high-stakes hold rule the
	// server action enforces.
	const canSkipHitl = $derived(
		!!(data.user?.permissions?.includes('*') || data.user?.permissions?.includes('agent.skip_hitl'))
	);
	const maxTags = $derived(
		typeof data.siteSettings?.['content.max_tags'] === 'number'
			? (data.siteSettings['content.max_tags'] as number)
			: 5
	);
</script>

<SeoHead title={`${t('studio.tabs.prompts', 'Prompts')} — ${t('studio.title', 'Studio — {site}')}`} robots="noindex" description={t('studio.prompts_description', 'Reusable generation prompts for your content pipeline.')} />

{#if data.lookupWarnings.length > 0}
	<div class="lookup-warning" role="status">
		{t('studio.prompts.lookup_failed', 'Some option lists failed to load ({list}) — those pickers may be incomplete.', { list: data.lookupWarnings.join(', ') })}
	</div>
{/if}

<div class="composer-grid" data-mobile-section={studioMobile.section}>
	<div class="composer-main">
		<PromptBuilder
			bind:values
			contentTypes={data.contentTypes}
			canPersona={data.canPersona}
			personas={data.personas}
			voices={data.voices}
			{spaceName}
			{canSkipHitl}
		/>
		<ImportPanel
			contentTypes={data.contentTypes}
			{spaces}
			drafts={data.drafts}
			visibilityOptions={data.visibilityOptions}
			statusOptions={data.statusOptions}
			{canSkipHitl}
		/>
	</div>

	<aside class="composer-right">
		<PromptRightPanel
			bind:values
			bind:spaces
			bind:series
			assessmentDimensions={data.assessmentDimensions}
			{maxTags}
			canPersona={data.canPersona}
			personas={data.personas}
			voices={data.voices}
			statusOptions={data.statusOptions}
			visibilityOptions={data.visibilityOptions}
			{canSkipHitl}
		/>
	</aside>
</div>

<style>
	.lookup-warning {
		margin-bottom: 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-elevated);
		padding: 0.5rem 0.75rem;
		font-size: 0.75rem;
		color: var(--color-warning);
	}

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

	/* Mobile: the layout's swap FAB shows one column at a time. */
	@media (max-width: 639px) {
		.composer-grid[data-mobile-section='editor'] > .composer-right {
			display: none;
		}
		.composer-grid[data-mobile-section='config'] > .composer-main {
			display: none;
		}
	}
</style>
