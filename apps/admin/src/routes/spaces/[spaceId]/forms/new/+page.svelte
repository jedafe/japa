<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, adminCatalogApi, type CatalogOption } from '$lib/api';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { t } from '$lib/stores/i18n.svelte';

	const spaceId = $derived(page.params.spaceId);

	type FieldDef = {
		label: string;
		type: string;
		required: boolean;
		options: string[];
	};

	let formName = $state('');
	let formDescription = $state('');
	let fields = $state<FieldDef[]>([{ label: '', type: 'text', required: false, options: [] }]);
	let saving = $state(false);
	let errorMessage = $state('');

	// Field types come from the admin catalog (same subset the backend's
	// CreateSpaceForm path accepts); fall back to the minimal set if the
	// catalog call fails.
	let fieldTypeOptions = $state<CatalogOption[]>([
		{ value: 'text', label: 'Text' },
		{ value: 'number', label: 'Number' },
		{ value: 'email', label: 'Email' },
		{ value: 'boolean', label: 'Yes / No' },
		{ value: 'select', label: 'Select (dropdown)' },
		{ value: 'textarea', label: 'Long text' },
		{ value: 'url', label: 'URL' }
	]);

	const fieldTypeFetcher = (q: string): AutocompleteItem[] =>
		fieldTypeOptions
			.filter((o) => o.label.toLowerCase().includes(q.toLowerCase()) || o.value.includes(q.toLowerCase()))
			.map((o) => ({ id: o.value, label: o.label }));

	async function loadFieldTypes() {
		try {
			const res = await adminCatalogApi.get('forms.field_type');
			if (res.ok && res.data?.length) fieldTypeOptions = res.data;
		} catch {
			// fallback stays
		}
	}

	function addField() {
		fields = [...fields, { label: '', type: 'text', required: false, options: [] }];
	}

	function removeField(index: number) {
		fields = fields.filter((_, i) => i !== index);
	}

	function addOption(fieldIndex: number) {
		fields[fieldIndex].options = [...fields[fieldIndex].options, ''];
		fields = fields;
	}

	function removeOption(fieldIndex: number, optIndex: number) {
		fields[fieldIndex].options = fields[fieldIndex].options.filter((_, i) => i !== optIndex);
		fields = fields;
	}

	function updateOption(fieldIndex: number, optIndex: number, value: string) {
		fields[fieldIndex].options[optIndex] = value;
		fields = fields;
	}

	async function save() {
		if (!formName.trim()) {
			errorMessage = t('space_forms.name_required');
			return;
		}
		const validFields = fields.filter((f) => f.label.trim());
		if (validFields.length === 0) {
			errorMessage = t('space_forms.fields_required');
			return;
		}
		// A select field with no options can never collect a value.
		if (validFields.some((f) => f.type === 'select' && !f.options.some((o) => o.trim()))) {
			errorMessage = t('space_forms.options_required', 'Select fields need at least one option');
			return;
		}

		saving = true;
		errorMessage = '';
		try {
			const result = await api<{ id: string }>(`/spaces/${spaceId}/forms`, {
				method: 'POST',
				body: {
					name: formName.trim(),
					description: formDescription.trim(),
					fields: validFields.map((f, i) => ({
						label: f.label.trim(),
						type: f.type,
						required: f.required,
						sort_order: i,
						options: f.type === 'select' ? f.options.filter((o) => o.trim()) : []
					}))
				}
			});
			if (result.ok) {
				goto(`/spaces/${spaceId}/forms/${result.data.id}`);
			} else {
				errorMessage = result.error || t('space_forms.create_error');
			}
		} catch {
			errorMessage = t('space_forms.create_error');
		} finally {
			saving = false;
		}
	}

	$effect(() => {
		loadFieldTypes();
	});
</script>

<svelte:head>
	<title>Create Form — {siteName()} Admin</title>
</svelte:head>

<div class="mb-6">
	<h1 class="text-2xl font-bold tracking-tight">{t('space_forms.create_title')}</h1>
	<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('space_forms.create_description')}</p>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="space-y-6">
	<!-- Form metadata -->
	<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('space_forms.sections.details')}</h2>
		<div class="space-y-4">
			<div>
				<label for="form-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space_forms.field_label')} *</label>
				<p class="mt-1 block text-xs" style="color: var(--color-muted)">{t('spaces.form_name_hint', 'The form name admins and users see.')}</p>
				<input
					id="form-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder={t('space_forms.name_placeholder')}
					bind:value={formName}
				/>
			</div>
			<div>
				<label for="form-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space_forms.description_label')}</label>
				<p class="mt-1 block text-xs" style="color: var(--color-muted)">{t('spaces.form_desc_hint', 'What this form collects.')}</p>
				<textarea
					id="form-desc"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="2"
					placeholder={t('space_forms.desc_placeholder')}
					bind:value={formDescription}
				></textarea>
			</div>
		</div>
	</div>

	<!-- Fields -->
	<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="mb-4 flex items-center justify-between">
			<h2 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('space_forms.sections.fields')}</h2>
			<button
				class="rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
				style="border-color: var(--color-border)"
				onclick={addField}
			>
				{t('space_forms.add_field')}
			</button>
		</div>

		<div class="space-y-4">
			{#each fields as field, i (i)}
				<div class="rounded-md border p-4" style="border-color: var(--color-border); background-color: var(--color-background)">
					<div class="mb-3 flex items-center justify-between">
						<span class="text-xs font-medium" style="color: var(--color-muted)">{t('space_forms.field_counter').replace('{i}', String(i + 1))}</span>
						{#if fields.length > 1}
							<button
								class="text-xs hover:underline"
								style="color: var(--color-danger)"
								onclick={() => removeField(i)}
							>
							{t('space_forms.remove')}
										</button>
						{/if}
					</div>

					<div class="grid gap-3 sm:grid-cols-2">
						<div>
							<label for="field-label-{i}" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space_forms.field_label')} *</label>
							<input
								id="field-label-{i}"
								type="text"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background-color: var(--color-surface)"
								placeholder={t('space_forms.label_placeholder')}
								bind:value={field.label}
							/>
						</div>
						<div>
						<label for="field-type-{i}" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space_forms.field_type')}</label>
						<Autocomplete
							id="field-type-{i}"
							placeholder={t('space_forms.field_type')}
							emptyMessage={t('common.no_data', 'No matching types')}
							fetcher={async (q: string) => fieldTypeFetcher(q)}
							initialFetcher={async () => fieldTypeFetcher('')}
							selectedId={field.type || null}
							selectedLabel={fieldTypeOptions.find((o) => o.value === field.type)?.label ?? field.type}
							onSelect={(item: AutocompleteItem) => (field.type = item.id)}
						/>
						</div>
					</div>

					<div class="mt-3 flex items-center gap-4">
						<label class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
							<input type="checkbox" bind:checked={field.required} class="rounded" />
						{t('space_forms.field_required')}
					</label>
					</div>

					{#if field.type === 'select'}
						<div class="mt-3">
							<div class="mb-2 flex items-center justify-between">
								<span class="text-xs font-medium" style="color: var(--color-muted)">{t('space_forms.field_options')}</span>
								<button
									class="text-xs hover:underline"
									style="color: var(--color-info)"
									onclick={() => addOption(i)}
								>
									{t('space_forms.add_option')}
								</button>
							</div>
							<div class="space-y-2">
								{#each field.options as opt, oi (oi)}
									<div class="flex items-center gap-2">
										<input
											type="text"
											class="flex-1 rounded-md border px-3 py-1.5 text-sm"
											style="border-color: var(--color-border); background-color: var(--color-surface)"
											placeholder={t('space_forms.option_placeholder')}
											value={opt}
											oninput={(e) => updateOption(i, oi, (e.target as HTMLInputElement).value)}
										/>
										<button
											class="text-xs hover:underline"
											style="color: var(--color-danger)"
											onclick={() => removeOption(i, oi)}
										>
							{t('space_forms.remove')}
										</button>
									</div>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	</div>

	<!-- Actions -->
	<div class="flex items-center gap-3">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
			style="background-color: var(--color-primary)"
			disabled={saving}
			onclick={save}
		>
			{saving ? t('space_forms.creating') : t('space_forms.create_form')}
		</button>
		<a
			href="/spaces/{spaceId}/forms"
			class="rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
			style="border-color: var(--color-border)"
		>
			{t('common.cancel')}
		</a>
	</div>
</div>
