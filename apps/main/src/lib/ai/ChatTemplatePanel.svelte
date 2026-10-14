<script lang="ts">
	// ChatTemplatePanel — the /ai right rail: saved generation-spec presets
	// plus a Prompt|Structure editor, in the PromptRightPanel idiom.
	//
	// Ownership split: the panel owns editor UI + list interactions and
	// binds `values`/`activeId`/`active` upward; the parent (JapaAIChat)
	// owns persistence (onpersist → PUT /me/ai-preferences) and send-time
	// spec injection.
	//
	// "active" means the spec auto-attaches to every outgoing message;
	// "Insert" drops a readable version into the composer instead and
	// detaches so a send doesn't carry the spec twice.
	import { Tabs } from 'bits-ui';
	import { Plus, Trash2, X, Check, Loader2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { AutocompleteSelect } from '$lib/components/ui';
	// Shared option vocabularies + the composer's own Journey/Settings
	// pickers — same components the studio prompt rail embeds (Rule 16).
	import { TONE_OPTIONS, FRAMEWORK_OPTIONS, HUMANIZE_OPTIONS, type SelectOption } from '$lib/studio/types';
	import type { MeSpace, BlogSeries, ContentTypeMeta, CustomFieldGroup } from '$lib/types';
	import { isHighStakesCategory } from '$lib/studio/validation';
	import { api } from '$lib/api/client';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import SpaceSelector from '$lib/components/create/SpaceSelector.svelte';
	import TagInput from '$lib/components/create/TagInput.svelte';
	import SeriesSelector from '$lib/components/create/panel/SeriesSelector.svelte';
	import CreateJourneyPanel from '$lib/components/create/panel/CreateJourneyPanel.svelte';
	import {
		DEFAULT_CHAT_TEMPLATE_VALUES,
		normalizeTemplateValues,
		templateFingerprint,
		type ChatTemplate,
		type ChatTemplateFramework,
		type ChatTemplateHumanize,
		type ChatTemplateValues
	} from './chatTemplate';

	interface Props {
		templates: ChatTemplate[];
		values: ChatTemplateValues;
		activeId: string;
		active: boolean;
		/** GET /content-types option list (has_category/categories drive
		 * the guide-category selector) — empty renders a text input. */
		contentTypes: ContentTypeMeta[];
		/** Member spaces + blog series for the Settings tab selectors —
		 * bindable because the selectors append freshly created rows. */
		spaces: MeSpace[];
		series: BlogSeries[];
		/** GET /assessment dimensions — Journey tab's roadmap pickers. */
		assessmentDimensions: string[];
		/** /ai/voices catalog — the persona voice the reply renders with;
		 * empty hides the Voice field (same rule as the composer picker). */
		voices: { id: string; name: string }[];
		maxTags: number;
		/** /content/{status,visibility}-options — live catalogs (Rule 16). */
		statusOptions: SelectOption[];
		visibilityOptions: SelectOption[];
		/** Persist a full template list server-side; resolves to an error
		 * message or null on success. The parent reassigns `templates`. */
		onpersist: (next: ChatTemplate[]) => Promise<string | null>;
		/** Insert a readable version of the spec into the composer input. */
		oninsert: () => void;
		onclose?: () => void;
	}

	let {
		templates,
		values = $bindable(),
		activeId = $bindable(),
		active = $bindable(),
		contentTypes,
		spaces = $bindable(),
		series = $bindable(),
		assessmentDimensions,
		voices,
		maxTags,
		statusOptions,
		visibilityOptions,
		onpersist,
		oninsert,
		onclose
	}: Props = $props();

	let saveName = $state('');
	let saving = $state(false);
	let saveErr = $state('');

	const activeTemplate = $derived(templates.find((tpl) => tpl.id === activeId));
	// Working-copy divergence — edits to a loaded template that haven't
	// been saved back yet. Drives the "unsaved changes" hint.
	const dirty = $derived(
		!!activeTemplate &&
			templateFingerprint(values) !==
				templateFingerprint(normalizeTemplateValues(activeTemplate.values))
	);
	// Broader than `dirty` — also true for an unsaved custom spec (no
	// activeId), so switching templates can't silently drop typed edits.
	const editsPresent = $derived(
		templateFingerprint(values) !==
			templateFingerprint(
				activeTemplate
					? normalizeTemplateValues(activeTemplate.values)
					: DEFAULT_CHAT_TEMPLATE_VALUES
			)
	);
	function confirmDiscard(): boolean {
		return (
			!editsPresent ||
			window.confirm(t('ai.templates.discard_changes', 'Discard unsaved template changes?'))
		);
	}

	const toneOptions = $derived(
		TONE_OPTIONS.map((o) => ({ ...o, label: t(`ai.body.tone_${o.value}`, o.label) }))
	);
	const frameworkOptions = $derived(
		FRAMEWORK_OPTIONS.map((o) =>
			o.value === '' ? { ...o, label: t('ai.body.framework_none', 'No framework') } : o
		)
	);
	const humanizeOptions = $derived(
		HUMANIZE_OPTIONS.map((o) => ({
			...o,
			label: t(`ai.body.humanize_${o.value}`, o.label)
		}))
	);

	// Same "Default voice" prepend rule as the composer's voiceOptions —
	// the backend may already emit an empty-id row (/ai/voices). A saved
	// voice no longer in the catalog stays visible as an extra option
	// instead of rendering a blank pick.
	const voiceOptions = $derived([
		...(voices.some((v) => v.id === '')
			? []
			: [{ value: '', label: t('ai.chat.voice_default', 'Default voice') }]),
		...voices.map((v) => ({ value: v.id, label: v.name })),
		...(values.voice_id && !voices.some((v) => v.id === values.voice_id)
			? [
					{
						value: values.voice_id,
						label: t('ai.templates.voice_unavailable', '{id} (no longer available)', {
							id: values.voice_id
						})
					}
				]
			: [])
	]);

	const typeOptions = $derived(
		contentTypes.map((ct) => ({ value: ct.value, label: ct.label }))
	);
	const emptyMsg = t('common.no_results', 'No results found');

	// Guide category — the selector only renders when the chosen content
	// type declares categories (same has_category rule the studio builder
	// uses). Free-text stays allowed: an admin may not have configured
	// categories yet, and a typed slug still lands in the spec.
	const selectedType = $derived(contentTypes.find((ct) => ct.value === values.content_type));
	const categories = $derived(selectedType?.categories ?? []);
	const hasCategory = $derived(selectedType?.has_category === true || categories.length > 0);
	const guideCategoryItems = $derived(categories.map((c) => ({ id: c.Slug, label: c.Label })));
	const guideCategoryFetcher = $derived(staticAutocompleteFetcher(guideCategoryItems));
	const guideCategoryLabel = $derived(
		categories.find((c) => c.Slug === values.guide_category)?.Label ?? values.guide_category
	);
	$effect(() => {
		// Switching to a type that declares no categories clears a stale
		// slug — but only when the type actually resolved (a free-text or
		// unloaded type must not wipe a saved template's category).
		if (selectedType && !hasCategory && values.guide_category) {
			values.guide_category = '';
		}
	});

	// High-stakes guide categories can't publish/schedule — same rule as
	// the studio rail: filter the status options and coerce a stale pick.
	const highStakes = $derived(isHighStakesCategory(values.guide_category));
	const statusOpts = $derived(
		statusOptions.filter(
			(s) => !highStakes || (s.value !== 'published' && s.value !== 'scheduled')
		)
	);
	$effect(() => {
		if (highStakes && (values.status === 'published' || values.status === 'scheduled')) {
			values.status = 'in_review';
		}
	});

	// Category-specific custom-field schema — same /custom-field-groups
	// endpoint the composer uses (§14). Rendered as a hint under
	// field_values so the template's keys match what the category expects.
	let categoryFieldKeys = $state<string[]>([]);
	let fieldGroupRequest = 0;
	$effect(() => {
		const cat = values.guide_category;
		const type = values.content_type;
		if (!cat || !hasCategory) {
			categoryFieldKeys = [];
			return;
		}
		const token = ++fieldGroupRequest;
		void (async () => {
			const res = await api.get<CustomFieldGroup[]>(
				`/custom-field-groups?entity_class=${encodeURIComponent(`content_type:${type}:${cat}`)}`
			);
			if (token !== fieldGroupRequest) return;
			categoryFieldKeys = res.error
				? []
				: (res.data ?? []).flatMap((g) => g.fields.map((f) => f.key));
		})();
	});

	// Journey tab's roadmap dimension/purpose live inside field_values,
	// matching the composer contract (§71).
	const dimension = $derived(values.field_values.roadmap_dimension || null);
	const purpose = $derived(values.field_values.roadmap_purpose || null);
	function setFieldValue(key: string, v: string | null) {
		if (v) values.field_values[key] = v;
		else delete values.field_values[key];
	}

	function select(tpl: ChatTemplate) {
		if (!confirmDiscard()) return;
		values = normalizeTemplateValues(tpl.values);
		activeId = tpl.id;
		active = true;
		saveName = '';
		saveErr = '';
	}

	function newTemplate() {
		if (!confirmDiscard()) return;
		values = structuredClone(DEFAULT_CHAT_TEMPLATE_VALUES);
		activeId = '';
		active = true;
		saveName = '';
		saveErr = '';
	}

	// JSON round-trip clone — required because `values` is a $state proxy
	// and structuredClone can't clone proxies (values are JSON-safe by
	// construction: they persist to ai_prefs verbatim).
	function cloneValues(v: ChatTemplateValues): ChatTemplateValues {
		return JSON.parse(JSON.stringify(v)) as ChatTemplateValues;
	}

	async function run(next: ChatTemplate[]): Promise<boolean> {
		saving = true;
		saveErr = '';
		const err = await onpersist(next);
		saving = false;
		if (err) {
			saveErr = err;
			return false;
		}
		return true;
	}

	// Two templates with the same name are indistinguishable in the
	// picker — reject the collision before persisting.
	function nameTaken(name: string, excludeId = ''): boolean {
		const n = name.trim().toLowerCase();
		return templates.some((x) => x.id !== excludeId && x.name.trim().toLowerCase() === n);
	}

	async function save() {
		if (saving) return;
		const now = new Date().toISOString();
		if (activeId && activeTemplate) {
			// Overwrite the loaded template — a typed name renames it.
			const name = saveName.trim() || activeTemplate.name;
			if (nameTaken(name, activeId)) {
				saveErr = t('ai.templates.name_taken', 'A template with this name already exists.');
				return;
			}
			const next = templates.map((tpl) =>
				tpl.id === activeId
					? { ...tpl, name, values: cloneValues(values), updated_at: now }
					: tpl
			);
			if (await run(next)) saveName = '';
			return;
		}
		const name = saveName.trim();
		if (!name) {
			saveErr = t('ai.templates.name_required', 'Name the template first.');
			return;
		}
		if (nameTaken(name)) {
			saveErr = t('ai.templates.name_taken', 'A template with this name already exists.');
			return;
		}
		const tpl: ChatTemplate = {
			id: crypto.randomUUID(),
			name,
			values: cloneValues(values),
			created_at: now,
			updated_at: now
		};
		if (await run([tpl, ...templates])) {
			activeId = tpl.id;
			saveName = '';
		}
	}

	async function saveAsNew() {
		if (saving) return;
		const name =
			saveName.trim() ||
			(activeTemplate ? `${activeTemplate.name} (copy)` : t('ai.templates.untitled', 'Untitled'));
		if (nameTaken(name)) {
			saveErr = t('ai.templates.name_taken', 'A template with this name already exists.');
			return;
		}
		const now = new Date().toISOString();
		const tpl: ChatTemplate = {
			id: crypto.randomUUID(),
			name,
			values: cloneValues(values),
			created_at: now,
			updated_at: now
		};
		if (await run([tpl, ...templates])) {
			activeId = tpl.id;
			saveName = '';
		}
	}

	async function remove(id: string) {
		if (saving) return;
		const tpl = templates.find((x) => x.id === id);
		if (
			tpl &&
			!window.confirm(
				t('ai.templates.delete_confirm', 'Delete template "{name}"? This cannot be undone.', {
					name: tpl.name
				})
			)
		)
			return;
		const next = templates.filter((x) => x.id !== id);
		if (await run(next) && activeId === id) {
			// Keep the working values as an ad-hoc spec — deleting the saved
			// row shouldn't silently change what the next send carries.
			activeId = '';
		}
	}
</script>

{#snippet numField(label: string, value: number, onInput: (v: number) => void)}
	<div class="tpl-field">
		<span class="tpl-label">{label}</span>
		<input
			type="number"
			min="0"
			class="tpl-input"
			{value}
			oninput={(e) => onInput(Number(e.currentTarget.value) || 0)}
		/>
	</div>
{/snippet}

<aside
	class="flex h-full w-full flex-col overflow-hidden border-l border-[var(--color-border)] bg-[var(--color-surface)]"
>
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-3 py-2.5">
		<h2 class="text-sm font-semibold text-[var(--color-text)]">
			{t('ai.templates.title', 'Chat templates')}
		</h2>
		<div class="flex items-center gap-1">
			<button
				type="button"
				onclick={newTemplate}
				class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
			>
				<Plus size={13} />
				{t('ai.templates.new', 'New')}
			</button>
			{#if onclose}
				<button
					type="button"
					onclick={onclose}
					class="rounded p-1 text-[var(--color-muted)] hover:text-[var(--color-text)] lg:hidden"
					aria-label={t('common.close', 'Close')}
				>
					<X size={15} />
				</button>
			{/if}
		</div>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto">
		<!-- Saved templates -->
		{#if templates.length > 0}
			<ul class="border-b border-[var(--color-border)] p-2">
				{#each templates as tpl (tpl.id)}
					<li class="group relative">
						<button
							type="button"
							onclick={() => select(tpl)}
							class="w-full rounded-[var(--radius)] px-2.5 py-1.5 text-left text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] {activeId ===
							tpl.id
								? 'bg-[var(--color-primary)]/10 font-medium'
								: ''}"
						>
							<span class="flex items-center gap-1.5">
								<span class="truncate">{tpl.name}</span>
								{#if activeId === tpl.id && active}
									<Check size={12} class="shrink-0 text-[var(--color-primary)]" />
								{/if}
							</span>
						</button>
						<button
							type="button"
							onclick={() => remove(tpl.id)}
							class="absolute right-1 top-1/2 -translate-y-1/2 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-danger)] focus:opacity-100 group-hover:opacity-100"
							aria-label={t('ai.templates.delete', 'Delete template')}
							title={t('ai.templates.delete', 'Delete template')}
						>
							<Trash2 size={12} />
						</button>
					</li>
				{/each}
			</ul>
		{/if}

		<Tabs.Root value="prompt">
			<Tabs.List class="tpl-tabs">
				<Tabs.Trigger value="prompt" class="tpl-tab">
					{t('studio.prompts.tab.prompt', 'Prompt')}
				</Tabs.Trigger>
				<Tabs.Trigger value="structure" class="tpl-tab">
					{t('studio.prompts.tab.structure', 'Structure')}
				</Tabs.Trigger>
				<Tabs.Trigger value="journey" class="tpl-tab">
					{t('create.panel.journey', 'Journey')}
				</Tabs.Trigger>
				<Tabs.Trigger value="settings" class="tpl-tab">
					{t('create.panel.settings', 'Settings')}
				</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="prompt" class="pt-2">
				<div class="tpl-stack">
					<div class="tpl-field">
						<span class="tpl-label">{t('ai.templates.content_type', 'Content type')}</span>
						{#if typeOptions.length > 0}
							<AutocompleteSelect
								value={values.content_type}
								onValueChange={(v) => {
									values.content_type = v;
									values.guide_category = '';
								}}
								options={typeOptions}
								ariaLabel={t('ai.templates.content_type', 'Content type')}
							/>
						{:else}
							<input type="text" class="tpl-input" bind:value={values.content_type} />
						{/if}
					</div>

					{#if hasCategory}
						<div class="tpl-field">
							<span class="tpl-label"
								>{t('studio.prompts.guide_category', 'Guide category')}</span
							>
							<Autocomplete
								id="tpl-guide-category"
								placeholder={t('studio.prompts.guide_category', 'Guide category')}
								emptyMessage={emptyMsg}
								fetcher={guideCategoryFetcher}
								initialFetcher={guideCategoryFetcher}
								selectedId={values.guide_category || null}
								selectedLabel={guideCategoryLabel}
								onSelect={(item: AutocompleteItem) => {
									values.guide_category = item.id;
								}}
								onClear={() => {
									values.guide_category = '';
								}}
								allowFreeText
							/>
						</div>
					{/if}

					<div class="tpl-pair">
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.tone', 'Tone')}</span>
							<AutocompleteSelect
								bind:value={values.tone}
								options={toneOptions}
								ariaLabel={t('studio.prompts.tone', 'Tone')}
							/>
						</div>
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.framework', 'Framework')}</span>
							<AutocompleteSelect
								value={values.framework}
								onValueChange={(v) => (values.framework = v as ChatTemplateFramework)}
								options={frameworkOptions}
								ariaLabel={t('studio.prompts.framework', 'Framework')}
							/>
						</div>
					</div>

					<div class:tpl-pair={voiceOptions.length > 0}>
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.humanize', 'Humanize')}</span>
							<AutocompleteSelect
								value={values.humanize_level}
								onValueChange={(v) => (values.humanize_level = v as ChatTemplateHumanize)}
								options={humanizeOptions}
								ariaLabel={t('studio.prompts.humanize', 'Humanize')}
							/>
						</div>
						{#if voiceOptions.length > 0}
							<div class="tpl-field">
								<span class="tpl-label">{t('ai.templates.voice', 'Voice')}</span>
								<AutocompleteSelect
									bind:value={values.voice_id}
									options={voiceOptions}
									ariaLabel={t('ai.templates.voice', 'Voice')}
								/>
							</div>
						{/if}
					</div>

					<div class="tpl-field">
						<span class="tpl-label">{t('studio.prompts.audience', 'Audience')}</span>
						<input
							type="text"
							class="tpl-input"
							bind:value={values.audience}
							placeholder={t(
								'studio.prompts.audience_placeholder',
								'e.g. Nigerians relocating to Canada — defaults to general JapaConnect readers'
							)}
						/>
					</div>

					<div class="tpl-field">
						<label class="tpl-label" for="tpl-instructions"
							>{t('ai.templates.instructions', 'Extra instructions')}</label
						>
						<textarea
							id="tpl-instructions"
							rows="3"
							class="tpl-input tpl-textarea"
							bind:value={values.instructions}
							placeholder={t(
								'ai.templates.instructions_placeholder',
								'e.g. Always cite official sources; end with next steps'
							)}
						></textarea>
					</div>
				</div>
			</Tabs.Content>

			<Tabs.Content value="structure" class="pt-2">
				<div class="tpl-stack">
					<div class="tpl-pair">
						{@render numField(t('studio.prompts.min_words', 'Min words'), values.min_words, (v) => (values.min_words = v))}
						{@render numField(t('studio.prompts.heading_count', 'H2 headings'), values.heading_count, (v) => (values.heading_count = v))}
					</div>
					<div class="tpl-pair">
						{@render numField(t('studio.prompts.paragraphs_per_heading', 'Paras / heading'), values.min_paragraphs_per_heading, (v) => (values.min_paragraphs_per_heading = v))}
						{@render numField(t('studio.prompts.paragraph_count', 'Total paras'), values.paragraph_count, (v) => (values.paragraph_count = v))}
					</div>
					<div class="tpl-pair">
						{@render numField(t('studio.prompts.internal_links', 'Internal links'), values.internal_links, (v) => (values.internal_links = v))}
						{@render numField(t('studio.prompts.external_links', 'External links'), values.external_links, (v) => (values.external_links = v))}
					</div>

					<div class="tpl-toggles">
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_intro} />
							{t('studio.prompts.inc_intro', 'Intro')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_conclusion} />
							{t('studio.prompts.inc_conclusion', 'Conclusion')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_faqs} />
							{t('studio.prompts.inc_faqs', 'FAQs')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_steps} />
							{t('studio.prompts.inc_steps', 'Steps')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_key_takeaways} />
							{t('studio.prompts.inc_takeaways', 'Key takeaways')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_inline_image} />
							{t('studio.prompts.inc_images', 'Image suggestions')}</label
						>
					</div>
				</div>
			</Tabs.Content>

			<Tabs.Content value="journey" class="pt-2">
				<div class="tpl-stack">
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
						onedit={() => {}}
					/>
				</div>
			</Tabs.Content>

			<Tabs.Content value="settings" class="pt-2">
				<div class="tpl-stack">
					<div class="tpl-field">
						<span class="tpl-label">{t('studio.prompts.series', 'Series')}</span>
						<SeriesSelector
							bind:series
							seriesId={values.series_id || null}
							onselect={(id) => (values.series_id = id ?? '')}
						/>
					</div>

					<div class="tpl-field">
						<label class="tpl-label" for="tpl-publish-at"
							>{t('studio.prompts.publish_at', 'Publish at')}</label
						>
						<input
							id="tpl-publish-at"
							type="datetime-local"
							class="tpl-input"
							bind:value={values.publish_at}
						/>
					</div>

					<div class="tpl-field">
						<span class="tpl-label">{t('studio.prompts.space', 'Space')}</span>
						<SpaceSelector
							bind:spaces
							spaceIds={values.space_id ? [values.space_id] : []}
							onChange={(ids) => (values.space_id = ids[0] ?? '')}
						/>
					</div>

					<div class="tpl-field">
						<span class="tpl-label">{t('studio.prompts.tags_label', 'Tags')}</span>
						<TagInput tags={values.tags} onTags={(next) => (values.tags = next)} {maxTags} />
					</div>

					<div class="tpl-pair">
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.status', 'Status')}</span>
							<AutocompleteSelect
								bind:value={values.status}
								options={statusOpts}
								ariaLabel={t('studio.prompts.status', 'Status')}
							/>
						</div>
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.visibility', 'Visibility')}</span>
							<AutocompleteSelect
								bind:value={values.visibility}
								options={visibilityOptions}
								ariaLabel={t('studio.prompts.visibility', 'Visibility')}
							/>
						</div>
					</div>
					{#if highStakes}
						<p class="tpl-hint tpl-hint-warn">
							{t(
								'studio.prompts.high_stakes',
								'High-stakes category — saved as in_review regardless of selected status.'
							)}
						</p>
					{/if}

					<label class="tpl-toggle"
						><input type="checkbox" bind:checked={values.seo_indexable} />
						{t('studio.prompts.seo_indexable', 'Indexable')}</label
					>

					<div class="tpl-field">
						<label class="tpl-label" for="tpl-field-values">
							{t('studio.prompts.field_values', 'Field values (key: value, one per line)')}
						</label>
						<textarea
							id="tpl-field-values"
							rows="3"
							class="tpl-input tpl-textarea"
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
						{#if categoryFieldKeys.length > 0}
							<p class="tpl-hint">
								{t(
									'ai.templates.category_fields',
									'This category expects these field keys:'
								)}
								{categoryFieldKeys.join(', ')}
							</p>
						{/if}
					</div>
				</div>
			</Tabs.Content>
		</Tabs.Root>
	</div>

	<!-- Footer: attach toggle + insert + save -->
	<div class="space-y-2 border-t border-[var(--color-border)] p-3">
		<label class="tpl-toggle">
			<input type="checkbox" bind:checked={active} />
			{t('ai.templates.attach', 'Attach spec to every message')}
		</label>

		<button
			type="button"
			onclick={oninsert}
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>
			{t('ai.templates.insert', 'Insert as editable prompt')}
		</button>

		<div class="flex gap-2">
			<input
				type="text"
				bind:value={saveName}
				maxlength="200"
				class="tpl-input flex-1"
				placeholder={activeTemplate
					? activeTemplate.name
					: t('ai.templates.name_placeholder', 'Template name')}
				aria-label={t('ai.templates.name_placeholder', 'Template name')}
			/>
			<button
				type="button"
				onclick={save}
				disabled={saving || (!activeId && !saveName.trim())}
				class="inline-flex items-center gap-1 rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if saving}<Loader2 size={12} class="animate-spin" />{/if}
				{t('common.save', 'Save')}
			</button>
		</div>
		{#if activeId}
			<button
				type="button"
				onclick={saveAsNew}
				disabled={saving}
				class="w-full text-center text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)] disabled:opacity-50"
			>
				{t('ai.templates.save_as_new', 'Save as new template')}
			</button>
		{/if}

		{#if dirty}
			<p class="text-[10px] text-[var(--color-warning)]">
				{t('ai.templates.unsaved', 'Unsaved changes — the live spec is what sends, save to keep it.')}
			</p>
		{/if}
		{#if saveErr}
			<p class="text-xs text-[var(--color-danger)]">{saveErr}</p>
		{/if}
	</div>
</aside>

<style>
	/* Bits UI applies the class props to its rendered elements, so these
	 * must be global — scoped selectors would be flagged unused and
	 * dropped (same idiom as PromptRightPanel's right-panel-tab). */
	:global(.tpl-tabs) {
		display: flex;
		border-bottom: 1px solid var(--color-border);
		/* Four tabs overflow the w-72 rail at lg — scroll rather than clip. */
		overflow-x: auto;
		scrollbar-width: none;
	}
	:global(.tpl-tab) {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 40px;
		padding: 8px 12px;
		white-space: nowrap;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-muted);
		cursor: pointer;
		transition: color 0.15s;
	}
	:global(.tpl-tab:hover) {
		color: var(--color-text);
	}
	:global(.tpl-tab[data-state='active']) {
		color: var(--color-text);
		border-bottom: 2px solid var(--color-primary);
	}
	.tpl-stack {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 0.5rem 0.75rem 0.75rem;
	}
	.tpl-pair {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 8px;
		align-items: start;
	}
	.tpl-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	.tpl-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.tpl-input {
		width: 100%;
		height: 2.125rem;
		padding: 0 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.8125rem;
		outline: none;
		transition:
			border-color 0.15s,
			box-shadow 0.15s;
	}
	.tpl-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.tpl-textarea {
		height: auto;
		padding: 8px 12px;
		font-family: inherit;
		resize: vertical;
	}
	.tpl-toggles {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 8px 12px;
	}
	.tpl-toggle {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 0.8125rem;
		color: var(--color-text);
		cursor: pointer;
	}
	.tpl-hint {
		font-size: 0.75rem;
		line-height: 1.4;
		color: var(--color-muted);
	}
	.tpl-hint-warn {
		color: var(--color-warning);
	}
</style>
