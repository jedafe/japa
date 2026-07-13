<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Shield } from 'lucide-svelte';

	type Permission = {
		resource: string;
		actions: string[];
	};

	let { data } = $props();
	let permissions = $derived(data.data ?? []);
</script>

<svelte:head>
	<title>{t('me.permissions.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.permissions.title')}</h1>

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="max-w-xl rounded-lg border" style="border-color: var(--color-border)">
		<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
			<h2 class="font-medium flex items-center gap-2"><Shield class="h-4 w-4" style="color: var(--color-muted)" /> {t('me.permissions.header')}</h2>
		</div>
		{#if permissions.length === 0}
			<p class="px-4 py-6 text-center text-sm" style="color: var(--color-muted)">{t('me.permissions.empty')}</p>
		{:else}
			<div class="divide-y" style="border-color: var(--color-border)">
				{#each permissions as perm (perm.resource)}
					<div class="px-4 py-3">
						<p class="font-medium text-sm">{perm.resource}</p>
						<p class="text-xs" style="color: var(--color-muted)">{perm.actions.join(', ')}</p>
					</div>
				{/each}
			</div>
		{/if}
	</div>
{/if}
