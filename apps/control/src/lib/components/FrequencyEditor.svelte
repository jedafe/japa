<!-- §122 structured frequency editor — edits the rule's frequency jsonb
     ({mode, max_impressions, cooldown_hours, snooze_default_hours}) as
     typed inputs instead of raw JSON. -->
<script lang="ts">
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

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
		<AutocompleteSelect
			value={typeof value?.mode === 'string' ? value.mode : 'recurring'}
			onValueChange={(v) => set('mode', v)}
			class="mt-1"
			options={[{ value: 'recurring', label: 'recurring' }, { value: 'once', label: 'once' }]}
		/>
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
