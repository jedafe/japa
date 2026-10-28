<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { onMount } from 'svelte';
	import { Save, FolderOpen, TestTube, Loader2, Check, AlertCircle, Trash2 } from 'lucide-svelte';
	import { promptStore } from '$lib/stores/prompt.svelte';
	import { open } from '@tauri-apps/plugin-dialog';
	import { initSettingsDb, loadSettings, saveSettings, type StudioSettings } from '$lib/services/settings';
	import { createApiClient, mapPublicContentType } from '$lib/services/api';
	import { refreshAuth } from '$lib/api/auth';
	import {
		type StudioContentType,
		type Persona,
		type PersonaVoice,
		type Space,
		type SelectOption
	} from '$lib/types/prompt';
	import SelectField from '$lib/components/prompt/SelectField.svelte';
	import TextField from '$lib/components/prompt/TextField.svelte';

	let settings = $state<StudioSettings>(loadSettings());
	let saving = $state(false);
	let testing = $state(false);
	let testResult = $state<{ ok: boolean; message: string } | null>(null);
	let statusMessage = $state<string | null>(null);
	let tokenStatus = $state<string | null>(null);
	let optionErrors = $state<string[]>([]);
	let contentTypes = $state<StudioContentType[]>([]);
	let loadingContentTypes = $state(false);
	let personas = $state<Persona[]>([]);
	let voices = $state<PersonaVoice[]>([]);
	let spaces = $state<Space[]>([]);
	let statusOptions = $state<SelectOption[]>([]);
	let visibilityOptions = $state<SelectOption[]>([]);

	const contentTypeOptions = $derived(
		contentTypes.map((t) => ({ value: t.type, label: t.label }))
	);
	const defaultGuideCategoryOptions = $derived(
		contentTypes.find((t) => t.type === settings.default_content_type)?.guide_categories ?? []
	);
	const personaOptions = $derived(
		personas.map((p) => ({ value: p.id, label: p.profile_name }))
	);
	const voiceOptions = $derived(
		voices.map((v) => ({ value: v.id, label: v.name }))
	);
	const spaceOptions = $derived(
		spaces.map((s) => ({ value: s.id, label: s.name }))
	);
	const defaultTagsInput = $derived(settings.default_tags.join(', '));

	onMount(async () => {
		await initSettingsDb();
		settings = loadSettings();
		promptStore.refreshTemplates();
		void loadOptions();
	});

	async function loadOptions() {
		if (!settings.baseUrl || !settings.apiKey) return;
		loadingContentTypes = true;
		optionErrors = [];
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
				statusOptions = statusRes.data;
			}
			if (visibilityRes.ok && visibilityRes.data) {
				visibilityOptions = visibilityRes.data;
			}

			optionErrors = [
				!contentRes.ok && `content types: ${contentRes.error?.message}`,
				!personaRes.ok && `personas: ${personaRes.error?.message}`,
				!voiceRes.ok && `voices: ${voiceRes.error?.message}`,
				!spaceRes.ok && `spaces: ${spaceRes.error?.message}`,
				!statusRes.ok && `status options: ${statusRes.error?.message}`,
				!visibilityRes.ok && `visibility options: ${visibilityRes.error?.message}`
			].filter(Boolean) as string[];
		} catch (e) {
			optionErrors = [e instanceof Error ? e.message : 'Failed to load options.'];
		} finally {
			loadingContentTypes = false;
		}
	}

	function handleDefaultContentTypeChange(contentType: string) {
		settings = { ...settings, default_content_type: contentType };
		const ct = contentTypes.find((t) => t.type === contentType);
		if (!ct?.guide_categories?.some((c) => c.value === settings.default_guide_category)) {
			settings = { ...settings, default_guide_category: '' };
		}
	}

	function handleDefaultTagsInput(value: string) {
		settings = {
			...settings,
			default_tags: value
				.split(',')
				.map((t) => t.trim())
				.filter((t) => t.length > 0)
		};
	}

	function toggleAutoSyncContentType(type: string) {
		const set = new Set(settings.autoSyncContentTypes);
		if (set.has(type)) set.delete(type);
		else set.add(type);
		update('autoSyncContentTypes', Array.from(set));
	}

	async function handleSelectFolder() {
		try {
			const selected = await open({
				directory: true,
				multiple: false,
				title: 'Select the project root containing jc-content/',
				defaultPath: settings.projectRoot || undefined
			});
			if (typeof selected === 'string' && selected.length > 0) {
				settings = { ...settings, projectRoot: selected };
			}
		} catch (e) {
			testResult = {
				ok: false,
				message: e instanceof Error ? e.message : 'Failed to open folder dialog.'
			};
		}
	}

	async function handleTestConnection() {
		if (!settings.baseUrl || !settings.apiKey) {
			testResult = { ok: false, message: 'Base URL and API key are required.' };
			return;
		}
		testing = true;
		testResult = null;
		tokenStatus = null;
		try {
			const client = createApiClient(settings.baseUrl);
			const res = await client.getMe();
			if (res.ok) {
				testResult = { ok: true, message: `Connected as ${res.data?.email ?? res.data?.id}.` };
				settings = loadSettings();
				await loadOptions();
			} else {
				testResult = { ok: false, message: res.error?.message ?? 'Connection failed.' };
			}
		} catch (e) {
			testResult = {
				ok: false,
				message: e instanceof Error ? e.message : 'Connection failed.'
			};
		} finally {
			testing = false;
		}
	}

	async function handleRefreshSession() {
		if (!settings.baseUrl || !settings.apiKey) {
			tokenStatus = 'Base URL and API key are required.';
			return;
		}
		testing = true;
		tokenStatus = null;
		try {
			const ok = await refreshAuth();
			if (ok) {
				settings = loadSettings();
				tokenStatus = settings.tokenExpiresAt
					? `Session refreshed. Expires ${new Date(settings.tokenExpiresAt).toLocaleString()}.`
					: 'Session refreshed.';
			} else {
				tokenStatus = 'Could not refresh session. Check the API key.';
			}
		} catch (e) {
			tokenStatus = e instanceof Error ? e.message : 'Session refresh failed.';
		} finally {
			testing = false;
		}
	}

	async function handleSave() {
		saving = true;
		await saveSettings(settings);
		statusMessage = 'Settings saved.';
		setTimeout(() => {
			statusMessage = null;
			saving = false;
		}, 1200);
	}

	function update<K extends keyof StudioSettings>(key: K, value: StudioSettings[K]) {
		settings = { ...settings, [key]: value };
	}
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6">
		<h1 class="text-2xl font-bold">Settings</h1>
		<p class="text-sm text-muted-foreground">Configure the {siteName()} connection and watched project root.</p>
	</header>

	<div class="max-w-2xl space-y-4 rounded-xl border border-border bg-card p-6">
		{#if statusMessage}
			<div class="rounded-lg border border-success/20 bg-success/10 p-3 text-sm text-success" role="status">
				<div class="flex items-center gap-2">
					<Check class="h-4 w-4" />
					<span>{statusMessage}</span>
				</div>
			</div>
		{/if}

		{#if testResult}
			<div
				class="rounded-lg border p-4 text-sm {testResult.ok
					? 'border-success/20 bg-success/10 text-success'
					: 'border-danger/20 bg-danger/5 text-danger'}"
				role="alert"
			>
				<div class="flex items-start gap-2">
					{#if testResult.ok}
						<Check class="mt-0.5 h-4 w-4 shrink-0" />
					{:else}
						<AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
					{/if}
					<p>{testResult.message}</p>
				</div>
			</div>
		{/if}

		<label class="block text-sm font-medium">
			Project Name
			<input
				type="text"
				class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
				placeholder="My Japa Project"
				value={settings.projectName}
				oninput={(event) => update('projectName', (event.currentTarget as HTMLInputElement).value)}
			/>
				<p class="mt-1 text-xs text-muted-foreground">The name shown in the app header and exports.</p>
		</label>

		<label class="block text-sm font-medium">
			{siteName()} Base URL
			<input
				type="url"
				class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
				placeholder="https://api.japa.to"
				value={settings.baseUrl}
				oninput={(event) => update('baseUrl', (event.currentTarget as HTMLInputElement).value)}
			/>
				<p class="mt-1 text-xs text-muted-foreground">The API server this instance talks to — usually https://api.japa.to.</p>
		</label>

		<label class="block text-sm font-medium">
			API Key
			<input
				type="password"
				class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
				placeholder="jc_live_..."
				value={settings.apiKey}
				oninput={(event) => update('apiKey', (event.currentTarget as HTMLInputElement).value)}
			/>
			<p class="mt-1 text-xs text-muted-foreground">
				Stored locally in the app database. Use a service account key with the <code class="rounded bg-muted px-1">api.write.content</code> scope.
			</p>

			<div class="mt-3 flex gap-4 text-xs">
				<label class="inline-flex items-center gap-2">
					<input
						type="radio"
						value="operator"
						checked={settings.mode === 'operator'}
						onchange={() => update('mode', 'operator')}
					/>
					<span>Operator mode (post as a persona)</span>
				</label>
				<label class="inline-flex items-center gap-2">
					<input
						type="radio"
						value="member"
						checked={settings.mode === 'member'}
						onchange={() => update('mode', 'member')}
					/>
					<span>Member mode (post as myself)</span>
				</label>
			</div>
			<p class="mt-1 text-xs text-muted-foreground">
				Member mode hides persona/voice fields and does not reassign authorship.
			</p>

			{#if tokenStatus}
				<p class="mt-2 text-xs {tokenStatus.startsWith('Session refreshed') ? 'text-success' : 'text-danger'}">{tokenStatus}</p>
			{/if}

			<button
				type="button"
				class="mt-2 inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-3 py-1.5 text-xs hover:bg-accent disabled:opacity-60"
				disabled={testing || !settings.baseUrl || !settings.apiKey}
				onclick={handleRefreshSession}
			>
				{#if testing}
					<Loader2 class="h-3 w-3 animate-spin" />
				{/if}
				<span>Refresh session token</span>
			</button>
		</label>

		<div class="block text-sm font-medium">
			Project Root Folder
			<div class="mt-1 flex gap-2">
				<input
					type="text"
					class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
					placeholder="/home/user/japa-studio/project-a"
					value={settings.projectRoot}
					oninput={(event) => update('projectRoot', (event.currentTarget as HTMLInputElement).value)}
				/>
				<button
					type="button"
					class="inline-flex shrink-0 items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
					onclick={handleSelectFolder}
				>
					<FolderOpen class="h-4 w-4" />
					<span>Browse</span>
				</button>
			</div>
			<p class="mt-1 text-xs text-muted-foreground">
				The folder containing <code class="rounded bg-muted px-1">.claude/CLAUDE.md</code> and
				<code class="rounded bg-muted px-1">jc-content/</code>.
			</p>
		</div>

		<label class="inline-flex items-center gap-2 text-sm font-medium">
			<input
				type="checkbox"
				class="h-4 w-4 rounded border-border"
				checked={settings.autoSync}
				onchange={(event) =>
					update('autoSync', (event.currentTarget as HTMLInputElement).checked)}
			/>
			<span>Enable auto-sync</span>
		</label>
					<p class="mt-1 text-xs text-muted-foreground">Automatically sync new posts when you save.</p>

		{#if settings.autoSync}
			<div class="rounded-lg border border-border bg-surface p-3">
				<p class="text-xs font-medium text-muted-foreground mb-2">
					Auto-sync content types (leave empty to sync all)
				</p>
				{#if loadingContentTypes}
					<div class="flex items-center gap-2 text-xs text-muted-foreground">
						<Loader2 class="h-3 w-3 animate-spin" />
						<span>Loading content types...</span>
					</div>
				{:else if contentTypes.length === 0}
					<p class="text-xs text-muted-foreground">Connect and test the API to load content types.</p>
				{:else}
					<div class="flex flex-wrap gap-2">
						{#each contentTypes as ct (ct.type)}
							<label class="inline-flex items-center gap-1.5 rounded border border-border bg-card px-2 py-1 text-xs">
								<input
									type="checkbox"
									class="h-3 w-3 rounded border-border"
									checked={settings.autoSyncContentTypes.includes(ct.type)}
									onchange={() => toggleAutoSyncContentType(ct.type)}
								/>
								<span>{ct.label}</span>
							</label>
						{/each}
					</div>
				{/if}
			</div>
		{/if}

		<div class="rounded-lg border border-border bg-surface p-4 space-y-4">
			<h2 class="text-sm font-semibold">Project Defaults</h2>
			<p class="text-xs text-muted-foreground">
				These values pre-fill the prompt builder and new posts. You can still override them each time.
			</p>

			{#if optionErrors.length > 0}
				<div class="rounded-lg border border-danger/20 bg-danger/5 p-3 text-xs text-danger" role="alert">
					<p class="font-medium">Some lookup options could not be loaded:</p>
					<ul class="mt-1 list-disc space-y-1 pl-4">
						{#each optionErrors as msg}
							<li>{msg}</li>
						{/each}
					</ul>
				</div>
			{/if}

			{#if loadingContentTypes}
				<div class="flex items-center gap-2 text-xs text-muted-foreground">
					<Loader2 class="h-3 w-3 animate-spin" />
					<span>Loading defaults options...</span>
				</div>
			{:else if !settings.baseUrl || !settings.apiKey}
				<p class="text-xs text-muted-foreground">
					Configure the API connection to load content types, personas, voices, and spaces.
				</p>
			{:else}
				<SelectField
					label="Default content type"
					value={settings.default_content_type}
					options={contentTypeOptions}
					placeholder="Select a content type"
					onchange={handleDefaultContentTypeChange}
				/>

				{#if defaultGuideCategoryOptions.length > 0}
					<SelectField
						label="Default guide category"
						value={settings.default_guide_category}
						options={defaultGuideCategoryOptions}
						placeholder="Select a guide category"
						onchange={(value) => update('default_guide_category', value)}
					/>
				{/if}

				{#if settings.mode !== 'member'}
					<SelectField
						label="Default persona"
						value={settings.default_persona_id}
						options={personaOptions}
						placeholder="Select a persona"
						onchange={(value) => update('default_persona_id', value)}
					/>

					<SelectField
						label="Default voice"
						value={settings.default_voice_id}
						options={voiceOptions}
						placeholder="Select a voice"
						onchange={(value) => update('default_voice_id', value)}
					/>
				{/if}

				<SelectField
					label="Default space"
					value={settings.default_space_id}
					options={spaceOptions}
					placeholder="Select a space"
					onchange={(value) => update('default_space_id', value)}
				/>

				<div class="grid grid-cols-2 gap-4">
					<SelectField
						label="Default status"
						value={settings.default_status}
						options={statusOptions}
						onchange={(value) => update('default_status', value)}
					/>
					<SelectField
						label="Default visibility"
						value={settings.default_visibility}
						options={visibilityOptions}
						onchange={(value) => update('default_visibility', value)}
					/>
				</div>

				<TextField
					label="Default tags"
					value={defaultTagsInput}
					placeholder="comma, separated, tags"
					oninput={handleDefaultTagsInput}
				/>
			{/if}
		</div>

		<div class="rounded-lg border border-border bg-surface p-4 space-y-4">
			<h2 class="text-sm font-semibold">Prompt Templates</h2>
			<p class="text-xs text-muted-foreground">
				Manage saved prompt builder presets. Use the prompt builder to save new templates.
			</p>

			{#if promptStore.templates.length === 0}
				<p class="text-xs text-muted-foreground">No saved templates yet.</p>
			{:else}
				<div class="space-y-2">
					{#each promptStore.templates as template (template.id)}
						<div class="flex items-center gap-2">
							<input
								type="text"
								class="flex-1 rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
								value={template.name}
								onchange={(event) =>
									promptStore.renameTemplate(
										template.id,
										(event.currentTarget as HTMLInputElement).value
									)}
							/>
							<button
								type="button"
								class="inline-flex items-center gap-1 rounded-lg border border-border bg-surface px-3 py-2 text-sm text-danger hover:bg-danger/5"
								onclick={() => promptStore.removeTemplate(template.id)}
								aria-label="Delete template"
							>
								<Trash2 class="h-4 w-4" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<div class="flex flex-wrap gap-2 pt-4">
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
				disabled={testing || !settings.baseUrl || !settings.apiKey}
				onclick={handleTestConnection}
			>
				{#if testing}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<TestTube class="h-4 w-4" />
				{/if}
				<span>Test Connection</span>
			</button>
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover disabled:opacity-60"
				disabled={saving}
				onclick={handleSave}
			>
				{#if saving}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Save class="h-4 w-4" />
				{/if}
				<span>Save Settings</span>
			</button>
		</div>
	</div>
</div>
