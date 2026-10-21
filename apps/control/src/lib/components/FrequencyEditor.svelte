<!-- §122 structured frequency editor — edits the rule's frequency jsonb
     ({mode, max_impressions, cooldown_hours, snooze_default_hours}) as
     typed inputs instead of raw JSON. -->
<script lang="ts">
	interface Props {
		value: Record<string, unknown>;
	}

	let { value = $bindable() }: Props = $props();

	const FIELDS = [
		{ key: 'max_impressions', label: 'Max impressions (0 = unlimited)' },
		{ key: 'cooldown_hours', label: 'Cooldown hours between shows' },
		{ key: 'snooze_default_hours', label: 'Snooze default hours' }
	] as const;

	function num(key: string): number | '' {
		const v = value?.[key];
		return typeof v === 'number' && !Number.isNaN(v) ? v : '';
	}

	function set(key: string, v: unknown) {
		value = { ...(value ?? {}), [key]: v };
	}
</script>

<div class="grid grid-cols-2 gap-3">
	<label class="block text-sm">
		Mode
		<select
			value={typeof value?.mode === 'string' ? value.mode : 'recurring'}
			onchange={(e) => set('mode', (e.target as HTMLSelectElement).value)}
			class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5"
		>
			<option value="recurring">recurring</option>
			<option value="once">once</option>
		</select>
	</label>
	{#each FIELDS as f}
		<label class="block text-sm">
			{f.label}
			<input
				type="number"
				min="0"
				value={num(f.key)}
				oninput={(e) => {
					const raw = (e.target as HTMLInputElement).value;
					set(f.key, raw === '' ? 0 : Number(raw));
				}}
				class="border-border bg-background mt-1 w-full rounded-[var(--radius)] border px-3 py-1.5"
			/>
		</label>
	{/each}
</div>
<p class="text-muted-foreground mt-2 text-xs">
	<code>once</code> shows the rule a single time per user; <code>recurring</code> re-shows after cooldown. Recurring rules with push channels require cooldown_hours &gt; 0.
</p>
