<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { ArrowLeft, Save } from 'lucide-svelte';

	let name = $state('');
	let slug = $state('');
	let description = $state('');
	let isPublic = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);

	function generateSlug() {
		slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	async function handleCreate(e: Event) {
		e.preventDefault();
		if (!name.trim() || !slug.trim() || saving) return;
		saving = true;
		error = null;
		try {
			const result = await api.createComparisonType({
				name: name.trim(),
				slug: slug.trim(),
				description: description.trim(),
				is_public: isPublic
			});
			goto('/comparisons/' + result.slug);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create comparison type';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>New Comparison Type | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<a
		href="/comparisons"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to comparisons
	</a>

	<h1 class="text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">Create Comparison Type</h1>

	{#if error}
		<div class="mt-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700">{error}</div>
	{/if}

	<form onsubmit={handleCreate} class="mt-6 space-y-5">
		<div>
			<label for="name" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">Name *</label>
			<input id="name" type="text" bind:value={name} oninput={generateSlug} required placeholder="e.g., Visa Comparison" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="slug" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">Slug *</label>
			<input id="slug" type="text" bind:value={slug} required placeholder="visa-comparison" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="description" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">Description</label>
			<textarea id="description" bind:value={description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input type="checkbox" id="is_public" bind:checked={isPublic} class="h-4 w-4" />
			<label for="is_public" class="text-sm" style="color: var(--color-foreground)">Public</label>
		</div>
		<button type="submit" disabled={!name.trim() || !slug.trim() || saving} class="inline-flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50" style="background: var(--color-primary)">
			<Save class="h-4 w-4" />
			{saving ? 'Creating...' : 'Create Type'}
		</button>
	</form>
</main>
