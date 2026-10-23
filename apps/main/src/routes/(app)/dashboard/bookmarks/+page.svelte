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
	import { academyUrl, docsUrl } from '$lib/appOrigins';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Bookmark as BookmarkIcon, ExternalLink, FolderPlus, Trash2, Loader2, Folder, Pencil, Check, X, ChevronRight, ChevronDown } from 'lucide-svelte';
	import type { Bookmark, BookmarkFolder } from '$lib/types';
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';

	let { data } = $props();
	let bookmarks = $state<Bookmark[]>(untrack(() => data.bookmarks ?? []));
	let folders = $state<BookmarkFolder[]>(untrack(() => data.folders ?? []));
	let activeFolder = $state<string | null>(untrack(() => data.activeFolderId ?? null));
	let activeType = $state<string>(untrack(() => data.activeType ?? ''));

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

	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let deletingFolderId = $state<string | null>(null);
	let folderError = $state('');
	let removingBookmarkId = $state<string | null>(null);
	let loadingBookmarks = $state(false);
	let loadingMore = $state(false);

	// The list endpoint paginates (default 20, max 100) and returns no meta
	// total — SSR fetches the first PAGE_SIZE and Load-more continues via
	// page_offset while a full page comes back. nextOffset tracks rows
	// fetched server-side so deletions don't skew it.
	const PAGE_SIZE = 100;
	let nextOffset = $state(0);
	let hasMore = $state(false);
	// Total across ALL folders — shown on the "All" badge. The list endpoint
	// returns no meta total, so on a ?folder= deep link the count is fetched
	// in the background by paging the list (loadTotalCount). countType tracks
	// which entity_type the cached total describes so a type change recounts.
	let totalCount = $state<number | null>(null);
	let countType = $state<string>('__init__');

	// Re-sync list state whenever the server load re-runs (?folder= navigation,
	// back/forward history, invalidation). Client-appended pages and optimistic
	// removals live in `bookmarks`; data.bookmarks is always the first server
	// page for the active folder. The "All" badge uses totalCount — only
	// seed it when the unfiltered list fit in a single page (exact count);
	// otherwise loadTotalCount pages the full list for a real number.
	$effect(() => {
		bookmarks = data.bookmarks ?? [];
		folders = data.folders ?? [];
		activeFolder = data.activeFolderId ?? null;
		const type = data.activeType ?? '';
		activeType = type;
		if (type !== countType) {
			countType = type;
			totalCount = null;
		}
		nextOffset = data.bookmarks?.length ?? 0;
		hasMore = (data.bookmarks?.length ?? 0) === PAGE_SIZE;
		if (!data.activeFolderId && !hasMore) totalCount = nextOffset;
		loadingBookmarks = false;
	});

	// Fetch the total bookmark count across all folders (capped at 10 pages =
	// 1000 rows) so the "All" badge is correct even when the SSR load was
	// folder-scoped. Honours the active entity_type filter so the badge
	// counts the same set the list would show. Runs client-side only, only
	// when the total is unknown.
	async function loadTotalCount() {
		let offset = 0;
		let total = 0;
		for (let i = 0; i < 10; i++) {
			const params = new URLSearchParams({ page_size: String(PAGE_SIZE), page_offset: String(offset) });
			if (activeType) params.set('entity_type', activeType);
			const r = await api.get<Bookmark[]>(`/me/bookmarks?${params.toString()}`);
			if (!r.data) return;
			total += r.data.length;
			if (r.data.length < PAGE_SIZE) break;
			offset += PAGE_SIZE;
		}
		totalCount = total;
	}

	$effect(() => {
		if (totalCount == null) loadTotalCount();
	});

	// Rename state
	let editingFolderId = $state<string | null>(null);
	let editFolderName = $state('');
	let renamingFolder = $state(false);

	// Re-fetch bookmarks client-side with the folder_id filter (BL-002).
	// The server load() only runs on full navigation; switching folders from
	// the sidebar must call the API directly so the list reflects the filter
	// without a full page reload. Mirrors the academy app pattern.
	async function loadBookmarks(folderId: string | null, append = false) {
		if (append) loadingMore = true;
		else loadingBookmarks = true;
		folderError = '';
		const params = new URLSearchParams({
			page_size: String(PAGE_SIZE),
			page_offset: String(append ? nextOffset : 0)
		});
		if (folderId) params.set('folder_id', folderId);
		if (activeType) params.set('entity_type', activeType);
		const res = await api.get<Bookmark[]>(`/me/bookmarks?${params.toString()}`);
		if (append) loadingMore = false;
		else loadingBookmarks = false;
		if (res.data) {
			bookmarks = append ? [...bookmarks, ...res.data] : res.data;
			nextOffset = (append ? nextOffset : 0) + res.data.length;
			hasMore = res.data.length === PAGE_SIZE;
			if (!folderId) totalCount = nextOffset;
		} else if (res.error) {
			folderError = res.error.message ?? t('bookmarks.folder_error', 'Failed to load bookmarks');
		}
	}

	function selectFolder(id: string | null) {
		activeFolder = id;
		loadingBookmarks = true;
		// Changing ?folder re-runs the server load, which returns the
		// folder-filtered first page — the $effect above syncs it. A direct
		// client fetch here would double-fetch the same endpoint.
		const url = new URL(window.location.href);
		if (id) {
			url.searchParams.set('folder', id);
		} else {
			url.searchParams.delete('folder');
		}
		goto(url.pathname + url.search, { keepFocus: true, noScroll: true, replaceState: false });
	}

	function selectType(type: string) {
		activeType = type;
		loadingBookmarks = true;
		const url = new URL(window.location.href);
		if (type) {
			url.searchParams.set('type', type);
		} else {
			url.searchParams.delete('type');
		}
		goto(url.pathname + url.search, { keepFocus: true, noScroll: true });
	}

	// academyUrl/docsUrl throw when the cross-app origin env var is unset — a
	// throwing href expression would crash the render, so guard them.
	function safeAcademyUrl(path: string): string {
		try { return academyUrl(path); } catch { return '#'; }
	}
	function safeDocsUrl(path: string): string {
		try { return docsUrl(path); } catch { return '#'; }
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
			case 'forum_reply':
				return `/forum/${idOrSlug}#reply-${b.entity_id}`;
			case 'space':
				return `/spaces/${idOrSlug}`;
			case 'tag':
				return `/tags/${idOrSlug}`;
			case 'comment':
				return `/content/${b.entity_id}`;
			// Doc pages live on the Docs app under /docs/{audience}/{slug} —
			// the bookmark row carries the page's audience slug.
			case 'doc_page':
				return safeDocsUrl(`/docs/${b.audience ?? 'users'}/${idOrSlug}`);
			// Academy tracks live on the Academy app (:3003 in dev) — a relative
			// /tracks/… link would 404 here. ADR-060 cross-app deep links.
			case 'academy_track':
				return safeAcademyUrl(`/tracks/${idOrSlug}`);
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
			// The create response returns the raw row (no join) — normalize the
			// missing bookmark_count to 0 so the new folder is treated as
			// empty and hidden under the toggle until it has content.
			folders = [...folders, { ...res.data, bookmark_count: res.data.bookmark_count ?? 0 }];
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
								<span class="ms-auto text-xs text-[var(--color-muted)]">{folder.bookmark_count}</span>
							{/if}
						</button>
						<!-- Rename/delete apply to all folders: DeleteBookmarkFolder
							has ON DELETE SET NULL on bookmarks.folder_id — deleting a
							non-empty folder unfiles its bookmarks rather than failing. -->
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

			{#if hiddenEmptyCount > 0}
				<button
					type="button"
					onclick={() => (showEmptyFolders = !showEmptyFolders)}
					aria-expanded={showEmptyFolders}
					class="flex w-full items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-xs text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text-subtle)]"
				>
					{#if showEmptyFolders}
						<ChevronDown size={12} />
						{t('bookmarks.hide_empty_folders', 'Hide empty folders')}
					{:else}
						<ChevronRight class="rtl:-scale-x-100" size={12} />
						{t('bookmarks.show_empty_folders', 'Show {count} empty folders', { count: hiddenEmptyCount })}
					{/if}
				</button>
			{/if}

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
			<!-- entity_type filter (backend ?entity_type= supports these joined types) -->
			<div class="mb-3 flex justify-end">
				<AutocompleteSelect
					class="w-44"
					value={activeType}
					options={[
						{ value: '', label: t('bookmarks.filter_all_types', 'All types') },
						{ value: 'content', label: t('bookmarks.type.content', 'Content') },
						{ value: 'blog_post', label: t('bookmarks.type.blog_post', 'Blog post') },
						{ value: 'forum_thread', label: t('bookmarks.type.forum_thread', 'Forum thread') },
						{ value: 'forum_reply', label: t('bookmarks.type.forum_reply', 'Forum reply') },
						{ value: 'space', label: t('bookmarks.type.space', 'Space') },
						{ value: 'tag', label: t('bookmarks.type.tag', 'Tag') },
						{ value: 'comment', label: t('bookmarks.type.comment', 'Comment') },
						{ value: 'academy_track', label: t('bookmarks.type.academy_track', 'Learning track') },
						{ value: 'doc_page', label: t('bookmarks.type.doc_page', 'Doc page') }
					]}
					ariaLabel={t('bookmarks.filter_type', 'Bookmark type')}
					onValueChange={(v) => selectType(v)}
				/>
			</div>
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
								<ExternalLink class="h-4 w-4 shrink-0 text-[var(--color-muted)] group-hover:text-[var(--color-primary)] rtl:-scale-x-100" />
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
						{t('bookmarks.load_more', 'Load more')}
					</button>
				{/if}
			{/if}
		</div>
	</div>
</section>
