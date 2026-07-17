<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import PageBuilder from '$lib/page-builder/PageBuilder.svelte';
	import type { BlockInstance } from '$lib/page-builder/types';
	import { ArrowLeft, Edit } from 'lucide-svelte';

	let { data } = $props();
	let blocks = $derived<BlockInstance[]>(data.page?.page_schema?.blocks ?? []);
</script>

<svelte:head>
	<title>{data.page?.title ?? 'Page'} — JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-5xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<div class="flex items-center gap-3">
			<a href="/" class="rounded-md p-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10">
				<ArrowLeft class="h-5 w-5" style="color: var(--color-muted)" />
			</a>
			<h1 class="text-xl font-bold" style="color: var(--color-foreground)">{data.page?.title ?? 'Untitled'}</h1>
		</div>
		<a
			href="/pages/{data.page?.id}/edit"
			class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-foreground)"
		>
			<Edit class="h-4 w-4" />
			Edit
		</a>
	</div>

	{#if blocks.length > 0}
		<PageBuilder {blocks} readOnly />
	{:else}
		<div class="rounded-xl border p-12 text-center" style="border-color: var(--color-border); background: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{t('pages.id.this_page_has_no_content_blocks')}</p>
		</div>
	{/if}
</main>
