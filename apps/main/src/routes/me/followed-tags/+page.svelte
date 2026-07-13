<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Trash2 } from 'lucide-svelte';

	type FollowedTag = { tag: string; followed_at: string };

	let { data } = $props();
	let followedTags = $derived(data.data ?? []);
	let errorMessage = $state('');

	async function unfollow(tag: string) {
		errorMessage = '';
		try {
			await request<void>(`/me/followed-tags/${tag}`, { method: 'DELETE' });
			await invalidateAll();
		} catch {
			errorMessage = t('common.error');
		}
	}
</script>

<svelte:head>
	<title>{t('me.followedTags.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.followedTags.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={() => invalidateAll()}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="max-w-xl space-y-2">
		{#each followedTags as ft (ft.tag)}
			<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border)">
				<span class="text-sm">#{ft.tag}</span>
				<button class="text-xs underline" onclick={() => unfollow(ft.tag)}>{t('me.followedTags.unfollow')}</button>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.followedTags.empty')}</p>
		{/each}
	</div>
{/if}
