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
	<div class="flex flex-col gap-1">
		<label class="text-sm font-medium text-[var(--color-text)]" for="cf-{field.id}">
			{field.label}{#if field.is_required}<span class="text-red-500">*</span>{/if}
		</label>
		{#if field.field_type === 'boolean'}
			<input
				id="cf-{field.id}"
				type="checkbox"
				checked={values[field.key] === true}
				onchange={(e) => setField(field.key, e.currentTarget.checked)}
			/>
		{:else if field.field_type === 'select'}
			<select
				id="cf-{field.id}"
				class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
				value={values[field.key] ?? ''}
				onchange={(e) => setField(field.key, e.currentTarget.value)}
			>
				<option value=""></option>
				{#each field.options ?? [] as opt (opt)}
					<option value={opt}>{opt}</option>
				{/each}
			</select>
		{:else if field.field_type === 'number'}
			<input
				id="cf-{field.id}"
				type="number"
				class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.valueAsNumber)}
			/>
		{:else}
			<input
				id="cf-{field.id}"
				type="text"
				class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm outline-none"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{/if}
		{#if field.help_text}
			<span class="text-xs text-[var(--color-muted)]">{field.help_text}</span>
		{/if}
	</div>
{/each}
