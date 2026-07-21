<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Select } from '$lib/components/ui/select';

	let name = $state('');
	let slug = $state('');
	let fieldType = $state('text');
	let entityType = $state('content');
	let required = $state(false);
	let searchable = $state(false);
	let sortable = $state(false);
	let showInFrontend = $state(true);
	let saving = $state(false);
	let error = $state('');

	// Options for select/multiselect: {value, label} pairs
	type FieldOption = { value: string; label: string };
	let options = $state<FieldOption[]>([]);
	let newOptionValue = $state('');
	let newOptionLabel = $state('');

	// Number options (for number, currency, percentage)
	let numMin = $state('');
	let numMax = $state('');
	let numIsDecimal = $state(false);

	// Image/file options (for image_single, image_multi, media)
	let fileMaxFiles = $state(1);
	let fileMaxSizeMb = $state(10);

	// Reference options
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
			return { value_label_pairs: options.map((o) => ({ value: o.value, label: o.label })) };
		}
		if (['number', 'currency', 'percentage'].includes(fieldType)) {
			const o: Record<string, unknown> = {};
			if (numMin) o.min = Number(numMin);
			if (numMax) o.max = Number(numMax);
			if (numIsDecimal) o.is_decimal = true;
			return Object.keys(o).length > 0 ? o : null;
		}
		if (['image_single', 'image_multi', 'media'].includes(fieldType)) {
			return {
				max_files: fileMaxFiles,
				max_size_mb: fileMaxSizeMb,
			};
		}
		if (fieldType === 'reference') {
			if (refEntityClass.trim()) {
				return { ref_entity_class: refEntityClass.trim() };
			}
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
		try {
			const body: Record<string, unknown> = {
				name: name.trim(),
				slug: slug.trim() || autoSlug(),
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
			if (fieldType === 'platform_embed') {
				body.embed_platforms = ['youtube', 'github', 'twitter', 'vimeo', 'spotify', 'figma', 'codepen', 'google_maps'];
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
			error = t('common.error');
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Create Custom Field — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('custom_fields.create_title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('custom_fields.create_description')}</p>

{#if error}
	<div class="mb-4 rounded-md border border-red-300 bg-danger px-4 py-3 text-sm text-danger dark:border-red-800 dark:bg-red-950 dark:text-danger">
		{error}
	</div>
{/if}

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
			placeholder={t('custom_fields.slug_placeholder')}
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
			Show in frontend
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
	<div class="flex gap-3 pt-2">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
			style="background-color: var(--color-primary)"
			disabled={saving}
			onclick={save}
		>
			{saving ? t('common.saving') : t('custom_fields.create')}
		</button>
		<a
			href="/custom-fields"
			class="rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
			style="border-color: var(--color-border)"
		>
			{t('common.cancel')}
		</a>
	</div>
</div>
