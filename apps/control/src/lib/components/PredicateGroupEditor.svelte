<!-- §122 structured predicate-group editor — edits a {all:[], any:[]}
     ConditionGroup against the code-registered vocabulary fetched from
     /admin/next-actions/predicates. Emits the JSON object the API expects. -->
<script lang="ts">
	import { getPredicateSpecs, type PredicateSpec } from '$lib/api/nextactions';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, X } from 'lucide-svelte';

	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	interface Props {
		value: { all?: any[]; any?: any[] };
	}

	let { value = $bindable() }: Props = $props();

	let specs = $state<PredicateSpec[]>([]);
	$effect(() => {
		getPredicateSpecs().then((r) => {
			if (r.ok && r.data) specs = r.data;
		});
	});

	const specByName = $derived(new Map(specs.map((s) => [s.name, s])));

	type Groups = { all: any[]; any: any[] };

	function ensure() {
		if (!value || typeof value !== 'object') value = {};
		if (!Array.isArray(value.all)) value.all = [];
		if (!Array.isArray(value.any)) value.any = [];
	}
	ensure();
	const groups = $derived(value as Groups);

	function addPred(group: 'all' | 'any') {
		groups[group] = [...groups[group], { predicate: specs[0]?.name ?? '' }];
	}

	function removePred(group: 'all' | 'any', i: number) {
		groups[group] = groups[group].filter((_: any, j: number) => j !== i);
	}

	function changeName(group: 'all' | 'any', i: number, name: string) {
		// Reset args when the predicate changes — stale args from the old
		// predicate would silently ship.
		groups[group][i] = { predicate: name };
		groups[group] = [...groups[group]];
	}

	function setArg(group: 'all' | 'any', i: number, key: string, raw: string, type: string) {
		const p = { ...groups[group][i] };
		if (raw === '' || raw == null) {
			delete p[key];
		} else if (type === 'number') {
			const n = Number(raw);
			if (!Number.isNaN(n)) p[key] = n;
		} else if (type === 'bool') {
			if (raw === 'true' || raw === 'false') p[key] = raw === 'true';
		} else if (type === 'string_list') {
			p[key] = raw.split(',').map((s) => s.trim()).filter(Boolean);
		} else {
			p[key] = raw;
		}
		groups[group][i] = p;
		groups[group] = [...groups[group]];
	}

	function argVal(p: any, key: string): string {
		const v = p?.[key];
		if (v == null) return '';
		if (Array.isArray(v)) return v.join(', ');
		return String(v);
	}
</script>

{#snippet groupEditor(group: 'all' | 'any', label: string)}
	<div class="border-border rounded-[var(--radius)] border p-2">
		<div class="mb-2 flex items-center justify-between">
			<span class="text-muted-foreground text-xs font-medium uppercase">{label}</span>
			<button type="button" onclick={() => addPred(group)} disabled={specs.length === 0}
				class="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs">
				<Plus size={12} /> add predicate
			</button>
		</div>
		{#each groups[group] as p, i}
			{@const spec = specByName.get(p.predicate)}
			<div class="border-border mb-2 flex flex-wrap items-center gap-2 rounded-[var(--radius)] border p-2">
				<AutocompleteSelect
					value={p.predicate}
					onValueChange={(v) => changeName(group, i, v)}
					inputClass="font-mono"
					compact
					options={specs.map((s) => ({ value: s.name, label: s.name }))}
				/>
				{#each spec?.args ?? [] as arg}
					<label class="flex items-center gap-1 text-xs">
						<span class="text-muted-foreground font-mono">{arg.key}{arg.required ? '*' : ''}</span>
						{#if arg.type === 'bool'}
							<AutocompleteSelect
								value={argVal(p, arg.key) || 'true'}
								onValueChange={(v) => setArg(group, i, arg.key, v, 'bool')}
								compact
								options={[{ value: 'true', label: 'true' }, { value: 'false', label: 'false' }]}
							/>
						{:else}
							<input
								type={arg.type === 'number' ? 'number' : 'text'}
								value={argVal(p, arg.key)}
								placeholder={arg.type === 'string_list' ? 'a, b, c' : arg.options || arg.key}
								oninput={(e) => setArg(group, i, arg.key, (e.target as HTMLInputElement).value, arg.type)}
								class="border-border bg-background w-32 rounded-[var(--radius)] border px-1.5 py-1 font-mono text-xs"
							/>
						{/if}
					</label>
				{/each}
				<button type="button" onclick={() => removePred(group, i)} aria-label={t('nextactions.remove_predicate', 'Remove predicate')}
					class="text-muted-foreground hover:text-destructive ms-auto p-0.5">
					<X size={13} />
				</button>
			</div>
		{/each}
		{#if groups[group].length === 0}
			<p class="text-muted-foreground px-1 pb-1 text-xs">{group === 'all' ? 'All must match (AND)' : 'Any may match (OR)'} — empty.</p>
		{/if}
	</div>
{/snippet}

<div class="space-y-2">
	{@render groupEditor('all', 'all — every predicate must pass')}
	{@render groupEditor('any', 'any — at least one must pass')}
</div>
