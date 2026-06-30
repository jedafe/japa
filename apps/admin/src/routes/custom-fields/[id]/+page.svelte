<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';

	type FieldOption = { id?: string; label: string; sort_order: number };

	type FieldDefinition = {
		id: string;
		name: string;
		slug: string;
		field_type: string;
		entity_type: string;
		required: boolean;
		searchable: boolean;
		sortable: boolean;
		options: FieldOption[];
	};

	let field = $state<FieldDefinition | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let deleting = $state(false);
	let error = $state('');
	let success = $state('');

	let name = $state('');
	let slug = $state('');
	let fieldType = $state('text');
	let entityType = $state('content');
	let required = $state(false);
	let searchable = $state(false);
	let sortable = $state(false);
	let options = $state<FieldOption[]>([]);
	let newOption = $state('');

	const fieldTypes = ['text', 'number', 'select', 'boolean'];
	const entityTypes = ['content', 'user', 'space', 'tag'];

	const fieldId = $derived(page.params.id);

	async function loadField() {
		loading = true;
		try {
			const result = await api<FieldDefinition>(`/custom-field-groups/${fieldId}`);
			if (result.ok) {
				field = result.data;
				name = result.data.name;
				slug = result.data.slug;
				fieldType = result.data.field_type;
				entityType = result.data.entity_type;
				required = result.data.required;
				searchable = result.data.searchable;
				sortable = result.data.sortable;
				options = result.data.options ?? [];
			} else {
				error = result.error;
			}
		} catch {
			error = t('custom_fields.load_error');
		} finally {
			loading = false;
		}
	}

	function autoSlug() {
		slug = name
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function addOption() {
		const trimmed = newOption.trim();
		if (trimmed && !options.some((o) => o.label === trimmed)) {
			options = [...options, { label: trimmed, sort_order: options.length }];
			newOption = '';
		}
	}

	function removeOption(opt: FieldOption) {
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
			error = t('custom_fields.name_required');
			return;
		}
		saving = true;
		error = '';
		success = '';
		try {
			const body: Record<string, unknown> = {
				name: name.trim(),
				slug: slug.trim(),
				field_type: fieldType,
				entity_type: entityType,
				required,
				searchable,
				sortable
			};
			if (fieldType === 'select') {
				body.options = options.map((o, idx) => ({ label: o.label, sort_order: idx }));
			}
		const result = await api(`/custom-field-groups/${fieldId}`, {
			method: 'PATCH',
				body
			});
			if (result.ok) {
				success = t('custom_fields.updated');
				loadField();
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function deleteField() {
		if (!confirm(t('custom_fields.delete_confirm'))) return;
		deleting = true;
		error = '';
		try {
			const result = await api(`/custom-field-groups/${fieldId}`, { method: 'DELETE' });
			if (result.ok) {
				goto('/custom-fields');
			} else {
				error = result.error;
			}
		} catch {
			error = t('custom_fields.delete_error');
		} finally {
			deleting = false;
		}
	}

	onMount(loadField);
</script>

<svelte:head>
	<title>Edit Custom Field — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('custom_fields.edit_title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('custom_fields.edit_description')}</p>

{#if error}
	<div class="mb-4 rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-400">
		{error}
	</div>
{/if}

{#if success}
	<div class="mb-4 rounded-md border border-emerald-300 bg-emerald-50 px-4 py-3 text-sm text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-400">
		{success}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if field}
	<div class="max-w-xl space-y-5">
		<!-- Name -->
		<div>
			<label for="name" class="mb-1 block text-sm font-medium">{t('custom_fields.name')}</label>
			<input
				id="name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('custom_fields.name_placeholder')}
				bind:value={name}
				onblur={autoSlug}
			/>
		</div>

		<!-- Slug -->
		<div>
			<label for="slug" class="mb-1 block text-sm font-medium">{t('custom_fields.slug')}</label>
			<input
				id="slug"
				type="text"
				class="w-full rounded-md border px-3 py-2 font-mono text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				bind:value={slug}
			/>
		</div>

		<!-- Field Type -->
		<div>
			<label for="field-type" class="mb-1 block text-sm font-medium">{t('custom_fields.type')}</label>
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
			<label for="entity-type" class="mb-1 block text-sm font-medium">{t('custom_fields.entity_type')}</label>
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
				{t('custom_fields.required')}
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={searchable} class="rounded" />
				{t('custom_fields.searchable')}
			</label>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={sortable} class="rounded" />
				{t('custom_fields.sortable')}
			</label>
		</div>

		<!-- Options (for select type) -->
		{#if fieldType === 'select'}
			<div>
				<label for="new-option" class="mb-1 block text-sm font-medium">{t('custom_fields.add_option')}</label>
				<div class="flex gap-2">
					<input
						id="new-option"
						type="text"
						class="flex-1 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-surface)"
						placeholder={t('custom_fields.add_option')}
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
								{opt.label}
								<button type="button" class="ml-0.5 text-black/40 hover:text-black dark:text-white/40 dark:hover:text-white" onclick={() => removeOption(opt)}>&times;</button>
							</span>
						{/each}
					</div>
				{/if}
			</div>
		{/if}

		<!-- Actions -->
		<div class="flex items-center gap-3 pt-2">
			<button
				class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				disabled={saving}
				onclick={save}
			>
				{saving ? t('common.saving') : t('common.save')}
			</button>
			<a
				href="/custom-fields"
				class="rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				style="border-color: var(--color-border)"
			>
				{t('common.cancel')}
			</a>
			<div class="flex-1"></div>
			<button
				class="rounded-md border border-red-300 px-4 py-2 text-sm font-medium text-red-600 transition-colors hover:bg-red-50 disabled:opacity-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
				disabled={deleting}
				onclick={deleteField}
			>
				{deleting ? t('common.saving') : t('common.delete')}
			</button>
		</div>
	</div>
{:else}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('custom_fields.not_found')}</p>
{/if}
