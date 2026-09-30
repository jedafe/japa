<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §4 — right rail in
	// the CreateRightPanel idiom: Prompt | Structure | Journey | Settings.
	// The Prompt tab holds the generation fields — tone + voice sit side by
	// side and voice options are filtered by the selected tone (per the
	// §105 contract: tone is an inline prompt instruction, voice_id resolves
	// to a shared persona voice whose voice_prompt is injected in operator
	// mode). Structure carries the counts/links/toggles; Journey and
	// Settings reuse the composer's own panels, with space/tags/status/
	// field_values appended to Settings. All selects render as Autocomplete.
	import { Tabs } from 'bits-ui';
	import { t } from '$lib/i18n';
	import type { MeSpace, BlogSeries } from '$lib/types';
	import type { Persona, PromptFormValues, PromptMode, PromptTone, PromptFramework, HumanizeLevel } from '$lib/studio/types';
	import {
		MODE_OPTIONS,
		TONE_OPTIONS,
		FRAMEWORK_OPTIONS,
		HUMANIZE_OPTIONS,
		DEFAULT_PROMPT_VALUES,
		type SelectOption
	} from '$lib/studio/types';
	import { isHighStakesCategory } from '$lib/studio/validation';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import SpaceSelector from '$lib/components/create/SpaceSelector.svelte';
	import TagInput from '$lib/components/create/TagInput.svelte';
	import SeriesSelector from '$lib/components/create/panel/SeriesSelector.svelte';
	import CreateJourneyPanel from '$lib/components/create/panel/CreateJourneyPanel.svelte';

	interface VoiceOption {
		id: string;
		name: string;
		voice_prompt?: string;
	}

	interface Props {
		values: PromptFormValues;
		spaces: MeSpace[];
		series: BlogSeries[];
		assessmentDimensions: string[];
		maxTags: number;
		canPersona: boolean;
		personas: Persona[];
		voices: VoiceOption[];
		/** GET /content/status-options — the authoritative status list (Rule 16). */
		statusOptions: SelectOption[];
		/** GET /content/visibility-options — the authoritative visibility list. */
		visibilityOptions: SelectOption[];
	}

	let {
		values = $bindable(),
		spaces = $bindable(),
		series = $bindable(),
		assessmentDimensions,
		maxTags,
		canPersona,
		personas,
		voices,
		statusOptions,
		visibilityOptions
	}: Props = $props();

	const highStakes = $derived(isHighStakesCategory(values.guide_category));

	// Adapters between the prompt form's scalar fields and the composer's
	// multi-value components.
	const spaceIds = $derived(values.space_id ? [values.space_id] : []);
	const dimension = $derived(values.field_values.roadmap_dimension || null);
	const purpose = $derived(values.field_values.roadmap_purpose || null);

	function setFieldValue(key: string, v: string | null) {
		if (v) values.field_values[key] = v;
		else delete values.field_values[key];
	}

	const noop = () => {};
	const emptyMsg = t('common.no_results', 'No results found');

	// Static option lists → AutocompleteItem[] for the select-style fields.
	// Labels are prompt-DSL vocabulary, but they're still user-facing UI —
	// routed through t() so non-English locales don't leak English (§7).
	const optLabel = (group: string, value: string, fallback: string) =>
		t(`studio.prompts.opt.${group}.${value || 'none'}`, fallback);
	const modeItems = $derived(MODE_OPTIONS.map((o) => ({ id: o.value, label: optLabel('mode', o.value, o.label) })));
	const toneItems = $derived(TONE_OPTIONS.map((o) => ({ id: o.value, label: optLabel('tone', o.value, o.label) })));
	const frameworkItems = $derived(FRAMEWORK_OPTIONS.map((o) => ({ id: o.value, label: optLabel('framework', o.value, o.label) })));
	const humanizeItems = $derived(HUMANIZE_OPTIONS.map((o) => ({ id: o.value, label: optLabel('humanize', o.value, o.label) })));
	const personaItems = $derived(personas.map((p) => ({ id: p.id, label: p.profile_name })));

	// Ref: apps/studio-desktop prompt page — voices are named for the tone
	// they embody ("Professional", "Professional Warm", ...). The tone of a
	// voice is inferred from its label prefix, and the Voice field only
	// offers voices matching the selected Tone.
	function voiceToneFromLabel(label: string): string {
		const lower = label.toLowerCase();
		for (const tone of TONE_OPTIONS) {
			if (lower === tone.value || lower.startsWith(tone.value + ' ')) {
				return tone.value;
			}
		}
		return 'natural';
	}

	const voiceItems = $derived(
		voices
			.filter((v) => voiceToneFromLabel(v.name) === values.tone)
			.map((v) => ({ id: v.id, label: v.name }))
	);

	$effect(() => {
		// Keep voice consistent with tone — clear it when the selected tone
		// no longer matches the chosen voice.
		if (values.voice_id && !voiceItems.some((v) => v.id === values.voice_id)) {
			values.voice_id = '';
		}
	});
	const statusItems = $derived(
		statusOptions
			.filter((s) => !highStakes || (s.value !== 'published' && s.value !== 'scheduled'))
			.map((s) => ({ id: s.value, label: s.label }))
	);
	$effect(() => {
		// High-stakes categories can never publish/schedule — the option is
		// filtered out above, but a stale value picked before the category
		// was set would otherwise linger with an empty label.
		if (highStakes && (values.status === 'published' || values.status === 'scheduled')) {
			values.status = 'in_review';
		}
	});
	const visibilityItems = $derived(
		visibilityOptions.map((s) => ({ id: s.value, label: s.label }))
	);

	function labelFor(items: { id: string; label: string }[], id: string): string {
		return items.find((i) => i.id === id)?.label ?? '';
	}
</script>

{#snippet selectField(
	label: string,
	items: AutocompleteItem[],
	selectedId: string,
	onPick: (item: AutocompleteItem) => void,
	onClear?: () => void
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
			{onClear}
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

{#snippet textField(label: string, value: string, onInput: (v: string) => void, placeholder = '')}
	<div class="field-row">
		<span class="field-label">{label}</span>
		<input
			type="text"
			class="field-input"
			{value}
			{placeholder}
			oninput={(e) => onInput(e.currentTarget.value)}
		/>
	</div>
{/snippet}

<Tabs.Root value="prompt">
	<Tabs.List class="right-panel-tabs">
		<Tabs.Trigger value="prompt" class="right-panel-tab">
			{t('studio.prompts.tab.prompt', 'Prompt')}
		</Tabs.Trigger>
		<Tabs.Trigger value="structure" class="right-panel-tab">
			{t('studio.prompts.tab.structure', 'Structure')}
		</Tabs.Trigger>
		<Tabs.Trigger value="journey" class="right-panel-tab">
			{t('create.panel.journey')}
		</Tabs.Trigger>
		<Tabs.Trigger value="settings" class="right-panel-tab">
			{t('create.panel.settings')}
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="prompt" class="pt-2">
		<div class="panel-stack">
			{@render selectField(t('studio.prompts.mode', 'Mode'), modeItems, values.mode, (i) => (values.mode = i.id as PromptMode), () => (values.mode = DEFAULT_PROMPT_VALUES.mode))}
			{@render textField(t('studio.prompts.audience', 'Audience'), values.audience, (v) => (values.audience = v), t('studio.prompts.audience_placeholder', 'e.g. Nigerians relocating to Canada — defaults to general JapaConnect readers'))}

			{#if canPersona}
				{@render selectField(t('studio.prompts.persona', 'Persona'), personaItems, values.persona_id, (i) => (values.persona_id = i.id), () => (values.persona_id = ''))}
			{/if}
			{@render selectField(t('studio.prompts.tone', 'Tone'), toneItems, values.tone, (i) => (values.tone = i.id as PromptTone), () => (values.tone = DEFAULT_PROMPT_VALUES.tone))}
			{#if canPersona}
				{@render selectField(t('studio.prompts.voice', 'Voice'), voiceItems, values.voice_id, (i) => (values.voice_id = i.id), () => (values.voice_id = ''))}
			{/if}

			<div class="pair">
				{@render selectField(t('studio.prompts.framework', 'Framework'), frameworkItems, values.framework, (i) => (values.framework = i.id as PromptFramework), () => (values.framework = DEFAULT_PROMPT_VALUES.framework))}
				{@render selectField(t('studio.prompts.humanize', 'Humanize'), humanizeItems, values.humanize_level, (i) => (values.humanize_level = i.id as HumanizeLevel), () => (values.humanize_level = DEFAULT_PROMPT_VALUES.humanize_level))}
			</div>
		</div>
	</Tabs.Content>

	<Tabs.Content value="structure" class="pt-2">
		<div class="panel-stack">
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

	<Tabs.Content value="journey" class="pt-2">
		<CreateJourneyPanel
			journeyStage={values.journey_stage || null}
			visaRoute={values.visa_route || null}
			sourceCountry={values.source_country || null}
			targetCountry={values.target_country || null}
			{dimension}
			{purpose}
			{assessmentDimensions}
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
		<div class="panel-stack">
			<div class="field-row">
				<span class="field-label">{t('studio.prompts.series', 'Series')}</span>
				<SeriesSelector
					bind:series
					seriesId={values.series_id || null}
					onselect={(id) => (values.series_id = id ?? '')}
				/>
			</div>

			<div class="field-row">
				<label class="field-label" for="pp-publish-at">{t('studio.prompts.publish_at', 'Publish at')}</label>
				<input
					id="pp-publish-at"
					type="datetime-local"
					class="field-input"
					value={values.publish_at}
					oninput={(e) => (values.publish_at = e.currentTarget.value)}
				/>
			</div>

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

			<div class="pair">
				{@render selectField(t('studio.prompts.status', 'Status'), statusItems, values.status, (i) => (values.status = i.id), () => (values.status = DEFAULT_PROMPT_VALUES.status))}
				{@render selectField(t('studio.prompts.visibility', 'Visibility'), visibilityItems, values.visibility, (i) => (values.visibility = i.id), () => (values.visibility = DEFAULT_PROMPT_VALUES.visibility))}
			</div>
			{#if highStakes}
				<p class="hint warn">
					{t('studio.prompts.high_stakes', 'High-stakes category — saved as in_review regardless of selected status.')}
				</p>
			{/if}

			<label class="toggle"><input type="checkbox" bind:checked={values.seo_indexable} /> {t('studio.prompts.seo_indexable', 'Indexable')}</label>

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
					onchange={(e) => {
						// Commit on blur, not on input — writing field_values
						// mid-keystroke re-renders value= and would erase a
						// line the user is still typing (no colon yet).
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
</Tabs.Root>

<style>
	/* The right-panel-tabs/right-panel-tab globals are defined in
	 * CreateRightPanel, but that component's CSS ships in the /studio/create
	 * code-split chunk — duplicating them here so the prompts rail renders
	 * the same chrome without a prefetch. */
	:global(.right-panel-tabs) {
		display: flex;
		border-bottom: 1px solid var(--color-border);
	}
	:global(.right-panel-tab) {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		padding: 10px 12px;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-muted);
		cursor: pointer;
		transition: color 0.15s;
	}
	:global(.right-panel-tab:hover) {
		color: var(--color-text);
	}
	:global([data-state='active'].right-panel-tab) {
		color: var(--color-text);
		border-bottom: 2px solid var(--color-primary);
	}
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
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 8px 12px;
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
