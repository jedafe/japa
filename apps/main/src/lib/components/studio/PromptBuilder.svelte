<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §4 — prompt builder
	// mirroring AiDraftPanel + §105. Member mode by default; persona/voice
	// selectors only render when the viewer holds persona.view (canPersona).
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import type { ContentTypeMeta, MeSpace } from '$lib/types';
	import type { Persona, PromptFormValues } from '$lib/studio/types';
	import {
		DEFAULT_PROMPT_VALUES,
		promptFromDefaults,
		TONE_OPTIONS,
		FRAMEWORK_OPTIONS,
		MODE_OPTIONS,
		HUMANIZE_OPTIONS,
		type StudioMode
	} from '$lib/studio/types';
	import { buildPrompt, buildProjectInstruction, defaultFrameworkFor, type PromptLookups } from '$lib/studio/prompt';
	import { isHighStakesCategory } from '$lib/studio/validation';
	import { loadStudioDefaults, type StudioWebDefaults } from '$lib/studio/defaults';
	import { Copy, Download, Check, Wand2 } from 'lucide-svelte';

	interface VoiceOption {
		id: string;
		name: string;
		voice_prompt?: string;
	}

	interface Props {
		contentTypes: ContentTypeMeta[];
		spaces: MeSpace[];
		canPersona: boolean;
		personas: Persona[];
		voices: VoiceOption[];
	}

	let { contentTypes, spaces, canPersona, personas, voices }: Props = $props();

	const mode: StudioMode = $derived(canPersona ? 'operator' : 'member');

	let values = $state<PromptFormValues>(structuredClone(DEFAULT_PROMPT_VALUES));
	let copied = $state(false);
	let frameworkTouched = $state(false);

	// Project defaults apply once on mount — before the user can interact —
	// which satisfies "pre-fill only untouched fields" in practice.
	onMount(() => {
		const defaults: StudioWebDefaults = loadStudioDefaults();
		if (canPersona) {
			values = promptFromDefaults(defaults);
		} else {
			// Member mode: persona/voice defaults are meaningless.
			const { default_persona_id: _p, default_voice_id: _v, ...memberDefaults } = defaults;
			values = promptFromDefaults(memberDefaults);
		}
	});

	const selectedType = $derived(contentTypes.find((ct) => ct.value === values.content_type));
	const categories = $derived(selectedType?.categories ?? []);
	const highStakes = $derived(isHighStakesCategory(values.guide_category));

	$effect(() => {
		if (!frameworkTouched) {
			values.framework = (defaultFrameworkFor(values.content_type) || '') as PromptFormValues['framework'];
		}
	});

	const lookups = $derived<PromptLookups>({
		contentTypeName: selectedType?.label ?? '',
		guideCategoryName: categories.find((c) => c.Slug === values.guide_category)?.Label ?? '',
		personaName: personas.find((p) => p.id === values.persona_id)?.profile_name ?? '',
		voiceName: voices.find((v) => v.id === values.voice_id)?.name ?? '',
		voicePrompt: voices.find((v) => v.id === values.voice_id)?.voice_prompt,
		spaceName: spaces.find((s) => s.id === values.space_id)?.name ?? ''
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

	const inputCls =
		'mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]';
	const labelCls = 'block text-xs font-medium text-[var(--color-muted)]';
</script>

<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
		<Wand2 size={16} /> {t('studio.prompts.builder', 'Prompt builder')}
	</h2>

	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
		<label class={labelCls}>
			{t('studio.prompts.mode', 'Mode')}
			<select bind:value={values.mode} class={inputCls}>
				{#each MODE_OPTIONS as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</label>

		<label class={labelCls}>
			{t('studio.prompts.content_type', 'Content type')}
			<select bind:value={values.content_type} class={inputCls}>
				{#each contentTypes as ct (ct.value)}
					<option value={ct.value}>{ct.label}</option>
				{/each}
			</select>
		</label>

		{#if categories.length > 0}
			<label class={labelCls}>
				{t('studio.prompts.guide_category', 'Guide category')}
				<select bind:value={values.guide_category} class={inputCls}>
					<option value="">{t('studio.prompts.none', '—')}</option>
					{#each categories as cat (cat.Slug)}
						<option value={cat.Slug}>{cat.Label}</option>
					{/each}
				</select>
			</label>
		{/if}

		<label class={labelCls}>
			{t('studio.prompts.topic', 'Topic')}
			<input type="text" bind:value={values.topic} class={inputCls} />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.title', 'Title')}
			<input type="text" bind:value={values.title} class={inputCls} />
		</label>

		{#if canPersona}
			<label class={labelCls}>
				{t('studio.prompts.persona', 'Persona')}
				<select bind:value={values.persona_id} class={inputCls}>
					<option value="">{t('studio.prompts.none', '—')}</option>
					{#each personas as p (p.id)}
						<option value={p.id}>{p.profile_name}</option>
					{/each}
				</select>
			</label>

			<label class={labelCls}>
				{t('studio.prompts.voice', 'Voice')}
				<select bind:value={values.voice_id} class={inputCls}>
					<option value="">{t('studio.prompts.none', '—')}</option>
					{#each voices as v (v.id)}
						<option value={v.id}>{v.name}</option>
					{/each}
				</select>
			</label>
		{/if}

		<label class={labelCls}>
			{t('studio.prompts.tone', 'Tone')}
			<select bind:value={values.tone} class={inputCls}>
				{#each TONE_OPTIONS as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</label>

		<label class={labelCls}>
			{t('studio.prompts.framework', 'Framework')}
			<select
				bind:value={values.framework}
				onchange={() => (frameworkTouched = true)}
				class={inputCls}
			>
				{#each FRAMEWORK_OPTIONS as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</label>

		<label class={labelCls}>
			{t('studio.prompts.humanize', 'Humanize')}
			<select bind:value={values.humanize_level} class={inputCls}>
				{#each HUMANIZE_OPTIONS as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</label>

		<label class={labelCls}>
			{t('studio.prompts.min_words', 'Min words')}
			<input type="number" min="0" bind:value={values.min_words} class={inputCls} />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.heading_count', 'H2 headings')}
			<input type="number" min="0" bind:value={values.heading_count} class={inputCls} />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.paragraphs_per_heading', 'Paragraphs per heading')}
			<input type="number" min="0" bind:value={values.min_paragraphs_per_heading} class={inputCls} />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.internal_links', 'Internal links')}
			<input type="number" min="0" bind:value={values.internal_links} class={inputCls} />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.external_links', 'External links')}
			<input type="number" min="0" bind:value={values.external_links} class={inputCls} />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.space', 'Space')}
			<select bind:value={values.space_id} class={inputCls}>
				<option value="">{t('studio.prompts.none', '—')}</option>
				{#each spaces as s (s.id)}
					<option value={s.id}>{s.name}</option>
				{/each}
			</select>
		</label>

		<label class={labelCls}>
			{t('studio.prompts.status', 'Status')}
			<select bind:value={values.status} class={inputCls}>
				{#each ['draft', 'in_review', 'published', 'scheduled'] as st (st)}
					<option value={st} disabled={highStakes && (st === 'published' || st === 'scheduled')}>{st}</option>
				{/each}
			</select>
		</label>

		<label class={labelCls}>
			{t('studio.prompts.visibility', 'Visibility')}
			<select bind:value={values.visibility} class={inputCls}>
				{#each ['public', 'members', 'private'] as v (v)}
					<option value={v}>{v}</option>
				{/each}
			</select>
		</label>

		{#if values.status === 'scheduled'}
			<label class={labelCls}>
				{t('studio.prompts.publish_at', 'Publish at')}
				<input type="datetime-local" bind:value={values.publish_at} class={inputCls} />
			</label>
		{/if}

		<label class={labelCls}>
			{t('studio.prompts.audience', 'Audience')}
			<input type="text" bind:value={values.audience} class={inputCls} />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.source_country', 'Source country')}
			<input type="text" bind:value={values.source_country} class={inputCls} placeholder="NG" />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.target_country', 'Target country')}
			<input type="text" bind:value={values.target_country} class={inputCls} placeholder="CA" />
		</label>

		<label class={labelCls}>
			{t('studio.prompts.tags', 'Tags (comma-separated)')}
			<input
				type="text"
				class={inputCls}
				value={values.tags.join(', ')}
				oninput={(e) => {
					values.tags = e.currentTarget.value
						.split(',')
						.map((x) => x.trim())
						.filter(Boolean);
				}}
			/>
		</label>
	</div>

	{#if highStakes}
		<p class="mt-3 text-xs text-[var(--color-warning)]">
			{t('studio.prompts.high_stakes', 'High-stakes category — saved as in_review regardless of selected status.')}
		</p>
	{/if}

	<div class="mt-3 flex flex-wrap gap-4 text-sm text-[var(--color-text)]">
		<label class="flex items-center gap-1.5"><input type="checkbox" bind:checked={values.include_intro} /> {t('studio.prompts.inc_intro', 'Intro')}</label>
		<label class="flex items-center gap-1.5"><input type="checkbox" bind:checked={values.include_conclusion} /> {t('studio.prompts.inc_conclusion', 'Conclusion')}</label>
		<label class="flex items-center gap-1.5"><input type="checkbox" bind:checked={values.include_faqs} /> {t('studio.prompts.inc_faqs', 'FAQs')}</label>
		<label class="flex items-center gap-1.5"><input type="checkbox" bind:checked={values.include_steps} /> {t('studio.prompts.inc_steps', 'Steps')}</label>
		<label class="flex items-center gap-1.5"><input type="checkbox" bind:checked={values.include_key_takeaways} /> {t('studio.prompts.inc_takeaways', 'Key takeaways')}</label>
		<label class="flex items-center gap-1.5"><input type="checkbox" bind:checked={values.include_inline_image} /> {t('studio.prompts.inc_images', 'Image suggestions')}</label>
	</div>

	<details class="mt-4">
		<summary class="cursor-pointer text-xs font-medium text-[var(--color-muted)]">
			{t('studio.prompts.advanced', 'Advanced (journey, SEO, field values)')}
		</summary>
		<div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
			<label class={labelCls}>
				{t('studio.prompts.journey_stage', 'Journey stage')}
				<input type="text" bind:value={values.journey_stage} class={inputCls} />
			</label>
			<label class={labelCls}>
				{t('studio.prompts.visa_route', 'Visa route')}
				<input type="text" bind:value={values.visa_route} class={inputCls} />
			</label>
			<label class={labelCls}>
				{t('studio.prompts.meta_title', 'Meta title')}
				<input type="text" bind:value={values.meta_title} class={inputCls} />
			</label>
			<label class={labelCls}>
				{t('studio.prompts.meta_description', 'Meta description')}
				<input type="text" bind:value={values.meta_description} class={inputCls} />
			</label>
			<label class={labelCls}>
				{t('studio.prompts.canonical_url', 'Canonical URL')}
				<input type="url" bind:value={values.canonical_url} class={inputCls} />
			</label>
			<label class={labelCls}>
				{t('studio.prompts.og_image', 'OG image URL')}
				<input type="url" bind:value={values.og_image_url} class={inputCls} />
			</label>
			<label class="flex items-center gap-1.5 text-sm text-[var(--color-text)]">
				<input type="checkbox" bind:checked={values.seo_indexable} /> {t('studio.prompts.seo_indexable', 'Indexable')}
			</label>
			<label class={`${labelCls} sm:col-span-2`}>
				{t('studio.prompts.field_values', 'Field values (key: value, one per line)')}
				<textarea
					rows="3"
					class={inputCls}
					value={Object.entries(values.field_values).map(([k, v]) => `${k}: ${v}`).join('\n')}
					oninput={(e) => {
						const fv: Record<string, string> = {};
						for (const line of e.currentTarget.value.split('\n')) {
							const idx = line.indexOf(':');
							if (idx > 0) fv[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
						}
						values.field_values = fv;
					}}
				></textarea>
			</label>
		</div>
	</details>

	<div class="mt-4">
		<div class="mb-2 flex items-center justify-between">
			<span class="text-xs font-medium text-[var(--color-muted)]">{t('studio.prompts.preview', 'Generated prompt')}</span>
			<div class="flex gap-2">
				<button
					type="button"
					onclick={copyPrompt}
					class="flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90"
				>
					{#if copied}<Check size={14} />{:else}<Copy size={14} />{/if}
					{copied ? t('studio.prompts.copied', 'Copied') : t('studio.prompts.copy', 'Copy prompt')}
				</button>
				<button
					type="button"
					onclick={downloadInstructions}
					class="flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-elevated)]"
				>
					<Download size={14} /> {t('studio.prompts.download', 'Download CLAUDE.md')}
				</button>
			</div>
		</div>
		<pre class="max-h-64 overflow-y-auto whitespace-pre-wrap rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-xs text-[var(--color-text)]">{promptText}</pre>
	</div>
</div>
