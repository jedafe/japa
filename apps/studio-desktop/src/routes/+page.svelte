<script lang="ts">
	import { onMount } from 'svelte';
	import {
		RefreshCw,
		CheckSquare,
		Square,
		Send,
		Eye,
		RotateCcw,
		Loader2,
		AlertCircle,
		Sparkles,
		Settings
	} from 'lucide-svelte';
	import { join } from '@tauri-apps/api/path';
	import { readTextFile } from '@tauri-apps/plugin-fs';
	import { queueStore } from '$lib/stores/queue.svelte';
	import { scanProject, type ScannedFile } from '$lib/services/file-scanner';
	import { parseFrontmatter } from '$lib/services/markdown';
	import { publishContent } from '$lib/services/publisher';
	import { loadSettings, type StudioSettings } from '$lib/services/settings';
	import type { FileState } from '$lib/types/queue';

	type QueueRow = FileState & { guideCategory?: string };

	const stateOptions: { value: FileState['syncState']; label: string }[] = [
		{ value: 'new', label: 'New' },
		{ value: 'modified', label: 'Modified' },
		{ value: 'published', label: 'Published' },
		{ value: 'in_review', label: 'In Review' },
		{ value: 'failed', label: 'Failed' },
		{ value: 'archived', label: 'Archived' }
	];

	let settings = $state<StudioSettings>(loadSettings());
	let scanning = $state(false);
	let publishing = $state(false);
	let error = $state<string | null>(null);
	let statusMessage = $state<string | null>(null);
	let scanned = $state<ScannedFile[]>([]);
	let selected = $state<Set<string>>(new Set());
	let statusTimeout = $state<ReturnType<typeof setTimeout> | undefined>(undefined);

	onMount(() => {
		queueStore.load();
		settings = loadSettings();
		if (settings.projectRoot) {
			void scan();
		}
	});

	function formatDate(iso: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleString();
	}

	function showStatus(message: string) {
		if (statusTimeout) clearTimeout(statusTimeout);
		statusMessage = message;
		statusTimeout = setTimeout(() => {
			statusMessage = null;
		}, 2500);
	}

	function clearError() {
		error = null;
	}

	function clearStatus() {
		if (statusTimeout) clearTimeout(statusTimeout);
		statusMessage = null;
	}

	function buildFileStates(scan: ScannedFile[], existing: FileState[]): FileState[] {
		return scan
			.map((sf) => {
				const prev = existing.find((f) => f.relativePath === sf.relativePath);
				const lastPublishedHash = prev?.lastPublishedHash ?? '';
				const lastPublishedAt = prev?.lastPublishedAt ?? '';
				const contentId = prev?.contentId;
				const lastError = prev?.lastError ?? '';
				let syncState: FileState['syncState'] = 'new';

				if (prev) {
					if (sf.hash === prev.lastHash) {
						syncState = prev.syncState;
					} else if (sf.hash === lastPublishedHash) {
						syncState = 'published';
					} else {
						syncState = 'modified';
					}
				}

				return {
					relativePath: sf.relativePath,
					contentType: sf.contentType,
					slug: sf.slug,
					title: sf.title,
					lastHash: sf.hash,
					lastPublishedHash,
					lastPublishedAt,
					syncState,
					lastError,
					contentId
				};
			})
			.sort((a, b) => a.relativePath.localeCompare(b.relativePath));
	}

	function buildQueueRows(scan: ScannedFile[], existing: FileState[]): QueueRow[] {
		const lookup = new Map<string, string | undefined>(
			scan.map((sf) => [sf.relativePath, sf.guideCategory])
		);
		return buildFileStates(scan, existing).map((state) => ({
			...state,
			guideCategory: lookup.get(state.relativePath)
		}));
	}

	const rows = $derived(buildQueueRows(scanned, queueStore.files));
	const allSelected = $derived(rows.length > 0 && selected.size === rows.length);

	async function scan() {
		if (!settings.projectRoot) return;
		scanning = true;
		error = null;
		try {
			scanned = await scanProject(settings.projectRoot);
			queueStore.setFiles(buildFileStates(scanned, queueStore.files));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to scan project files.';
		} finally {
			scanning = false;
		}
	}

	function toggleSelectAll() {
		if (allSelected) {
			selected = new Set();
		} else {
			selected = new Set(rows.map((r) => r.relativePath));
		}
	}

	function toggleRow(path: string) {
		const next = new Set(selected);
		if (next.has(path)) next.delete(path);
		else next.add(path);
		selected = next;
	}

	function handleStateChange(path: string, value: string) {
		if (!stateOptions.some((o) => o.value === value)) return;
		queueStore.updateFile(path, { syncState: value as FileState['syncState'] });
	}

	async function publishRow(row: QueueRow) {
		if (!settings.projectRoot) return { ok: false, error: 'Project root not set.' };
		const filePath = await join(settings.projectRoot, row.relativePath);
		const text = await readTextFile(filePath);
		const { frontmatter, body } = parseFrontmatter(text);
		return publishContent({
			relativePath: row.relativePath,
			frontmatter,
			body,
			raw: text
		});
	}

	async function publishSelected() {
		if (selected.size === 0) return;
		if (!settings.baseUrl || !settings.apiKey) {
			error = 'Cannot publish: API settings are missing.';
			return;
		}
		publishing = true;
		error = null;
		let published = 0;
		let failed = 0;

		for (const path of selected) {
			const row = rows.find((r) => r.relativePath === path);
			if (!row) continue;
			const result = await publishRow(row);
			if (result.ok) {
				published++;
			} else {
				failed++;
				queueStore.updateFile(path, { syncState: 'failed', lastError: result.error ?? '' });
			}
		}

		publishing = false;
		selected = new Set();
		showStatus(`Published ${published} file(s)${failed > 0 ? `, ${failed} failed` : ''}`);
	}

	function markSelectedForReview() {
		if (selected.size === 0) return;
		for (const path of selected) {
			queueStore.updateFile(path, { syncState: 'in_review' });
		}
		showStatus(`Marked ${selected.size} file(s) for review`);
		selected = new Set();
	}

	async function publishFile(row: QueueRow) {
		if (!settings.baseUrl || !settings.apiKey) {
			error = 'Cannot publish: API settings are missing.';
			return;
		}
		publishing = true;
		error = null;
		const result = await publishRow(row);
		publishing = false;
		if (!result.ok) {
			error = result.error ?? 'Publish failed.';
			queueStore.updateFile(row.relativePath, { syncState: 'failed', lastError: result.error ?? '' });
			return;
		}
		showStatus(`Published "${row.title}" (${result.status ?? ''}).`);
	}

	function previewFile(row: QueueRow) {
		console.log('Preview file:', row.relativePath);
		showStatus(`Preview "${row.title}" — not yet implemented`);
	}

	function stateBadgeClasses(state: FileState['syncState']): string {
		switch (state) {
			case 'new':
				return 'bg-primary/10 text-primary border-primary/20';
			case 'modified':
				return 'bg-orange-500/10 text-orange-600 border-orange-500/20';
			case 'published':
				return 'bg-success/10 text-success border-success/20';
			case 'in_review':
				return 'bg-purple-500/10 text-purple-600 border-purple-500/20';
			case 'failed':
				return 'bg-danger/10 text-danger border-danger/20';
			case 'archived':
				return 'bg-muted text-muted-foreground border-border';
			default:
				return 'bg-surface text-foreground border-border';
		}
	}
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6 flex items-center justify-between gap-4">
		<div>
			<h1 class="text-2xl font-bold">Content Queue</h1>
			<p class="text-sm text-muted-foreground">Review, edit, preview, and publish JapaConnect content.</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<a
				href="/prompt"
				class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover"
			>
				<Sparkles class="h-4 w-4" />
				<span>New Prompt</span>
			</a>
			<a
				href="/settings"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
			>
				<Settings class="h-4 w-4" />
				<span>Settings</span>
			</a>
		</div>
	</header>

	{#if statusMessage}
		<div
			class="mb-4 rounded-lg border border-success/20 bg-success/10 p-3 text-sm text-success"
			role="status"
		>
			<div class="flex items-center justify-between">
				<span>{statusMessage}</span>
				<button
					type="button"
					class="text-xs underline hover:no-underline"
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
				<div class="flex items-start gap-2">
					<AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
					<div>
						<p class="font-medium">Something went wrong</p>
						<p class="mt-1">{error}</p>
					</div>
				</div>
				<button
					type="button"
					class="text-xs underline hover:no-underline"
					onclick={clearError}
					aria-label="Dismiss error"
				>
					Dismiss
				</button>
			</div>
		</div>
	{/if}

	{#if !settings.projectRoot}
		<div class="rounded-xl border border-border bg-card p-8 text-center">
			<h2 class="text-lg font-semibold">Project root not configured</h2>
			<p class="mt-2 text-sm text-muted-foreground">
				Set a project root that contains <code class="rounded bg-muted px-1">jc-content/</code> in
				Settings.
			</p>
			<a
				href="/settings"
				class="mt-4 inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover"
			>
				<Settings class="h-4 w-4" />
				<span>Go to Settings</span>
			</a>
		</div>
	{:else if scanning}
		<div class="flex h-64 items-center justify-center gap-2 text-muted-foreground">
			<Loader2 class="h-5 w-5 animate-spin" />
			<span>Scanning project files...</span>
		</div>
	{:else if rows.length > 0}
		<div class="mb-4 flex flex-wrap items-center gap-2">
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
				disabled={scanning}
				onclick={scan}
			>
				{#if scanning}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<RefreshCw class="h-4 w-4" />
				{/if}
				<span>Rescan</span>
			</button>

			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
				onclick={toggleSelectAll}
			>
				{#if allSelected}
					<CheckSquare class="h-4 w-4" />
				{:else}
					<Square class="h-4 w-4" />
				{/if}
				<span>{allSelected ? 'Clear selection' : 'Select all'}</span>
			</button>

			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover disabled:opacity-60"
				disabled={publishing || selected.size === 0}
				onclick={publishSelected}
			>
				{#if publishing && selected.size > 0}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Send class="h-4 w-4" />
				{/if}
				<span>Publish selected</span>
			</button>

			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
				disabled={selected.size === 0}
				onclick={markSelectedForReview}
			>
				<RotateCcw class="h-4 w-4" />
				<span>Mark selected for review</span>
			</button>
		</div>

		<div class="overflow-hidden rounded-xl border border-border bg-card">
			<table class="w-full text-sm">
				<thead class="border-b border-border bg-accent">
					<tr>
						<th class="w-10 px-3 py-2 text-left">
							<button
								type="button"
								class="inline-flex items-center"
								onclick={toggleSelectAll}
								aria-label={allSelected ? 'Clear selection' : 'Select all'}
							>
								{#if allSelected}
									<CheckSquare class="h-4 w-4" />
								{:else}
									<Square class="h-4 w-4" />
								{/if}
							</button>
						</th>
						<th class="px-4 py-2 text-left font-medium">Title</th>
						<th class="px-4 py-2 text-left font-medium">Content Type</th>
						<th class="px-4 py-2 text-left font-medium">Guide Category</th>
						<th class="px-4 py-2 text-left font-medium">Sync State</th>
						<th class="px-4 py-2 text-left font-medium">Last Modified</th>
						<th class="px-4 py-2 text-left font-medium">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as row (row.relativePath)}
						<tr class="border-b border-border last:border-0 hover:bg-accent/50">
							<td class="px-3 py-2">
								<input
									type="checkbox"
									class="h-4 w-4 rounded border-border"
									checked={selected.has(row.relativePath)}
									onchange={() => toggleRow(row.relativePath)}
								/>
							</td>
							<td class="px-4 py-2">
								<a
									href="/editor/{encodeURIComponent(row.relativePath)}"
									class="font-medium text-primary hover:underline"
								>
									{row.title}
								</a>
							</td>
							<td class="px-4 py-2 capitalize">{row.contentType}</td>
							<td class="px-4 py-2">{row.guideCategory ?? ''}</td>
							<td class="px-4 py-2">
								<select
									class="rounded border border-border bg-surface px-2 py-1 text-xs capitalize focus:outline-none focus:ring-2 focus:ring-primary"
									value={row.syncState}
									onchange={(event) =>
										handleStateChange(
											row.relativePath,
											(event.currentTarget as HTMLSelectElement).value
										)}
								>
									{#each stateOptions as option}
										<option value={option.value}>{option.label}</option>
									{/each}
								</select>
								<span
									class="ml-2 inline-flex items-center rounded border px-2 py-0.5 text-xs capitalize {stateBadgeClasses(
										row.syncState
									)}"
								>
									{row.syncState.replace('_', ' ')}
								</span>
							</td>
							<td class="px-4 py-2 text-muted-foreground">
								{row.syncState === 'published' ? formatDate(row.lastPublishedAt) : '—'}
							</td>
							<td class="px-4 py-2">
								<div class="flex items-center gap-2">
									<a
										href="/editor/{encodeURIComponent(row.relativePath)}"
										class="text-xs text-primary hover:underline"
									>
										Edit
									</a>
									<button
										type="button"
										class="inline-flex items-center rounded p-1 hover:bg-accent"
										onclick={() => previewFile(row)}
										aria-label="Preview"
									>
										<Eye class="h-4 w-4 text-muted-foreground" />
									</button>
									<button
										type="button"
										class="inline-flex items-center rounded p-1 hover:bg-accent disabled:opacity-60"
										disabled={publishing}
										onclick={() => publishFile(row)}
										aria-label="Publish"
									>
										<Send class="h-4 w-4 text-primary" />
									</button>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<div class="rounded-xl border border-border bg-card p-8 text-center">
			<h2 class="text-lg font-semibold">No content files found</h2>
			<p class="mt-2 text-sm text-muted-foreground">
				Generate prompts in the Prompt Builder to create
				<code class="rounded bg-muted px-1">jc-content/[content-type]/[slug].md</code> files,
				then rescan.
			</p>
			<div class="mt-4 flex items-center justify-center gap-2">
				<button
					type="button"
					class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
					onclick={scan}
				>
					{#if scanning}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						<RefreshCw class="h-4 w-4" />
					{/if}
					<span>Rescan</span>
				</button>
				<a
					href="/prompt"
					class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover"
				>
					<Sparkles class="h-4 w-4" />
					<span>New Prompt</span>
				</a>
			</div>
		</div>
	{/if}
</div>
