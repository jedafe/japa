<script lang="ts">
	import PageBuilder from '$lib/page-builder/PageBuilder.svelte';
	import type { BlockInstance } from '$lib/page-builder/types';
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Save, Loader2 } from 'lucide-svelte';

	let blocks = $state<BlockInstance[]>([]);
	let title = $state('');
	let slug = $state('');
	let saving = $state(false);
	let errorMessage = $state('');

	async function savePage() {
		if (!title.trim()) {
			errorMessage = 'Title is required.';
			return;
		}
		saving = true;
		errorMessage = '';
		try {
			const result = await api.createPage({
				title,
				slug: slug || title.toLowerCase().replace(/[^a-z0-9]+/g, '-'),
				page_schema: { schema_version: 1, blocks }
			});
			goto(`/pages/${result.id}`);
		} catch {
			errorMessage = 'Failed to save page.';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>New Page — JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-5xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<div class="flex items-center gap-3">
			<a href="/" class="rounded-md p-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10">
				<ArrowLeft class="h-5 w-5" style="color: var(--color-muted)" />
			</a>
			<h1 class="text-xl font-bold" style="color: var(--color-foreground)">New Page</h1>
		</div>
		<button
			class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
			style="background-color: var(--color-primary)"
			disabled={saving}
			onclick={savePage}
		>
			{#if saving}
				<Loader2 class="h-4 w-4 animate-spin" />
				Saving...
			{:else}
				<Save class="h-4 w-4" />
				Save Page
			{/if}
		</button>
	</div>

	{#if errorMessage}
		<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{errorMessage}
		</div>
	{/if}

	<div class="mb-6 grid gap-4 sm:grid-cols-2">
		<div>
			<label for="page-title" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">Title</label>
			<input
				id="page-title"
				type="text"
				bind:value={title}
				placeholder="Page title"
				class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:ring-2"
				style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
			/>
		</div>
		<div>
			<label for="page-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">Slug</label>
			<input
				id="page-slug"
				type="text"
				bind:value={slug}
				placeholder="auto-generated from title"
				class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:ring-2"
				style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
			/>
		</div>
	</div>

	<div class="rounded-xl border" style="border-color: var(--color-border); background: var(--color-surface)">
		<PageBuilder {blocks} onChange={(b) => (blocks = b)} />
	</div>
</main>
