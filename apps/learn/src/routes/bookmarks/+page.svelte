<script lang="ts">
	// Bookmarks — CSR-only. Bookmarked learning tracks and lessons for the
	// learner, organized into folders.
	// The backend bookmark endpoint is polymorphic; this page fetches
	// /me/bookmarks?entity_type=learning_track,content and filters to
	// learning-related entities. Folders are managed client-side via
	// POST/PATCH/DELETE /me/bookmark-folders.
	// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
	import { Bookmark as BookmarkIcon, ExternalLink, FolderPlus, Trash2, Loader2, Folder, Pencil, Check, X } from 'lucide-svelte';
	import type { Bookmark, BookmarkFolder } from '$lib/types';

	// Learning-related entity types shown on this page.
	const LEARNING_ENTITY_TYPES = 'learning_track,content';

	let bookmarks = $state<Bookmark[]>([]);
	let folders = $state<BookmarkFolder[]>([]);
	let loading = $state(true);
	let activeFolder = $state<string | null>(null);

	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let deletingFolderId = $state<string | null>(null);
	let folderError = $state('');
	let removingBookmarkId = $state<string | null>(null);

	// Rename state
	let editingFolderId = $state<string | null>(null);
	let editFolderName = $state('');
	let renamingFolder = $state(false);

	async function loadBookmarks(folderId: string | null) {
		loading = true;
		const params = new URLSearchParams({ entity_type: LEARNING_ENTITY_TYPES });
		if (folderId) params.set('folder_id', folderId);
		const r = await api.get<Bookmark[]>(`/me/bookmarks?${params.toString()}`);
		loading = false;
		if (r.data) bookmarks = r.data;
	}

	async function loadFolders() {
		const r = await api.get<BookmarkFolder[]>('/me/bookmark-folders');
		if (r.data) folders = r.data;
	}

	$effect(() => {
		(async () => {
			await Promise.all([loadBookmarks(null), loadFolders()]);
		})();
	});

	function selectFolder(id: string | null) {
		activeFolder = id;
		loadBookmarks(id);
		const url = new URL(window.location.href);
		if (id) {
			url.searchParams.set('folder', id);
		} else {
			url.searchParams.delete('folder');
		}
		window.history.pushState({}, '', url);
	}

	function entityUrl(b: Bookmark): string {
		const idOrSlug = b.slug || b.entity_id;
		switch (b.entity_type) {
			case 'learning_track':
				return `/tracks/${idOrSlug}`;
			case 'content':
			case 'blog_post':
				return `/tracks/${idOrSlug}`;
			default:
				return `/tracks/${b.entity_id}`;
		}
	}

	function entityLabel(b: Bookmark): string {
		if (b.title) return b.title;
		return `${b.entity_type} #${b.entity_id.slice(0, 8)}`;
	}

	async function createFolder() {
		if (!newFolderName.trim() || creatingFolder) return;
		creatingFolder = true;
		folderError = '';
		const res = await api.post<BookmarkFolder>('/me/bookmark-folders', { name: newFolderName.trim() });
		if (res.data) {
			folders = [...folders, res.data];
			newFolderName = '';
		} else {
			folderError = res.error?.message ?? t('learn.bookmarks_folder_error', 'Failed to create folder');
		}
		creatingFolder = false;
	}

	async function deleteFolder(id: string) {
		if (deletingFolderId) return;
		deletingFolderId = id;
		folderError = '';
		const res = await api.delete(`/me/bookmark-folders/${id}`);
		if (!res.error) {
			folders = folders.filter((f) => f.id !== id);
			if (activeFolder === id) selectFolder(null);
		} else {
			folderError = res.error?.message ?? t('learn.bookmarks_folder_error', 'Failed to delete folder');
		}
		deletingFolderId = null;
	}

	function startRename(folder: BookmarkFolder) {
		editingFolderId = folder.id;
		editFolderName = folder.name;
		folderError = '';
	}

	function cancelRename() {
		editingFolderId = null;
		editFolderName = '';
	}

	async function confirmRename(id: string) {
		if (!editFolderName.trim() || renamingFolder) return;
		renamingFolder = true;
		folderError = '';
		const res = await api.patch<BookmarkFolder>(`/me/bookmark-folders/${id}`, { name: editFolderName.trim() });
		if (res.data) {
			folders = folders.map((f) => (f.id === id ? res.data! : f));
			renamingFolder = false;
			editingFolderId = null;
			editFolderName = '';
		} else {
			renamingFolder = false;
			folderError = res.error?.message ?? t('learn.bookmarks_folder_error', 'Failed to rename folder');
		}
	}

	async function removeBookmark(id: string) {
		if (removingBookmarkId) return;
		removingBookmarkId = id;
		folderError = '';
		const res = await api.delete(`/me/bookmarks/${id}`);
		if (!res.error) {
			bookmarks = bookmarks.filter((b) => b.id !== id);
		} else {
			folderError = res.error?.message ?? t('learn.bookmarks_remove_error', 'Failed to remove bookmark');
		}
		removingBookmarkId = null;
	}
</script>

<svelte:head>
	<title>{t('learn.bookmarks')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<BookmarkIcon size={24} class="text-[var(--color-primary)]" />
			{t('learn.bookmarks')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{t('learn.bookmarks_subtitle')}</p>
	</header>

	<div class="grid gap-4 sm:grid-cols-[200px_1fr]">
		<!-- Folder sidebar -->
		<aside class="space-y-2">
			{#if folderError}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-xs text-red-500" role="alert">{folderError}</p>
			{/if}
			<button
				type="button"
				onclick={() => selectFolder(null)}
				class="flex w-full items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm transition-colors {activeFolder === null ? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-text)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				<BookmarkIcon size={14} />
				{t('learn.bookmarks_all', 'All')}
				<span class="ml-auto text-xs text-[var(--color-muted)]">{bookmarks.length}</span>
			</button>

			{#each folders as folder (folder.id)}
				<div class="group flex items-center gap-1">
					{#if editingFolderId === folder.id}
						<input
							type="text"
							bind:value={editFolderName}
							onkeydown={(e) => { if (e.key === 'Enter') confirmRename(folder.id); if (e.key === 'Escape') cancelRename(); }}
							class="w-full rounded-[var(--radius)] border border-[var(--color-primary)] bg-[var(--color-surface)] px-2 py-1.5 text-xs text-[var(--color-text)] outline-none"
						/>
						<button
							type="button"
							onclick={() => confirmRename(folder.id)}
							disabled={renamingFolder || !editFolderName.trim()}
							class="shrink-0 rounded p-1 text-[var(--color-primary)] disabled:opacity-50"
							aria-label={t('learn.bookmarks_confirm_rename')}
						>
							{#if renamingFolder}
								<Loader2 size={12} class="animate-spin" />
							{:else}
								<Check size={12} />
							{/if}
						</button>
						<button
							type="button"
							onclick={cancelRename}
							class="shrink-0 rounded p-1 text-[var(--color-muted)]"
							aria-label={t('learn.bookmarks_cancel_rename')}
						>
							<X size={12} />
						</button>
					{:else}
						<button
							type="button"
							onclick={() => selectFolder(folder.id)}
							class="flex flex-1 items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm transition-colors {activeFolder === folder.id ? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-text)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							<Folder size={14} />
							<span class="truncate">{folder.name}</span>
							{#if folder.bookmark_count != null}
								<span class="ml-auto text-xs text-[var(--color-muted)]">{folder.bookmark_count}</span>
							{/if}
						</button>
						<button
							type="button"
							onclick={() => startRename(folder)}
							class="shrink-0 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-primary)] group-hover:opacity-100"
							aria-label={t('learn.bookmarks_rename_folder')}
						>
							<Pencil size={12} />
						</button>
						<button
							type="button"
							onclick={() => deleteFolder(folder.id)}
							disabled={deletingFolderId === folder.id}
							class="shrink-0 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-danger)] group-hover:opacity-100 disabled:opacity-50"
							aria-label={t('learn.bookmarks_delete_folder')}
						>
							{#if deletingFolderId === folder.id}
								<Loader2 size={12} class="animate-spin" />
							{:else}
								<Trash2 size={12} />
							{/if}
						</button>
					{/if}
				</div>
			{/each}

			<!-- Create folder -->
			<form class="flex items-center gap-1 pt-2" onsubmit={(e) => { e.preventDefault(); createFolder(); }}>
				<input
					type="text"
					bind:value={newFolderName}
					placeholder={t('learn.bookmarks_new_folder')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-xs text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
				/>
				<button
					type="submit"
					disabled={creatingFolder || !newFolderName.trim()}
					class="shrink-0 rounded p-1.5 text-[var(--color-muted)] hover:text-[var(--color-primary)] disabled:opacity-50"
					aria-label={t('learn.bookmarks_create_folder')}
				>
					{#if creatingFolder}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						<FolderPlus size={14} />
					{/if}
				</button>
			</form>
		</aside>

		<!-- Bookmark list -->
		<div>
			{#if loading}
				<div class="space-y-3">
					{#each Array(3) as _}
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="h-5 w-3/4 animate-pulse rounded bg-[var(--color-surface-subtle)]"></div>
							<div class="mt-2 h-4 w-1/3 animate-pulse rounded bg-[var(--color-surface-subtle)]"></div>
						</div>
					{/each}
				</div>
			{:else if bookmarks.length === 0}
				<EmptyState title={t('learn.bookmarks_empty_title')} description={t('learn.bookmarks_empty_body')} />
			{:else}
				<ul class="space-y-3">
					{#each bookmarks as bm (bm.id)}
						<li class="group flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<a href={entityUrl(bm)} class="flex min-w-0 flex-1 items-center justify-between gap-3 hover:text-[var(--color-primary)]">
								<div class="min-w-0 flex-1">
									<p class="font-medium text-[var(--color-text)]">{entityLabel(bm)}</p>
									<p class="mt-1 text-xs text-[var(--color-muted)]">
										{bm.entity_type} · {new Date(bm.created_at).toLocaleDateString()}
									</p>
								</div>
								<ExternalLink class="h-4 w-4 shrink-0 text-[var(--color-muted)] group-hover:text-[var(--color-primary)]" />
							</a>
							<button
								type="button"
								onclick={() => removeBookmark(bm.id)}
								disabled={removingBookmarkId === bm.id}
								class="shrink-0 rounded p-1.5 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-danger)] group-hover:opacity-100 disabled:opacity-50"
								aria-label={t('learn.bookmarks_remove', 'Remove bookmark')}
							>
								{#if removingBookmarkId === bm.id}
									<Loader2 size={14} class="animate-spin" />
								{:else}
									<Trash2 size={14} />
								{/if}
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</div>
</div>
