<script lang="ts">
	import type { CustomFieldDef } from '$lib/types';

	interface Props {
		fields: CustomFieldDef[];
		values: Record<string, unknown>;
		onvalues: (v: Record<string, unknown>) => void;
		onedit: () => void;
	}

	let { fields, values, onvalues, onedit }: Props = $props();

	function setField(key: string, val: unknown) {
		onvalues({ ...values, [key]: val });
		onedit();
	}

	// Repeater helpers — add/remove/move items in a string[] value.
	function repeaterAdd(key: string) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as string[])] : [];
		arr.push('');
		setField(key, arr);
	}
	function repeaterRemove(key: string, idx: number) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as string[])] : [];
		arr.splice(idx, 1);
		setField(key, arr);
	}
	function repeaterSet(key: string, idx: number, val: string) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as string[])] : [];
		arr[idx] = val;
		setField(key, arr);
	}

	// Multiselect helpers — toggle a value in/out of the array.
	function multiselectToggle(key: string, opt: string) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as string[])] : [];
		const i = arr.indexOf(opt);
		if (i >= 0) arr.splice(i, 1);
		else arr.push(opt);
		setField(key, arr);
	}
	function multiselectChecked(key: string, opt: string): boolean {
		return Array.isArray(values[key]) && (values[key] as string[]).includes(opt);
	}

	const label = (f: CustomFieldDef) => f.label + (f.is_required ? ' *' : '');
</script>

{#each fields as field (field.id)}
	<div class="cf-field">
		{#if field.field_type === 'boolean'}
			<label class="cf-checkbox">
				<input
					type="checkbox"
					checked={values[field.key] === true}
					onchange={(e) => setField(field.key, e.currentTarget.checked)}
				/>
				<span>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</span>
			</label>
		{:else if field.field_type === 'select'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<select
				id={field.key}
				class="cf-input"
				value={values[field.key] ?? ''}
				onchange={(e) => setField(field.key, e.currentTarget.value)}
			>
				<option value="">— Select —</option>
				{#each field.options ?? [] as opt (opt)}
					<option value={opt}>{opt}</option>
				{/each}
			</select>
		{:else if field.field_type === 'multiselect'}
			<span class="cf-label">{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</span>
			<div class="cf-multiselect">
				{#each field.options ?? [] as opt (opt)}
					<label class="cf-checkbox">
						<input
							type="checkbox"
							checked={multiselectChecked(field.key, opt)}
							onchange={() => multiselectToggle(field.key, opt)}
						/>
						<span>{opt}</span>
					</label>
				{/each}
			</div>
		{:else if field.field_type === 'number' || field.field_type === 'currency' || field.field_type === 'percentage' || field.field_type === 'rating'}
			<input
				type="number"
				class="cf-input"
				placeholder={label(field)}
				value={values[field.key] ?? ''}
				step={field.field_type === 'rating' ? '1' : 'any'}
				min={field.field_type === 'rating' ? '1' : undefined}
				max={field.field_type === 'rating' ? '5' : undefined}
				oninput={(e) => setField(field.key, e.currentTarget.valueAsNumber)}
			/>
		{:else if field.field_type === 'date'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="date"
				class="cf-input"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'datetime'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="datetime-local"
				class="cf-input"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'time'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="time"
				class="cf-input"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'duration'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="text"
				class="cf-input"
				placeholder="e.g. 2 weeks, 3 months, 30 days"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'textarea'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<textarea
				id={field.key}
				class="cf-input cf-textarea"
				rows="3"
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			>{values[field.key] ?? ''}</textarea>
		{:else if field.field_type === 'url'}
			<input
				type="url"
				class="cf-input"
				placeholder={label(field)}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'email'}
			<input
				type="email"
				class="cf-input"
				placeholder={label(field)}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'phone'}
			<input
				type="tel"
				class="cf-input"
				placeholder={label(field)}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'color'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="color"
				class="cf-color"
				value={values[field.key] ?? '#000000'}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'repeater'}
			<span class="cf-label">{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</span>
			<div class="cf-repeater">
				{#each (Array.isArray(values[field.key]) ? (values[field.key] as string[]) : []) as item, idx (idx)}
					<div class="cf-repeater-row">
						<input
							type="text"
							class="cf-input"
							placeholder={(field.options?.[0] ?? 'Add item...')}
							value={item}
							oninput={(e) => repeaterSet(field.key, idx, e.currentTarget.value)}
						/>
						<button type="button" class="cf-repeater-remove" onclick={() => repeaterRemove(field.key, idx)} aria-label="Remove">×</button>
					</div>
				{/each}
				<button type="button" class="cf-repeater-add" onclick={() => repeaterAdd(field.key)}>+ Add</button>
			</div>
		{:else if field.field_type === 'group'}
			<!-- Group: a labelled container for sub-fields. The sub-fields are
			rendered as separate custom_fields rows by the parent, so this is a
			visual label only. -->
			<div class="cf-group-label">{field.label}</div>
		{:else}
			<input
				type="text"
				class="cf-input"
				placeholder={label(field)}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{/if}
	</div>
{/each}

<style>
	.cf-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.cf-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.cf-required {
		color: var(--destructive, #dc2626);
		margin-left: 2px;
	}
	.cf-input {
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.cf-input::placeholder {
		color: var(--color-muted);
	}
	.cf-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.cf-textarea {
		resize: vertical;
		min-height: 60px;
		font-family: inherit;
	}
	.cf-checkbox {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.875rem;
		color: var(--color-text);
		cursor: pointer;
	}
	.cf-checkbox input {
		width: 16px;
		height: 16px;
		accent-color: var(--color-primary);
		cursor: pointer;
	}
	.cf-multiselect {
		display: flex;
		flex-direction: column;
		gap: 6px;
		max-height: 160px;
		overflow-y: auto;
		padding: 8px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
	}
	.cf-color {
		width: 100%;
		height: 40px;
		padding: 4px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		cursor: pointer;
	}
	.cf-repeater {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.cf-repeater-row {
		display: flex;
		gap: 6px;
		align-items: center;
	}
	.cf-repeater-remove {
		flex-shrink: 0;
		width: 32px;
		height: 32px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-destructive, #dc2626);
		font-size: 1.25rem;
		line-height: 1;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.cf-repeater-remove:hover {
		background: var(--color-destructive-muted, #fef2f2);
	}
	.cf-repeater-add {
		align-self: flex-start;
		padding: 4px 12px;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-muted);
		font-size: 0.8125rem;
		cursor: pointer;
	}
	.cf-repeater-add:hover {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}
	.cf-group-label {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding: 4px 0;
		border-bottom: 1px solid var(--color-border);
		margin-bottom: 2px;
	}
</style>
