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
			<select
				class="cf-input"
				value={values[field.key] ?? ''}
				onchange={(e) => setField(field.key, e.currentTarget.value)}
			>
				<option value="">{field.label}{#if field.is_required} *{/if}</option>
				{#each field.options ?? [] as opt (opt)}
					<option value={opt}>{opt}</option>
				{/each}
			</select>
		{:else if field.field_type === 'number'}
			<input
				type="number"
				class="cf-input"
				placeholder={field.label + (field.is_required ? ' *' : '')}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.valueAsNumber)}
			/>
		{:else}
			<input
				type="text"
				class="cf-input"
				placeholder={field.label + (field.is_required ? ' *' : '')}
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
</style>
