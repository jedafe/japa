<script lang="ts">
	// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (bookmarks)
	// Lists the user's bookmarks with folder management. The backend returns
	// bookmark records enriched with the entity's title + slug (joined
	// server-side per entity_type), so the page shows the bookmarked item's
	// title and links to its canonical URL.
	// Folders are managed client-side via POST/PATCH/DELETE /me/bookmark-folders.
	// Filtering is done server-side via ?folder_id=uuid (BL-002/BL-007).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Bookmark as BookmarkIcon, ExternalLink, FolderPlus, Trash2, Loader2, Folder, Pencil, Check, X } from 'lucide-svelte';
	import type { Bookmark, BookmarkFolder } from '$lib/types';
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';

	let { data } = $props();
	let bookmarks = $state<Bookmark[]>(untrack(() => data.bookmarks ?? []));
	let folders = $state<BookmarkFolder[]>(untrack(() => data.folders ?? []));
	let activeFolder = $state<string | null>(untrack(() => data.activeFolderId ?? null));

	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let deletingFolderId = $state<string | null>(null);
	let folderError = $state('');
	let removingBookmarkId = $state<string | null>(null);
	let loadingBookmarks = $state(false);

	// Rename state
	let editingFolderId = $state<string | null>(null);
	let editFolderName = $state('');
	let renamingFolder = $state(false);

	// Re-fetch bookmarks client-side with the folder_id filter (BL-002).
	// The server load() only runs on full navigation; switching folders from
	// the sidebar must call the API directly so the list reflects the filter
	// without a full page reload. Mirrors the learn app pattern.
	async function loadBookmarks(folderId: string | null) {
		loadingBookmarks = true;
		folderError = '';
		const path = folderId
			? `/me/bookmarks?folder_id=${encodeURIComponent(folderId)}`
			: '/me/bookmarks';
		const res = await api.get<Bookmark[]>(path);
		loadingBookmarks = false;
		if (res.data) {
			bookmarks = res.data;
		} else if (res.error) {
			folderError = res.error.message ?? t('bookmarks.folder_error', 'Failed to load bookmarks');
		}
	}

	function selectFolder(id: string | null) {
		activeFolder = id;
		// Re-fetch the filtered list immediately so the UI updates.
		loadBookmarks(id);
		// Sync the URL via goto so a reload preserves the selected folder.
		// goto to the same path with a changed query re-runs load(), but the
		// local bookmarks state is already updated by loadBookmarks above.
		const url = new URL(window.location.href);
		if (id) {
			url.searchParams.set('folder', id);
		} else {
			url.searchParams.delete('folder');
		}
		goto(url.pathname + url.search, { keepFocus: true, noScroll: true, replaceState: false });
	}

	function entityUrl(b: Bookmark): string {
		// Prefer the slug for content/blog posts (canonical, SEO-friendly URL).
		const idOrSlug = b.slug || b.entity_id;
		switch (b.entity_type) {
			case 'content':
			case 'blog_post':
				return `/blog/${idOrSlug}`;
			case 'forum_thread':
				return `/forum/${idOrSlug}`;
			case 'space':
				return `/spaces/${idOrSlug}`;
			case 'comment':
				return `/content/${b.entity_id}`;
			default:
				return `/content/${b.entity_id}`;
		}
	}

	function entityLabel(b: Bookmark): string {
		// Show the bookmarked item's title when available; fall back to the
		// localized type label + short id so the row is never blank.
		if (b.title) return b.title;
		const key = `bookmarks.type.${b.entity_type}`;
		return `${t(key, b.entity_type)} #${b.entity_id.slice(0, 8)}`;
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
			folderError = res.error?.message ?? t('bookmarks.folder_error', 'Failed to create folder');
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
			folderError = res.error?.message ?? t('bookmarks.folder_error', 'Failed to delete folder');
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
			folderError = res.error?.message ?? t('bookmarks.folder_error', 'Failed to rename folder');
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
			folderError = res.error?.message ?? t('bookmarks.remove_error', 'Failed to remove bookmark');
		}
		removingBookmarkId = null;
	}
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.bookmarks')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('bookmarks.subtitle', 'Organize your saved items into folders')}</p>
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
				{t('bookmarks.all', 'All')}
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
							aria-label={t('bookmarks.confirm_rename', 'Confirm rename')}
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
							aria-label={t('bookmarks.cancel_rename', 'Cancel rename')}
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
							aria-label={t('bookmarks.rename_folder', 'Rename folder')}
						>
							<Pencil size={12} />
						</button>
						<button
							type="button"
							onclick={() => deleteFolder(folder.id)}
							disabled={deletingFolderId === folder.id}
							class="shrink-0 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-danger)] group-hover:opacity-100 disabled:opacity-50"
							aria-label={t('bookmarks.delete_folder', 'Delete folder')}
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
					placeholder={t('bookmarks.new_folder', 'New folder…')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-xs text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
				/>
				<button
					type="submit"
					disabled={creatingFolder || !newFolderName.trim()}
					class="shrink-0 rounded p-1.5 text-[var(--color-muted)] hover:text-[var(--color-primary)] disabled:opacity-50"
					aria-label={t('bookmarks.create_folder', 'Create folder')}
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
			{#if loadingBookmarks}
				<div class="space-y-2">
					{#each Array(3) as _}
						<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="h-5 w-3/4 animate-pulse rounded bg-[var(--color-surface-subtle)]"></div>
							<div class="mt-2 h-4 w-1/3 animate-pulse rounded bg-[var(--color-surface-subtle)]"></div>
						</div>
					{/each}
				</div>
			{:else if bookmarks.length === 0}
				<EmptyState
					icon={BookmarkIcon}
					title={t('bookmarks.empty_title', 'No bookmarks yet')}
					body={t('bookmarks.empty_body', 'Save items to find them here later.')}
				/>
			{:else}
				<ul class="space-y-2">
					{#each bookmarks as b (b.id)}
						<li class="group flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40">
							<a href={entityUrl(b)} class="flex min-w-0 flex-1 items-center justify-between gap-3">
								<div class="min-w-0 flex-1">
									<p class="text-sm font-medium text-[var(--color-text)]">
										{entityLabel(b)}
									</p>
									<p class="mt-0.5 truncate text-xs text-[var(--color-muted)]">
										{#if b.folder}{b.folder} · {/if}
										{new Date(b.created_at).toLocaleDateString()}
									</p>
								</div>
								<ExternalLink class="h-4 w-4 shrink-0 text-[var(--color-muted)] group-hover:text-[var(--color-primary)]" />
							</a>
							<button
								type="button"
								onclick={() => removeBookmark(b.id)}
								disabled={removingBookmarkId === b.id}
								class="shrink-0 rounded p-1.5 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-danger)] group-hover:opacity-100 disabled:opacity-50"
								aria-label={t('bookmarks.remove', 'Remove bookmark')}
							>
								{#if removingBookmarkId === b.id}
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
</section>
