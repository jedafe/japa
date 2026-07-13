<script lang="ts">
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Search } from 'lucide-svelte';

	let { data } = $props();

	type AutocompleteResult = {
		type: string;
		id: string;
		title: string;
	};

	// svelte-ignore state_referenced_locally
	let query = $state(data.query ?? '');
	// svelte-ignore state_referenced_locally
	let results = $state<AutocompleteResult[]>(data.results ?? []);
	let loading = $state(false);
	let errorMessage = $state('');

	async function search() {
		if (!query.trim()) return;
		loading = true;
		errorMessage = '';
		try {
			results = await request<AutocompleteResult[]>(`/search/autocomplete?q=${encodeURIComponent(query.trim())}`);
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{t('search.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('search.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex gap-2">
	<input type="text" bind:value={query} onkeydown={(e) => e.key === 'Enter' && search()} placeholder={t('search.placeholder')} class="flex-1 rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
	<button class="btn btn-primary" onclick={search} disabled={loading}>
		<Search class="h-4 w-4" />
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="space-y-2">
		{#each results as item (item.id)}
			<a href="/{item.type}s/{item.id}" class="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-black/5" style="border-color: var(--color-border)">
				<div>
					<p class="font-medium text-sm">{item.title}</p>
					<p class="text-xs" style="color: var(--color-muted)">{item.type}</p>
				</div>
			</a>
		{:else}
			{#if query}
				<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('search.no_results')}</p>
			{/if}
		{/each}
	</div>
{/if}
