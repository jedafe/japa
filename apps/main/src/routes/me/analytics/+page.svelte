<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';

	type Analytics = {
		views: number;
		clicks: number;
		engagements: number;
		period: string;
	};

	let { data } = $props();
	let analytics = $derived(data.data ?? null);
</script>

<svelte:head>
	<title>{t('me.analytics.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.analytics.title')}</h1>

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if analytics}
	<div class="grid gap-4 sm:grid-cols-3">
		<div class="rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{t('me.analytics.views')}</p>
			<p class="mt-1 text-3xl font-bold">{analytics.views}</p>
		</div>
		<div class="rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{t('me.analytics.clicks')}</p>
			<p class="mt-1 text-3xl font-bold">{analytics.clicks}</p>
		</div>
		<div class="rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{t('me.analytics.engagements')}</p>
			<p class="mt-1 text-3xl font-bold">{analytics.engagements}</p>
		</div>
	</div>
{/if}
