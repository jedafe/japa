<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api } from '$lib/api';

	const spaceId = $derived($page.params.spaceId);

	type FieldDef = {
		label: string;
		type: 'text' | 'number' | 'email' | 'checkbox' | 'radio';
		required: boolean;
		options: string[];
	};

	let formName = $state('');
	let formDescription = $state('');
	let fields = $state<FieldDef[]>([{ label: '', type: 'text', required: false, options: [] }]);
	let saving = $state(false);
	let errorMessage = $state('');

	const fieldTypes = ['text', 'number', 'email', 'checkbox', 'radio'] as const;

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
			errorMessage = 'Form name is required.';
			return;
		}
		const validFields = fields.filter((f) => f.label.trim());
		if (validFields.length === 0) {
			errorMessage = 'Add at least one field with a label.';
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
						options: f.type === 'radio' ? f.options.filter((o) => o.trim()) : []
					}))
				}
			});
			if (result.ok) {
				goto(`/spaces/${spaceId}/forms/${result.data.id}`);
			} else {
				errorMessage = result.error || 'Failed to create form.';
			}
		} catch {
			errorMessage = 'Failed to create form.';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Create Form — JapaConnect Admin</title>
</svelte:head>

<div class="mb-6">
	<h1 class="text-2xl font-bold tracking-tight">Create Form</h1>
	<p class="mt-1 text-sm" style="color: var(--color-muted)">Build a new form for this space</p>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="space-y-6">
	<!-- Form metadata -->
	<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Form Details</h2>
		<div class="space-y-4">
			<div>
				<label for="form-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Name *</label>
				<input
					id="form-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="e.g. Application Form"
					bind:value={formName}
				/>
			</div>
			<div>
				<label for="form-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Description</label>
				<textarea
					id="form-desc"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="2"
					placeholder="Optional description"
					bind:value={formDescription}
				></textarea>
			</div>
		</div>
	</div>

	<!-- Fields -->
	<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="mb-4 flex items-center justify-between">
			<h2 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Fields</h2>
			<button
				class="rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				style="border-color: var(--color-border)"
				onclick={addField}
			>
				+ Add Field
			</button>
		</div>

		<div class="space-y-4">
			{#each fields as field, i}
				<div class="rounded-md border p-4" style="border-color: var(--color-border); background-color: var(--color-background)">
					<div class="mb-3 flex items-center justify-between">
						<span class="text-xs font-medium" style="color: var(--color-muted)">Field {i + 1}</span>
						{#if fields.length > 1}
							<button
								class="text-xs hover:underline"
								style="color: var(--color-danger)"
								onclick={() => removeField(i)}
							>
								Remove
							</button>
						{/if}
					</div>

					<div class="grid gap-3 sm:grid-cols-2">
						<div>
							<label for="field-label-{i}" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Label *</label>
							<input
								id="field-label-{i}"
								type="text"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background-color: var(--color-surface)"
								placeholder="e.g. Full Name"
								bind:value={field.label}
							/>
						</div>
						<div>
							<label for="field-type-{i}" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Type</label>
							<select
								id="field-type-{i}"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background-color: var(--color-surface)"
								bind:value={field.type}
							>
								{#each fieldTypes as ft}
									<option value={ft}>{ft}</option>
								{/each}
							</select>
						</div>
					</div>

					<div class="mt-3 flex items-center gap-4">
						<label class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
							<input type="checkbox" bind:checked={field.required} class="rounded" />
							Required
						</label>
					</div>

					{#if field.type === 'radio'}
						<div class="mt-3">
							<div class="mb-2 flex items-center justify-between">
								<span class="text-xs font-medium" style="color: var(--color-muted)">Options</span>
								<button
									class="text-xs hover:underline"
									style="color: var(--color-info)"
									onclick={() => addOption(i)}
								>
									+ Add Option
								</button>
							</div>
							<div class="space-y-2">
								{#each field.options as opt, oi}
									<div class="flex items-center gap-2">
										<input
											type="text"
											class="flex-1 rounded-md border px-3 py-1.5 text-sm"
											style="border-color: var(--color-border); background-color: var(--color-surface)"
											placeholder="Option text"
											value={opt}
											oninput={(e) => updateOption(i, oi, (e.target as HTMLInputElement).value)}
										/>
										<button
											class="text-xs hover:underline"
											style="color: var(--color-danger)"
											onclick={() => removeOption(i, oi)}
										>
											Remove
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
			{saving ? 'Creating...' : 'Create Form'}
		</button>
		<a
			href="/spaces/{spaceId}/forms"
			class="rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="border-color: var(--color-border)"
		>
			Cancel
		</a>
	</div>
</div>
