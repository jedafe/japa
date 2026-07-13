<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';

	type Certificate = {
		id: string;
		track_title: string;
		issued_at: string;
		download_url: string;
	};

	let { data } = $props();
	let certificates = $derived(data.data ?? []);
</script>

<svelte:head>
	<title>{t('me.certificates.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.certificates.title')}</h1>

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
		{#each certificates as cert (cert.id)}
			<a href={cert.download_url} class="rounded-lg border p-4 transition-colors hover:bg-black/5" style="border-color: var(--color-border)">
				<p class="font-medium text-sm">{cert.track_title}</p>
				<p class="text-xs" style="color: var(--color-muted)">				{t('me.certificates.issued')}: {new Date(cert.issued_at).toLocaleDateString()}</p>
			</a>
		{:else}
			<p class="col-span-full py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.certificates.empty')}</p>
		{/each}
	</div>
{/if}
