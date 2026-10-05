<script lang="ts">
	import { clsx, type ClassValue } from 'clsx';
	import type { FieldDef } from './index';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let {
		fields,
		value = $bindable({}),
		errors = {},
		disabled = false,
		editing = false,
		onsubmit,
		class: className
	}: {
		fields: FieldDef[];
		value?: Record<string, any>;
		errors?: Record<string, string>;
		disabled?: boolean;
		/** The row being edited (null on create) — truthy drives
		 *  readonlyInEdit and feeds autocomplete fetcher context. */
		editing?: any | null;
		onsubmit?: () => void;
		class?: ClassValue;
	} = $props();

	// Input types that map directly to a native <input type="..."> element.
	const inputTypes = new Set([
		'text', 'email', 'password', 'url', 'date', 'datetime-local', 'number', 'color'
	]);

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		onsubmit?.();
	}

	// excludeSelf fields must not offer the editing row itself as an
	// option (e.g. a parent template picker on the row's own edit form).
	function filterSelf(
		field: Extract<FieldDef, { type: 'autocomplete' }>,
		items: { id: string; label: string; sublabel?: string }[]
	) {
		const selfId = field.excludeSelf && editing ? String(editing.id ?? '') : '';
		return selfId ? items.filter((i) => i.id !== selfId) : items;
	}

	// hideInEdit fields are omitted from the edit form but still seeded into
	// formValue by EntityPage.openEdit, so their values reach the update body.
	const visibleFields = $derived(
		editing ? fields.filter((f) => !f.hideInEdit) : fields
	);
</script>

<form class={clsx('flex flex-col gap-4', className)} onsubmit={handleSubmit}>
	{#each visibleFields as field (field.key)}
		<div class="flex flex-col gap-1">
			<label for={field.key} class="text-sm font-medium" style="color: var(--color-foreground)">
				{field.label}{#if field.required}<span style="color: var(--color-danger)">*</span>{/if}
			</label>

			{#if field.type === 'textarea'}
				<textarea
					id={field.key}
					aria-label={field.label}
					placeholder={field.placeholder}
					disabled={disabled}
					readonly={field.readonlyInEdit && editing}
					value={value[field.key] ?? ''}
					oninput={(e) => value[field.key] = e.currentTarget.value}
					class="rounded-lg border p-2 text-sm focus:outline-none focus:ring-2 disabled:cursor-not-allowed disabled:opacity-50 read-only:cursor-not-allowed read-only:opacity-70"
					style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
				></textarea>
			{:else if field.type === 'select'}
				<!-- Select fields render through Autocomplete (static options filtered
				     client-side) — every dropdown uses the shared Autocomplete. -->
				{@const opts = field.options ?? []}
				{@const selVal = (value[field.key] ?? '') as string}
				<div class:opacity-50={disabled} class:pointer-events-none={disabled}>
					<Autocomplete
						id={field.key}
						placeholder={field.placeholder ?? field.label}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={async (q: string) => {
							const needle = q.trim().toLowerCase();
							return opts
								.filter((o) => !needle || o.label.toLowerCase().includes(needle) || o.value.toLowerCase().includes(needle))
								.map((o) => ({ id: o.value, label: o.label }));
						}}
						initialFetcher={async () => opts.map((o) => ({ id: o.value, label: o.label }))}
						selectedId={selVal || null}
						selectedLabel={selVal ? (opts.find((o) => o.value === selVal)?.label ?? selVal) : null}
						onSelect={(item: AutocompleteItem) => { value[field.key] = item.id; }}
						onClear={() => { value[field.key] = ''; }}
					/>
				</div>
			{:else if field.type === 'autocomplete'}
				{@const acVal = value[field.key] as string | undefined}
				<Autocomplete
					id={field.key}
					placeholder={field.placeholder}
					emptyMessage={field.emptyMessage}
					fetcher={async (q: string) => filterSelf(field, await field.fetcher(q))}
					initialFetcher={field.initialFetcher ? async () => filterSelf(field, await field.initialFetcher!()) : undefined}
					selectedId={acVal ?? null}
					selectedLabel={acVal ? field.getSelectedLabel(acVal) : null}
					onSelect={(item: AutocompleteItem) => { value[field.key] = item.id; }}
					onClear={() => { value[field.key] = ''; }}
				/>
			{:else if field.type === 'checkbox'}
				<input
					id={field.key}
					type="checkbox"
					aria-label={field.label}
					disabled={disabled}
					checked={value[field.key] ?? false}
					onchange={(e) => value[field.key] = e.currentTarget.checked}
					class="h-4 w-4 rounded disabled:cursor-not-allowed disabled:opacity-50"
					style="accent-color: var(--color-primary)"
				/>
			{:else if field.type === 'color'}
				<input
					id={field.key}
					type="color"
					aria-label={field.label}
					disabled={disabled}
					value={value[field.key] ?? ''}
					oninput={(e) => value[field.key] = e.currentTarget.value}
					class="h-10 w-full cursor-pointer rounded-lg border p-1 focus:outline-none focus:ring-2 disabled:cursor-not-allowed disabled:opacity-50"
					style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
				/>
			{:else if inputTypes.has(field.type)}
				<input
					id={field.key}
					type={field.type}
					aria-label={field.label}
					placeholder={field.placeholder}
					disabled={disabled}
					readonly={field.readonlyInEdit && editing}
					min={field.min}
					max={field.max}
					step={field.step}
					value={value[field.key] ?? ''}
					oninput={(e) => value[field.key] = field.type === 'number' ? e.currentTarget.valueAsNumber : e.currentTarget.value}
					class="rounded-lg border p-2 text-sm focus:outline-none focus:ring-2 disabled:cursor-not-allowed disabled:opacity-50 read-only:cursor-not-allowed read-only:opacity-70"
					style="border-color: var(--color-border-input); background-color: var(--color-surface); color: var(--color-foreground)"
				/>
			{/if}

			{#if field.help}
				<span class="text-xs" style="color: var(--color-muted)">{field.help}</span>
			{/if}

			{#if errors[field.key]}
				<span class="text-xs" style="color: var(--color-danger)">{errors[field.key]}</span>
			{/if}
		</div>
	{/each}
</form>
