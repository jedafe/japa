<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Select } from '$lib/components/ui/select';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';

	type FieldOption = { value: string; label: string };

	type FieldDefinition = {
		id: string;
		name: string;
		slug: string;
		field_type: string;
		entity_type: string;
		required: boolean;
		searchable: boolean;
		sortable: boolean;
		show_in_frontend: boolean;
		options: FieldOption[];
	};

	let field = $state<FieldDefinition | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let deleting = $state(false);
	let showDeleteModal = $state(false);
	let error = $state('');
	let success = $state('');

	let name = $state('');
	let slug = $state('');
	let fieldType = $state('text');
	let entityType = $state('content');
	let required = $state(false);
	let searchable = $state(false);
	let sortable = $state(false);
	let showInFrontend = $state(true);
	let options = $state<FieldOption[]>([]);
	let newOptionValue = $state('');
	let newOptionLabel = $state('');

	let numMin = $state('');
	let numMax = $state('');
	let numIsDecimal = $state(false);

	let fileMaxFiles = $state(1);
	let fileMaxSizeMb = $state(10);

	let refEntityClass = $state('');

	const fieldTypes = [
		'text', 'textarea', 'rich_text',
		'number', 'currency', 'percentage',
		'boolean',
		'date', 'datetime', 'time', 'duration',
		'select', 'multiselect',
		'url', 'email', 'phone', 'color', 'rating',
		'image_single', 'image_multi', 'media',
		'platform_embed', 'reference'
	];
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
				showInFrontend = result.data.show_in_frontend ?? true;
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
		const v = newOptionValue.trim();
		const l = newOptionLabel.trim();
		if (v && l && !options.some((o) => o.value === v)) {
			options = [...options, { value: v, label: l }];
			newOptionValue = '';
			newOptionLabel = '';
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

	function buildOptionsJSON(): Record<string, unknown> | null {
		const needsOptions = ['select', 'multiselect'].includes(fieldType);
		if (needsOptions) {
			if (options.length > 0) {
				return { value_label_pairs: options.map((o) => ({ value: o.value, label: o.label })) };
			}
			return null;
		}
		if (['number', 'currency', 'percentage'].includes(fieldType)) {
			const o: Record<string, unknown> = {};
			if (numMin) o.min = Number(numMin);
			if (numMax) o.max = Number(numMax);
			if (numIsDecimal) o.is_decimal = true;
			return Object.keys(o).length > 0 ? o : null;
		}
		if (['image_single', 'image_multi', 'media'].includes(fieldType)) {
			return { max_files: fileMaxFiles, max_size_mb: fileMaxSizeMb };
		}
		if (fieldType === 'reference' && refEntityClass.trim()) {
			return { ref_entity_class: refEntityClass.trim() };
		}
		return null;
	}

	function buildOptionsArray(): FieldOption[] {
		if (['select', 'multiselect'].includes(fieldType)) {
			return options;
		}
		return [];
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
				sortable,
				show_in_frontend: showInFrontend,
			};
			const optsArr = buildOptionsArray();
			if (optsArr.length > 0) {
				body.options = optsArr;
			}
			const optsJSON = buildOptionsJSON();
			if (optsJSON) {
				body.options_json = optsJSON;
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
		showDeleteModal = true;
	}

	async function executeDelete() {
		showDeleteModal = false;
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

	$effect(() => {
		loadField();
	});
</script>

<svelte:head>
	<title>{t('custom_fields.edit_title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('custom_fields.edit_title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('custom_fields.edit_description')}</p>

{#if error}
	<div class="mb-4 rounded-md border border-red-300 bg-danger px-4 py-3 text-sm text-danger dark:border-red-800 dark:bg-red-950 dark:text-danger">
		{error}
	</div>
{/if}

{#if success}
	<div class="mb-4 rounded-md border border-success bg-success px-4 py-3 text-sm text-success dark:dark:border-success dark:dark:bg-success dark:text-success">
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
			<Select
				options={fieldTypes.map((ft) => ({ value: ft, label: ft }))}
				bind:value={fieldType}
			/>
		</div>

		<!-- Entity Type -->
		<div>
			<label for="entity-type" class="mb-1 block text-sm font-medium">{t('custom_fields.entity_type')}</label>
			<Select
				options={entityTypes.map((et) => ({ value: et, label: et }))}
				bind:value={entityType}
			/>
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
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={showInFrontend} class="rounded" />
				{t('custom_fields.show_in_frontend', 'Show in frontend')}
			</label>
		</div>

		<!-- Options for select / multiselect -->
		{#if fieldType === 'select' || fieldType === 'multiselect'}
			<div>
				<span class="mb-1 block text-sm font-medium">{t('custom_fields.add_option')}</span>
				<div class="flex gap-2">
					<input
						type="text"
						class="w-32 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-surface)"
						placeholder={t('custom_fields.option_value', 'Value')}
						bind:value={newOptionValue}
						onkeydown={handleOptionKeydown}
					/>
					<input
						type="text"
						class="flex-1 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-surface)"
						placeholder={t('custom_fields.option_label', 'Label')}
						bind:value={newOptionLabel}
						onkeydown={handleOptionKeydown}
					/>
					<button
						type="button"
						class="rounded-md border px-3 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
						style="border-color: var(--color-border)"
						onclick={addOption}
					>
						Add
					</button>
				</div>
				{#if options.length > 0}
					<div class="mt-2 flex flex-wrap gap-2">
						{#each options as opt}
							<span class="inline-flex items-center gap-1 rounded-full bg-black/5 px-3 py-1 text-xs dark:bg-surface/10">
								<span style="color: var(--color-muted)">{opt.value}</span>: {opt.label}
								<button type="button" class="ml-0.5 text-black/40 hover:text-black dark:text-white/40 dark:hover:text-white" onclick={() => removeOption(opt)}>&times;</button>
							</span>
						{/each}
					</div>
				{/if}
			</div>
		{/if}

		<!-- Number options -->
		{#if fieldType === 'number' || fieldType === 'currency' || fieldType === 'percentage'}
			<div class="rounded-md border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<span class="mb-2 block text-sm font-medium">Number constraints</span>
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="num-min" class="mb-1 block text-xs" style="color: var(--color-muted)">Min</label>
						<input id="num-min" type="number" class="w-full rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={numMin} />
					</div>
					<div>
						<label for="num-max" class="mb-1 block text-xs" style="color: var(--color-muted)">Max</label>
						<input id="num-max" type="number" class="w-full rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={numMax} />
					</div>
				</div>
				<label class="mt-3 flex items-center gap-2 text-xs" style="color: var(--color-muted)">
					<input type="checkbox" bind:checked={numIsDecimal} class="rounded" />
					Allow decimal values
				</label>
			</div>
		{/if}

		<!-- Image / file options -->
		{#if fieldType === 'image_single' || fieldType === 'image_multi' || fieldType === 'media'}
			<div class="rounded-md border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<span class="mb-2 block text-sm font-medium">File upload settings</span>
				{#if fieldType === 'image_multi'}
					<div class="mb-3">
						<label for="file-max-files" class="mb-1 block text-xs" style="color: var(--color-muted)">Max files</label>
						<input id="file-max-files" type="number" min="1" max="50" class="w-full rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={fileMaxFiles} />
					</div>
				{/if}
				<div>
					<label for="file-max-size" class="mb-1 block text-xs" style="color: var(--color-muted)">Max size (MB)</label>
					<input id="file-max-size" type="number" min="1" max="100" class="w-full rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={fileMaxSizeMb} />
				</div>
			</div>
		{/if}

		<!-- Reference options -->
		{#if fieldType === 'reference'}
			<div class="rounded-md border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<span class="mb-2 block text-sm font-medium">Reference settings</span>
				<div>
					<label for="ref-entity" class="mb-1 block text-xs" style="color: var(--color-muted)">Target entity class</label>
					<input id="ref-entity" type="text" class="w-full rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" placeholder={t('custom_fields.ref_entity_placeholder', 'e.g. content, user, space')} bind:value={refEntityClass} />
				</div>
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
				class="rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
				style="border-color: var(--color-border)"
			>
				{t('common.cancel')}
			</a>
			<div class="flex-1"></div>
			<button
				class="rounded-md border border-red-300 px-4 py-2 text-sm font-medium text-danger transition-colors hover:bg-danger disabled:opacity-50 dark:border-red-800 dark:text-danger dark:hover:bg-red-950"
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

<Modal bind:open={showDeleteModal} title={t('custom_fields.delete_confirm')} size="sm">
	<p class="text-sm" style="color: var(--color-muted)">
		{t('custom_fields.delete_warning', 'This action cannot be undone. All field data will be lost.')}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('common.cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t('common.delete')}</Button>
	</div>
</Modal>
