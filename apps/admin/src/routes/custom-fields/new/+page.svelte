<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';

	let name = $state('');
	let slug = $state('');
	let fieldType = $state('text');
	let entityType = $state('content');
	let required = $state(false);
	let searchable = $state(false);
	let sortable = $state(false);
	let options = $state<string[]>([]);
	let newOption = $state('');
	let saving = $state(false);
	let error = $state('');

	const fieldTypes = ['text', 'number', 'select', 'boolean'];
	const entityTypes = ['content', 'user', 'space', 'tag'];

	function autoSlug() {
		slug = name
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function addOption() {
		const trimmed = newOption.trim();
		if (trimmed && !options.includes(trimmed)) {
			options = [...options, trimmed];
			newOption = '';
		}
	}

	function removeOption(opt: string) {
		options = options.filter((o) => o !== opt);
	}

	function handleOptionKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			addOption();
		}
	}

	async function save() {
		if (!name.trim()) {
			error = 'Name is required';
			return;
		}
		saving = true;
		error = '';
		try {
			const body: Record<string, unknown> = {
				name: name.trim(),
				slug: slug.trim() || autoSlug(),
				field_type: fieldType,
				entity_type: entityType,
				required,
				searchable,
				sortable
			};
			if (fieldType === 'select' && options.length > 0) {
				body.options = options.map((label, idx) => ({ label, sort_order: idx }));
			}
			const result = await api<{ id: string }>(`/custom-field-groups`, {
				method: 'POST',
				body
			});
			if (result.ok) {
				goto('/custom-fields');
			} else {
				error = result.error;
			}
		} catch {
			error = 'Failed to save';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Create Custom Field — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Create Custom Field</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">Define a new field for an entity type</p>

{#if error}
	<div class="mb-4 rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-400">
		{error}
	</div>
{/if}

<div class="max-w-xl space-y-5">
	<!-- Name -->
	<div>
		<label for="name" class="mb-1 block text-sm font-medium">Name</label>
		<input
			id="name"
			type="text"
			class="w-full rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			placeholder="e.g. Experience Level"
			bind:value={name}
			onblur={autoSlug}
		/>
	</div>

	<!-- Slug -->
	<div>
		<label for="slug" class="mb-1 block text-sm font-medium">Slug</label>
		<input
			id="slug"
			type="text"
			class="w-full rounded-md border px-3 py-2 font-mono text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			placeholder="auto-generated-from-name"
			bind:value={slug}
		/>
	</div>

	<!-- Field Type -->
	<div>
		<label for="field-type" class="mb-1 block text-sm font-medium">Field Type</label>
		<select
			id="field-type"
			class="w-full rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			bind:value={fieldType}
		>
			{#each fieldTypes as ft}
				<option value={ft}>{ft}</option>
			{/each}
		</select>
	</div>

	<!-- Entity Type -->
	<div>
		<label for="entity-type" class="mb-1 block text-sm font-medium">Entity Type</label>
		<select
			id="entity-type"
			class="w-full rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			bind:value={entityType}
		>
			{#each entityTypes as et}
				<option value={et}>{et}</option>
			{/each}
		</select>
	</div>

	<!-- Checkboxes -->
	<div class="flex flex-wrap gap-6">
		<label class="flex items-center gap-2 text-sm">
			<input type="checkbox" bind:checked={required} class="rounded" />
			Required
		</label>
		<label class="flex items-center gap-2 text-sm">
			<input type="checkbox" bind:checked={searchable} class="rounded" />
			Searchable
		</label>
		<label class="flex items-center gap-2 text-sm">
			<input type="checkbox" bind:checked={sortable} class="rounded" />
			Sortable
		</label>
	</div>

	<!-- Options (for select type) -->
	{#if fieldType === 'select'}
		<div>
			<label for="new-option" class="mb-1 block text-sm font-medium">Options</label>
			<div class="flex gap-2">
				<input
					id="new-option"
					type="text"
					class="flex-1 rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					placeholder="Add an option"
					bind:value={newOption}
					onkeydown={handleOptionKeydown}
				/>
				<button
					type="button"
					class="rounded-md border px-3 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="border-color: var(--color-border)"
					onclick={addOption}
				>
					Add
				</button>
			</div>
			{#if options.length > 0}
				<div class="mt-2 flex flex-wrap gap-2">
					{#each options as opt}
						<span class="inline-flex items-center gap-1 rounded-full bg-black/5 px-3 py-1 text-xs dark:bg-white/10">
							{opt}
							<button type="button" class="ml-0.5 text-black/40 hover:text-black dark:text-white/40 dark:hover:text-white" onclick={() => removeOption(opt)}>&times;</button>
						</span>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	<!-- Actions -->
	<div class="flex gap-3 pt-2">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
			style="background-color: var(--color-primary)"
			disabled={saving}
			onclick={save}
		>
			{saving ? 'Saving...' : 'Create Field'}
		</button>
		<a
			href="/custom-fields"
			class="rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="border-color: var(--color-border)"
		>
			Cancel
		</a>
	</div>
</div>
