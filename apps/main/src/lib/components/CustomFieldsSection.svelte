<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { api, type CustomField, type CustomFieldGroup } from '$lib/api';

	interface Props {
		entityType: string;
		entityId?: string;
		onValuesChange?: (values: Record<string, unknown>) => void;
	}

	let { entityType, entityId, onValuesChange }: Props = $props();

	let groups = $state<CustomFieldGroup[]>([]);
	let values = $state<Record<string, unknown>>({});
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			const result = await api.getCustomFieldGroups(entityType);
			groups = result ?? [];

			// Load field definitions for each group
			await Promise.all(
				groups.map(async (group, i) => {
					const fieldsResult = await api.getCustomFieldGroupFields(group.id);
					groups[i] = { ...group, fields: fieldsResult ?? [] };
				})
			);

			// If editing an existing entity, preload saved values. The backend
			// returns rows keyed by field_id with typed value columns.
			if (entityId) {
				const saved = await api.getCustomFieldValues(entityType, entityId);
				const idToKey = new Map<string, string>();
				for (const g of groups) {
					for (const f of g.fields ?? []) idToKey.set(f.id, f.key);
				}
				const loaded: Record<string, unknown> = {};
				for (const row of saved ?? []) {
					const key = idToKey.get(row.field_id);
					if (!key) continue;
					loaded[key] =
						row.value_text ??
						row.value_number ??
						row.value_bool ??
						row.value_date ??
						undefined;
				}
				values = loaded;
			}
		} catch {
			error = 'Failed to load custom fields';
		} finally {
			loading = false;
		}
	});

	function updateValue(key: string, value: unknown) {
		values = { ...values, [key]: value };
		onValuesChange?.(values);
	}

	function fieldInputType(field: CustomField): string {
		switch (field.field_type) {
			case 'number': return 'number';
			case 'date': return 'date';
			case 'datetime': return 'datetime-local';
			case 'email': return 'email';
			case 'url': return 'url';
			case 'color': return 'color';
			default: return 'text';
		}
	}
</script>

{#if loading}
	<div class="flex items-center gap-2 py-3 text-sm" style="color: var(--color-muted)">
		<div class="h-4 w-4 animate-spin rounded-full border-2 border-t-transparent"
			style="border-color: var(--color-primary); border-top-color: transparent"></div>
		Loading custom fields…
	</div>
{:else if error}
	<p class="text-sm text-red-500">{error}</p>
{:else if groups.length > 0}
	<div class="space-y-6">
		{#each groups as group (group.id)}
			{#if (group.fields ?? []).length > 0}
				<fieldset class="rounded-xl border p-4"
					style="border-color: var(--color-border); background: var(--color-surface)">
					<legend class="px-1 text-sm font-semibold" style="color: var(--color-foreground)">
						{group.name}
					</legend>
					<div class="mt-3 grid gap-4 sm:grid-cols-2">
						{#each group.fields ?? [] as field (field.id)}
							<div class="flex flex-col gap-1">
								<label for="cf_{field.key}" class="text-sm font-medium"
									style="color: var(--color-foreground)">
									{field.label}{field.required ? ' *' : ''}
								</label>

								{#if field.field_type === 'textarea' || field.field_type === 'rich_text'}
									<textarea
										id="cf_{field.key}"
										name="custom_fields[{field.key}]"
										placeholder={field.placeholder ?? ''}
										required={field.required}
										rows="3"
										class="rounded-lg border px-3 py-2 text-sm transition-colors focus:outline-none focus:ring-2"
										style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
										value={String(values[field.key] ?? '')}
										oninput={(e) => updateValue(field.key, (e.target as HTMLTextAreaElement).value)}
									></textarea>

								{:else if field.field_type === 'boolean'}
									<label class="flex cursor-pointer items-center gap-2">
										<input
											id="cf_{field.key}"
											type="checkbox"
											name="custom_fields[{field.key}]"
											checked={Boolean(values[field.key])}
											onchange={(e) => updateValue(field.key, (e.target as HTMLInputElement).checked)}
											class="h-4 w-4 rounded"
											style="accent-color: var(--color-primary)"
										/>
										<span class="text-sm" style="color: var(--color-muted)">{field.description ?? ''}</span>
									</label>

								{:else if field.field_type === 'select' || field.field_type === 'multiselect'}
									<select
										id="cf_{field.key}"
										name="custom_fields[{field.key}]"
										required={field.required}
										multiple={field.field_type === 'multiselect'}
										class="rounded-lg border px-3 py-2 text-sm focus:outline-none focus:ring-2"
										style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
										onchange={(e) => {
											const sel = e.target as HTMLSelectElement;
											const val = field.field_type === 'multiselect'
												? Array.from(sel.selectedOptions).map(o => o.value)
												: sel.value;
											updateValue(field.key, val);
										}}
									>
										{#if field.field_type === 'select'}
											<option value="">{t('lib.components.CustomFieldsSection.svelte.select')}</option>
										{/if}
										{#each field.options ?? [] as option}
											<option value={option}
												selected={field.field_type === 'multiselect'
													? (values[field.key] as string[] ?? []).includes(option)
													: values[field.key] === option}>
												{option}
											</option>
										{/each}
									</select>

								{:else if field.field_type === 'rating'}
									<div class="flex gap-1">
										{#each Array(5) as _, star (star)}
											<button
												type="button"
												onclick={() => updateValue(field.key, star + 1)}
												class="text-2xl transition-transform hover:scale-110"
												style="color: {Number(values[field.key] ?? 0) > star ? 'var(--color-warning, #f59e0b)' : 'var(--color-border)'}"
											>★</button>
										{/each}
									</div>

								{:else}
									<!-- text, number, date, datetime, email, url, color, phone -->
									<input
										id="cf_{field.key}"
										type={fieldInputType(field)}
										name="custom_fields[{field.key}]"
										placeholder={field.placeholder ?? ''}
										required={field.required}
										value={String(values[field.key] ?? '')}
										oninput={(e) => updateValue(field.key, (e.target as HTMLInputElement).value)}
										class="rounded-lg border px-3 py-2 text-sm transition-colors focus:outline-none focus:ring-2"
										style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
									/>
								{/if}

								{#if field.description && field.field_type !== 'boolean'}
									<p class="text-xs" style="color: var(--color-muted)">{field.description}</p>
								{/if}
							</div>
						{/each}
					</div>
				</fieldset>
			{/if}
		{/each}
	</div>
{/if}
