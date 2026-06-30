<script lang="ts">
	import { GitCompare, ArrowRight, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	let types = $state<Array<{ id: string; name: string; slug: string; description: string; comparison_count: number }>>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			types = await api.getComparisonTypes({ limit: 50 });
		} catch {
			types = [];
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Comparisons | JapaConnect</title>
	<meta name="description" content="Compare countries, universities, and more on JapaConnect." />
</svelte:head>

{#if !isFeatureEnabled('feature.comparison')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('comparisons.not_available')}</h1>
			<p class="mt-2" style="color: var(--color-muted)">{t('forum.feature_disabled')}</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">{t('forum.go_home')}</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
			{t('comparisons.title')}
		</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('comparisons.subtitle')}
		</p>
	</header>

	{#if loading}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _}
				<div
					class="h-40 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if types.length === 0}
		<EmptyState
			icon={GitCompare}
			title={t('comparisons.none')}
			message={t('comparisons.none_msg')}
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each types as compType (compType.id)}
				<a
					href="/comparisons/{compType.slug}"
					class="group rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-start justify-between">
						<h3
							class="text-base font-semibold group-hover:underline"
							style="color: var(--color-foreground)"
						>
							{compType.name}
						</h3>
						<ArrowRight
							class="h-5 w-5 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
							style="color: var(--color-primary)"
						/>
					</div>
					<p class="mt-2 text-sm line-clamp-2" style="color: var(--color-muted)">
						{compType.description}
					</p>
					<div class="mt-4 text-xs" style="color: var(--color-muted)">
						{compType.comparison_count} comparison{compType.comparison_count !== 1 ? 's' : ''}
					</div>
				</a>
			{/each}
		</div>
	{/if}
	</main>
{/if}
