<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Bookmark, Eye } from 'lucide-svelte';

	type Bookmark = {
		id: string;
		content_id: string;
		title: string;
		content_type: string;
		folder_id?: string;
		created_at: string;
	};

	let { data } = $props();
	let bookmarks = $derived(data.data ?? []);
</script>

<svelte:head>
	<title>{t('me.bookmarks.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.bookmarks.title')}</h1>

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={() => invalidateAll()}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="space-y-2">
		{#each bookmarks as b (b.id)}
			<a href="/{b.content_type}s/{b.content_id}" class="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-black/5" style="border-color: var(--color-border)">
				<div class="flex items-center gap-3">
					<Bookmark class="h-4 w-4" style="color: var(--color-primary)" />
					<div>
						<p class="font-medium text-sm">{b.title}</p>
						<p class="text-xs" style="color: var(--color-muted)">{b.content_type} • {new Date(b.created_at).toLocaleDateString()}</p>
					</div>
				</div>
			</a>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.bookmarks.empty')}</p>
		{/each}
	</div>
{/if}
