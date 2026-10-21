<!-- §122 structured audience editor — edits the rule's audience jsonb
     ({user_class, surfaces, archetype, japa_stage, destination_country,
     origin_country}) as typed inputs instead of raw JSON. Bounded
     vocabularies are checkboxes; free-form lists are comma-separated. -->
<script lang="ts">
	interface Props {
		value: Record<string, unknown>;
	}

	let { value = $bindable() }: Props = $props();

	const USER_CLASSES = ['guest', 'regular', 'pro', 'business'];
	const SURFACES = ['main', 'tools', 'academy', 'docs', 'mobile'];

	// Free-form audience axes — validated only for shape server-side.
	const LIST_FIELDS = [
		{ key: 'archetype', label: 'Archetypes', placeholder: 'aspiring, …' },
		{ key: 'japa_stage', label: 'Japa stages', placeholder: 'researching, …' },
		{ key: 'destination_country', label: 'Destination countries', placeholder: 'CA, GB, …' },
		{ key: 'origin_country', label: 'Origin countries', placeholder: 'NG, GH, …' }
	] as const;

	function asList(key: string): string[] {
		const v = value?.[key];
		return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
	}

	function setList(key: string, list: string[]) {
		const next = { ...(value ?? {}) };
		if (list.length === 0) delete next[key];
		else next[key] = list;
		value = next;
	}

	function toggle(key: string, item: string, on: boolean) {
		const cur = asList(key);
		setList(key, on ? [...new Set([...cur, item])] : cur.filter((x) => x !== item));
	}

	function onText(key: string, raw: string) {
		setList(
			key,
			raw
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean)
		);
	}
</script>

<div class="space-y-3">
	<div>
		<span class="text-muted-foreground text-xs">User classes</span>
		<div class="mt-1 flex flex-wrap gap-3">
			{#each USER_CLASSES as uc}
				<label class="flex items-center gap-1 text-sm">
					<input
						type="checkbox"
						checked={asList('user_class').includes(uc)}
						onchange={(e) => toggle('user_class', uc, (e.target as HTMLInputElement).checked)}
					/>
					{uc}
				</label>
			{/each}
		</div>
	</div>
	<div>
		<span class="text-muted-foreground text-xs">Surfaces</span>
		<div class="mt-1 flex flex-wrap gap-3">
			{#each SURFACES as sf}
				<label class="flex items-center gap-1 text-sm">
					<input
						type="checkbox"
						checked={asList('surfaces').includes(sf)}
						onchange={(e) => toggle('surfaces', sf, (e.target as HTMLInputElement).checked)}
					/>
					{sf}
				</label>
			{/each}
		</div>
	</div>
	<div class="grid grid-cols-2 gap-3">
		{#each LIST_FIELDS as f}
			<label class="block text-sm">
				{f.label}
				<input
					value={asList(f.key).join(', ')}
					oninput={(e) => onText(f.key, (e.target as HTMLInputElement).value)}
					placeholder={f.placeholder}
					class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5 font-mono text-xs"
				/>
			</label>
		{/each}
	</div>
	<p class="text-muted-foreground text-xs">All lists empty = everyone. Checked user classes including <code>guest</code> restrict the rule to pull-render channels.</p>
</div>
