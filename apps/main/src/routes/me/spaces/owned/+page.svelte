<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, UserPlus } from 'lucide-svelte';

	type Space = {
		id: string;
		name: string;
		role: string;
		joined_at: string;
	};

	let { data } = $props();
	let spaces = $derived(data.data ?? []);
</script>

<svelte:head>
	<title>{t('me.ownedSpaces.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.ownedSpaces.title')}</h1>

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
		{#each spaces as space (space.id)}
			<a href="/spaces/{space.id}" class="rounded-lg border p-4 transition-colors hover:bg-black/5" style="border-color: var(--color-border)">
				<p class="font-medium">{space.name}</p>
				<p class="text-xs" style="color: var(--color-muted)">{t('me.ownedSpaces.role')}: {space.role} • {new Date(space.joined_at).toLocaleDateString()}</p>
			</a>
		{:else}
			<p class="col-span-full py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.ownedSpaces.empty')}</p>
		{/each}
	</div>
{/if}
