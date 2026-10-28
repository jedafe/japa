<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { onMount } from 'svelte';
	import { Sparkles, Copy, Save, Loader2 } from 'lucide-svelte';
	import { promptStore } from '$lib/stores/prompt.svelte';
	import { createApiClient, mapPublicContentType } from '$lib/services/api';
	import {
		buildPrompt,
		buildProjectInstruction,
		defaultFrameworkFor,
		type PromptLookups
	} from '$lib/services/prompt';
	import { copyToClipboard } from '$lib/services/clipboard';
	import { saveProjectInstruction } from '$lib/services/claude-md';
	import { initSettingsDb, loadSettings, type StudioSettings } from '$lib/services/settings';
	import { isHighStakes, isHighStakesCategory } from '$lib/services/validation';
	import {
		TONE_OPTIONS,
		FRAMEWORK_OPTIONS,
		MODE_OPTIONS,
		HUMANIZE_OPTIONS,
		type StudioContentType,
		type Persona,
		type PersonaVoice,
		type Space,
		type PromptMode,
		type PromptTone,
		type PromptFramework,
		type HumanizeLevel,
		type PromptFormValues,
		type SelectOption
	} from '$lib/types/prompt';
	import SelectField from '$lib/components/prompt/SelectField.svelte';
	import TextField from '$lib/components/prompt/TextField.svelte';
	import NumberField from '$lib/components/prompt/NumberField.svelte';
	import CheckboxGroup from '$lib/components/prompt/CheckboxGroup.svelte';
	import Preview from '$lib/components/prompt/Preview.svelte';


	const STRUCTURE_TOGGLES = [
		{ key: 'include_intro', label: 'Intro' },
		{ key: 'include_conclusion', label: 'Conclusion' },
		{ key: 'include_faqs', label: 'FAQs' },
		{ key: 'include_steps', label: 'Steps' },
		{ key: 'include_key_takeaways', label: 'Key takeaways' },
		{ key: 'include_inline_image', label: 'Inline images' }
	] as const;

	let settings = $state<StudioSettings>(loadSettings());
	let contentTypes = $state<StudioContentType[]>([]);
	let personas = $state<Persona[]>([]);
	let voices = $state<PersonaVoice[]>([]);
	let spaces = $state<Space[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let statusMessage = $state<string | null>(null);
	let saving = $state(false);
	let frameworkManuallySet = $state(false);
	let tagsInput = $state('');
	let statusTimeout = $state<ReturnType<typeof setTimeout> | undefined>(undefined);
	let templateName = $state('');
	let selectedTemplateId = $state('');
	let allStatusOptions = $state<SelectOption[]>([]);
	let visibilityOptions = $state<SelectOption[]>([]);

	const contentTypeOptions = $derived(
		contentTypes.map((t) => ({ value: t.type, label: t.label }))
	);
	const selectedContentType = $derived(
		contentTypes.find((t) => t.type === promptStore.values.content_type)
	);
	const guideCategoryOptions = $derived(selectedContentType?.guide_categories ?? []);
	const personaOptions = $derived(
		personas.map((p) => ({ value: p.id, label: p.profile_name }))
	);
	const voiceOptions = $derived(
		voices.map((v) => ({ value: v.id, label: v.name }))
	);

	function voiceToneFromLabel(label: string): string {
		const lower = label.toLowerCase();
		for (const tone of TONE_OPTIONS) {
			if (lower === tone.value || lower.startsWith(tone.value + ' ')) {
				return tone.value;
			}
		}
		return 'natural';
	}

	const filteredVoiceOptions = $derived(
		voiceOptions.filter((v) => voiceToneFromLabel(v.label) === promptStore.values.tone)
	);
	const spaceOptions = $derived(
		spaces.map((s) => ({ value: s.id, label: s.name }))
	);
	const templateOptions = $derived(
		[{ value: '', label: 'Load a template...' }].concat(
			promptStore.templates.map((t) => ({ value: t.id, label: t.name }))
		)
	);
	const highStakes = $derived(isHighStakes(promptStore.values));
	const statusOptions = $derived(
		highStakes
			? allStatusOptions.filter((o) => o.value !== 'published' && o.value !== 'scheduled')
			: allStatusOptions
	);
	const structureValues = $derived({
		include_intro: promptStore.values.include_intro,
		include_conclusion: promptStore.values.include_conclusion,
		include_faqs: promptStore.values.include_faqs,
		include_steps: promptStore.values.include_steps,
		include_key_takeaways: promptStore.values.include_key_takeaways,
		include_inline_image: promptStore.values.include_inline_image
	} as Record<string, boolean>);

	onMount(async () => {
		await initSettingsDb();
		loadSettingsAndOptions();
	});

	$effect(() => {
		tagsInput = promptStore.values.tags.join(', ');
	});

	$effect(() => {
		if (
			highStakes &&
			(promptStore.values.status === 'published' || promptStore.values.status === 'scheduled')
		) {
			promptStore.setValue('status', 'in_review');
		}
	});

	$effect(() => {
		// Auto-select a framework the first time a content type is seen,
		// mirroring the behaviour of the AI draft panel.
		const contentType = promptStore.values.content_type;
		const target = defaultFrameworkFor(contentType) as PromptFramework;
		if (!frameworkManuallySet && promptStore.values.framework !== target) {
			promptStore.setValue('framework', target);
		}
	});

	$effect(() => {
		// Keep the selected voice consistent with the selected tone.
		// Voices are filtered by tone; if the current voice no longer matches,
		// clear it so the user picks a matching voice.
		const voiceId = promptStore.values.voice_id;
		const available = filteredVoiceOptions;
		if (voiceId && !available.some((v) => v.value === voiceId)) {
			promptStore.setValue('voice_id', '');
		}
	});

	function loadSettingsAndOptions() {
		settings = loadSettings();
		promptStore.reset();
		loadFieldValuePairs({});
		promptStore.refreshTemplates();

		if (!settings.baseUrl || !settings.apiKey) {
			loading = false;
			return;
		}

		void fetchOptions();
	}

	async function fetchOptions() {
		loading = true;
		error = null;
		try {
			const client = createApiClient(settings.baseUrl);
			const isMember = settings.mode === 'member';

			// Ref: Section 105 — publishable content types.
			const contentRes = await client.getContentTypes();
			const spaceRes = isMember
				? await client.getMySpaces()
				: await client.getSpaces();
			const [statusRes, visibilityRes] = await Promise.all([
				client.getStatusOptions(),
				client.getVisibilityOptions()
			]);

			let personaRes: { ok: boolean; data?: Persona[]; error?: { message: string } } = { ok: true, data: [] };
			let voiceRes: { ok: boolean; data?: PersonaVoice[]; error?: { message: string } } = { ok: true, data: [] };
			if (!isMember) {
				[personaRes, voiceRes] = await Promise.all([client.getPersonas(), client.getPersonaVoices()]);
			}

			// Populate what we can so the form is usable even when some
			// operator-only lookups are denied by RBAC.
			if (contentRes.ok && contentRes.data) {
				contentTypes = contentRes.data.map(mapPublicContentType);
			}
			if (personaRes.ok && personaRes.data) {
				personas = personaRes.data;
			}
			if (voiceRes.ok && voiceRes.data) {
				voices = voiceRes.data;
			}
			if (spaceRes.ok && spaceRes.data) {
				spaces = spaceRes.data;
			}
			if (statusRes.ok && statusRes.data) {
				allStatusOptions = statusRes.data;
			}
			if (visibilityRes.ok && visibilityRes.data) {
				visibilityOptions = visibilityRes.data;
			}

			const messages = [
				!contentRes.ok && `content types: ${contentRes.error?.message}`,
				!personaRes.ok && `personas: ${personaRes.error?.message}`,
				!voiceRes.ok && `voices: ${voiceRes.error?.message}`,
				!spaceRes.ok && `spaces: ${spaceRes.error?.message}`,
				!statusRes.ok && `status options: ${statusRes.error?.message}`,
				!visibilityRes.ok && `visibility options: ${visibilityRes.error?.message}`
			].filter(Boolean) as string[];

			if (messages.length > 0) {
				error = messages.join('; ') || 'Failed to load prompt options.';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load prompt options.';
		} finally {
			loading = false;
		}
	}

	function setStoreValue(key: string, value: unknown) {
		(promptStore.setValue as (k: string, v: unknown) => void)(key, value);
	}

	function handleContentTypeChange(contentType: string) {
		promptStore.setValue('content_type', contentType);

		const ct = contentTypes.find((t) => t.type === contentType);
		if (!ct?.guide_categories?.some((c) => c.value === promptStore.values.guide_category)) {
			promptStore.setValue('guide_category', '');
		}

		if (!frameworkManuallySet) {
			promptStore.setValue('framework', defaultFrameworkFor(contentType) as PromptFramework);
		}
	}

	function handleGuideCategoryChange(guideCategory: string) {
		promptStore.setValue('guide_category', guideCategory);
		if (isHighStakesCategory(guideCategory)) {
			promptStore.setValue('status', 'in_review');
		}
	}

	function handleFrameworkChange(framework: string) {
		frameworkManuallySet = true;
		promptStore.setValue('framework', framework as PromptFramework);
	}

	function handleTagsInput(value: string) {
		tagsInput = value;
		promptStore.setValue(
			'tags',
			value
				.split(',')
				.map((t) => t.trim())
				.filter((t) => t.length > 0)
		);
	}

	function buildLookups(): PromptLookups {
		const contentType = contentTypes.find((t) => t.type === promptStore.values.content_type);
		const persona = personas.find((p) => p.id === promptStore.values.persona_id);
		const voice = voices.find((v) => v.id === promptStore.values.voice_id);
		const space = spaces.find((s) => s.id === promptStore.values.space_id);

		return {
			contentTypeName: contentType?.label ?? promptStore.values.content_type,
			guideCategoryName: promptStore.values.guide_category,
			personaName: persona?.profile_name ?? '',
			voiceName: voice?.name ?? '',
			voicePrompt: voice?.voice_prompt ?? '',
			spaceName: space?.name ?? ''
		};
	}

	function handleGenerate() {
		try {
			const lookups = buildLookups();
			promptStore.setGenerated(buildPrompt(promptStore.values, lookups, settings.mode));
			showStatus('Prompt generated.');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to generate prompt.';
		}
	}

	async function handleCopy() {
		if (!promptStore.generated) return;
		try {
			await copyToClipboard(promptStore.generated);
			showStatus('Copied!');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to copy to clipboard.';
		}
	}

	async function handleSave() {
		if (!promptStore.generated) return;
		if (!settings.projectRoot) {
			error = 'Project root is not configured. Set it in Settings before saving.';
			return;
		}
		saving = true;
		try {
			const instruction = buildProjectInstruction(promptStore.generated);
			await saveProjectInstruction(settings.projectRoot, instruction);
			showStatus('Saved!');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save to CLAUDE.md.';
		} finally {
			saving = false;
		}
	}

	function handleLoadTemplate(id: string) {
		selectedTemplateId = id;
		if (!id) return;
		const template = promptStore.templates.find((t) => t.id === id);
		if (!template) return;
		promptStore.loadTemplate(template);
		loadFieldValuePairs(promptStore.values.field_values ?? {});
		frameworkManuallySet = !!promptStore.values.framework;
		showStatus(`Loaded template "${template.name}".`);
	}

	function handleSaveAsTemplate() {
		const name = templateName.trim();
		if (!name) {
			error = 'Enter a name for the template.';
			return;
		}
		promptStore.saveAsTemplate(name);
		templateName = '';
		showStatus(`Saved template "${name}".`);
	}

	function showStatus(message: string) {
		if (statusTimeout) {
			clearTimeout(statusTimeout);
		}
		statusMessage = message;
		statusTimeout = setTimeout(() => {
			statusMessage = null;
		}, 2000);
	}

	function clearStatus() {
		if (statusTimeout) {
			clearTimeout(statusTimeout);
		}
		statusMessage = null;
	}

	function clearError() {
		error = null;
	}

	const canGenerate = $derived(
		promptStore.values.title.trim() !== '' || promptStore.values.topic.trim() !== ''
	);

	let fieldValuePairs = $state<{ key: string; value: string }[]>([]);

	function syncFieldValuesFromPairs() {
		const obj: Record<string, string> = {};
		for (const p of fieldValuePairs) {
			if (p.key.trim()) obj[p.key] = p.value;
		}
		promptStore.setValue('field_values', obj);
	}

	function addFieldValue() {
		fieldValuePairs = [...fieldValuePairs, { key: '', value: '' }];
		syncFieldValuesFromPairs();
	}

	function removeFieldValue(index: number) {
		fieldValuePairs = fieldValuePairs.filter((_, i) => i !== index);
		syncFieldValuesFromPairs();
	}

	function updateFieldValueKey(index: number, key: string) {
		fieldValuePairs = fieldValuePairs.map((p, i) => (i === index ? { ...p, key } : p));
		syncFieldValuesFromPairs();
	}

	function updateFieldValueValue(index: number, value: string) {
		fieldValuePairs = fieldValuePairs.map((p, i) => (i === index ? { ...p, value } : p));
		syncFieldValuesFromPairs();
	}

	function loadFieldValuePairs(values: Record<string, string>) {
		fieldValuePairs = Object.entries(values ?? {}).map(([key, value]) => ({ key, value }));
	}
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6">
		<h1 class="text-2xl font-bold">Prompt Builder</h1>
		<p class="text-sm text-muted-foreground">Build a prompt for Claude Code to generate {siteName()} content.</p>
	</header>

	{#if statusMessage}
		<div
			class="mb-4 rounded-lg border border-success/20 bg-success/10 p-3 text-sm text-success"
			role="status"
		>
			<div class="flex items-center justify-between">
				<span>{statusMessage}</span>
				<button
					class="text-xs underline hover:no-underline"
					type="button"
					onclick={clearStatus}
					aria-label="Dismiss message"
				>
					Dismiss
				</button>
			</div>
		</div>
	{/if}

	{#if error}
		<div
			class="mb-4 rounded-lg border border-danger/20 bg-danger/5 p-4 text-sm text-danger"
			role="alert"
		>
			<div class="flex items-start justify-between gap-4">
				<div>
					<p class="font-medium">Something went wrong</p>
					<p class="mt-1">{error}</p>
				</div>
				<button
					class="text-xs underline hover:no-underline"
					type="button"
					onclick={clearError}
					aria-label="Dismiss error"
				>
					Dismiss
				</button>
			</div>
		</div>
	{/if}

	{#if !settings.baseUrl || !settings.apiKey}
		<div class="rounded-xl border border-border bg-card p-6">
			<p class="text-sm text-muted-foreground">
				Configure Settings first to load prompt options and save generated prompts.
			</p>
			<a
				href="/settings"
				class="mt-4 inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover"
			>
				<span>Go to Settings</span>
			</a>
		</div>
	{:else if loading}
		<div class="flex h-64 items-center justify-center gap-2 text-muted-foreground">
			<Loader2 class="h-5 w-5 animate-spin" />
			<span>Loading prompt options...</span>
		</div>
	{:else}
		<div class="grid gap-6 lg:grid-cols-2">
			<div class="space-y-4 rounded-xl border border-border bg-card p-6">
				<SelectField
					label="Mode"
					value={promptStore.values.mode}
					options={MODE_OPTIONS}
					onchange={(value) => promptStore.setValue('mode', value as PromptMode)}
				
					hint="What the prompt should produce — draft, outline, or rewrite."
				/>

				<TextField
					label="Topic / Title"
					value={promptStore.values.title}
					placeholder="e.g. Cost of Living in Toronto vs Lagos"
					oninput={(value) => promptStore.setValue('title', value)}
				
					hint="The subject the AI writes about."
				/>

				<SelectField
					label="Content Type"
					value={promptStore.values.content_type}
					options={contentTypeOptions}
					placeholder="Select a content type"
					onchange={handleContentTypeChange}
				
					hint="The kind of content to generate."
				/>

				{#if guideCategoryOptions.length > 0}
					<SelectField
						label="Guide Category"
						value={promptStore.values.guide_category}
						options={guideCategoryOptions}
						placeholder="Select a guide category"
						onchange={handleGuideCategoryChange}
					
					hint="The guide category this targets."
				/>
				{/if}

				{#if settings.mode !== 'member'}
					<SelectField
						label="Persona"
						value={promptStore.values.persona_id}
						options={personaOptions}
						placeholder="Select a persona"
						onchange={(value) => promptStore.setValue('persona_id', value)}
					
					hint="The audience the text is written for."
				/>
				{/if}

				<div class="grid grid-cols-2 gap-4">
					<SelectField
						label="Tone Variant"
						value={promptStore.values.tone}
						options={TONE_OPTIONS}
						onchange={(value) => promptStore.setValue('tone', value as PromptTone)}
					
					hint="How the writing should sound."
				/>
					{#if settings.mode !== 'member'}
						<SelectField
							label="Voice"
							value={promptStore.values.voice_id}
							options={filteredVoiceOptions}
							placeholder="Select a voice"
							onchange={(value) => promptStore.setValue('voice_id', value)}
						
					hint="The voice preset for audio output."
				/>
					{/if}
				</div>

				<div class="grid grid-cols-2 gap-4">
					<SelectField
						label="Framework"
						value={promptStore.values.framework}
						options={FRAMEWORK_OPTIONS}
						onchange={handleFrameworkChange}
					
					hint="The structural framework for the output."
				/>
					<SelectField
						label="Humanize level"
						value={promptStore.values.humanize_level}
						options={HUMANIZE_OPTIONS}
						onchange={(value) =>
							promptStore.setValue('humanize_level', value as HumanizeLevel)}
					
					hint="How natural-sounding the result should be."
				/>
				</div>

				<div class="grid grid-cols-3 gap-4">
					<NumberField
						label="Min words"
						value={promptStore.values.min_words}
						min={0}
						oninput={(value) => promptStore.setValue('min_words', value)}
					
					hint="Minimum length the draft must reach."
				/>
					<NumberField
						label="Paragraphs"
						value={promptStore.values.paragraph_count}
						min={0}
						oninput={(value) => promptStore.setValue('paragraph_count', value)}
					
					hint="How many paragraphs to generate."
				/>
					<NumberField
						label="Min headings"
						value={promptStore.values.heading_count}
						min={0}
						oninput={(value) => promptStore.setValue('heading_count', value)}
					
					hint="Minimum section headings required."
				/>
				</div>

				<div class="grid grid-cols-3 gap-4">
					<NumberField
						label="Min paragraphs per heading"
						value={promptStore.values.min_paragraphs_per_heading}
						min={0}
						oninput={(value) => promptStore.setValue('min_paragraphs_per_heading', value)}
					
					hint="Paragraphs required under each heading."
				/>
					<NumberField
						label="Min external links"
						value={promptStore.values.external_links}
						min={0}
						oninput={(value) => promptStore.setValue('external_links', value)}
					
					hint="External sources the draft must cite."
				/>
					<NumberField
						label="Min internal links"
						value={promptStore.values.internal_links}
						min={0}
						oninput={(value) => promptStore.setValue('internal_links', value)}
					
					hint="Links to other JapaConnect pages to include."
				/>
				</div>

				<CheckboxGroup
					label="Structure"
					values={structureValues}
					items={STRUCTURE_TOGGLES}
					onchange={(key, value) => setStoreValue(key, value)}
				
					hint="Which structural elements to include."
				/>

				<SelectField
					label="Space"
					value={promptStore.values.space_id}
					options={spaceOptions}
					placeholder="Select a space"
					onchange={(value) => promptStore.setValue('space_id', value)}
				
					hint="The community space to publish into."
				/>

				<div class="grid grid-cols-2 gap-4">
					<SelectField
						label="Visibility"
						value={promptStore.values.visibility}
						options={visibilityOptions}
						onchange={(value) => promptStore.setValue('visibility', value)}
					
					hint="Who can see the result."
				/>
					<SelectField
						label="Status"
						value={promptStore.values.status}
						options={statusOptions}
						onchange={(value) => promptStore.setValue('status', value)}
					
					hint="Draft or publish-ready state."
				/>
				</div>

				<TextField
					label="Publish at"
					type="datetime-local"
					value={promptStore.values.publish_at}
					oninput={(value) => promptStore.setValue('publish_at', value)}
				/>

				<TextField
					label="Tags"
					value={tagsInput}
					placeholder="comma, separated, tags"
					oninput={handleTagsInput}
				/>

				<div class="grid grid-cols-2 gap-4">
					<TextField
						label="Source country"
						value={promptStore.values.source_country}
						oninput={(value) => promptStore.setValue('source_country', value)}
					/>
					<TextField
						label="Target country"
						value={promptStore.values.target_country}
						oninput={(value) => promptStore.setValue('target_country', value)}
					/>
				</div>

				<div class="grid grid-cols-3 gap-4">
					<TextField
						label="Audience"
						value={promptStore.values.audience}
						oninput={(value) => promptStore.setValue('audience', value)}
					/>
					<TextField
						label="Journey stage"
						value={promptStore.values.journey_stage}
						oninput={(value) => promptStore.setValue('journey_stage', value)}
					/>
					<TextField
						label="Visa route"
						value={promptStore.values.visa_route}
						oninput={(value) => promptStore.setValue('visa_route', value)}
					/>
				</div>

				<TextField
					label="Meta title"
					value={promptStore.values.meta_title}
					oninput={(value) => promptStore.setValue('meta_title', value)}
				/>

				<TextField
					label="Meta description"
					value={promptStore.values.meta_description}
					oninput={(value) => promptStore.setValue('meta_description', value)}
				/>

				<div class="grid grid-cols-2 gap-4">
					<TextField
						label="Canonical URL"
						value={promptStore.values.canonical_url}
						oninput={(value) => promptStore.setValue('canonical_url', value)}
					/>
					<TextField
						label="OG image URL"
						value={promptStore.values.og_image_url}
						oninput={(value) => promptStore.setValue('og_image_url', value)}
					/>
				</div>

				<div class="grid grid-cols-2 gap-4">
					<TextField
						label="Header image ID"
						value={promptStore.values.header_image_id}
						placeholder="Media UUID"
						oninput={(value) => promptStore.setValue('header_image_id', value)}
					/>
					<TextField
						label="Featured image ID"
						value={promptStore.values.featured_image_id}
						placeholder="Media UUID"
						oninput={(value) => promptStore.setValue('featured_image_id', value)}
					/>
				</div>

				<div class="space-y-2 rounded-lg border border-border bg-surface p-4">
					<div class="flex items-center justify-between">
						<h3 class="text-sm font-semibold">Field values</h3>
						<button
							type="button"
							class="inline-flex items-center gap-1 rounded border border-border bg-background px-2 py-1 text-xs hover:bg-accent"
							onclick={addFieldValue}
						>
							<span>Add</span>
						</button>
					</div>
					{#each fieldValuePairs as pair, i (i)}
						<div class="grid grid-cols-[1fr_1fr_auto] gap-2">
							<TextField
								label=""
								value={pair.key}
								placeholder="Key"
								oninput={(value) => updateFieldValueKey(i, value)}
							/>
							<TextField
								label=""
								value={pair.value}
								placeholder="Value"
								oninput={(value) => updateFieldValueValue(i, value)}
							/>
							<button
								type="button"
								class="self-center rounded p-1 text-danger hover:bg-danger/10"
								onclick={() => removeFieldValue(i)}
								aria-label="Remove field value"
							>
								<span class="text-lg leading-none">&times;</span>
							</button>
						</div>
					{/each}
				</div>

				<label class="inline-flex items-center gap-2 text-sm font-medium">
					<input
						type="checkbox"
						class="h-4 w-4 rounded border-border"
						checked={promptStore.values.seo_indexable}
						onchange={(event) =>
							promptStore.setValue(
								'seo_indexable',
								(event.currentTarget as HTMLInputElement).checked
							)}
					/>
					<span>SEO indexable</span>
				</label>

				<div class="space-y-4 rounded-lg border border-border bg-surface p-4">
					<h3 class="text-sm font-semibold">Templates</h3>

					<SelectField
						label="Load a template"
						value={selectedTemplateId}
						options={templateOptions}
						onchange={handleLoadTemplate}
					/>

					<div class="flex gap-2">
						<div class="flex-1">
							<TextField
								label="Template name"
								value={templateName}
								placeholder="e.g. Canada relocation guide"
								oninput={(value) => (templateName = value)}
							/>
						</div>
						<button
							type="button"
							class="mt-6 inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
							disabled={!templateName.trim()}
							onclick={handleSaveAsTemplate}
						>
							<Save class="h-4 w-4" />
							<span>Save as template</span>
						</button>
					</div>
				</div>

				<div class="flex flex-wrap gap-2 pt-2">
					<button
						type="button"
						class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover disabled:opacity-60"
						disabled={!canGenerate}
						onclick={handleGenerate}
					>
						<Sparkles class="h-4 w-4" />
						<span>Generate Prompt</span>
					</button>
					<button
						type="button"
						class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
						disabled={!promptStore.generated}
						onclick={handleCopy}
					>
						<Copy class="h-4 w-4" />
						<span>Copy</span>
					</button>
					<button
						type="button"
						class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
						disabled={!promptStore.generated || saving}
						onclick={handleSave}
					>
						{#if saving}
							<Loader2 class="h-4 w-4 animate-spin" />
						{:else}
							<Save class="h-4 w-4" />
						{/if}
						<span>Save to CLAUDE.md</span>
					</button>
				</div>
			</div>

			<Preview value={promptStore.generated} />
		</div>
	{/if}
</div>
