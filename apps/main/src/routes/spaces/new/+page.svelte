<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { ArrowLeft, Loader2, Plus } from 'lucide-svelte';
	import { api, type SpaceType } from '$lib/api';

	let name = $state('');
	let description = $state('');
	let spaceType = $state('');
	let error = $state('');
	let loading = $state(false);

	// Space types come from the backend (Rule 16 — no hardcoded domain data).
	let spaceTypes = $state<{ value: string; label: string }[]>([]);

	onMount(async () => {
		try {
			const result = await api.listSpaceTypes();
			spaceTypes = (result.space_types ?? []).map((t: SpaceType) => ({
				value: t.id,
				label: t.name
			}));
			if (spaceTypes.length > 0) spaceType = spaceTypes[0].value;
		} catch {
			// Fall back to an empty list; the select will prompt to retry.
			spaceTypes = [];
		}
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			const result = await api.createSpace({
				name,
				slug: name.toLowerCase().replace(/\s+/g, '-'),
				description,
				space_type_id: spaceType
			});

			goto(`/spaces/${result.slug}`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Create Space | JapaConnect</title>
	<meta name="description" content="Create a new community space on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-lg px-4 py-8">
	<a
		href="/spaces"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to spaces
	</a>

	<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<h1 class="text-xl font-bold" style="color: var(--color-foreground)">Create a space</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">
			Build a community around a shared topic.
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
				<label for="name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Name
				</label>
				<input
					id="name"
					type="text"
					required
					maxlength="100"
					bind:value={name}
					placeholder="e.g. Nigerians in Toronto"
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				/>
			</div>

			<div>
				<label for="description" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Description
				</label>
				<textarea
					id="description"
					required
					rows="3"
					bind:value={description}
					placeholder="What is this space about?"
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				></textarea>
			</div>

			<div>
				<label for="space_type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					Type
				</label>
				<select
					id="space_type"
					bind:value={spaceType}
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				>
					{#each spaceTypes as type}
						<option value={type.value}>{type.label}</option>
					{/each}
				</select>
			</div>

			<button
				type="submit"
				disabled={loading || !name.trim() || !description.trim()}
				class="flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if loading}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<Plus class="h-4 w-4" />
				{/if}
				Create space
			</button>
		</form>
	</div>
</main>
