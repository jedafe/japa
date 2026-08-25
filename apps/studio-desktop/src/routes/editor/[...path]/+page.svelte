<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { join } from '@tauri-apps/api/path';
	import { readTextFile, writeTextFile } from '@tauri-apps/plugin-fs';
	import { Loader2, Save, Eye, FileWarning, ArrowLeft } from 'lucide-svelte';
	import { loadSettings, type StudioSettings } from '$lib/services/settings';
	import { queueStore } from '$lib/stores/queue.svelte';
	import { hashFile } from '$lib/services/hash';
	import { parseFrontmatter, markdownToHtml } from '$lib/services/markdown';

	let settings = $state<StudioSettings>(loadSettings());
	let loading = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let statusMessage = $state<string | null>(null);
	let raw = $state('');
	let relativePath = $state('');
	let filePath = $state('');
	const pageParam = $derived($page.params.path);

	const parsed = $derived(parseFrontmatter(raw));
	const preview = $derived(markdownToHtml(parsed.body));

	onMount(() => {
		queueStore.load();
		settings = loadSettings();
		void loadFile();
	});

	async function loadFile() {
		const param = pageParam;
		if (!param) {
			error = 'No file path provided.';
			loading = false;
			return;
		}

		relativePath = Array.isArray(param) ? param.map(decodeURIComponent).join('/') : decodeURIComponent(param);

		if (!settings.projectRoot) {
			error = 'Project root is not configured. Go to Settings.';
			loading = false;
			return;
		}

		filePath = await join(settings.projectRoot, relativePath);

		try {
			raw = await readTextFile(filePath);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to read file.';
		} finally {
			loading = false;
		}
	}

	async function handleSave() {
		if (!filePath) return;
		saving = true;
		try {
			await writeTextFile(filePath, raw);
			const newHash = await hashFile(raw);
			const existing = queueStore.files.find((f) => f.relativePath === relativePath);
			if (existing) {
				const syncState = newHash === existing.lastPublishedHash ? 'published' : 'modified';
				queueStore.updateFile(relativePath, { lastHash: newHash, syncState });
			} else {
				const frontmatter = parseFrontmatter(raw).frontmatterFields;
				const slug = frontmatter.slug || relativePath.split('/').pop()?.replace(/\.md$/i, '') || '';
				const contentType = relativePath.split('/')[1] || '';
				queueStore.setFiles([
					...queueStore.files,
					{
						relativePath,
						contentType,
						slug,
						title: frontmatter.title || slug,
						lastHash: newHash,
						lastPublishedHash: '',
						lastPublishedAt: '',
						syncState: 'modified',
						lastError: ''
					}
				]);
			}
			showStatus('Saved.');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save file.';
		} finally {
			saving = false;
		}
	}

	function showStatus(message: string) {
		statusMessage = message;
		setTimeout(() => {
			statusMessage = null;
		}, 2000);
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
				<h1 class="text-lg font-bold leading-tight">{parsed.frontmatterFields.title || 'Untitled'}</h1>
				<p class="text-sm text-muted-foreground">{relativePath}</p>
			</div>
		</div>
		<div class="flex items-center gap-2">
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
				<span>Save</span>
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
						<Eye class="h-4 w-4" />
						<span>Source</span>
					</div>
				</div>
				<textarea
					class="h-full w-full flex-1 resize-none rounded-b-xl bg-surface p-4 text-sm font-mono leading-relaxed focus:outline-none"
					value={raw}
					oninput={(event) => handleRawInput((event.currentTarget as HTMLTextAreaElement).value)}
					spellcheck="false"
				></textarea>
			</div>

			<div class="flex min-h-0 flex-col rounded-xl border border-border bg-card">
				<div class="border-b border-border bg-accent px-4 py-2 text-sm font-medium">Preview</div>
				<div class="text-sm leading-relaxed h-full w-full flex-1 overflow-y-auto p-4">
					{@html preview}
				</div>
			</div>
		</div>
	{/if}
</div>
