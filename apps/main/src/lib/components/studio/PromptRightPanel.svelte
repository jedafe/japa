<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §4 — right rail in
	// the CreateRightPanel idiom. The "Prompt" tab holds all generation
	// fields (tone + voice side by side, per the §105 wiring contract: tone
	// is an inline prompt instruction, voice_id resolves to a shared persona
	// voice whose voice_prompt is injected in operator mode). The remaining
	// tabs reuse the composer's own panel components. All selects render as
	// Autocomplete fields.
	import { Tabs } from 'bits-ui';
	import { t } from '$lib/i18n';
	import type { MeSpace, Visibility, MediaAsset } from '$lib/types';
	import type { Persona, PromptFormValues, PromptMode, PromptTone, PromptFramework, HumanizeLevel } from '$lib/studio/types';
	import {
		MODE_OPTIONS,
		TONE_OPTIONS,
		FRAMEWORK_OPTIONS,
		HUMANIZE_OPTIONS
	} from '$lib/studio/types';
	import { slugify } from '$lib/studio/prompt';
	import { isHighStakesCategory } from '$lib/studio/validation';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import SpaceSelector from '$lib/components/create/SpaceSelector.svelte';
	import TagInput from '$lib/components/create/TagInput.svelte';
	import CreateJourneyPanel from '$lib/components/create/panel/CreateJourneyPanel.svelte';
	import CreateSettingsPanel from '$lib/components/create/panel/CreateSettingsPanel.svelte';
	import CreateSeoPanel from '$lib/components/create/panel/CreateSeoPanel.svelte';

	interface VoiceOption {
		id: string;
		name: string;
		voice_prompt?: string;
	}

	interface Props {
		values: PromptFormValues;
		spaces: MeSpace[];
		visibilityOptions: { value: Visibility; label: string }[];
		maxTags: number;
		canPersona: boolean;
		personas: Persona[];
		voices: VoiceOption[];
	}

	let {
		values,
		spaces = $bindable(),
		visibilityOptions,
		maxTags,
		canPersona,
		personas,
		voices
	}: Props = $props();

	const highStakes = $derived(isHighStakesCategory(values.guide_category));

	// Adapters between the prompt form's scalar fields and the composer's
	// multi-value components.
	const spaceIds = $derived(values.space_id ? [values.space_id] : []);
	const effectiveSlug = $derived(
		values.slug || slugify(values.title || values.topic || 'untitled')
	);
	const dimension = $derived(values.field_values.roadmap_dimension || null);
	const purpose = $derived(values.field_values.roadmap_purpose || null);

	function setFieldValue(key: string, v: string | null) {
		if (v) values.field_values[key] = v;
		else delete values.field_values[key];
	}

	const noop = () => {};
	const emptyMsg = t('common.no_results', 'No results found');

	// Static option lists → AutocompleteItem[] for the select-style fields.
	const modeItems = MODE_OPTIONS.map((o) => ({ id: o.value, label: o.label }));
	const toneItems = TONE_OPTIONS.map((o) => ({ id: o.value, label: o.label }));
	const frameworkItems = FRAMEWORK_OPTIONS.map((o) => ({ id: o.value, label: o.label }));
	const humanizeItems = HUMANIZE_OPTIONS.map((o) => ({ id: o.value, label: o.label }));
	const personaItems = $derived(personas.map((p) => ({ id: p.id, label: p.profile_name })));
	const voiceItems = $derived(voices.map((v) => ({ id: v.id, label: v.name })));
	const statusItems = $derived(
		['draft', 'in_review', 'published', 'scheduled']
			.filter((s) => !highStakes || (s !== 'published' && s !== 'scheduled'))
			.map((s) => ({ id: s, label: s }))
	);

	function labelFor(items: { id: string; label: string }[], id: string): string {
		return items.find((i) => i.id === id)?.label ?? '';
	}
</script>

{#snippet selectField(
	label: string,
	items: AutocompleteItem[],
	selectedId: string,
	onPick: (item: AutocompleteItem) => void
)}
	<div class="field-row">
		<span class="field-label">{label}</span>
		<Autocomplete
			placeholder={label}
			emptyMessage={emptyMsg}
			fetcher={staticAutocompleteFetcher(items)}
			initialFetcher={staticAutocompleteFetcher(items)}
			selectedId={selectedId || null}
			selectedLabel={labelFor(items, selectedId)}
			onSelect={onPick}
		/>
	</div>
{/snippet}

{#snippet numField(label: string, value: number, onInput: (v: number) => void)}
	<div class="field-row">
		<span class="field-label">{label}</span>
		<input
			type="number"
			min="0"
			class="field-input"
			{value}
			oninput={(e) => onInput(Number(e.currentTarget.value) || 0)}
		/>
	</div>
{/snippet}

{#snippet textField(label: string, value: string, onInput: (v: string) => void)}
	<div class="field-row">
		<span class="field-label">{label}</span>
		<input
			type="text"
			class="field-input"
			{value}
			oninput={(e) => onInput(e.currentTarget.value)}
		/>
	</div>
{/snippet}

<Tabs.Root value="prompt">
	<Tabs.List class="right-panel-tabs">
		<Tabs.Trigger value="prompt" class="right-panel-tab">
			{t('studio.prompts.tab.prompt', 'Prompt')}
		</Tabs.Trigger>
		<Tabs.Trigger value="guide" class="right-panel-tab">
			{t('create.panel.guide')}
		</Tabs.Trigger>
		<Tabs.Trigger value="journey" class="right-panel-tab">
			{t('create.panel.journey')}
		</Tabs.Trigger>
		<Tabs.Trigger value="settings" class="right-panel-tab">
			{t('create.panel.settings')}
		</Tabs.Trigger>
		<Tabs.Trigger value="seo" class="right-panel-tab">
			{t('create.panel.seo')}
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="prompt" class="pt-2">
		<div class="panel-stack">
			{@render selectField(t('studio.prompts.mode', 'Mode'), modeItems, values.mode, (i) => (values.mode = i.id as PromptMode))}
			{@render textField(t('studio.prompts.topic', 'Topic'), values.topic, (v) => (values.topic = v))}
			{@render textField(t('studio.prompts.audience', 'Audience'), values.audience, (v) => (values.audience = v))}

			{#if canPersona}
				{@render selectField(t('studio.prompts.persona', 'Persona'), personaItems, values.persona_id, (i) => (values.persona_id = i.id))}
				<div class="pair">
					{@render selectField(t('studio.prompts.tone', 'Tone'), toneItems, values.tone, (i) => (values.tone = i.id as PromptTone))}
					{@render selectField(t('studio.prompts.voice', 'Voice'), voiceItems, values.voice_id, (i) => (values.voice_id = i.id))}
				</div>
			{:else}
				{@render selectField(t('studio.prompts.tone', 'Tone'), toneItems, values.tone, (i) => (values.tone = i.id as PromptTone))}
			{/if}

			{@render selectField(t('studio.prompts.framework', 'Framework'), frameworkItems, values.framework, (i) => (values.framework = i.id as PromptFramework))}
			{@render selectField(t('studio.prompts.humanize', 'Humanize'), humanizeItems, values.humanize_level, (i) => (values.humanize_level = i.id as HumanizeLevel))}

			<h5 class="subsection">{t('studio.prompts.structure', 'Structure')}</h5>
			<div class="pair">
				{@render numField(t('studio.prompts.min_words', 'Min words'), values.min_words, (v) => (values.min_words = v))}
				{@render numField(t('studio.prompts.heading_count', 'H2 headings'), values.heading_count, (v) => (values.heading_count = v))}
			</div>
			<div class="pair">
				{@render numField(t('studio.prompts.paragraphs_per_heading', 'Paras / heading'), values.min_paragraphs_per_heading, (v) => (values.min_paragraphs_per_heading = v))}
				{@render numField(t('studio.prompts.paragraph_count', 'Total paras'), values.paragraph_count, (v) => (values.paragraph_count = v))}
			</div>
			<div class="pair">
				{@render numField(t('studio.prompts.internal_links', 'Internal links'), values.internal_links, (v) => (values.internal_links = v))}
				{@render numField(t('studio.prompts.external_links', 'External links'), values.external_links, (v) => (values.external_links = v))}
			</div>

			<div class="toggle-col">
				<label class="toggle"><input type="checkbox" bind:checked={values.include_intro} /> {t('studio.prompts.inc_intro', 'Intro')}</label>
				<label class="toggle"><input type="checkbox" bind:checked={values.include_conclusion} /> {t('studio.prompts.inc_conclusion', 'Conclusion')}</label>
				<label class="toggle"><input type="checkbox" bind:checked={values.include_faqs} /> {t('studio.prompts.inc_faqs', 'FAQs')}</label>
				<label class="toggle"><input type="checkbox" bind:checked={values.include_steps} /> {t('studio.prompts.inc_steps', 'Steps')}</label>
				<label class="toggle"><input type="checkbox" bind:checked={values.include_key_takeaways} /> {t('studio.prompts.inc_takeaways', 'Key takeaways')}</label>
				<label class="toggle"><input type="checkbox" bind:checked={values.include_inline_image} /> {t('studio.prompts.inc_images', 'Image suggestions')}</label>
			</div>
		</div>
	</Tabs.Content>

	<Tabs.Content value="guide" class="pt-2">
		<div class="panel-stack">
			<div class="field-row">
				<span class="field-label">{t('studio.prompts.space', 'Space')}</span>
				<SpaceSelector
					bind:spaces
					{spaceIds}
					onChange={(ids) => (values.space_id = ids[0] ?? '')}
				/>
			</div>

			<div class="field-row">
				<span class="field-label">{t('studio.prompts.tags_label', 'Tags')}</span>
				<TagInput tags={values.tags} onTags={(next) => (values.tags = next)} {maxTags} />
			</div>
		</div>
	</Tabs.Content>

	<Tabs.Content value="journey" class="pt-2">
		<CreateJourneyPanel
			journeyStage={values.journey_stage || null}
			visaRoute={values.visa_route || null}
			sourceCountry={values.source_country || null}
			targetCountry={values.target_country || null}
			{dimension}
			{purpose}
			assessmentDimensions={[]}
			onjourney={(v) => (values.journey_stage = v ?? '')}
			onvisa={(v) => (values.visa_route = v ?? '')}
			onsource={(v) => (values.source_country = v ?? '')}
			ontarget={(v) => (values.target_country = v ?? '')}
			ondimension={(v) => setFieldValue('roadmap_dimension', v)}
			onpurpose={(v) => setFieldValue('roadmap_purpose', v)}
			onedit={noop}
		/>
	</Tabs.Content>

	<Tabs.Content value="settings" class="pt-2">
		<CreateSettingsPanel
			slug={effectiveSlug}
			visibility={values.visibility as Visibility}
			canonicalUrl={values.canonical_url}
			publishAt={values.publish_at || null}
			{visibilityOptions}
			onslug={(v) => (values.slug = v)}
			onvisibility={(v) => (values.visibility = v)}
			oncanonical={(v) => (values.canonical_url = v)}
			onpublishat={(v) => (values.publish_at = v ?? '')}
			onedit={noop}
		/>
		<div class="panel-stack">
			{@render selectField(t('studio.prompts.status', 'Status'), statusItems, values.status, (i) => (values.status = i.id))}
			{#if highStakes}
				<p class="hint warn">
					{t('studio.prompts.high_stakes', 'High-stakes category — saved as in_review regardless of selected status.')}
				</p>
			{/if}
			<div class="field-row">
				<label class="field-label" for="pp-field-values">
					{t('studio.prompts.field_values', 'Field values (key: value, one per line)')}
				</label>
				<textarea
					id="pp-field-values"
					rows="3"
					class="field-input field-textarea"
					value={Object.entries(values.field_values)
						.map(([k, v]) => `${k}: ${v}`)
						.join('\n')}
					oninput={(e) => {
						const fv: Record<string, string> = {};
						for (const line of e.currentTarget.value.split('\n')) {
							const idx = line.indexOf(':');
							if (idx > 0) fv[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
						}
						values.field_values = fv;
					}}
				></textarea>
			</div>
		</div>
	</Tabs.Content>

	<Tabs.Content value="seo" class="pt-2">
		<CreateSeoPanel
			metaTitle={values.meta_title || null}
			metaDescription={values.meta_description || null}
			ogImageUrl={values.og_image_url || null}
			ogImagePreviewUrl={values.og_image_url || null}
			seoIndexable={values.seo_indexable}
			title={values.title}
			slug={effectiveSlug}
			canonicalUrl={values.canonical_url}
			visibility={values.visibility}
			bodyHtml=""
			contentType={values.content_type}
			onmetatitle={(v) => (values.meta_title = v ?? '')}
			onmetadescription={(v) => (values.meta_description = v ?? '')}
			onogimageselect={(asset: MediaAsset | null) => (values.og_image_url = asset?.url ?? '')}
			onseoindexable={(v) => (values.seo_indexable = v)}
			onedit={noop}
		/>
	</Tabs.Content>
</Tabs.Root>

<style>
	/* Reuses the global right-panel-tabs/right-panel-tab classes defined by
	 * CreateRightPanel — same visual contract. */
	.panel-stack {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 0.5rem;
	}
	.pair {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 8px;
		align-items: start;
	}
	.field-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	.field-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.subsection {
		margin: 0;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-text);
		border-top: 1px solid var(--color-border);
		padding-top: 10px;
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
	.field-textarea {
		height: auto;
		padding: 8px 14px;
		font-family: inherit;
	}
	.toggle-col {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.toggle {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 0.875rem;
		color: var(--color-text);
	}
	.hint {
		font-size: 0.75rem;
		color: var(--color-muted);
		line-height: 1.4;
	}
	.warn {
		color: var(--color-warning);
	}
</style>
