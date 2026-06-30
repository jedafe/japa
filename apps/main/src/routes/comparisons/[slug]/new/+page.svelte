<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { ArrowLeft, Loader2, Plus, X } from 'lucide-svelte';
	import { api } from '$lib/api';

	let slug = $derived($page.params.slug);
	let title = $state('');
	let items = $state<string[]>(['', '']);
	let error = $state('');
	let loading = $state(false);

	function addItem() {
		items = [...items, ''];
	}

	function removeItem(index: number) {
		if (items.length <= 2) return;
		items = items.filter((_, i) => i !== index);
	}

	function updateItem(index: number, value: string) {
		items = items.map((item, i) => (i === index ? value : item));
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		const validItems = items.filter((i) => i.trim());
		if (validItems.length < 2) {
			error = 'Add at least two items to compare.';
			loading = false;
			return;
		}

		try {
			await api.createComparison({
				type_id: slug ?? '',
				title,
				items: validItems.map((name) => ({ name, values: {} }))
			});

			goto(`/comparisons/${slug}`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Create Comparison | JapaConnect</title>
	<meta name="description" content="Create a new comparison on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-lg px-4 py-8">
	<a
		href="/comparisons/{slug}"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to comparisons
	</a>

	<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<h1 class="text-xl font-bold" style="color: var(--color-foreground)">Create a comparison</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">
			Add items to compare side by side.
		</p>

		{#if error}
			<div
				class="mt-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{error}
			</div>
		{/if}

		<form onsubmit={handleSubmit} class="mt-6 space-y-4">
			<div>
				<label for="title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Title
				</label>
				<input
					id="title"
					type="text"
					required
					maxlength="200"
					bind:value={title}
					placeholder="e.g. UK vs Canada vs Australia"
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				/>
			</div>

			<div>
				<span class="mb-2 block text-sm font-medium" style="color: var(--color-foreground)">
					Items to compare
				</span>
				<div class="space-y-2">
					{#each items as item, i}
						<div class="flex gap-2">
							<input
								type="text"
								required
								maxlength="200"
								value={item}
								oninput={(e) => updateItem(i, (e.target as HTMLInputElement).value)}
								placeholder="Item {i + 1}"
								class="flex-1 rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
								style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
							/>
							{#if items.length > 2}
								<button
									type="button"
									onclick={() => removeItem(i)}
									class="shrink-0 rounded-lg p-2 transition-colors hover:opacity-80"
									style="color: var(--color-muted)"
								>
									<X class="h-4 w-4" />
								</button>
							{/if}
						</div>
					{/each}
				</div>
				<button
					type="button"
					onclick={addItem}
					class="mt-3 flex items-center gap-1.5 text-sm font-medium transition-colors hover:opacity-80"
					style="color: var(--color-primary)"
				>
					<Plus class="h-4 w-4" />
					Add item
				</button>
			</div>

			<button
				type="submit"
				disabled={loading || !title.trim()}
				class="flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
			{#if loading}
				<Loader2 class="h-4 w-4 animate-spin" />
			{:else}
				<Plus class="h-4 w-4" />
			{/if}
				Create comparison
			</button>
		</form>
	</div>
</main>
