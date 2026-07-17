<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowLeft, BarChart3, Edit, Plus, Vote, Users } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import { formatTimestamp } from '$lib/utils/timezone';

	let { data } = $props();
	let compType = $derived(data.type);
	let comparisons = $derived(data.comparisons);

	function formatDate(dateStr: string) {
		return formatTimestamp(dateStr);
	}
</script>

<svelte:head>
	<title>{compType.name} Comparisons | JapaConnect</title>
	<meta name="description" content={compType.description} />
</svelte:head>

{#if !isFeatureEnabled('feature.comparison')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('comparisons.slug.comparisons_is_not_available')}</h1>
			<p class="mt-2" style="color: var(--color-muted)">{t('comparisons.slug.this_feature_is_currently_disabled')}</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">{t('comparisons.slug.go_home')}</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-6xl px-4 py-8">
	<a
		href="/comparisons"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to comparisons
	</a>

	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
				{compType.name}
			</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{compType.description}
			</p>
		</div>
		<div class="flex items-center gap-2">
			<a
				href="/comparisons/{compType.slug}/edit"
				class="flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm font-medium transition-colors hover:opacity-80"
				style="border-color: var(--color-border); color: var(--color-foreground)"
			>
				<Edit class="h-4 w-4" />
				Edit
			</a>
			<a
				href="/comparisons/{compType.slug}/new"
				class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background-color: var(--color-primary)"
			>
				<Plus class="h-4 w-4" />
				Create comparison
			</a>
		</div>
	</header>

	{#if comparisons.length === 0}
		<EmptyState
			icon={BarChart3}
			title="No comparisons yet"
			message="Be the first to create a comparison for this category."
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each comparisons as comp (comp.id)}
				<div
					class="rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<h3 class="text-base font-semibold" style="color: var(--color-foreground)">
						{comp.title}
					</h3>
					<div class="mt-4 flex items-center gap-4 text-xs" style="color: var(--color-muted)">
						<span class="flex items-center gap-1">
							<Users class="h-3.5 w-3.5" />
							{comp.item_count ?? 0} item{(comp.item_count ?? 0) !== 1 ? 's' : ''}
						</span>
						<span class="flex items-center gap-1">
							<Vote class="h-3.5 w-3.5" />
							{comp.vote_count ?? 0} vote{(comp.vote_count ?? 0) !== 1 ? 's' : ''}
						</span>
						<span>{formatDate(comp.created_at)}</span>
					</div>
					<a
						href="/comparisons/{compType.slug}/{comp.id}"
						class="mt-4 block w-full rounded-lg border px-3 py-1.5 text-center text-sm font-medium transition-colors hover:opacity-90"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						View comparison
					</a>
				</div>
			{/each}
		</div>
	{/if}
	</main>
{/if}
