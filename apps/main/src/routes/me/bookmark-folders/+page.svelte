<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Bookmark } from 'lucide-svelte';

	type BookmarkFolder = {
		id: string;
		name: string;
		item_count: number;
		created_at: string;
	};

	let { data } = $props();
	let folders = $derived(data.data ?? []);
</script>

<svelte:head>
	<title>{t('me.bookmarkFolders.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.bookmarkFolders.title')}</h1>

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={() => invalidateAll()}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each folders as folder (folder.id)}
			<a href="/me/bookmarks?folder={folder.id}" class="rounded-lg border p-4 transition-colors hover:bg-black/5" style="border-color: var(--color-border)">
				<div class="flex items-center gap-3">
					<Bookmark class="h-5 w-5" style="color: var(--color-primary)" />
					<div>
						<p class="font-medium">{folder.name}</p>
						<p class="text-xs" style="color: var(--color-muted)">{folder.item_count} {t('me.bookmarkFolders.items')}</p>
					</div>
				</div>
			</a>
		{:else}
			<p class="col-span-full py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.bookmarkFolders.empty')}</p>
		{/each}
	</div>
{/if}
