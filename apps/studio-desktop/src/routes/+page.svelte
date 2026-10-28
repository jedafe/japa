<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
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
		Settings,
		X,
		Search,
		Upload,
		ArrowUpDown,
		ArrowUp,
		ArrowDown,
		Trash2
	} from 'lucide-svelte';
	import { join } from '@tauri-apps/api/path';
	import { mkdir, readTextFile, writeTextFile, remove } from '@tauri-apps/plugin-fs';
	import { queueStore } from '$lib/stores/queue.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { scanProject } from '$lib/services/file-scanner';
	import { parseFrontmatter, buildMarkdown, markdownToHtml, escapeHtml } from '$lib/services/markdown';
	import { isPublishing, publishWorker } from '$lib/services/publisher';
	import { initSettingsDb, loadSettings, saveSettings, type StudioSettings } from '$lib/services/settings';
	import { isHighStakes } from '$lib/services/validation';
	import { createApiClient, mapPublicContentType } from '$lib/services/api';
	import { hashFile } from '$lib/services/hash';
	import { slugify } from '$lib/services/prompt';
	import type { FileState } from '$lib/types/queue';
	import type { Persona, Space, StudioContentType } from '$lib/types/prompt';

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
	let error = $state<string | null>(null);
	let statusMessage = $state<string | null>(null);
	let selected = $state<Set<string>>(new Set());
	let statusTimeout = $state<ReturnType<typeof setTimeout> | undefined>(undefined);

	let search = $state('');
	let filterStatus = $state<FileState['syncState'] | 'all'>('all');
	let filterContentType = $state<string>('all');
	let filterGuideCategory = $state<string>('all');
	let filterSpace = $state<string>('all');
	let filterPersona = $state<string>('all');
	let sortBy = $state<'modified' | 'published' | 'title'>('modified');
	let sortDir = $state<'asc' | 'desc'>('desc');

	let previewRow = $state<FileState | null>(null);
	let previewHtml = $state<string>('');
	let previewLoading = $state(false);

	let newPostOpen = $state(false);
	let newPostType = $state('guide');
	let newPostTitle = $state('');
	let newPostCreating = $state(false);

	let spaces = $state<Map<string, Space>>(new Map());
	let personas = $state<Map<string, Persona>>(new Map());
	let contentTypes = $state<Map<string, StudioContentType>>(new Map());

	onMount(async () => {
		await initSettingsDb();
		await queueStore.load();
		settings = loadSettings();
		if (settings.projectRoot) {
			void scan();
		}
		if (settings.baseUrl && settings.apiKey) {
			void loadLookups();
		}
	});

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

	async function loadLookups() {
		const client = createApiClient(settings.baseUrl);
		const isMember = settings.mode === 'member';

		const spaceRes = isMember ? await client.getMySpaces() : await client.getSpaces();
		// Ref: Section 105 — publishable content types.
		const typeRes = await client.getContentTypes();

		let personaRes: { ok: boolean; data?: Persona[]; error?: { message: string } } = { ok: true, data: [] };
		if (!isMember) {
			personaRes = await client.getPersonas();
		}

		if (spaceRes.ok && spaceRes.data) {
			spaces = new Map(spaceRes.data.map((s) => [s.id, s]));
		}
		if (personaRes.ok && personaRes.data) {
			personas = new Map(personaRes.data.map((p) => [p.id, p]));
		}
		if (typeRes.ok && typeRes.data) {
			contentTypes = new Map(
				typeRes.data.map((t) => [t.value, mapPublicContentType(t)])
			);
		}
	}

	async function scan() {
		if (!settings.projectRoot) return;
		scanning = true;
		error = null;
		try {
			const scanned = await scanProject(settings.projectRoot);
			await queueStore.mergeScanned(scanned);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to scan project files.';
		} finally {
			scanning = false;
		}
	}

	function formatDate(iso: string | undefined): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleString();
	}

	function formatId(id: string | undefined): string {
		if (!id) return '—';
		return id.length > 12 ? `${id.slice(0, 8)}…` : id;
	}

	function displayName(
		id: string | undefined,
		map: Map<string, { name?: string; profile_name?: string; label?: string }>
	): string {
		if (!id) return '—';
		const item = map.get(id);
		if (item && 'name' in item && item.name) return item.name;
		if (item && 'profile_name' in item && item.profile_name) return item.profile_name;
		if (item && 'label' in item && item.label) return item.label;
		return formatId(id);
	}

	function contentTypeLabel(type: string): string {
		return contentTypes.get(type)?.label ?? type;
	}

	function findGuideCategoryLabel(value: string | undefined): string {
		if (!value) return '';
		for (const type of contentTypes.values()) {
			const match = type.guide_categories?.find((c) => c.value === value);
			if (match) return match.label;
		}
		return value;
	}

	function guideCategoryLabel(type: string | undefined, value: string | undefined): string {
		if (!value) return '';
		if (type) {
			const match = contentTypes.get(type)?.guide_categories?.find((c) => c.value === value);
			if (match) return match.label;
		}
		return findGuideCategoryLabel(value);
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

	const rows = $derived(queueStore.files);

	const filteredRows = $derived(
		rows
			.filter((r) => {
				const q = search.toLowerCase();
				const matchesSearch =
					!search || r.title.toLowerCase().includes(q) || r.slug.toLowerCase().includes(q);
				const matchesStatus = filterStatus === 'all' || r.syncState === filterStatus;
				const matchesType = filterContentType === 'all' || r.contentType === filterContentType;
				const matchesGuideCategory =
					filterGuideCategory === 'all' || r.guideCategory === filterGuideCategory;
				const matchesSpace = filterSpace === 'all' || r.spaceId === filterSpace;
				const matchesPersona = filterPersona === 'all' || r.personaId === filterPersona;
				return (
					matchesSearch &&
					matchesStatus &&
					matchesType &&
					matchesGuideCategory &&
					matchesSpace &&
					matchesPersona
				);
			})
			.sort((a, b) => {
				let cmp = 0;
				if (sortBy === 'title') {
					cmp = a.title.localeCompare(b.title);
				} else if (sortBy === 'published') {
					const ta = a.lastPublished ? new Date(a.lastPublished).getTime() : 0;
					const tb = b.lastPublished ? new Date(b.lastPublished).getTime() : 0;
					cmp = ta - tb;
				} else {
					const ta = a.lastModified ? new Date(a.lastModified).getTime() : 0;
					const tb = b.lastModified ? new Date(b.lastModified).getTime() : 0;
					cmp = ta - tb;
				}
				return sortDir === 'asc' ? cmp : -cmp;
			})
	);

	const allSelected = $derived(filteredRows.length > 0 && selected.size === filteredRows.length);

	const contentTypeFilterOptions = $derived(
		['all', ...new Set([...contentTypes.keys(), ...rows.map((r) => r.contentType)])].filter(
			(t, i, arr) => t && arr.indexOf(t) === i
		).sort()
	);

	const guideCategoryFilterOptions = $derived(
		['all', ...new Set(rows.map((r) => r.guideCategory).filter((v): v is string => !!v))].sort()
	);

	const spaceFilterOptions = $derived(['all', ...spaces.keys()].sort());

	const personaFilterOptions = $derived(
		settings.mode === 'member' ? [] : ['all', ...personas.keys()].sort()
	);

	const newPostTypeOptions = $derived([...contentTypes.values()].map((t) => t.type).sort());

	function toggleSelectAll() {
		if (allSelected) {
			selected = new Set();
		} else {
			selected = new Set(filteredRows.map((r) => r.relativePath));
		}
	}

	function toggleRow(path: string) {
		const next = new Set(selected);
		if (next.has(path)) next.delete(path);
		else next.add(path);
		selected = next;
	}

	async function handleStateChange(path: string, value: string) {
		if (!stateOptions.some((o) => o.value === value)) return;
		await queueStore.updateFile(path, {
			syncState: value as FileState['syncState'],
			localStatus: value as FileState['syncState']
		});
	}

	function setSort(field: 'modified' | 'published' | 'title') {
		if (sortBy === field) {
			sortDir = sortDir === 'asc' ? 'desc' : 'asc';
		} else {
			sortBy = field;
			sortDir = 'desc';
		}
	}

	async function publishFile(row: FileState) {
		if (!settings.baseUrl || !settings.apiKey) {
			error = 'Cannot publish: API settings are missing.';
			return;
		}
		error = null;
		publishWorker.enqueue([row.relativePath], 'manual');
	}

	async function publishSelected() {
		if (selected.size === 0) return;
		if (!settings.baseUrl || !settings.apiKey) {
			error = 'Cannot publish: API settings are missing.';
			return;
		}
		error = null;
		publishWorker.enqueue(Array.from(selected), 'manual');
		showStatus(`Queued ${selected.size} file(s) for publish`);
		selected = new Set();
	}

	async function markSelectedForReview() {
		if (selected.size === 0) return;
		for (const path of selected) {
			await queueStore.updateFile(path, {
				syncState: 'in_review',
				status: 'in_review',
				localStatus: 'in_review',
				japaStatus: 'in_review'
			});
		}
		showStatus(`Marked ${selected.size} file(s) for review`);
		selected = new Set();
	}

	async function retryFile(row: FileState) {
		await queueStore.updateFile(row.relativePath, {
			syncState: 'modified',
			lastError: '',
			localStatus: 'modified'
		});
		await publishFile(row);
	}

	async function openPreview(row: FileState) {
		previewRow = row;
		previewLoading = true;
		try {
			const filePath = await join(settings.projectRoot, row.relativePath);
			const text = await readTextFile(filePath);
			const { body } = parseFrontmatter(text);
			previewHtml = markdownToHtml(body, { sanitize: true });
		} catch (e) {
			previewHtml = `<p class="text-danger">Could not load preview: ${escapeHtml(e instanceof Error ? e.message : String(e))}</p>`;
		} finally {
			previewLoading = false;
		}
	}

	function closePreview() {
		previewRow = null;
		previewHtml = '';
	}

	function toggleAutoSync() {
		const next = { ...settings, autoSync: !settings.autoSync };
		settings = next;
		void saveSettings(next);
		showStatus(next.autoSync ? 'Auto-sync enabled' : 'Auto-sync disabled');
	}

	function updateAutoSyncTypes(event: Event) {
		const select = event.currentTarget as HTMLSelectElement;
		const values = Array.from(select.selectedOptions).map((o) => o.value);
		const next = { ...settings, autoSyncContentTypes: values };
		settings = next;
		void saveSettings(next);
	}

	function openNewPost() {
		newPostType =
			settings.default_content_type ||
			newPostTypeOptions[0] ||
			contentTypeFilterOptions.find((t) => t !== 'all') ||
			'guide';
		newPostTitle = '';
		newPostOpen = true;
	}

	function closeNewPost() {
		newPostOpen = false;
		newPostCreating = false;
	}

	async function createNewPost() {
		if (!settings.projectRoot || !newPostTitle.trim()) return;
		const type = newPostType || 'guide';
		const slug = slugify(newPostTitle);
		if (!slug) {
			error = 'Could not generate a slug from the title.';
			return;
		}
		newPostCreating = true;
		error = null;
		try {
			const dir = await join(settings.projectRoot, 'jc-content', type);
			const filePath = await join(dir, `${slug}.md`);
			await mkdir(dir, { recursive: true });

			const typeMeta = contentTypes.get(type);
			const guideCategory =
				typeMeta?.guide_categories?.some((c) => c.value === settings.default_guide_category)
					? settings.default_guide_category
					: '';
			const baseStatus = settings.default_status || 'draft';
			const highStakes = isHighStakes({ guide_category: guideCategory, tags: settings.default_tags });
			const status =
				highStakes && (baseStatus === 'published' || baseStatus === 'scheduled')
					? 'in_review'
					: baseStatus;
			const visibility = settings.default_visibility || 'public';

			const frontmatter: string[] = [
				'---',
				`title: "${newPostTitle.replace(/"/g, '\\"')}"`,
				`slug: "${slug}"`,
				`content_type: "${type}"`
			];
			if (guideCategory) {
				frontmatter.push(`guide_category: "${guideCategory.replace(/"/g, '\\"')}"`);
			}
			if (settings.mode !== 'member' && settings.default_persona_id) {
				frontmatter.push(`persona_id: "${settings.default_persona_id}"`);
			}
			if (settings.mode !== 'member' && settings.default_voice_id) {
				frontmatter.push(`voice_id: "${settings.default_voice_id}"`);
			}
			if (settings.default_space_id) {
				frontmatter.push(`space_id: "${settings.default_space_id}"`);
			}
			frontmatter.push(`status: "${status}"`);
			frontmatter.push(`visibility: "${visibility}"`);
			if (settings.default_tags?.length) {
				frontmatter.push('tags:');
				for (const tag of settings.default_tags) {
					frontmatter.push(`  - "${tag.replace(/"/g, '\\"')}"`);
				}
			} else {
				frontmatter.push('tags: []');
			}
			frontmatter.push('---');
			frontmatter.push('');
			frontmatter.push(`# ${newPostTitle}`);
			const markdown = frontmatter.join('\n') + '\n';

			await writeTextFile(filePath, markdown);
			await scan();
			closeNewPost();
			showStatus(`Created ${type}/${slug}.md`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create new post.';
		} finally {
			newPostCreating = false;
		}
	}

	async function deleteFile(row: FileState) {
		if (!settings.projectRoot) return;
		if (!confirm(`Delete local file ${row.relativePath}? This cannot be undone.`)) return;
		error = null;
		try {
			const filePath = await join(settings.projectRoot, row.relativePath);
			await remove(filePath);
			await queueStore.deleteFile(row.relativePath);
			showStatus(`Deleted ${row.relativePath}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete local file.';
		}
	}

	async function revertFile(row: FileState) {
		if (!confirm(`Revert "${row.title}" to the last server state?`)) return;
		error = null;

		if (!row.contentId || !settings.baseUrl || !settings.apiKey) {
			// No server record to restore from; just reset queue bookkeeping.
			const targetState: FileState['syncState'] = 'new';
			await queueStore.updateFile(row.relativePath, {
				syncState: targetState,
				lastError: '',
				localStatus: targetState
			});
			showStatus(`Reverted "${row.title}" to ${targetState}`);
			return;
		}

		try {
			const client = createApiClient(settings.baseUrl);
			const res = await client.getMyContent(row.contentId);
			if (!res.ok || !res.data) {
				error = res.error?.message ?? 'Could not fetch the server content for this file.';
				return;
			}
			const detail = res.data;

			const filePath = await join(settings.projectRoot, row.relativePath);
			const raw = await readTextFile(filePath);
			const { frontmatter } = parseFrontmatter(raw);

			// Restore body and server-known metadata, while preserving local-only
			// frontmatter such as guide_category, tags, persona_id/voice_id.
			frontmatter.title = detail.title;
			frontmatter.slug = detail.slug;
			frontmatter.content_type = detail.content_type;
			frontmatter.status = detail.status;
			frontmatter.visibility = detail.visibility;
			if (detail.space_id) frontmatter.space_id = detail.space_id;
			if (detail.field_values) frontmatter.field_values = detail.field_values;
			if (detail.header_image_id) frontmatter.header_image_id = detail.header_image_id;
			if (detail.featured_image_id) frontmatter.featured_image_id = detail.featured_image_id;
			if (detail.meta_title) frontmatter.meta_title = detail.meta_title;
			if (detail.meta_description) frontmatter.meta_description = detail.meta_description;
			if (detail.canonical_url) frontmatter.canonical_url = detail.canonical_url;
			if (detail.og_image_url) frontmatter.og_image_url = detail.og_image_url;
			if (detail.seo_indexable !== undefined) frontmatter.seo_indexable = detail.seo_indexable;
			if (detail.publish_at) frontmatter.publish_at = detail.publish_at;

			const newRaw = buildMarkdown(frontmatter, detail.body);
			await writeTextFile(filePath, newRaw);
			const newHash = await hashFile(newRaw);

			const targetState: FileState['syncState'] =
				detail.status === 'published'
					? 'published'
					: detail.status === 'in_review'
						? 'in_review'
						: detail.status === 'archived'
							? 'archived'
							: 'modified';

			await queueStore.updateFile(row.relativePath, {
				title: detail.title,
				slug: detail.slug,
				contentType: detail.content_type,
				status: detail.status,
				visibility: detail.visibility,
				spaceId: detail.space_id,
				syncState: targetState,
				localStatus: targetState,
				lastHash: newHash,
				lastError: ''
			});
			showStatus(`Reverted "${row.title}" to the server ${detail.status} state.`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to revert file.';
		}
	}
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6 flex items-center justify-between gap-4">
		<div>
			<h1 class="text-2xl font-bold">Content Queue</h1>
			<p class="text-sm text-muted-foreground">Review, edit, preview, and publish {siteName()} content.</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
				onclick={openNewPost}
			>
				<Upload class="h-4 w-4" />
				<span>New Post</span>
			</button>
			<label class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-3 py-2 text-sm">
				<input
					type="checkbox"
					class="h-4 w-4 rounded border-border"
					checked={settings.autoSync}
					onchange={toggleAutoSync}
				/>
				<span>Auto-sync</span>
			</label>
			{#if settings.autoSync}
				<div class="flex flex-col gap-1">
					<label for="auto-sync-types" class="text-xs text-muted-foreground">Auto-sync types</label>
					<!-- Native multi-select list kept intentionally: AutocompleteSelect is single-select. -->
					<select
						id="auto-sync-types"
						multiple
						size={Math.min(3, contentTypes.size)}
						class="rounded-lg border border-border bg-surface px-3 py-1 text-xs focus:outline-none focus:ring-2 focus:ring-primary"
						onchange={updateAutoSyncTypes}
					>
						{#each [...contentTypes.values()].sort((a, b) => a.label.localeCompare(b.label)) as type}
							<option
								value={type.type}
								selected={settings.autoSyncContentTypes.includes(type.type)}
							>
								{type.label}
							</option>
						{/each}
					</select>
				</div>
			{/if}
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
		<div class="mb-4 rounded-lg border border-success/20 bg-success/10 p-3 text-sm text-success" role="status">
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
		<div class="mb-4 rounded-lg border border-danger/20 bg-danger/5 p-4 text-sm text-danger" role="alert">
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
	{:else if scanning && rows.length === 0}
		<div class="flex h-64 items-center justify-center gap-2 text-muted-foreground">
			<Loader2 class="h-5 w-5 animate-spin" />
			<span>Scanning project files...</span>
		</div>
	{:else if rows.length > 0}
		<div class="mb-4 flex flex-wrap items-center gap-2">
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
				disabled={scanning || $isPublishing}
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
				disabled={$isPublishing || selected.size === 0}
				onclick={publishSelected}
			>
				{#if $isPublishing && selected.size > 0}
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

		<div class="mb-4 flex flex-wrap items-center gap-2">
			<div class="relative">
				<Search class="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
				<input
					type="text"
					placeholder="Search title or slug"
					class="rounded-lg border border-border bg-surface py-2 pl-9 pr-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
					value={search}
					oninput={(event) => (search = (event.currentTarget as HTMLInputElement).value)}
				/>
			</div>
			<AutocompleteSelect
				class="w-44"
				value={filterStatus}
				onValueChange={(v) => (filterStatus = v as FileState['syncState'] | 'all')}
				options={[{ value: 'all', label: 'All statuses' }, ...stateOptions]}
			/>
			<AutocompleteSelect
				class="w-44"
				value={filterContentType}
				onValueChange={(v) => (filterContentType = v)}
				options={contentTypeFilterOptions.map((type) => ({
					value: type,
					label: type === 'all' ? 'All types' : contentTypeLabel(type)
				}))}
			/>
			<AutocompleteSelect
				class="w-44"
				value={filterGuideCategory}
				onValueChange={(v) => (filterGuideCategory = v)}
				options={guideCategoryFilterOptions.map((value) => ({
					value,
					label: value === 'all' ? 'All categories' : findGuideCategoryLabel(value)
				}))}
			/>
			<AutocompleteSelect
				class="w-44"
				value={filterSpace}
				onValueChange={(v) => (filterSpace = v)}
				options={spaceFilterOptions.map((id) => ({
					value: id,
					label: id === 'all' ? 'All spaces' : displayName(id, spaces)
				}))}
			/>
			{#if settings.mode !== 'member'}
				<AutocompleteSelect
					class="w-44"
					value={filterPersona}
					onValueChange={(v) => (filterPersona = v)}
					options={personaFilterOptions.map((id) => ({
						value: id,
						label: id === 'all' ? 'All personas' : displayName(id, personas)
					}))}
				/>
			{/if}
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
						<th
							class="cursor-pointer px-4 py-2 text-left font-medium"
							onclick={() => setSort('title')}
						>
							<div class="flex items-center gap-1">
								<span>Title</span>
								{#if sortBy === 'title'}
									{#if sortDir === 'asc'}
										<ArrowUp class="h-3 w-3" />
									{:else}
										<ArrowDown class="h-3 w-3" />
									{/if}
								{:else}
									<ArrowUpDown class="h-3 w-3" />
								{/if}
							</div>
						</th>
						<th class="px-4 py-2 text-left font-medium">Slug</th>
						<th class="px-4 py-2 text-left font-medium">Content type</th>
						<th class="px-4 py-2 text-left font-medium">Guide category</th>
						<th class="px-4 py-2 text-left font-medium">Space</th>
						{#if settings.mode !== 'member'}
							<th class="px-4 py-2 text-left font-medium">Persona</th>
						{/if}
						<th class="px-4 py-2 text-left font-medium">Local status</th>
						<th class="px-4 py-2 text-left font-medium">{siteName()} status</th>
						<th
							class="cursor-pointer px-4 py-2 text-left font-medium"
							onclick={() => setSort('modified')}
						>
							<div class="flex items-center gap-1">
								<span>Last modified</span>
								{#if sortBy === 'modified'}
									{#if sortDir === 'asc'}
										<ArrowUp class="h-3 w-3" />
									{:else}
										<ArrowDown class="h-3 w-3" />
									{/if}
								{:else}
									<ArrowUpDown class="h-3 w-3" />
								{/if}
							</div>
						</th>
						<th
							class="cursor-pointer px-4 py-2 text-left font-medium"
							onclick={() => setSort('published')}
						>
							<div class="flex items-center gap-1">
								<span>Last published</span>
								{#if sortBy === 'published'}
									{#if sortDir === 'asc'}
										<ArrowUp class="h-3 w-3" />
									{:else}
										<ArrowDown class="h-3 w-3" />
									{/if}
								{:else}
									<ArrowUpDown class="h-3 w-3" />
								{/if}
							</div>
						</th>
						<th class="px-4 py-2 text-left font-medium">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filteredRows as row (row.relativePath)}
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
							<td class="px-4 py-2 text-muted-foreground">{row.slug}</td>
							<td class="px-4 py-2">
								<span class="capitalize">{contentTypeLabel(row.contentType)}</span>
							</td>
							<td class="px-4 py-2 text-muted-foreground">
								{guideCategoryLabel(row.contentType, row.guideCategory) || '—'}
							</td>
							<td class="px-4 py-2 text-muted-foreground" title={row.spaceId}>
								{displayName(row.spaceId, spaces)}
							</td>
							{#if settings.mode !== 'member'}
								<td class="px-4 py-2 text-muted-foreground" title={row.personaId}>
									{displayName(row.personaId, personas)}
								</td>
							{/if}
							<td class="px-4 py-2">
								<AutocompleteSelect
									class="inline-block w-32 align-middle capitalize"
									inputClass="!px-2 !py-1 text-xs"
									value={row.syncState}
									onValueChange={(v) => handleStateChange(row.relativePath, v)}
									options={stateOptions}
								/>
								<span
									class="ml-2 inline-flex items-center rounded border px-2 py-0.5 text-xs capitalize {stateBadgeClasses(
										row.syncState
									)}"
								>
									{row.localStatus?.replace('_', ' ') ?? row.syncState.replace('_', ' ')}
								</span>
								{#if row.lastError}
									<p class="mt-1 max-w-xs truncate text-xs text-danger" title={row.lastError}>
										{row.lastError}
									</p>
								{/if}
							</td>
							<td class="px-4 py-2 text-muted-foreground">
								{row.japaStatus || row.status || '—'}
							</td>
							<td class="px-4 py-2 text-muted-foreground">{formatDate(row.lastModified)}</td>
							<td class="px-4 py-2 text-muted-foreground">{formatDate(row.lastPublished)}</td>
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
										onclick={() => openPreview(row)}
										aria-label="Preview"
										title="Preview"
									>
										<Eye class="h-4 w-4 text-muted-foreground" />
									</button>
									<button
										type="button"
										class="inline-flex items-center rounded p-1 hover:bg-accent disabled:opacity-60"
										disabled={$isPublishing}
										onclick={() => publishFile(row)}
										aria-label={row.syncState === 'failed' ? 'Retry publish' : 'Publish'}
										title={row.syncState === 'failed' ? 'Retry publish' : 'Publish'}
									>
										<Send class="h-4 w-4 text-primary" />
									</button>
									<button
										type="button"
										class="inline-flex items-center rounded p-1 hover:bg-accent"
										onclick={() => revertFile(row)}
										aria-label="Revert"
										title="Revert to last known state"
									>
										<RotateCcw class="h-4 w-4 text-muted-foreground" />
									</button>
									<button
										type="button"
										class="inline-flex items-center rounded p-1 hover:bg-accent"
										onclick={() => deleteFile(row)}
										aria-label="Delete local"
										title="Delete local"
									>
										<Trash2 class="h-4 w-4 text-danger" />
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
					disabled={scanning}
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

{#if previewRow}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		role="dialog"
		aria-modal="true"
		aria-label={`Preview ${previewRow.title}`}
		tabindex="-1"
		onclick={(event) => event.target === event.currentTarget && closePreview()}
		onkeydown={(event) => event.key === 'Escape' && closePreview()}
	>
		<div
			class="flex max-h-[80vh] w-full max-w-3xl flex-col rounded-xl border border-border bg-card p-6 shadow-lg"
			tabindex="-1"
		>
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-lg font-semibold">{previewRow.title}</h2>
				<button
					type="button"
					class="inline-flex items-center rounded p-1 hover:bg-accent"
					onclick={closePreview}
					aria-label="Close preview"
				>
					<X class="h-5 w-5" />
				</button>
			</div>
			<div class="prose prose-sm max-w-none flex-1 overflow-y-auto">
				{#if previewLoading}
					<div class="flex items-center gap-2 text-muted-foreground">
						<Loader2 class="h-4 w-4 animate-spin" />
						<span>Loading preview...</span>
					</div>
				{:else}
					{@html previewHtml}
				{/if}
			</div>
		</div>
	</div>
{/if}

{#if newPostOpen}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		role="dialog"
		aria-modal="true"
		aria-label="Create new post"
		tabindex="-1"
		onclick={(event) => event.target === event.currentTarget && closeNewPost()}
		onkeydown={(event) => event.key === 'Escape' && closeNewPost()}
	>
		<div
			class="flex w-full max-w-md flex-col rounded-xl border border-border bg-card p-6 shadow-lg"
			tabindex="-1"
		>
			<h2 class="mb-4 text-lg font-semibold">Create new post</h2>
			<div class="space-y-4">
				<label class="block text-sm font-medium">
					Title
					<input
						type="text"
						class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
						placeholder="e.g. Moving to Canada"
						value={newPostTitle}
						oninput={(event) => (newPostTitle = (event.currentTarget as HTMLInputElement).value)}
					/>
						<span class="mt-1 block text-xs text-muted-foreground">The title shown in the editor and used for the file name.</span>
				</label>
				<label class="block text-sm font-medium">
					Content type
					<AutocompleteSelect
						class="mt-1 w-full"
						value={newPostType}
						onValueChange={(v) => (newPostType = v)}
						options={newPostTypeOptions.length > 0
							? newPostTypeOptions.map((type) => ({ value: type, label: contentTypeLabel(type) }))
							: [{ value: 'guide', label: 'guide' }]}
					/>
				</label>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					type="button"
					class="rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent"
					onclick={closeNewPost}
				>
					Cancel
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover disabled:opacity-60"
					disabled={!newPostTitle.trim() || newPostCreating}
					onclick={createNewPost}
				>
					{#if newPostCreating}
						<Loader2 class="h-4 w-4 animate-spin" />
					{/if}
					<span>Create</span>
				</button>
			</div>
		</div>
	</div>
{/if}
