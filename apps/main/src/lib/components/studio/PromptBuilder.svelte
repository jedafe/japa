<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §4 — prompt builder
	// main window in composer idiom: composer-header (title + content type),
	// guide-category Autocomplete, TitleField, generated prompt preview.
	// All generation + metadata fields live in the right rail tabs
	// (PromptRightPanel).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import type { ContentTypeMeta, CustomFieldGroup } from '$lib/types';
	import type { Persona, PromptFormValues } from '$lib/studio/types';
	import { defaultFrameworkFor, buildPrompt, buildProjectInstruction, type PromptLookups } from '$lib/studio/prompt';
	import type { StudioMode } from '$lib/studio/types';
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

	let { values = $bindable(), contentTypes, canPersona, personas, voices, spaceName }: Props = $props();

	const mode: StudioMode = $derived(canPersona ? 'operator' : 'member');

	let copied = $state(false);

	const selectedType = $derived(contentTypes.find((ct) => ct.value === values.content_type));
	const categories = $derived(selectedType?.categories ?? []);
	// The selector renders whenever the type declares categories — even when
	// the admin hasn't configured any yet — so guide_category always has a
	// path into the generated prompt (free-text slug as fallback).
	const hasCategory = $derived(
		selectedType?.has_category === true || categories.length > 0
	);
	const highStakes = $derived(isHighStakesCategory(values.guide_category));

	const typeItems = $derived(contentTypes.map((ct) => ({ id: ct.value, label: ct.label })));
	const typeFetcher = $derived(staticAutocompleteFetcher(typeItems));

	const guideCategoryItems = $derived(categories.map((c) => ({ id: c.Slug, label: c.Label })));
	const guideCategoryFetcher = $derived(staticAutocompleteFetcher(guideCategoryItems));
	const guideCategoryLabel = $derived(
		categories.find((c) => c.Slug === values.guide_category)?.Label ?? values.guide_category
	);

	// The framework always follows the selected content type's default —
	// the user can override it in the right rail afterwards, and switching
	// type re-defaults it.
	$effect(() => {
		values.framework = (defaultFrameworkFor(values.content_type) || '') as PromptFormValues['framework'];
	});

	// Load category-specific custom field groups when a guide category is
	// selected — the same endpoint the composer uses (Section 14). The
	// schema is injected into the generated prompt so the agent knows
	// which field_values keys the category expects.
	let categoryFieldGroups = $state<CustomFieldGroup[]>([]);
	let fieldGroupRequest = 0;
	$effect(() => {
		const cat = values.guide_category;
		const type = values.content_type;
		if (!cat || type !== 'guide') {
			categoryFieldGroups = [];
			return;
		}
		const token = ++fieldGroupRequest;
		void (async () => {
			const res = await api.get<CustomFieldGroup[]>(
				`/custom-field-groups?entity_class=${encodeURIComponent('content_type:guide:' + cat)}`
			);
			if (token !== fieldGroupRequest) return;
			categoryFieldGroups = res.error ? [] : (res.data ?? []);
		})();
	});

	// The SSR voices list ships id+name only — the full catalog with
	// voice_prompt is ~2.6MB. The selected voice's prompt is fetched on
	// demand from the persona.view-gated admin read route (same gate as
	// canPersona) and cached per id so switching voices never refetches.
	const voicePrompts = $state<Record<string, string>>({});
	const voicePromptPending = new Set<string>();
	$effect(() => {
		const id = values.voice_id;
		if (!id || id in voicePrompts || voicePromptPending.has(id)) return;
		voicePromptPending.add(id);
		void (async () => {
			const res = await api.get<{ voice_prompt?: string }>(
				`/admin/persona-voices/${encodeURIComponent(id)}`
			);
			voicePromptPending.delete(id);
			voicePrompts[id] = res.data?.voice_prompt ?? '';
		})();
	});

	const lookups = $derived<PromptLookups>({
		contentTypeName: selectedType?.label ?? '',
		guideCategoryName: guideCategoryLabel,
		personaName: personas.find((p) => p.id === values.persona_id)?.profile_name ?? '',
		voiceName: voices.find((v) => v.id === values.voice_id)?.name ?? '',
		voicePrompt:
			voices.find((v) => v.id === values.voice_id)?.voice_prompt ??
			voicePrompts[values.voice_id],
		spaceName,
		categoryFields: categoryFieldGroups.flatMap((g) => g.fields)
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
	<div class="type-field">
		<Autocomplete
			placeholder={t('studio.prompts.content_type', 'Content type')}
			emptyMessage={t('common.no_results', 'No results found')}
			fetcher={typeFetcher}
			initialFetcher={typeFetcher}
			selectedId={values.content_type || null}
			selectedLabel={selectedType?.label ?? ''}
			onSelect={(item) => {
				values.content_type = item.id;
				values.guide_category = '';
			}}
		/>
	</div>
</div>

{#if hasCategory}
	<div class="guide-category-selector">
		<label for="guide-category" class="guide-category-label">
			{t('studio.prompts.guide_category', 'Guide category')}
		
		<HintIcon text={t('studio.prompts.category_hint', 'The guide category this prompt targets.')}  />
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
			allowFreeText
		/>
	</div>
{/if}

<TitleField title={values.title} onTitle={(v) => (values.title = v)} />

{#if highStakes}
	<p class="high-stakes-hint">
		{t('studio.prompts.high_stakes', 'High-stakes category — saved as in_review regardless of selected status.')}
	</p>
{/if}

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
	.type-field {
		width: 15rem;
		flex-shrink: 0;
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
</style>
