<script lang="ts">
	// KeyValueEditor — a structured editor for JSON object settings that map
	// a string key to either a string value or an array of strings.
	//
	// Two modes:
	//   - valueMode = "string": each row is key → text input (e.g.
	//     geo_personalization.tool_labels maps slug → "Display Label",
	//     geo_personalization.tool_urls maps slug → "/frontend/url")
	//   - valueMode = "csv": each row is key → comma-separated list (e.g.
	//     geo_personalization.stage_tool_matrix maps "archetype:stage" →
	//     ["tool-slug-1", "tool-slug-2", ...])
	//
	// The setting value is stored as a JSON object string. This component
	// parses it into rows, lets the user edit/add/remove rows, and emits the
	// serialized JSON back to the parent.
	//
	// Ref: japaconnect-engineering/adr/ADR-064a-geo-aware-recommendations.md
	// Ref: japaconnect-engineering/conventions/system-settings-catalog.md
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';

	interface KVRow {
		key: string;
		value: string;
	}

	let {
		value,
		valueMode = 'string',
		keyPlaceholder = 'key',
		valuePlaceholder = 'value',
		disabled = false,
		onchange
	}: {
		value: string;
		valueMode?: 'string' | 'csv';
		keyPlaceholder?: string;
		valuePlaceholder?: string;
		disabled?: boolean;
		onchange: (next: string) => void;
	} = $props();

	let rows = $state<KVRow[]>([]);
	let lastEmitted = '';

	// Re-parse when the external value changes (e.g. reset to default) and
	// the serialized form differs from what we last emitted (avoids loops).
	$effect(() => {
		const serialized = JSON.stringify(value ?? '{}');
		if (serialized === lastEmitted) return;
		rows = parseRows(value, valueMode);
	});

	function parseRows(v: string, mode: 'string' | 'csv'): KVRow[] {
		if (!v) return [];
		try {
			const obj = JSON.parse(v);
			if (typeof obj !== 'object' || obj === null || Array.isArray(obj)) return [];
			return Object.entries(obj).map(([k, val]) => ({
				key: k,
				value: mode === 'csv'
					? (Array.isArray(val) ? val.join(', ') : String(val ?? ''))
					: String(val ?? '')
			}));
		} catch {
			return [];
		}
	}

	function emit(next: KVRow[]) {
		rows = next;
		// Build the JSON object. For csv mode, split the value into an array.
		const obj: Record<string, unknown> = {};
		for (const row of next) {
			if (!row.key.trim()) continue;
			if (valueMode === 'csv') {
				obj[row.key] = row.value
					.split(',')
					.map((s) => s.trim())
					.filter(Boolean);
			} else {
				obj[row.key] = row.value;
			}
		}
		const serialized = JSON.stringify(obj);
		lastEmitted = serialized;
		onchange(serialized);
	}

	function updateKey(index: number, key: string) {
		emit(rows.map((r, i) => (i === index ? { ...r, key } : r)));
	}

	function updateValue(index: number, val: string) {
		emit(rows.map((r, i) => (i === index ? { ...r, value: val } : r)));
	}

	function removeRow(index: number) {
		emit(rows.filter((_, i) => i !== index));
	}

	function addRow() {
		emit([...rows, { key: '', value: '' }]);
	}
</script>

<div class="w-full max-w-lg space-y-1.5">
	{#if rows.length === 0}
		<p class="text-sm text-muted-foreground">
			{t('settings.kv_empty', 'No entries configured. Click Add to create one.')}
		</p>
	{/if}

	{#each rows as row, i (i)}
		<div class="flex items-center gap-2">
			<input
				type="text"
				value={row.key}
				placeholder={keyPlaceholder}
				class="h-8 w-40 shrink-0 rounded-md border border-border-input bg-background px-2 py-1 font-mono text-xs"
				disabled={disabled}
				onchange={(e) => updateKey(i, (e.currentTarget as HTMLInputElement).value)}
			/>
			<span class="shrink-0 text-muted-foreground">→</span>
			<input
				type="text"
				value={row.value}
				placeholder={valuePlaceholder}
				class="h-8 flex-1 rounded-md border border-border-input bg-background px-2 py-1 text-xs"
				class:font-mono={valueMode === 'string'}
				disabled={disabled}
				onchange={(e) => updateValue(i, (e.currentTarget as HTMLInputElement).value)}
			/>
			<button
				type="button"
				class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-red-500/10 hover:text-red-500 disabled:opacity-30"
				disabled={disabled}
				onclick={() => removeRow(i)}
				aria-label={t('common.remove', 'Remove')}
			>
				<Trash2 class="h-3.5 w-3.5" />
			</button>
		</div>
	{/each}

	<button
		type="button"
		class="flex w-full items-center justify-center gap-1.5 rounded-lg border border-dashed border-border-input px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted/50 disabled:opacity-50"
		disabled={disabled}
		onclick={addRow}
	>
		<Plus class="h-3.5 w-3.5" />
		{t('settings.kv_add', 'Add entry')}
	</button>
</div>
