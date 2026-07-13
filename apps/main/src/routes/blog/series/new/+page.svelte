<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { ArrowLeft, Save } from 'lucide-svelte';

	let title = $state('');
	let description = $state('');
	let saving = $state(false);
	let error = $state<string | null>(null);

	async function handleCreate(e: Event) {
		e.preventDefault();
		if (!title.trim() || saving) return;
		saving = true;
		error = null;
		try {
			const result = await api.createBlogSeries({
				title: title.trim(),
				description: description.trim() || undefined
			});
			goto('/blog/series/' + result.slug);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create series';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>New Series | JapaConnect Blog</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<a
		href="/blog/series"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to series
	</a>

	<h1 class="text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">Create New Series</h1>

	{#if error}
		<div class="mt-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-400">
			{error}
		</div>
	{/if}

	<form onsubmit={handleCreate} class="mt-6 space-y-5">
		<div>
			<label for="title" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">Title *</label>
			<input
				id="title"
				type="text"
				bind:value={title}
				required
				placeholder="e.g., Relocation Guide Series"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>

		<div>
			<label for="description" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">Description</label>
			<textarea
				id="description"
				bind:value={description}
				rows="3"
				placeholder="Brief description of this series..."
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			></textarea>
		</div>

		<button
			type="submit"
			disabled={!title.trim() || saving}
			class="inline-flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
			style="background: var(--color-primary)"
		>
			<Save class="h-4 w-4" />
			{saving ? 'Creating...' : 'Create Series'}
		</button>
	</form>
</main>
