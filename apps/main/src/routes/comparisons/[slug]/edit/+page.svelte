<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { ArrowLeft, Save, Trash2 } from 'lucide-svelte';

	let { data } = $props();
	let compType = $derived(data.type);

	let name = $state('');
	let description = $state('');
	let isPublic = $state(false);

	$effect(() => {
		name = compType.name;
		description = compType.description ?? '';
		isPublic = compType.is_public;
	});
	let saving = $state(false);
	let deleting = $state(false);
	let error = $state<string | null>(null);

	async function handleUpdate(e: Event) {
		e.preventDefault();
		if (!name.trim() || saving) return;
		saving = true;
		error = null;
		try {
			await api.updateComparisonType(compType.id, {
				name: name.trim(),
				description: description.trim(),
				is_public: isPublic
			});
			goto('/comparisons/' + compType.slug);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (deleting || !confirm('Delete this comparison type? All comparisons under it will be orphaned.')) return;
		deleting = true;
		error = null;
		try {
			await api.deleteComparisonType(compType.id);
			goto('/comparisons');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete';
			deleting = false;
		}
	}
</script>

<svelte:head>
	<title>Edit: {compType.name} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<a
		href="/comparisons/{compType.slug}"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to type
	</a>

	<h1 class="text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">Edit Comparison Type</h1>

	{#if error}
		<div class="mt-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700">{error}</div>
	{/if}

	<form onsubmit={handleUpdate} class="mt-6 space-y-5">
		<div>
			<label for="name" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">Name *</label>
			<input id="name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="description" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">Description</label>
			<textarea id="description" bind:value={description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input type="checkbox" id="is_public" bind:checked={isPublic} class="h-4 w-4" />
			<label for="is_public" class="text-sm" style="color: var(--color-foreground)">Public</label>
		</div>
		<div class="flex items-center gap-3">
			<button type="submit" disabled={!name.trim() || saving} class="inline-flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50" style="background: var(--color-primary)">
				<Save class="h-4 w-4" />
				{saving ? 'Saving...' : 'Save Changes'}
			</button>
			<button type="button" onclick={handleDelete} disabled={deleting} class="inline-flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-colors hover:bg-red-50 disabled:opacity-50" style="border-color: var(--color-border); color: #ef4444">
				<Trash2 class="h-4 w-4" />
				{deleting ? 'Deleting...' : 'Delete'}
			</button>
		</div>
	</form>
</main>
