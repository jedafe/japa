<script lang="ts">
	import { onMount } from 'svelte';
	import { Save, FolderOpen, TestTube, Loader2, Check, AlertCircle } from 'lucide-svelte';
	import { open } from '@tauri-apps/plugin-dialog';
	import { loadSettings, saveSettings, type StudioSettings } from '$lib/services/settings';
	import { createApiClient } from '$lib/services/api';

	let settings = $state<StudioSettings>(loadSettings());
	let saving = $state(false);
	let testing = $state(false);
	let testResult = $state<{ ok: boolean; message: string } | null>(null);
	let statusMessage = $state<string | null>(null);

	onMount(() => {
		settings = loadSettings();
	});

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
		try {
			const client = createApiClient(settings.baseUrl, settings.apiKey);
			const res = await client.getMe();
			if (res.ok) {
				testResult = { ok: true, message: `Connected as ${res.data?.email ?? res.data?.id}.` };
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

	function handleSave() {
		saving = true;
		saveSettings(settings);
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
		<p class="text-sm text-muted-foreground">Configure the JapaConnect connection and watched project root.</p>
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
		</label>

		<label class="block text-sm font-medium">
			JapaConnect Base URL
			<input
				type="url"
				class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
				placeholder="https://api.japaconnect.com"
				value={settings.baseUrl}
				oninput={(event) => update('baseUrl', (event.currentTarget as HTMLInputElement).value)}
			/>
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
			<p class="mt-1 text-xs text-muted-foreground">Stored locally in the browser. Use a service account key.</p>
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
