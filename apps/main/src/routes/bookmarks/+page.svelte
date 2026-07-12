<script lang="ts">
	import { onMount } from 'svelte';
	import { Folder, Plus, Trash2, Edit2, Loader2, Bookmark } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type BookmarkFolder = {
		id: string;
		name: string;
		description?: string;
		item_count: number;
		created_at: string;
	};

	type BookmarkedItem = {
		id: string;
		content_id: string;
		title: string;
		content_type: string;
		folder_id?: string;
		created_at: string;
	};

	let folders = $state<BookmarkFolder[]>([]);
	let bookmarks = $state<BookmarkedItem[]>([]);
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state<'folders' | 'all'>('folders');
	let selectedFolder = $state<string | null>(null);

	// Create folder
	let showCreateModal = $state(false);
	let newFolder = $state({ name: '', description: '' });
	let creating = $state(false);

	// Edit folder
	let editingFolder = $state<BookmarkFolder | null>(null);

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api.getBookmarkFolders();
			folders = result;
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function loadBookmarks(folderId?: string) {
		try {
			bookmarks = await api.getBookmarks(folderId);
		} catch {
			// silently fail
		}
	}

	function selectFolder(id: string | null) {
		selectedFolder = id;
		activeTab = id ? 'all' : 'folders';
		if (id) loadBookmarks(id);
	}

	async function createFolder() {
		if (!newFolder.name) return;
		creating = true;
		try {
			await api.createBookmarkFolder(newFolder);
			showCreateModal = false;
			newFolder = { name: '', description: '' };
			load();
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	function openEdit(folder: BookmarkFolder) {
		editingFolder = { ...folder };
		newFolder = { name: folder.name, description: folder.description || '' };
		showCreateModal = true;
	}

	async function updateFolder() {
		if (!editingFolder || !newFolder.name) return;
		creating = true;
		try {
			await api.updateBookmarkFolder(editingFolder.id, newFolder);
			showCreateModal = false;
			editingFolder = null;
			newFolder = { name: '', description: '' };
			load();
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	async function deleteFolder(id: string) {
		if (!confirm(t('bookmarks.confirm_delete_folder'))) return;
		try {
			await api.deleteBookmarkFolder(id);
			folders = folders.filter((f) => f.id !== id);
			if (selectedFolder === id) {
				selectedFolder = null;
				activeTab = 'folders';
			}
		} catch {
			error = t('common.error');
		}
	}

	function closeModal() {
		showCreateModal = false;
		editingFolder = null;
		newFolder = { name: '', description: '' };
	}
</script>

<svelte:head>
	<title>{t('bookmarks.title')} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{t('bookmarks.title')}</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('bookmarks.description')}
		</p>
	</header>

	{#if error}
		<div class="mb-4 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	<!-- Tabs -->
	<div class="mb-6 flex items-center justify-between">
		<div class="flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
			<button
				class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
				style={activeTab === 'folders' ? 'background-color: var(--color-primary); color: white' : ''}
				onclick={() => { activeTab = 'folders'; selectedFolder = null; }}
			>
				{t('bookmarks.folders')}
			</button>
			{#if selectedFolder}
				<button
					class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
					style="background-color: var(--color-primary); color: white"
				>
					{folders.find((f) => f.id === selectedFolder)?.name ?? t('bookmarks.all_items')}
				</button>
			{/if}
		</div>
		<button
			class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={() => { editingFolder = null; newFolder = { name: '', description: '' }; showCreateModal = true; }}
		>
			<Plus class="h-4 w-4" />
			{t('bookmarks.new_folder')}
		</button>
	</div>

	{#if loading}
		<div class="space-y-3">
			{#each [1, 2, 3] as _}
				<div class="h-20 animate-pulse rounded-xl" style="background-color: var(--color-background)"></div>
			{/each}
		</div>
	{:else if activeTab === 'folders'}
		{#if folders.length > 0}
			<div class="space-y-3">
				{#each folders as folder (folder.id)}
					<div
						role="button"
						tabindex="0"
						onclick={() => selectFolder(folder.id)}
						onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') selectFolder(folder.id); }}
						class="flex w-full items-center justify-between rounded-xl border p-4 text-left transition-colors hover:bg-black/5 dark:hover:bg-white/5 cursor-pointer"
						style="border-color: var(--color-border); background: var(--color-surface);"
					>
						<div class="flex items-center gap-3">
							<Folder class="h-8 w-8" style="color: var(--color-primary)" />
							<div>
								<p class="font-medium" style="color: var(--color-foreground)">{folder.name}</p>
								{#if folder.description}
									<p class="text-xs" style="color: var(--color-muted)">{folder.description}</p>
								{/if}
								<p class="text-xs" style="color: var(--color-muted)">
									{folder.item_count} {folder.item_count === 1 ? t('bookmarks.item') : t('bookmarks.items')}
								</p>
							</div>
						</div>
						<div class="flex items-center gap-2">
							<button
								onclick={(e) => { e.stopPropagation(); openEdit(folder); }}
								class="rounded p-1.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
								title={t('common.edit')}
							>
								<Edit2 class="h-4 w-4" style="color: var(--color-muted)" />
							</button>
							<button
								onclick={(e) => { e.stopPropagation(); deleteFolder(folder.id); }}
								class="rounded p-1.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
								title={t('common.delete')}
							>
								<Trash2 class="h-4 w-4" style="color: var(--color-danger)" />
							</button>
						</div>
					</div>
				{/each}
			</div>
		{:else}
			<div class="py-12 text-center">
				<Folder class="mx-auto h-12 w-12" style="color: var(--color-border)" />
				<p class="mt-4 text-sm" style="color: var(--color-muted)">
					{t('bookmarks.no_folders')}
				</p>
			</div>
		{/if}
	{:else}
		{#if bookmarks.length > 0}
			<div class="space-y-2">
				{#each bookmarks as bookmark (bookmark.id)}
					<a
						href={`/content/${bookmark.content_id}`}
						class="flex items-center gap-3 rounded-xl border p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
						style="border-color: var(--color-border); background: var(--color-surface);"
					>
						<Bookmark class="h-5 w-5 shrink-0" style="color: var(--color-primary)" />
						<div class="flex-1 min-w-0">
							<p class="font-medium truncate" style="color: var(--color-foreground)">{bookmark.title}</p>
							<p class="text-xs" style="color: var(--color-muted)">{bookmark.content_type}</p>
						</div>
						<p class="text-xs shrink-0" style="color: var(--color-muted)">
							{new Date(bookmark.created_at).toLocaleDateString()}
						</p>
					</a>
				{/each}
			</div>
		{:else}
			<div class="py-12 text-center">
				<Bookmark class="mx-auto h-12 w-12" style="color: var(--color-border)" />
				<p class="mt-4 text-sm" style="color: var(--color-muted)">
					{t('bookmarks.no_items')}
				</p>
			</div>
		{/if}
	{/if}
</main>

{#if showCreateModal}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="w-full max-w-md rounded-xl border p-6 shadow-xl" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">
				{editingFolder ? t('bookmarks.edit_folder') : t('bookmarks.new_folder')}
			</h2>
			<div class="space-y-4">
				<div>
					<label for="folder-name" class="mb-1 block text-sm font-medium">{t('bookmarks.name')}</label>
					<input
						id="folder-name"
						type="text"
						class="w-full rounded-lg border py-2.5 px-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						bind:value={newFolder.name}
					/>
				</div>
				<div>
					<label for="folder-desc" class="mb-1 block text-sm font-medium">{t('bookmarks.description')}</label>
					<textarea
						id="folder-desc"
						class="w-full rounded-lg border py-2.5 px-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						rows="2"
						bind:value={newFolder.description}
					></textarea>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="border-color: var(--color-border); color: var(--color-foreground)"
					onclick={closeModal}
				>
					{t('common.cancel')}
				</button>
				<button
					class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={creating || !newFolder.name}
					onclick={editingFolder ? updateFolder : createFolder}
				>
					{#if creating}
						<Loader2 class="h-4 w-4 animate-spin" />
					{/if}
					{editingFolder ? t('common.save') : t('common.create')}
				</button>
			</div>
		</div>
	</div>
{/if}
