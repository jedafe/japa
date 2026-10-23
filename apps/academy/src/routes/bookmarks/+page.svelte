<script lang="ts">
	// Bookmarks — CSR-only. The user's saved items organized into folders.
	// The backend bookmark endpoint is polymorphic and bookmark folders are
	// global across the platform apps, so the list is NOT filtered by
	// entity_type — non-learning items render too and link back to the Main
	// app (entityUrl). Folders are managed client-side via
	// POST/PATCH/DELETE /me/bookmark-folders.
	// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { loginUrl, MAIN_ORIGIN } from '$lib/appOrigins';
	import { goto } from '$app/navigation';
	import { EmptyState } from '$lib/components/ui';
	import { Bookmark as BookmarkIcon, ExternalLink, FolderPlus, Trash2, Loader2, Folder, Pencil, Check, X, ChevronRight, ChevronDown } from 'lucide-svelte';
	import type { Bookmark, BookmarkFolder } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let bookmarks = $state<Bookmark[]>([]);
	// Total across ALL folders — only known after an unfiltered load; null
	// hides the "All" badge (e.g. when landing on a ?folder= deep link).
	let totalCount = $state<number | null>(null);
	let folders = $state<BookmarkFolder[]>([]);
	let loading = $state(true);
	let loadingMore = $state(false);
	let activeFolder = $state<string | null>(null);

	// Empty folders are hidden behind a toggle by default so a folder only
	// appears in the sidebar once it holds at least one bookmark; emptying
	// one slides it back under the toggle. A missing bookmark_count (e.g.
	// the POST create response, which returns the raw row) counts as empty.
	// The active folder always stays visible so a ?folder= deep link to an
	// empty folder still shows the selection.
	function isEmptyFolder(f: BookmarkFolder): boolean {
		return (f.bookmark_count ?? 0) === 0;
	}
	let showEmptyFolders = $state(false);
	const hiddenEmptyCount = $derived(
		folders.filter((f) => isEmptyFolder(f) && f.id !== activeFolder).length
	);
	const visibleFolders = $derived(
		folders.filter((f) => !isEmptyFolder(f) || f.id === activeFolder || showEmptyFolders)
	);

	// The list endpoint paginates (default 20, max 100) and returns no meta
	// total — page at PAGE_SIZE and show Load-more while a full page comes back.
	// nextOffset tracks rows fetched server-side so deletions don't skew it.
	const PAGE_SIZE = 100;
	let nextOffset = $state(0);
	let hasMore = $state(false);

	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let deletingFolderId = $state<string | null>(null);
	let folderError = $state('');
	let removingBookmarkId = $state<string | null>(null);

	// Rename state
	let editingFolderId = $state<string | null>(null);
	let editFolderName = $state('');
	let renamingFolder = $state(false);

	async function loadBookmarks(folderId: string | null, append = false) {
		if (append) loadingMore = true;
		else loading = true;
		const params = new URLSearchParams({
			page_size: String(PAGE_SIZE),
			page_offset: String(append ? nextOffset : 0)
		});
		if (folderId) params.set('folder_id', folderId);
		const r = await api.get<Bookmark[]>(`/me/bookmarks?${params.toString()}`);
		if (append) loadingMore = false;
		else loading = false;
		if (r.data) {
			bookmarks = append ? [...bookmarks, ...r.data] : r.data;
			nextOffset = (append ? nextOffset : 0) + r.data.length;
			hasMore = r.data.length === PAGE_SIZE;
			if (!folderId) totalCount = nextOffset;
		}
	}

	async function loadFolders() {
		const r = await api.get<BookmarkFolder[]>('/me/bookmark-folders');
		if (r.data) folders = r.data;
	}

	// Fetch the total bookmark count across all folders (capped at 10 pages =
	// 1000 rows) so the "All" badge is correct on a ?folder= deep link, where
	// the initial load only fetched the folder's rows.
	async function loadTotalCount() {
		let offset = 0;
		let total = 0;
		for (let i = 0; i < 10; i++) {
			const r = await api.get<Bookmark[]>(`/me/bookmarks?page_size=${PAGE_SIZE}&page_offset=${offset}`);
			if (!r.data) return;
			total += r.data.length;
			if (r.data.length < PAGE_SIZE) break;
			offset += PAGE_SIZE;
		}
		totalCount = total;
	}

	let urlInitDone = false;
	$effect(() => {
		if (urlInitDone) return;
		urlInitDone = true;
		// Deep-link support: ?folder=<id> selects the folder on first load.
		const initial = new URL(window.location.href).searchParams.get('folder');
		activeFolder = initial;
		void Promise.all([loadBookmarks(initial), loadFolders()]);
		if (initial) void loadTotalCount();
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
		goto(`${url.pathname}${url.search}`, { replaceState: true, keepFocus: true, noScroll: true });
	}

	function entityUrl(b: Bookmark): string {
		const idOrSlug = b.slug || b.entity_id;
		switch (b.entity_type) {
			case 'academy_track':
				return `/tracks/${idOrSlug}`;
			case 'blog_post':
			case 'content':
				return `${MAIN_ORIGIN}/blog/${idOrSlug}`;
			case 'forum_thread':
				return `${MAIN_ORIGIN}/forum/${idOrSlug}`;
			case 'forum_reply':
				return `${MAIN_ORIGIN}/forum/${idOrSlug}#reply-${b.entity_id}`;
			case 'space':
				return `${MAIN_ORIGIN}/spaces/${idOrSlug}`;
			case 'comment':
				return `${MAIN_ORIGIN}/content/${b.entity_id}`;
			default:
				return `${MAIN_ORIGIN}/content/${b.entity_id}`;
		}
	}

	function entityLabel(b: Bookmark): string {
		if (b.title) return b.title;
		return `${b.entity_type} #${b.entity_id.slice(0, 8)}`;
	}

	async function createFolder() {
		if (!data.user) {
			window.location.replace(loginUrl('/bookmarks'));
			return;
		}
		if (!newFolderName.trim() || creatingFolder) return;
		creatingFolder = true;
		folderError = '';
		const res = await api.post<BookmarkFolder>('/me/bookmark-folders', { name: newFolderName.trim() });
		if (res.data) {
			// The create response returns the raw row (no join) — normalize the
			// missing bookmark_count to 0 so the new folder is treated as
			// empty and hidden under the toggle until it has content.
			folders = [...folders, { ...res.data, bookmark_count: res.data.bookmark_count ?? 0 }];
			newFolderName = '';
		} else {
			folderError = res.error?.message ?? t('academy.bookmarks_folder_error', 'Failed to create folder');
		}
		creatingFolder = false;
	}

	async function deleteFolder(id: string) {
		if (!data.user) {
			window.location.replace(loginUrl('/bookmarks'));
			return;
		}
		if (deletingFolderId) return;
		deletingFolderId = id;
		folderError = '';
		const res = await api.delete(`/me/bookmark-folders/${id}`);
		if (!res.error) {
			folders = folders.filter((f) => f.id !== id);
			if (activeFolder === id) selectFolder(null);
		} else {
			folderError = res.error?.message ?? t('academy.bookmarks_folder_error', 'Failed to delete folder');
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
		if (!data.user) {
			window.location.replace(loginUrl('/bookmarks'));
			return;
		}
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
			folderError = res.error?.message ?? t('academy.bookmarks_folder_error', 'Failed to rename folder');
		}
	}

	async function removeBookmark(id: string) {
		if (!data.user) {
			window.location.replace(loginUrl('/bookmarks'));
			return;
		}
		if (removingBookmarkId) return;
		removingBookmarkId = id;
		folderError = '';
		const res = await api.delete(`/me/bookmarks/${id}`);
		if (!res.error) {
			const removed = bookmarks.find((b) => b.id === id);
			bookmarks = bookmarks.filter((b) => b.id !== id);
			// Every bookmark counts toward "All" — decrement regardless of the
			// folder currently being viewed.
			if (totalCount != null) totalCount = Math.max(0, totalCount - 1);
			// Decrement the owning folder's count so a folder emptied by this
			// removal drops under the empty-folders toggle without a reload.
			if (removed?.folder_id) {
				folders = folders.map((f) =>
					f.id === removed.folder_id
						? { ...f, bookmark_count: Math.max(0, (f.bookmark_count ?? 1) - 1) }
						: f
				);
			}
		} else {
			folderError = res.error?.message ?? t('academy.bookmarks_remove_error', 'Failed to remove bookmark');
		}
		removingBookmarkId = null;
	}
</script>

<svelte:head>
	<title>{t('academy.bookmarks')} · {t('app.name')}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<BookmarkIcon size={24} class="text-[var(--color-primary)]" />
			{t('academy.bookmarks')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{t('academy.bookmarks_subtitle')}</p>
	</header>

	<div class="grid gap-4 sm:grid-cols-[200px_1fr]">
		<!-- Folder sidebar -->
		<aside class="space-y-2">
			{#if folderError}
				<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-xs text-[var(--color-danger)]" role="alert">{folderError}</p>
			{/if}
			<button
				type="button"
				onclick={() => selectFolder(null)}
				class="flex w-full items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm transition-colors {activeFolder === null ? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-text)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				<BookmarkIcon size={14} />
				{t('academy.bookmarks_all', 'All')}
				{#if totalCount != null}
					<span class="ms-auto text-xs text-[var(--color-muted)]">{totalCount}</span>
				{/if}
			</button>

			{#each visibleFolders as folder (folder.id)}
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
							aria-label={t('academy.bookmarks_confirm_rename')}
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
							aria-label={t('academy.bookmarks_cancel_rename')}
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
								<span class="ms-auto text-xs text-[var(--color-muted)]">{folder.bookmark_count}</span>
							{/if}
						</button>
						{#if isEmptyFolder(folder)}
							<button
								type="button"
								onclick={() => startRename(folder)}
								class="shrink-0 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-primary)] group-hover:opacity-100"
								aria-label={t('academy.bookmarks_rename_folder')}
							>
								<Pencil size={12} />
							</button>
							<button
								type="button"
								onclick={() => deleteFolder(folder.id)}
								disabled={deletingFolderId === folder.id}
								class="shrink-0 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-danger)] group-hover:opacity-100 disabled:opacity-50"
								aria-label={t('academy.bookmarks_delete_folder')}
							>
								{#if deletingFolderId === folder.id}
									<Loader2 size={12} class="animate-spin" />
								{:else}
									<Trash2 size={12} />
								{/if}
							</button>
						{/if}
					{/if}
				</div>
			{/each}

			{#if hiddenEmptyCount > 0}
				<button
					type="button"
					onclick={() => (showEmptyFolders = !showEmptyFolders)}
					aria-expanded={showEmptyFolders}
					class="flex w-full items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-xs text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text-subtle)]"
				>
					{#if showEmptyFolders}
						<ChevronDown size={12} />
						{t('academy.bookmarks_hide_empty_folders')}
					{:else}
						<ChevronRight class="rtl:-scale-x-100" size={12} />
						{t('academy.bookmarks_show_empty_folders').replace('{count}', String(hiddenEmptyCount))}
					{/if}
				</button>
			{/if}

			<!-- Create folder -->
			<form class="flex items-center gap-1 pt-2" onsubmit={(e) => { e.preventDefault(); createFolder(); }}>
				<input
					type="text"
					bind:value={newFolderName}
					placeholder={t('academy.bookmarks_new_folder')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-xs text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
				/>
				<button
					type="submit"
					disabled={creatingFolder || !newFolderName.trim()}
					class="shrink-0 rounded p-1.5 text-[var(--color-muted)] hover:text-[var(--color-primary)] disabled:opacity-50"
					aria-label={t('academy.bookmarks_create_folder')}
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
				<EmptyState title={t('academy.bookmarks_empty_title')} description={t('academy.bookmarks_empty_body')} />
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
								<ExternalLink class="h-4 w-4 shrink-0 text-[var(--color-muted)] group-hover:text-[var(--color-primary)] rtl:-scale-x-100" />
							</a>
							<button
								type="button"
								onclick={() => removeBookmark(bm.id)}
								disabled={removingBookmarkId === bm.id}
								class="shrink-0 rounded p-1.5 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-danger)] group-hover:opacity-100 disabled:opacity-50"
								aria-label={t('academy.bookmarks_remove', 'Remove bookmark')}
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
				{#if hasMore}
					<button
						type="button"
						onclick={() => loadBookmarks(activeFolder, true)}
						disabled={loadingMore}
						class="mt-3 inline-flex w-full items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if loadingMore}
							<Loader2 size={14} class="animate-spin" />
						{/if}
						{t('academy.load_more')}
					</button>
				{/if}
			{/if}
		</div>
	</div>
</div>
