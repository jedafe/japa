<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { GitCompare, Table } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import ComparisonMatrix from '$lib/components/comparison/ComparisonMatrix.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const comparison = $derived(data.comparison);
	const matrix = $derived(data.matrix ?? []);

	// Null-coalesce: the backend may return null (not []) for items/dimensions
	// on a freshly created comparison before they are populated, despite the
	// type declaring them as arrays. Coerce to [] to avoid a null-deref.
	const items = $derived(comparison.items ?? []);
	const dimensions = $derived(comparison.dimensions ?? []);

	const title = $derived(comparison.title || t('compare.detail.title', 'Comparison — JapaConnect'));
	const description = $derived(
		t('compare.detail.description', 'Compare items across multiple dimensions.')
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<GitCompare size={20} /> {comparison.title}
		</h1>
		{#if comparison.body}
			<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{comparison.body}</p>
		{:else}
			<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
				{t('compare.detail.subtitle', 'Items compared across dimensions.')}
			</p>
		{/if}
	</div>

	<div class="p-3">
		{#if items.length === 0 && dimensions.length === 0}
			<EmptyState
				icon={Table}
				title={t('compare.detail.empty', 'This comparison has no items or dimensions yet')}
			/>
		{:else if dimensions.length === 0}
			<EmptyState
				icon={Table}
				title={t('compare.detail.no_dimensions', 'No dimensions defined yet')}
			/>
		{:else if items.length === 0}
			<EmptyState
				icon={Table}
				title={t('compare.detail.no_items', 'No items added to this comparison yet')}
			/>
		{:else}
			<ComparisonMatrix {items} {dimensions} {matrix} />
		{/if}
	</div>
</div>
