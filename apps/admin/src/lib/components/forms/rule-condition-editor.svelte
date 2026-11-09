<script lang="ts">
	// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md (ADR-137)
	// Clause-list editor for visible_if / jump.when conditions. Emits the
	// `all` clause array shape; `any` groups remain JSON-editable elsewhere.
	// Value inputs adapt to the referenced field: canonical option values
	// (option_defs) for selects, true/false for booleans, free text otherwise.
	import { t } from '#lib/stores/i18n.svelte.js';
	import { Select } from '#lib/components/ui/select/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Plus, X } from 'lucide-svelte';

	export interface RuleFieldOption {
		key: string;
		label: string;
		type?: string;
		option_defs?: { value: string; label: string }[];
		/** Plain option labels — used as values when option_defs is absent
		 * (admin-authored options store value === label). */
		options?: string[];
	}
	export interface ClauseRow {
		field: string;
		op: string;
		value: string;
	}

	let {
		clauses = $bindable<ClauseRow[]>([]),
		fields = [] as RuleFieldOption[],
		/** Label for the "" field option (jump.when self-reference). */
		selfLabel = '',
	}: {
		clauses?: ClauseRow[];
		fields?: RuleFieldOption[];
		selfLabel?: string;
	} = $props();

	const OPS = [
		{ value: 'eq', label: 'is' },
		{ value: 'neq', label: 'is not' },
		{ value: 'in', label: 'is one of' },
		{ value: 'contains', label: 'includes' },
		{ value: 'answered', label: 'is answered' },
		{ value: 'not_answered', label: 'is not answered' },
	];
	const NO_VALUE_OPS = new Set(['answered', 'not_answered']);
	const BOOL_TYPES = new Set(['boolean', 'checkbox']);

	const fieldSelectOptions = $derived([
		...(selfLabel ? [{ value: '', label: selfLabel }] : []),
		...fields.map((f) => ({ value: f.key, label: f.label || f.key })),
	]);

	function fieldFor(key: string): RuleFieldOption | undefined {
		return fields.find((f) => f.key === key);
	}

	function valueOptions(row: ClauseRow): { value: string; label: string }[] | null {
		const f = fieldFor(row.field);
		if (f?.option_defs?.length) return f.option_defs.map((o) => ({ value: o.value, label: o.label }));
		if (f?.options?.length) return f.options.map((o) => ({ value: o, label: o }));
		if (f?.type && BOOL_TYPES.has(f.type)) {
			return [
				{ value: 'true', label: t('common.yes', 'Yes') },
				{ value: 'false', label: t('common.no', 'No') },
			];
		}
		return null;
	}

	function addClause() {
		clauses = [...clauses, { field: selfLabel ? '' : (fields[0]?.key ?? ''), op: 'eq', value: '' }];
	}

	function removeClause(i: number) {
		clauses = clauses.filter((_, j) => j !== i);
	}
</script>

<div class="space-y-2">
	{#each clauses as row, i (i)}
		<div class="flex items-center gap-2">
			<div class="min-w-0 flex-1">
				<Select options={fieldSelectOptions} bind:value={row.field} />
			</div>
			<div class="w-32 shrink-0">
				<Select options={OPS} bind:value={row.op} />
			</div>
			{#if !NO_VALUE_OPS.has(row.op)}
				<div class="min-w-0 flex-1">
					{#if valueOptions(row)}
						<Select options={valueOptions(row) ?? []} bind:value={row.value} />
					{:else}
						<input
							type="text"
							class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
							bind:value={row.value}
							placeholder={row.op === 'in' ? t('forms.logicValueList', 'a,b,c') : t('forms.logicValue', 'Value')}
						/>
					{/if}
				</div>
			{/if}
			<Button variant="ghost" size="sm" style="color: var(--color-danger)" onclick={() => removeClause(i)}>
				<X class="h-3.5 w-3.5" />
			</Button>
		</div>
	{/each}
	<div>
		<Button variant="outline" size="sm" onclick={addClause}>
			<Plus class="me-1 h-3.5 w-3.5" /> {t('forms.logicAddClause', 'Add condition')}
		</Button>
	</div>
</div>
