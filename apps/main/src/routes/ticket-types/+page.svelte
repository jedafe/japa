<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';

	let { data } = $props();

	let types = $derived(data.types);
</script>

<svelte:head>
	<title>{t('ticketTypes.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('ticketTypes.title')}</h1>

<div class="max-w-xl space-y-2">
	{#each types as tt (tt.id)}
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="font-medium text-sm">{tt.name}</p>
			{#if tt.description}
				<p class="text-xs" style="color: var(--color-muted)">{tt.description}</p>
			{/if}
		</div>
	{:else}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('ticketTypes.empty')}</p>
	{/each}
</div>
