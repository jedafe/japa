<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §4 — right rail in
	// the CreateRightPanel idiom: same Guide | Journey | Settings | SEO tabs,
	// reusing the composer's own panel components so the prompts tab looks
	// and behaves like the composer. Status + field_values are appended to
	// the Settings tab (the composer has no equivalent).
	import { Tabs } from 'bits-ui';
	import { t } from '$lib/i18n';
	import type { MeSpace, Visibility, MediaAsset } from '$lib/types';
	import type { Persona, PromptFormValues } from '$lib/studio/types';
	import { slugify } from '$lib/studio/prompt';
	import { isHighStakesCategory } from '$lib/studio/validation';
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
</script>

<Tabs.Root value="guide">
	<Tabs.List class="right-panel-tabs">
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

			{#if canPersona}
				<div class="field-row">
					<label class="field-label" for="pp-persona">{t('studio.prompts.persona', 'Persona')}</label>
					<select id="pp-persona" bind:value={values.persona_id} class="field-input">
						<option value="">{t('studio.prompts.none', '—')}</option>
						{#each personas as p (p.id)}
							<option value={p.id}>{p.profile_name}</option>
						{/each}
					</select>
				</div>
				<div class="field-row">
					<label class="field-label" for="pp-voice">{t('studio.prompts.voice', 'Voice')}</label>
					<select id="pp-voice" bind:value={values.voice_id} class="field-input">
						<option value="">{t('studio.prompts.none', '—')}</option>
						{#each voices as v (v.id)}
							<option value={v.id}>{v.name}</option>
						{/each}
					</select>
				</div>
			{/if}
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
			<div class="field-row">
				<label class="field-label" for="pp-status">{t('studio.prompts.status', 'Status')}</label>
				<select id="pp-status" bind:value={values.status} class="field-input">
					{#each ['draft', 'in_review', 'published', 'scheduled'] as st (st)}
						<option value={st} disabled={highStakes && (st === 'published' || st === 'scheduled')}>
							{st}
						</option>
					{/each}
				</select>
			</div>
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
	.field-textarea {
		height: auto;
		padding: 8px 14px;
		font-family: inherit;
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
