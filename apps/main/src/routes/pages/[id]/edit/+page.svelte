<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import PageBuilder from '$lib/page-builder/PageBuilder.svelte';
	import type { BlockInstance } from '$lib/page-builder/types';
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Save, Loader2, Trash2 } from 'lucide-svelte';

	let { data } = $props();

	let blocks = $derived(data.page?.page_schema?.blocks ?? []);
	let title = $derived(data.page?.title ?? '');
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
			await api.updatePage(data.page.id, {
				title,
				page_schema: { schema_version: 1, blocks }
			});
			goto(`/pages/${data.page.id}`);
		} catch {
			errorMessage = 'Failed to save page.';
		} finally {
			saving = false;
		}
	}

	async function deletePage() {
		if (!confirm('Are you sure you want to delete this page?')) return;
		try {
			await api.deletePage(data.page.id);
			goto('/');
		} catch {
			errorMessage = 'Failed to delete page.';
		}
	}
</script>

<svelte:head>
	<title>Edit: {title} — JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-5xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<div class="flex items-center gap-3">
			<a href="/pages/{data.page?.id}" class="rounded-md p-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10">
				<ArrowLeft class="h-5 w-5" style="color: var(--color-muted)" />
			</a>
			<h1 class="text-xl font-bold" style="color: var(--color-foreground)">{t('pages.id.edit.edit_page')}</h1>
		</div>
		<div class="flex items-center gap-2">
			<button
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="color: var(--color-danger)"
				onclick={deletePage}
			>
				<Trash2 class="h-4 w-4" />
				Delete
			</button>
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
					Save Changes
				{/if}
			</button>
		</div>
	</div>

	{#if errorMessage}
		<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{errorMessage}
		</div>
	{/if}

	<div class="mb-6">
		<label for="page-title" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('pages.id.edit.title')}</label>
		<input
			id="page-title"
			type="text"
			bind:value={title}
			placeholder="Page title"
			class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:ring-2"
			style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
		/>
	</div>

	<div class="rounded-xl border" style="border-color: var(--color-border); background: var(--color-surface)">
		<PageBuilder {blocks} onChange={(b) => (blocks = b)} />
	</div>
</main>
