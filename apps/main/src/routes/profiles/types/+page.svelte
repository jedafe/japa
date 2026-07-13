<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';

	type ProfileType = {
		slug: string;
		name: string;
		description?: string;
	};

	let types = $state<ProfileType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			types = await request<ProfileType[]>('/profiles/types');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('profiles.types.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('profiles.types.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each types as pt (pt.slug)}
			<a href="/professional/{pt.slug}" class="rounded-lg border p-4 transition-colors hover:bg-black/5" style="border-color: var(--color-border)">
				<p class="font-medium">{pt.name}</p>
				{#if pt.description}
					<p class="text-xs" style="color: var(--color-muted)">{pt.description}</p>
				{/if}
			</a>
		{:else}
			<p class="col-span-full py-8 text-center text-sm" style="color: var(--color-muted)">{t('profiles.types.empty')}</p>
		{/each}
	</div>
{/if}
