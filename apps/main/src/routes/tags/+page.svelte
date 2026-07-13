<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Tag } from 'lucide-svelte';

	type Tag = {
		slug: string;
		name: string;
		content_count: number;
		follower_count: number;
	};

	let tags = $state<Tag[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			tags = await request<Tag[]>('/tags');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('tags.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('tags.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="flex flex-wrap gap-2">
		{#each tags as tag (tag.slug)}
			<a href="/tags/{tag.slug}" class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-sm transition-colors hover:bg-black/5" style="border-color: var(--color-border)">
				<Tag class="h-3 w-3" style="color: var(--color-primary)" />
				{tag.name}
				<span class="text-xs" style="color: var(--color-muted)">({tag.content_count})</span>
			</a>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('tags.empty')}</p>
		{/each}
	</div>
{/if}
