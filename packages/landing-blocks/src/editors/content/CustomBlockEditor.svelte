<script lang="ts">
	// CustomBlockEditor.svelte — schema-driven content editor for custom blocks.
	// Generates a form from the block definition's `config_schema` fields.
	// Each field type maps to an appropriate input control.
	import type { CustomBlockProps, ConfigSchemaField } from '../../types';
	import { getCustomBlockDef } from '../../render/runtimeBlockRegistry';

	let {
		blockDefId,
		props,
		onchange,
		mediaPickerComponent,
	}: {
		blockDefId: string;
		props: CustomBlockProps;
		onchange: (props: CustomBlockProps) => void;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		mediaPickerComponent?: any;
	} = $props();

	const def = $derived(getCustomBlockDef(blockDefId));
	const fields = $derived(def?.config_schema?.fields ?? []);

	function update(key: string, value: unknown): void {
		onchange({ ...props, [key]: value });
	}

	function fieldValue(field: ConfigSchemaField): unknown {
		return props[field.key] ?? field.default ?? '';
	}

	/** Extract a URL string from a media field value (could be {url} or a plain string). */
	function mediaFieldValue(field: ConfigSchemaField): string {
		const v = fieldValue(field);
		if (typeof v === 'string') return v;
		if (v && typeof v === 'object' && 'url' in v) return String((v as { url?: string }).url ?? '');
		return '';
	}

	const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
	const labelClass = 'mb-1 block text-xs text-[var(--color-muted)]';
</script>

{#if !def}
	<div class="rounded border border-dashed p-3 text-sm text-[var(--color-muted)]">
		Custom block definition not found. It may have been deleted.
	</div>
{:else if fields.length === 0}
	<div class="rounded border border-dashed p-3 text-sm text-[var(--color-muted)]">
		This custom block has no configurable fields.
	</div>
{:else}
	<div class="space-y-3">
		{#each fields as field}
			<label class="block">
				<span class={labelClass}>
					{field.label}
					{#if field.required}<span class="text-[var(--color-danger)]">*</span>{/if}
				</span>

				{#if field.type === 'text'}
					<input
						type="text"
						value={String(fieldValue(field) ?? '')}
						oninput={(e) => update(field.key, e.currentTarget.value)}
						placeholder={field.placeholder ?? ''}
						class={inputClass}
					/>
				{:else if field.type === 'textarea'}
					<textarea
						value={String(fieldValue(field) ?? '')}
						oninput={(e) => update(field.key, e.currentTarget.value)}
						placeholder={field.placeholder ?? ''}
						rows="4"
						class={inputClass}
					></textarea>
				{:else if field.type === 'richtext'}
					<textarea
						value={String(fieldValue(field) ?? '')}
						oninput={(e) => update(field.key, e.currentTarget.value)}
						placeholder={field.placeholder ?? 'HTML allowed'}
						rows="6"
						class="{inputClass} font-mono text-xs"
					></textarea>
				{:else if field.type === 'number'}
					<input
						type="number"
						value={fieldValue(field) as number ?? ''}
						oninput={(e) => update(field.key, e.currentTarget.value ? Number(e.currentTarget.value) : undefined)}
						placeholder={field.placeholder ?? ''}
						class={inputClass}
					/>
				{:else if field.type === 'color'}
					<div class="flex gap-2">
						<input
							type="color"
							value={String(fieldValue(field) ?? '#000000')}
							oninput={(e) => update(field.key, e.currentTarget.value)}
							class="h-8 w-12 shrink-0 rounded border border-[var(--color-border)]"
						/>
						<input
							type="text"
							value={String(fieldValue(field) ?? '')}
							oninput={(e) => update(field.key, e.currentTarget.value)}
							placeholder="#000000"
							class={inputClass}
						/>
					</div>
				{:else if field.type === 'select'}
					<select
						value={String(fieldValue(field) ?? '')}
						onchange={(e) => update(field.key, e.currentTarget.value || undefined)}
						class={inputClass}
					>
						<option value="">—</option>
						{#each field.options ?? [] as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				{:else if field.type === 'checkbox'}
					<label class="flex items-center gap-2">
						<input
							type="checkbox"
							checked={Boolean(fieldValue(field))}
							onchange={(e) => update(field.key, e.currentTarget.checked)}
							class="h-4 w-4"
						/>
						<span class="text-sm text-[var(--color-text)]">{field.help ?? 'Enabled'}</span>
					</label>
				{:else if field.type === 'url'}
					<input
						type="url"
						value={String(fieldValue(field) ?? '')}
						oninput={(e) => update(field.key, e.currentTarget.value)}
						placeholder={field.placeholder ?? 'https://'}
						class={inputClass}
					/>
				{:else if field.type === 'media'}
					{#if mediaPickerComponent}
						{@const Mp = mediaPickerComponent}
						<Mp value={fieldValue(field)} onchange={(v: unknown) => update(field.key, v)} />
					{:else}
						<input
							type="url"
							value={mediaFieldValue(field)}
							oninput={(e) => update(field.key, { url: e.currentTarget.value, alt: '' })}
							placeholder="https://image-url.jpg"
							class={inputClass}
						/>
					{/if}
				{/if}

				{#if field.help}
					<p class="mt-1 text-[10px] text-[var(--color-muted)]">{field.help}</p>
				{/if}
			</label>
		{/each}
	</div>
{/if}
