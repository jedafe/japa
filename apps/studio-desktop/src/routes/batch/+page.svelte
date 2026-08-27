<script lang="ts">
	import { onMount } from 'svelte';
	import { Sparkles, Copy, Save, Loader2, AlertCircle } from 'lucide-svelte';
	import { initSettingsDb, loadSettings, type StudioSettings } from '$lib/services/settings';
	import { createApiClient, mapPublicContentType } from '$lib/services/api';
	import { copyToClipboard } from '$lib/services/clipboard';
	import { saveProjectInstruction } from '$lib/services/claude-md';
	import { buildProjectInstruction } from '$lib/services/prompt';
	import { parseBatchTopics, buildBatchPrompt } from '$lib/services/batch';
	import type { Persona, PersonaVoice, Space, StudioContentType } from '$lib/types/prompt';
	import type { BatchTopic } from '$lib/types/batch';
	import type { PromptLookups } from '$lib/services/prompt';
	import Preview from '$lib/components/prompt/Preview.svelte';

	let settings = $state<StudioSettings>(loadSettings());
	let contentTypes = $state<StudioContentType[]>([]);
	let personas = $state<Persona[]>([]);
	let voices = $state<PersonaVoice[]>([]);
	let spaces = $state<Space[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let statusMessage = $state<string | null>(null);
	let saving = $state(false);
	let statusTimeout = $state<ReturnType<typeof setTimeout> | undefined>(undefined);

	let rawInput = $state('');
	let generated = $state<string>('');
	let topics = $state<BatchTopic[]>([]);

	onMount(async () => {
		await initSettingsDb();
		settings = loadSettings();
		void loadOptions();
	});

	async function loadOptions() {
		if (!settings.baseUrl || !settings.apiKey) {
			loading = false;
			return;
		}
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

			let personaRes: { ok: boolean; data?: Persona[]; error?: { message: string } } = { ok: true, data: [] };
			let voiceRes: { ok: boolean; data?: PersonaVoice[]; error?: { message: string } } = { ok: true, data: [] };
			if (!isMember) {
				[personaRes, voiceRes] = await Promise.all([client.getPersonas(), client.getPersonaVoices()]);
			}

			const messages = [
				!contentRes.ok && contentRes.error?.message,
				!personaRes.ok && personaRes.error?.message,
				!voiceRes.ok && voiceRes.error?.message,
				!spaceRes.ok && spaceRes.error?.message
			].filter(Boolean);
			if (messages.length > 0) {
				throw new Error(messages.join('; ') || 'Failed to load batch options.');
			}

			contentTypes = (contentRes.data ?? []).map(mapPublicContentType);
			personas = personaRes.data ?? [];
			voices = voiceRes.data ?? [];
			spaces = spaceRes.data ?? [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load batch options.';
		} finally {
			loading = false;
		}
	}

	function buildLookups(): PromptLookups {
		const persona = personas.find((p) => p.id === settings.default_persona_id);
		const voice = voices.find((v) => v.id === settings.default_voice_id);
		const space = spaces.find((s) => s.id === settings.default_space_id);
		const contentType = contentTypes.find((t) => t.type === settings.default_content_type);
		return {
			contentTypeName: contentType?.label ?? settings.default_content_type,
			guideCategoryName: settings.default_guide_category,
			personaName: persona?.profile_name ?? '',
			voiceName: voice?.name ?? '',
			voicePrompt: voice?.voice_prompt,
			spaceName: space?.name ?? ''
		};
	}

	function handleParse() {
		error = null;
		topics = parseBatchTopics(rawInput);
		if (topics.length === 0) {
			error = 'No topics found. Enter one title per line, a CSV with a title column, or a JSON array.';
			return;
		}
		showStatus(`Parsed ${topics.length} topic(s).`);
	}

	function handleGenerate() {
		error = null;
		if (topics.length === 0) {
			handleParse();
			if (topics.length === 0) return;
		}
		try {
			const lookups = buildLookups();
			generated = buildBatchPrompt(topics, settings, lookups);
			showStatus('Batch prompt generated.');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to generate batch prompt.';
		}
	}

	async function handleCopy() {
		if (!generated) return;
		try {
			await copyToClipboard(generated);
			showStatus('Copied!');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to copy to clipboard.';
		}
	}

	async function handleSave() {
		if (!generated) return;
		if (!settings.projectRoot) {
			error = 'Project root is not configured. Set it in Settings before saving.';
			return;
		}
		saving = true;
		try {
			const instruction = buildProjectInstruction(generated);
			await saveProjectInstruction(settings.projectRoot, instruction);
			showStatus('Saved!');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save to CLAUDE.md.';
		} finally {
			saving = false;
		}
	}

	function showStatus(message: string) {
		if (statusTimeout) clearTimeout(statusTimeout);
		statusMessage = message;
		statusTimeout = setTimeout(() => {
			statusMessage = null;
		}, 2000);
	}

	function clearError() {
		error = null;
	}

	function clearStatus() {
		if (statusTimeout) clearTimeout(statusTimeout);
		statusMessage = null;
	}
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6">
		<h1 class="text-2xl font-bold">Batch Prompt</h1>
		<p class="text-sm text-muted-foreground">
			Import a list of topics and generate one Claude prompt that creates many .md files.
		</p>
	</header>

	{#if statusMessage}
		<div
			class="mb-4 rounded-lg border border-success/20 bg-success/10 p-3 text-sm text-success"
			role="status"
		>
			<div class="flex items-center justify-between">
				<span>{statusMessage}</span>
				<button class="text-xs underline hover:no-underline" type="button" onclick={clearStatus}>
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
				<div class="flex items-start gap-2">
					<AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
					<div>
						<p class="font-medium">Something went wrong</p>
						<p class="mt-1">{error}</p>
					</div>
				</div>
				<button class="text-xs underline hover:no-underline" type="button" onclick={clearError}>
					Dismiss
				</button>
			</div>
		</div>
	{/if}

	{#if !settings.baseUrl || !settings.apiKey}
		<div class="rounded-xl border border-border bg-card p-6">
			<p class="text-sm text-muted-foreground">
				Configure Settings first to load options and save generated prompts.
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
			<span>Loading batch options...</span>
		</div>
	{:else}
		<div class="grid gap-6 lg:grid-cols-2">
			<div class="space-y-4 rounded-xl border border-border bg-card p-6">
				<label class="block text-sm font-medium">
					Topics (one per line, CSV, or JSON array)
					<textarea
						class="mt-1 h-64 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
						placeholder={`e.g.\nCost of Living in Canada\nFinding Housing in Toronto, guide, relocation\n\nJSON: [{"title":"...","content_type":"guide"}]\nCSV: title,content_type,guide_category`}
						bind:value={rawInput}
					></textarea>
				</label>

				<div class="flex flex-wrap gap-2">
					<button
						type="button"
						class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
						onclick={handleParse}
					>
						<span>Parse</span>
					</button>
					<button
						type="button"
						class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover"
						onclick={handleGenerate}
					>
						<Sparkles class="h-4 w-4" />
						<span>Generate Batch Prompt</span>
					</button>
				</div>

				{#if topics.length > 0}
					<div class="rounded-lg border border-border bg-surface p-3 text-sm">
						<p class="font-medium">{topics.length} topic(s) parsed</p>
						<ul class="mt-1 space-y-1 text-xs text-muted-foreground">
							{#each topics.slice(0, 5) as topic (topic.title)}
								<li>{topic.title} → jc-content/{topic.content_type || settings.default_content_type || 'guide'}/{topic.title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')}.md</li>
							{/each}
							{#if topics.length > 5}
								<li>...and {topics.length - 5} more</li>
							{/if}
						</ul>
					</div>
				{/if}

				<div class="flex flex-wrap gap-2 pt-2">
					<button
						type="button"
						class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
						disabled={!generated}
						onclick={handleCopy}
					>
						<Copy class="h-4 w-4" />
						<span>Copy</span>
					</button>
					<button
						type="button"
						class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
						disabled={!generated || saving}
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

			<Preview value={generated} />
		</div>
	{/if}
</div>
