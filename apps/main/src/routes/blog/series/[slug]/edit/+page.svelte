<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { ArrowLeft, Save, Trash2 } from 'lucide-svelte';

	let { data } = $props();
	let series = $derived(data.series);

	let title = $state('');
	let description = $state('');

	$effect(() => {
		title = series.title;
		description = series.description ?? '';
	});
	let saving = $state(false);
	let deleting = $state(false);
	let error = $state<string | null>(null);

	async function handleUpdate(e: Event) {
		e.preventDefault();
		if (!title.trim() || saving) return;
		saving = true;
		error = null;
		try {
			await api.updateBlogSeries(series.id, {
				title: title.trim(),
				description: description.trim() || undefined
			});
			goto('/blog/series/' + series.slug);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update series';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (deleting || !confirm('Delete this series? Posts will not be deleted.')) return;
		deleting = true;
		error = null;
		try {
			await api.deleteBlogSeries(series.id);
			goto('/blog/series');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete series';
		} finally {
			deleting = false;
		}
	}
</script>

<svelte:head>
	<title>Edit: {series.title} | JapaConnect Blog</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<a
		href="/blog/series/{series.slug}"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to series
	</a>

	<h1 class="text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">{t('blog.series.slug.edit.edit_series')}</h1>

	{#if error}
		<div class="mt-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-400">
			{error}
		</div>
	{/if}

	<form onsubmit={handleUpdate} class="mt-6 space-y-5">
		<div>
			<label for="title" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">{t('blog.series.slug.edit.title')}</label>
			<input
				id="title"
				type="text"
				bind:value={title}
				required
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>

		<div>
			<label for="description" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">{t('blog.series.slug.edit.description')}</label>
			<textarea
				id="description"
				bind:value={description}
				rows="3"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			></textarea>
		</div>

		<div class="flex items-center gap-3">
			<button
				type="submit"
				disabled={!title.trim() || saving}
				class="inline-flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background: var(--color-primary)"
			>
				<Save class="h-4 w-4" />
				{saving ? 'Saving...' : 'Save Changes'}
			</button>

			<button
				type="button"
				onclick={handleDelete}
				disabled={deleting}
				class="inline-flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-colors hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
				style="border-color: var(--color-border); color: #ef4444"
			>
				<Trash2 class="h-4 w-4" />
				{deleting ? 'Deleting...' : 'Delete Series'}
			</button>
		</div>
	</form>
</main>
