<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { join } from '@tauri-apps/api/path';
	import { readTextFile, writeTextFile } from '@tauri-apps/plugin-fs';
	import {
		Loader2,
		Save,
		Upload,
		Eye,
		FileWarning,
		ArrowLeft,
		FileCode,
		FormInput,
		RotateCcw
	} from 'lucide-svelte';
	import { initSettingsDb, loadSettings, type StudioSettings } from '$lib/services/settings';
	import { queueStore } from '$lib/stores/queue.svelte';
	import { hashFile } from '$lib/services/hash';
	import { parseFrontmatter, buildMarkdown, markdownToHtml } from '$lib/services/markdown';
	import { publishContent } from '$lib/services/publisher';
	import { validateFrontmatter, validateBody, isHighStakes } from '$lib/services/validation';
	import { createApiClient, mapPublicContentType } from '$lib/services/api';
	import { slugify } from '$lib/services/prompt';
	import {
		type StudioContentType,
		type Persona,
		type PersonaVoice,
		type Space,
		type SelectOption
	} from '$lib/types/prompt';
	import SelectField from '$lib/components/prompt/SelectField.svelte';
	import TextField from '$lib/components/prompt/TextField.svelte';

	interface EditorOptions {
		contentTypes: StudioContentType[];
		personas: Persona[];
		voices: PersonaVoice[];
		spaces: Space[];
		statusOptions: SelectOption[];
		visibilityOptions: SelectOption[];
		loaded: boolean;
	}

	let settings = $state<StudioSettings>(loadSettings());
	let loading = $state(true);
	let saving = $state(false);
	let publishing = $state(false);
	let error = $state<string | null>(null);
	let statusMessage = $state<string | null>(null);
	let mode = $state<'source' | 'fields'>('fields');
	let raw = $state('');
	let frontmatter = $state<Record<string, unknown>>({});
	let body = $state('');
	let fieldValueEntries = $state<[string, string][]>([]);
	let relativePath = $state('');
	let filePath = $state('');
	let options = $state<EditorOptions>({
		contentTypes: [],
		personas: [],
		voices: [],
		spaces: [],
		statusOptions: [],
		visibilityOptions: [],
		loaded: false
	});
	const pageParam = $derived($page.params.path);

	const title = $derived(getString('title') || 'Untitled');
	const preview = $derived(
		mode === 'source'
			? markdownToHtml(parseFrontmatter(raw).body, { sanitize: true })
			: markdownToHtml(body, { sanitize: true })
	);

	const contentTypeOptions = $derived(
		options.contentTypes.map((t) => ({ value: t.type, label: t.label }))
	);
	const selectedContentType = $derived(
		options.contentTypes.find((t) => t.type === getString('content_type'))
	);
	const guideCategoryOptions = $derived(selectedContentType?.guide_categories ?? []);
	const personaOptions = $derived(options.personas.map((p) => ({ value: p.id, label: p.profile_name })));
	const voiceOptions = $derived(options.voices.map((v) => ({ value: v.id, label: v.name })));
	const spaceOptions = $derived(options.spaces.map((s) => ({ value: s.id, label: s.name })));
	const effectiveStatus = $derived(
		isHighStakes(frontmatter) ? 'in_review' : getString('status') || 'published'
	);
	const statusOptions = $derived(
		isHighStakes(frontmatter)
			? options.statusOptions.filter((o) => o.value === 'in_review')
			: options.statusOptions
	);
	const validationErrors = $derived(
		validateFrontmatter(
			frontmatter,
			effectiveStatus,
			settings.mode,
			options.statusOptions.map((o) => o.value),
			options.visibilityOptions.map((o) => o.value)
		)
	);
	const bodyError = $derived(
		validateBody(
			mode === 'source' ? parseFrontmatter(raw).body : body,
			effectiveStatus,
			settings.contentMinLength ?? 10
		)
	);
	const hasValidationErrors = $derived(Object.keys(validationErrors).length > 0 || !!bodyError);

	$effect(() => {
		// High-stakes categories must always route to in_review.
		// Ref: sections/105-japa-content-studio-desktop.md (High-Stakes HITL Gate)
		if (isHighStakes(frontmatter) && getString('status') !== 'in_review') {
			setString('status', 'in_review');
		}
	});

	onMount(async () => {
		await initSettingsDb();
		await queueStore.load();
		settings = loadSettings();
		void loadFile();
		if (settings.baseUrl && settings.apiKey) {
			void loadOptions();
		}
	});

	async function loadFile() {
		const param = pageParam;
		if (!param) {
			error = 'No file path provided.';
			loading = false;
			return;
		}

		relativePath = Array.isArray(param)
			? param.map(decodeURIComponent).join('/')
			: decodeURIComponent(param);

		if (!settings.projectRoot) {
			error = 'Project root is not configured. Go to Settings.';
			loading = false;
			return;
		}

		filePath = await join(settings.projectRoot, relativePath);

		try {
			raw = await readTextFile(filePath);
			syncFromRaw();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to read file.';
		} finally {
			loading = false;
		}
	}

	async function loadOptions() {
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

		const contentTypes: StudioContentType[] = (contentRes.data ?? []).map(mapPublicContentType);

		options = {
			contentTypes,
			personas: personaRes.data ?? [],
			voices: voiceRes.data ?? [],
			spaces: spaceRes.data ?? [],
			statusOptions: statusRes.data ?? [],
			visibilityOptions: visibilityRes.data ?? [],
			loaded: true
		};
	}

	function syncFromRaw() {
		const parsed = parseFrontmatter(raw);
		const cleaned = { ...parsed.frontmatter };
		// Member mode posts under the authenticated account; persona and voice
		// are operator-only and must not be written to frontmatter.
		// Ref: sections/105-japa-content-studio-desktop.md (Member Posting Mode)
		if (settings.mode === 'member') {
			delete cleaned['persona_id'];
			delete cleaned['voice_id'];
		}
		frontmatter = cleaned;
		body = parsed.body;
		fieldValueEntries = getFieldValueEntries();
	}

	function syncToRaw() {
		raw = buildOutputMarkdown();
	}

	function buildOutputMarkdown(): string {
		const cleaned = { ...frontmatter };
		// Member mode must not serialize operator-only persona/voice fields.
		// Ref: sections/105-japa-content-studio-desktop.md (Member Posting Mode)
		if (settings.mode === 'member') {
			delete cleaned['persona_id'];
			delete cleaned['voice_id'];
		}
		return buildMarkdown(cleaned, body);
	}

	function toggleMode() {
		if (mode === 'source') {
			syncFromRaw();
			mode = 'fields';
		} else {
			syncToRaw();
			mode = 'source';
		}
	}

	function getString(key: string, defaultValue = ''): string {
		const value = frontmatter[key];
		return typeof value === 'string' ? value : defaultValue;
	}

	function setString(key: string, value: string) {
		frontmatter[key] = value;
	}

	function getTags(key: string): string {
		const value = frontmatter[key];
		if (Array.isArray(value)) {
			return value
				.map((v) => (typeof v === 'string' ? v : String(v)))
				.filter((v) => v.length > 0)
				.join(', ');
		}
		return '';
	}

	function setTags(key: string, value: string) {
		frontmatter[key] = value
			.split(',')
			.map((t) => t.trim())
			.filter((t) => t.length > 0);
	}

	function getBoolean(key: string): boolean {
		return Boolean(frontmatter[key]);
	}

	function setBoolean(key: string, value: boolean) {
		frontmatter[key] = value;
	}

	function valueToFieldString(value: unknown): string {
		// field_values is Record<string, unknown>. Values are edited as text and
		// JSON-stringified for display when they are numbers, booleans, arrays,
		// or objects. parseFieldValue converts them back on save.
		if (value === undefined || value === null) return '';
		if (typeof value === 'string') return value;
		return JSON.stringify(value);
	}

	function parseFieldValue(raw: string): unknown {
		const trimmed = raw.trim();
		if (trimmed === '') return '';
		if (/^true$/i.test(trimmed)) return true;
		if (/^false$/i.test(trimmed)) return false;
		if (trimmed === 'null') return null;
		if (/^-?\d+(\.\d+)?$/.test(trimmed)) return Number(trimmed);
		if (
			(trimmed.startsWith('{') && trimmed.endsWith('}')) ||
			(trimmed.startsWith('[') && trimmed.endsWith(']'))
		) {
			try {
				return JSON.parse(trimmed);
			} catch {
				// fall through to raw string
			}
		}
		return raw;
	}

	function getFieldValueEntries(): [string, string][] {
		const value = frontmatter['field_values'];
		if (value && typeof value === 'object' && !Array.isArray(value)) {
			return Object.entries(value).map(([k, v]) => [k, valueToFieldString(v)]);
		}
		return [];
	}

	$effect(() => {
		// Keep frontmatter.field_values in sync with the editable key/value list,
		// skipping rows with empty keys so they do not pollute the saved YAML.
		// Raw string values are parsed so numbers, booleans, and JSON literals
		// round-trip correctly.
		const record: Record<string, unknown> = {};
		for (const [k, v] of fieldValueEntries) {
			const key = k.trim();
			if (key === '') continue;
			record[key] = parseFieldValue(v);
		}
		if (Object.keys(record).length === 0) {
			delete frontmatter['field_values'];
		} else {
			frontmatter['field_values'] = record;
		}
	});

	function addFieldValue() {
		fieldValueEntries = [...fieldValueEntries, ['', '']];
	}

	function removeFieldValue(index: number) {
		fieldValueEntries = fieldValueEntries.filter((_, i) => i !== index);
	}

	function updateFieldValueKey(index: number, newKey: string) {
		fieldValueEntries = fieldValueEntries.map((entry, i) =>
			i === index ? [newKey, entry[1]] : entry
		);
	}

	function updateFieldValueValue(index: number, newValue: string) {
		fieldValueEntries = fieldValueEntries.map((entry, i) =>
			i === index ? [entry[0], newValue] : entry
		);
	}

	function handleTitleChange(value: string) {
		setString('title', value);
		if (!getString('slug')) {
			setString('slug', slugify(value));
		}
	}

	async function handleSave(): Promise<boolean> {
		if (!filePath) return false;
		saving = true;
		try {
			const toSave = mode === 'source' ? raw : buildOutputMarkdown();
			await writeTextFile(filePath, toSave);
			const newHash = await hashFile(toSave);
			const existing = queueStore.files.find((f) => f.relativePath === relativePath);
			if (existing) {
				const syncState = newHash === existing.lastPublishedHash ? 'published' : 'modified';
				await queueStore.updateFile(relativePath, { lastHash: newHash, syncState });
			} else {
				const slug = getString('slug') || relativePath.split('/').pop()?.replace(/\.md$/i, '') || '';
				const contentType = relativePath.split('/')[1] || '';
				await queueStore.setFiles([
					...queueStore.files,
					{
						relativePath,
						contentType,
						slug,
						title: getString('title') || slug,
						lastHash: newHash,
						lastPublishedHash: '',
						lastPublishedAt: '',
						syncState: 'modified',
						lastError: ''
					}
				]);
			}
			showStatus('Saved.');
			return true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save file.';
			return false;
		} finally {
			saving = false;
		}
	}

	async function handlePublish() {
		if (!filePath || !settings.baseUrl || !settings.apiKey) {
			error = 'Cannot publish: API settings are missing.';
			return;
		}
		publishing = true;
		error = null;
		try {
			const saved = await handleSave();
			if (!saved) return;

			const currentRaw = mode === 'source' ? raw : buildOutputMarkdown();
			const result = await publishContent({
				relativePath,
				frontmatter,
				body,
				raw: currentRaw
			});

			if (!result.ok) {
				error = result.error ?? 'Publish failed.';
				return;
			}

			showStatus(`Published ${result.slug ?? ''} (${result.status ?? ''}).`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Publish failed.';
		} finally {
			publishing = false;
		}
	}

	function handleRevert() {
		syncFromRaw();
		showStatus('Reverted to last saved version.');
	}

	function showStatus(message: string) {
		statusMessage = message;
		setTimeout(() => {
			statusMessage = null;
		}, 2000);
	}

	function handleBodyInput(value: string) {
		body = value;
	}

	function handleRawInput(value: string) {
		raw = value;
	}
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-4 flex items-center justify-between">
		<div class="flex items-center gap-3">
			<button
				type="button"
				class="inline-flex items-center gap-1 rounded-lg border border-border bg-surface px-3 py-2 text-sm hover:bg-accent"
				onclick={() => goto('/')}
			>
				<ArrowLeft class="h-4 w-4" />
				<span>Queue</span>
			</button>
			<div>
				<h1 class="text-lg font-bold leading-tight">{title}</h1>
				<p class="text-sm text-muted-foreground">{relativePath}</p>
			</div>
		</div>
		<div class="flex items-center gap-2">
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
				onclick={toggleMode}
			>
				{#if mode === 'fields'}
					<FileCode class="h-4 w-4" />
					<span>Source</span>
				{:else}
					<FormInput class="h-4 w-4" />
					<span>Fields</span>
				{/if}
			</button>
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
				onclick={handleRevert}
			>
				<RotateCcw class="h-4 w-4" />
				<span>Revert</span>
			</button>
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover disabled:opacity-60"
				disabled={saving || publishing}
				onclick={handleSave}
			>
				{#if saving}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Save class="h-4 w-4" />
				{/if}
				<span>Save</span>
			</button>
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg bg-success px-4 py-2 text-sm text-white hover:opacity-90 disabled:opacity-60"
				disabled={saving || publishing || hasValidationErrors}
				onclick={handlePublish}
			>
				{#if publishing}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Upload class="h-4 w-4" />
				{/if}
				<span>Publish</span>
			</button>
		</div>
	</header>

	{#if statusMessage}
		<div class="mb-4 rounded-lg border border-success/20 bg-success/10 p-3 text-sm text-success" role="status">
			{statusMessage}
		</div>
	{/if}

	{#if error}
		<div class="mb-4 rounded-lg border border-danger/20 bg-danger/5 p-4 text-sm text-danger" role="alert">
			<div class="flex items-start gap-2">
				<FileWarning class="mt-0.5 h-4 w-4 shrink-0" />
				<div>
					<p class="font-medium">Error</p>
					<p class="mt-1">{error}</p>
				</div>
			</div>
		</div>
	{/if}

	{#if hasValidationErrors}
		<div class="mb-4 rounded-lg border border-orange-500/20 bg-orange-500/5 p-4 text-sm text-orange-600" role="alert">
			<p class="font-medium">Please fix the following before publishing:</p>
			<ul class="mt-1 list-inside list-disc space-y-1">
				{#each Object.values(validationErrors) as message}
					<li>{message}</li>
				{/each}
				{#if bodyError}
					<li>{bodyError}</li>
				{/if}
			</ul>
		</div>
	{/if}

	{#if loading}
		<div class="flex h-64 items-center justify-center gap-2 text-muted-foreground">
			<Loader2 class="h-5 w-5 animate-spin" />
			<span>Loading file...</span>
		</div>
	{:else}
		<div class="grid flex-1 gap-4 lg:grid-cols-2">
			<div class="flex min-h-0 flex-col rounded-xl border border-border bg-card">
				<div class="border-b border-border bg-accent px-4 py-2 text-sm font-medium">
					<div class="flex items-center gap-2">
						{#if mode === 'fields'}
							<FormInput class="h-4 w-4" />
							<span>Fields</span>
						{:else}
							<FileCode class="h-4 w-4" />
							<span>Source</span>
						{/if}
					</div>
				</div>

				{#if mode === 'fields'}
					<div class="flex-1 overflow-y-auto p-4">
						<div class="space-y-4">
							<TextField
								label="Title"
								value={getString('title')}
								oninput={handleTitleChange}
							/>
							<TextField
								label="Slug"
								value={getString('slug')}
								oninput={(value) => setString('slug', slugify(value))}
							/>

							<SelectField
								label="Content type"
								value={getString('content_type')}
								options={contentTypeOptions}
								placeholder="Select or type content type"
								onchange={(value) => setString('content_type', value)}
							/>

							{#if guideCategoryOptions.length > 0}
								<SelectField
									label="Guide category"
									value={getString('guide_category')}
									options={guideCategoryOptions}
									placeholder="Select a guide category"
									onchange={(value) => setString('guide_category', value)}
								/>
							{/if}

							<div class="grid grid-cols-2 gap-4">
								<SelectField
									label="Status"
									value={getString('status')}
									options={statusOptions}
									onchange={(value) => setString('status', value)}
								/>
								<SelectField
									label="Visibility"
									value={getString('visibility')}
									options={options.visibilityOptions}
									onchange={(value) => setString('visibility', value)}
								/>
							</div>

							<SelectField
								label="Space"
								value={getString('space_id')}
								options={spaceOptions}
								placeholder="Select a space"
								onchange={(value) => setString('space_id', value)}
							/>

							{#if settings.mode !== 'member'}
								<div class="grid grid-cols-2 gap-4">
									<SelectField
										label="Persona"
										value={getString('persona_id')}
										options={personaOptions}
										placeholder="Select a persona"
										onchange={(value) => setString('persona_id', value)}
									/>
									<SelectField
										label="Voice"
										value={getString('voice_id')}
										options={voiceOptions}
										placeholder="Select a voice"
										onchange={(value) => setString('voice_id', value)}
									/>
								</div>
							{/if}

							<TextField
								label="Tags"
								value={getTags('tags')}
								placeholder="comma, separated, tags"
								oninput={(value) => setTags('tags', value)}
							/>

							<div class="grid grid-cols-2 gap-4">
								<TextField
									label="Source country"
									value={getString('source_country')}
									oninput={(value) => setString('source_country', value)}
								/>
								<TextField
									label="Target country"
									value={getString('target_country')}
									oninput={(value) => setString('target_country', value)}
								/>
							</div>

							<div class="grid grid-cols-2 gap-4">
								<TextField
									label="Header image ID"
									value={getString('header_image_id')}
									oninput={(value) => setString('header_image_id', value)}
								/>
								<TextField
									label="Featured image ID"
									value={getString('featured_image_id')}
									oninput={(value) => setString('featured_image_id', value)}
								/>
							</div>

							<div class="space-y-2">
								<div class="flex items-center justify-between">
									<span class="text-sm font-medium">Field values</span>
									<button
										type="button"
										class="inline-flex items-center gap-1 rounded-lg border border-border bg-surface px-2 py-1 text-xs hover:bg-accent"
										onclick={addFieldValue}
									>
										Add
									</button>
								</div>
								{#each fieldValueEntries as [key, value], index (index)}
									<div class="grid grid-cols-[1fr_1fr_auto] items-end gap-2">
										<label class="block text-xs text-muted-foreground">
											Key
											<input
												type="text"
												class="mt-1 w-full rounded-lg border border-border bg-surface px-2 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
												value={key}
												oninput={(event) =>
													updateFieldValueKey(index, (event.currentTarget as HTMLInputElement).value)}
												spellcheck="false"
											/>
										</label>
										<label class="block text-xs text-muted-foreground">
											Value
											<input
												type="text"
												class="mt-1 w-full rounded-lg border border-border bg-surface px-2 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
												value={value}
												oninput={(event) =>
													updateFieldValueValue(index, (event.currentTarget as HTMLInputElement).value)}
												spellcheck="false"
											/>
										</label>
										<button
											type="button"
											class="inline-flex items-center rounded-lg border border-border bg-surface px-2 py-2 text-xs hover:bg-accent"
											onclick={() => removeFieldValue(index)}
										>
											Remove
										</button>
									</div>
								{/each}
							</div>

							<div class="grid grid-cols-3 gap-4">
								<TextField
									label="Audience"
									value={getString('audience')}
									oninput={(value) => setString('audience', value)}
								/>
								<TextField
									label="Journey stage"
									value={getString('journey_stage')}
									oninput={(value) => setString('journey_stage', value)}
								/>
								<TextField
									label="Visa route"
									value={getString('visa_route')}
									oninput={(value) => setString('visa_route', value)}
								/>
							</div>

							<TextField
								label="Publish at"
								type="datetime-local"
								value={getString('publish_at')}
								oninput={(value) => setString('publish_at', value)}
							/>

							<TextField
								label="Meta title"
								value={getString('meta_title')}
								oninput={(value) => setString('meta_title', value)}
							/>
							<TextField
								label="Meta description"
								value={getString('meta_description')}
								oninput={(value) => setString('meta_description', value)}
							/>

							<div class="grid grid-cols-2 gap-4">
								<TextField
									label="Canonical URL"
									value={getString('canonical_url')}
									oninput={(value) => setString('canonical_url', value)}
								/>
								<TextField
									label="OG image URL"
									value={getString('og_image_url')}
									oninput={(value) => setString('og_image_url', value)}
								/>
							</div>

							<label class="inline-flex items-center gap-2 text-sm font-medium">
								<input
									type="checkbox"
									class="h-4 w-4 rounded border-border"
									checked={getBoolean('seo_indexable')}
									onchange={(event) =>
										setBoolean('seo_indexable', (event.currentTarget as HTMLInputElement).checked)}
								/>
								<span>SEO indexable</span>
							</label>

							<label class="block text-sm font-medium">
								Body
								<textarea
									class="mt-1 h-64 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm font-mono leading-relaxed focus:outline-none focus:ring-2 focus:ring-primary"
									value={body}
									oninput={(event) =>
										handleBodyInput((event.currentTarget as HTMLTextAreaElement).value)}
									spellcheck="false"
								></textarea>
							</label>
						</div>
					</div>
				{:else}
					<textarea
						class="h-full w-full flex-1 resize-none rounded-b-xl bg-surface p-4 text-sm font-mono leading-relaxed focus:outline-none"
						value={raw}
						oninput={(event) => handleRawInput((event.currentTarget as HTMLTextAreaElement).value)}
						spellcheck="false"
					></textarea>
				{/if}
			</div>

			<div class="flex min-h-0 flex-col rounded-xl border border-border bg-card">
				<div class="border-b border-border bg-accent px-4 py-2 text-sm font-medium">
					<div class="flex items-center gap-2">
						<Eye class="h-4 w-4" />
						<span>Preview</span>
					</div>
				</div>
				<div class="h-full w-full flex-1 overflow-y-auto p-4 text-sm leading-relaxed">
					{@html preview}
				</div>
			</div>
		</div>
	{/if}
</div>
