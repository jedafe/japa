<script lang="ts">
	// Ref: japaconnect-engineering/sections/50-current-user.md (bookmarks)
	// Lists the user's bookmarks with folder management. The backend returns
	// thin records (entity_type + entity_id), so the page derives a link per
	// entity_type and shows the folder + bookmarked date.
	// Folders are managed client-side via POST/PATCH/DELETE /me/bookmark-folders.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Bookmark as BookmarkIcon, ExternalLink, FolderPlus, Trash2, Loader2, Folder } from 'lucide-svelte';
	import type { Bookmark, BookmarkFolder } from '$lib/types';
	import { untrack } from 'svelte';

	let { data } = $props();
	const bookmarks = $derived(data.bookmarks);
	let folders = $state<BookmarkFolder[]>(untrack(() => data.folders ?? []));

	let activeFolder = $state<string | null>(null);
	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let deletingFolderId = $state<string | null>(null);

	const filteredBookmarks = $derived(
		activeFolder
			? bookmarks.filter((b) => b.folder === folders.find((f) => f.id === activeFolder)?.name)
			: bookmarks
	);

	function entityUrl(b: Bookmark): string {
		switch (b.entity_type) {
			case 'content':
			case 'blog_post':
				return `/blog/${b.entity_id}`;
			case 'forum_thread':
				return `/forum/${b.entity_id}`;
			case 'space':
				return `/spaces/${b.entity_id}`;
			case 'comment':
				return `/content/${b.entity_id}`;
			default:
				return `/content/${b.entity_id}`;
		}
	}

	function entityLabel(type: string): string {
		const key = `bookmarks.type.${type}`;
		return t(key, type);
	}

	async function createFolder() {
		if (!newFolderName.trim() || creatingFolder) return;
		creatingFolder = true;
		const res = await api.post<BookmarkFolder>('/me/bookmark-folders', { name: newFolderName.trim() });
		if (res.data) {
			folders = [...folders, res.data];
			newFolderName = '';
		}
		creatingFolder = false;
	}

	async function deleteFolder(id: string) {
		if (deletingFolderId) return;
		deletingFolderId = id;
		const res = await api.delete(`/me/bookmark-folders/${id}`);
		if (!res.error) {
			folders = folders.filter((f) => f.id !== id);
			if (activeFolder === id) activeFolder = null;
		}
		deletingFolderId = null;
	}
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.bookmarks')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('bookmarks.subtitle')}</p>
	</header>

	<div class="grid gap-4 sm:grid-cols-[200px_1fr]">
		<!-- Folder sidebar -->
		<aside class="space-y-2">
			<button
				type="button"
				onclick={() => (activeFolder = null)}
				class="flex w-full items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm transition-colors {activeFolder === null ? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-text)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				<BookmarkIcon size={14} />
				{t('bookmarks.all', 'All')}
				<span class="ml-auto text-xs text-[var(--color-muted)]">{bookmarks.length}</span>
			</button>

			{#each folders as folder (folder.id)}
				<div class="group flex items-center gap-1">
					<button
						type="button"
						onclick={() => (activeFolder = folder.id)}
						class="flex flex-1 items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm transition-colors {activeFolder === folder.id ? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-text)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
					>
						<Folder size={14} />
						<span class="truncate">{folder.name}</span>
					</button>
					<button
						type="button"
						onclick={() => deleteFolder(folder.id)}
						disabled={deletingFolderId === folder.id}
						class="shrink-0 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-red-500 group-hover:opacity-100 disabled:opacity-50"
						aria-label={t('bookmarks.delete_folder', 'Delete folder')}
					>
						{#if deletingFolderId === folder.id}
							<Loader2 size={12} class="animate-spin" />
						{:else}
							<Trash2 size={12} />
						{/if}
					</button>
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
			{#if filteredBookmarks.length === 0}
				<EmptyState
					icon={BookmarkIcon}
					title={t('bookmarks.empty_title')}
					body={t('bookmarks.empty_body')}
				/>
			{:else}
				<ul class="space-y-2">
					{#each filteredBookmarks as b (b.id)}
						<li>
							<a
								href={entityUrl(b)}
								class="group flex items-center justify-between gap-3 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
							>
								<div class="min-w-0 flex-1">
									<p class="text-sm font-medium text-[var(--color-text)]">
										{entityLabel(b.entity_type)}
									</p>
									<p class="mt-0.5 truncate text-xs text-[var(--color-muted)]">
										#{b.entity_id.slice(0, 8)}
										{#if b.folder}· {b.folder}{/if}
										· {new Date(b.created_at).toLocaleDateString()}
									</p>
								</div>
								<ExternalLink class="h-4 w-4 shrink-0 text-[var(--color-muted)] group-hover:text-[var(--color-primary)]" />
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</div>
</section>
