<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §4 — prompt builder
	// rendered in the same composer idiom as CreateShell: composer-header with
	// title + type select, guide-category Autocomplete, TitleField, then the
	// generation field group and the live prompt preview. Metadata fields live
	// in the right rail (PromptRightPanel).
	import { t } from '$lib/i18n';
	import type { ContentTypeMeta } from '$lib/types';
	import type { Persona, PromptFormValues } from '$lib/studio/types';
	import {
		TONE_OPTIONS,
		FRAMEWORK_OPTIONS,
		MODE_OPTIONS,
		HUMANIZE_OPTIONS,
		type StudioMode
	} from '$lib/studio/types';
	import { buildPrompt, buildProjectInstruction, defaultFrameworkFor, type PromptLookups } from '$lib/studio/prompt';
	import { isHighStakesCategory } from '$lib/studio/validation';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import TitleField from '$lib/components/create/TitleField.svelte';
	import { Copy, Download, Check } from 'lucide-svelte';

	interface VoiceOption {
		id: string;
		name: string;
		voice_prompt?: string;
	}

	interface Props {
		values: PromptFormValues;
		contentTypes: ContentTypeMeta[];
		canPersona: boolean;
		personas: Persona[];
		voices: VoiceOption[];
		spaceName: string;
	}

	let { values, contentTypes, canPersona, personas, voices, spaceName }: Props = $props();

	const mode: StudioMode = $derived(canPersona ? 'operator' : 'member');

	let copied = $state(false);
	let frameworkTouched = $state(false);

	const selectedType = $derived(contentTypes.find((ct) => ct.value === values.content_type));
	const categories = $derived(selectedType?.categories ?? []);
	const highStakes = $derived(isHighStakesCategory(values.guide_category));

	const guideCategoryItems = $derived(
		categories.map((c) => ({ id: c.Slug, label: c.Label }))
	);
	const guideCategoryFetcher = $derived(staticAutocompleteFetcher(guideCategoryItems));
	const guideCategoryLabel = $derived(
		categories.find((c) => c.Slug === values.guide_category)?.Label ?? ''
	);

	$effect(() => {
		if (!frameworkTouched) {
			values.framework = (defaultFrameworkFor(values.content_type) || '') as PromptFormValues['framework'];
		}
	});

	const lookups = $derived<PromptLookups>({
		contentTypeName: selectedType?.label ?? '',
		guideCategoryName: guideCategoryLabel,
		personaName: personas.find((p) => p.id === values.persona_id)?.profile_name ?? '',
		voiceName: voices.find((v) => v.id === values.voice_id)?.name ?? '',
		voicePrompt: voices.find((v) => v.id === values.voice_id)?.voice_prompt,
		spaceName
	});

	const promptText = $derived(buildPrompt(values, lookups, mode));

	async function copyPrompt() {
		try {
			await navigator.clipboard.writeText(promptText);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard unavailable (permissions/non-secure context) — user can
			// still select the preview text manually.
		}
	}

	function downloadInstructions() {
		const blob = new Blob([buildProjectInstruction(promptText)], { type: 'text/markdown' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = 'CLAUDE.md';
		a.click();
		URL.revokeObjectURL(url);
	}
</script>

<div class="composer-header">
	<h2 class="composer-title">{t('studio.prompts.builder', 'Prompt builder')}</h2>
	<select
		class="composer-type-select"
		value={values.content_type}
		onchange={(e) => {
			values.content_type = e.currentTarget.value;
			values.guide_category = '';
		}}
	>
		{#each contentTypes as ty (ty.value)}
			<option value={ty.value}>{ty.label}</option>
		{/each}
	</select>
</div>

{#if categories.length > 0}
	<div class="guide-category-selector">
		<label for="guide-category" class="guide-category-label">
			{t('studio.prompts.guide_category', 'Guide category')}
		</label>
		<Autocomplete
			id="guide-category"
			placeholder={t('studio.prompts.guide_category', 'Guide category')}
			emptyMessage={t('common.no_results', 'No results found')}
			fetcher={guideCategoryFetcher}
			initialFetcher={guideCategoryFetcher}
			selectedId={values.guide_category || null}
			selectedLabel={guideCategoryLabel}
			onSelect={(item) => { values.guide_category = item.id; }}
			onClear={() => { values.guide_category = ''; }}
		/>
	</div>
{/if}

<TitleField title={values.title} onTitle={(v) => (values.title = v)} />

<div class="composer-field-groups">
	<h4 class="composer-field-group-title">{t('studio.prompts.generation', 'Generation')}</h4>

	<div class="field-grid">
		<div class="field-row">
			<label class="field-label" for="pb-mode">{t('studio.prompts.mode', 'Mode')}</label>
			<select id="pb-mode" bind:value={values.mode} class="field-input">
				{#each MODE_OPTIONS as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-topic">{t('studio.prompts.topic', 'Topic')}</label>
			<input id="pb-topic" type="text" bind:value={values.topic} class="field-input" />
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-audience">{t('studio.prompts.audience', 'Audience')}</label>
			<input id="pb-audience" type="text" bind:value={values.audience} class="field-input" />
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-tone">{t('studio.prompts.tone', 'Tone')}</label>
			<select id="pb-tone" bind:value={values.tone} class="field-input">
				{#each TONE_OPTIONS as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-framework">{t('studio.prompts.framework', 'Framework')}</label>
			<select
				id="pb-framework"
				bind:value={values.framework}
				onchange={() => (frameworkTouched = true)}
				class="field-input"
			>
				{#each FRAMEWORK_OPTIONS as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-humanize">{t('studio.prompts.humanize', 'Humanize')}</label>
			<select id="pb-humanize" bind:value={values.humanize_level} class="field-input">
				{#each HUMANIZE_OPTIONS as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-min-words">{t('studio.prompts.min_words', 'Min words')}</label>
			<input id="pb-min-words" type="number" min="0" bind:value={values.min_words} class="field-input" />
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-headings">{t('studio.prompts.heading_count', 'H2 headings')}</label>
			<input id="pb-headings" type="number" min="0" bind:value={values.heading_count} class="field-input" />
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-paras-heading">{t('studio.prompts.paragraphs_per_heading', 'Paragraphs per heading')}</label>
			<input id="pb-paras-heading" type="number" min="0" bind:value={values.min_paragraphs_per_heading} class="field-input" />
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-paras">{t('studio.prompts.paragraph_count', 'Total paragraphs')}</label>
			<input id="pb-paras" type="number" min="0" bind:value={values.paragraph_count} class="field-input" />
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-int-links">{t('studio.prompts.internal_links', 'Internal links')}</label>
			<input id="pb-int-links" type="number" min="0" bind:value={values.internal_links} class="field-input" />
		</div>

		<div class="field-row">
			<label class="field-label" for="pb-ext-links">{t('studio.prompts.external_links', 'External links')}</label>
			<input id="pb-ext-links" type="number" min="0" bind:value={values.external_links} class="field-input" />
		</div>
	</div>

	<div class="toggle-row">
		<label class="toggle"><input type="checkbox" bind:checked={values.include_intro} /> {t('studio.prompts.inc_intro', 'Intro')}</label>
		<label class="toggle"><input type="checkbox" bind:checked={values.include_conclusion} /> {t('studio.prompts.inc_conclusion', 'Conclusion')}</label>
		<label class="toggle"><input type="checkbox" bind:checked={values.include_faqs} /> {t('studio.prompts.inc_faqs', 'FAQs')}</label>
		<label class="toggle"><input type="checkbox" bind:checked={values.include_steps} /> {t('studio.prompts.inc_steps', 'Steps')}</label>
		<label class="toggle"><input type="checkbox" bind:checked={values.include_key_takeaways} /> {t('studio.prompts.inc_takeaways', 'Key takeaways')}</label>
		<label class="toggle"><input type="checkbox" bind:checked={values.include_inline_image} /> {t('studio.prompts.inc_images', 'Image suggestions')}</label>
	</div>

	{#if highStakes}
		<p class="high-stakes-hint">
			{t('studio.prompts.high_stakes', 'High-stakes category — saved as in_review regardless of selected status.')}
		</p>
	{/if}
</div>

<div class="composer-field-groups">
	<div class="preview-header">
		<h4 class="composer-field-group-title">{t('studio.prompts.preview', 'Generated prompt')}</h4>
		<div class="preview-actions">
			<button type="button" onclick={copyPrompt} class="btn-primary">
				{#if copied}<Check size={14} />{:else}<Copy size={14} />{/if}
				{copied ? t('studio.prompts.copied', 'Copied') : t('studio.prompts.copy', 'Copy prompt')}
			</button>
			<button type="button" onclick={downloadInstructions} class="btn-secondary">
				<Download size={14} /> {t('studio.prompts.download', 'Download CLAUDE.md')}
			</button>
		</div>
	</div>
	<pre class="prompt-preview">{promptText}</pre>
</div>

<style>
	/* Composer idiom — mirrors CreateShell's scoped classes. */
	.composer-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.composer-title {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.composer-type-select {
		height: 2.375rem;
		padding: 0 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
		cursor: pointer;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.composer-type-select:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.guide-category-selector {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.guide-category-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.composer-field-groups {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
	}
	.composer-field-group-title {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.field-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px;
	}
	.field-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.field-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.field-input {
		width: 100%;
		height: 2.375rem;
		padding: 0 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.field-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.toggle-row {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
	}
	.toggle {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 0.875rem;
		color: var(--color-text);
	}
	.high-stakes-hint {
		margin: 0;
		font-size: 0.75rem;
		color: var(--color-warning);
	}
	.preview-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.preview-actions {
		display: flex;
		gap: 8px;
	}
	.btn-primary {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 2rem;
		padding: 0 12px;
		border: none;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: #fff;
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		transition: opacity 0.15s;
	}
	.btn-primary:hover {
		opacity: 0.9;
	}
	.btn-secondary {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 2rem;
		padding: 0 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.15s;
	}
	.btn-secondary:hover {
		background: var(--color-surface-elevated);
	}
	.prompt-preview {
		margin: 0;
		max-height: 24rem;
		overflow-y: auto;
		white-space: pre-wrap;
		padding: 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		font-size: 0.75rem;
		color: var(--color-text);
	}
	@media (max-width: 640px) {
		.field-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
