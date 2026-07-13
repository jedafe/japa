<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';

	let { data } = $props();

	let plans = $derived(data.plans ?? []);
</script>

<svelte:head>
	<title>{t('pricing.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('pricing.title')}</h1>

{#if plans.length > 0}
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each plans as plan (plan.id)}
			<div class="rounded-lg border p-6" style="border-color: var(--color-border)">
				<p class="text-lg font-semibold">{plan.name}</p>
				<p class="mt-2 text-3xl font-bold">${plan.monthly.amount}<span class="text-sm font-normal" style="color: var(--color-muted)">/{plan.monthly.currency}</span></p>
				<ul class="mt-4 space-y-2">
					{#each plan.features as feature}
						<li class="text-sm" style="color: var(--color-muted)">• {feature.text}</li>
					{/each}
				</ul>
			</div>
		{:else}
			<p class="col-span-full py-8 text-center text-sm" style="color: var(--color-muted)">{t('pricing.empty')}</p>
		{/each}
	</div>
{:else}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{/if}
