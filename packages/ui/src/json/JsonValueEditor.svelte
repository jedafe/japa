<script lang="ts">
	// JsonValueEditor — user-friendly replacement for raw JSON textareas.
	// Binds a JSON *string* (same contract as the textareas it replaces):
	// renders objects as key/value rows, arrays as reorderable repeaters, and
	// scalars as typed inputs. A "Raw" toggle keeps a power-user escape hatch.
	import JsonNode from './JsonNode.svelte';
	import { Braces, List, Type, Hash, ToggleLeft, Pencil, LayoutList } from 'lucide-svelte';
	import type { JsonEditorLabels } from './types';

	let {
		value = $bindable(''),
		disabled = false,
		id,
		placeholder = '{}',
		labels,
		onchange
	}: {
		value?: any;
		disabled?: boolean;
		id?: string;
		placeholder?: string;
		labels?: JsonEditorLabels;
		/** Called with the emitted JSON string on user edits (not on the
		 *  initial parse or external reseeds) — for hosts that store the
		 *  value as a parsed object rather than a string. */
		onchange?: (json: string) => void;
	} = $props();

	function parseInitial(v: any): { data: any; invalid: boolean } {
		if (v === undefined || v === null || v === '') return { data: undefined, invalid: false };
		// Non-string seeds (object/array row values) are deep-cloned so editor
		// mutations never alias-mutate the caller's row/form object.
		if (typeof v !== 'string') {
			const cloned = typeof v === 'object' ? JSON.parse(JSON.stringify(v)) : v;
			return { data: cloned, invalid: false };
		}
		try {
			return { data: JSON.parse(v), invalid: false };
		} catch {
			// Non-JSON text — hold it as a raw string so nothing is lost.
			return { data: v, invalid: true };
		}
	}

	const initial = parseInitial(value);
	let data = $state<any>(initial.data);
	// Seeded invalid JSON opens in raw mode so the user can see/fix the text.
	let mode = $state<'form' | 'raw'>(initial.invalid ? 'raw' : 'form');
	let rawText = $state('');
	let rawError = $state('');

	// lastSig / lastEmitted distinguish self-emissions from external reseeds:
	// the editor must not rewrite the bound string on mount (that would mark
	// the field dirty) nor reparse the string it just emitted. Plain lets —
	// reactive bookkeeping would only cause spurious effect re-triggers.
	let lastSig = JSON.stringify(initial.data ?? null);
	let lastEmitted: any = value;

	// data → value: fires only on real mutations (structural signature change).
	$effect(() => {
		const sig = JSON.stringify(data ?? null);
		if (sig !== lastSig) {
			lastSig = sig;
			value = data === undefined ? '' : JSON.stringify(data, null, 2);
			lastEmitted = value;
			onchange?.(value);
		}
	});

	// value → data: external reseed (e.g. a different row opened in the form).
	// Compared by *signature*, not string identity — a host that stores the
	// emitted JSON parsed and feeds it back recomputes a differently formatted
	// string for the same data; reparsing that would reset `data` on every
	// keystroke and drop focus.
	$effect(() => {
		if (value !== lastEmitted) {
			const next = parseInitial(value);
			const nextSig = JSON.stringify(next.data ?? null);
			lastEmitted = value;
			if (nextSig !== lastSig) {
				data = next.data;
				lastSig = nextSig;
				if (next.invalid) mode = 'raw';
			}
		}
	});

	function startWith(kind: string) {
		switch (kind) {
			case 'array': data = []; break;
			case 'string': data = ''; break;
			case 'number': data = 0; break;
			case 'boolean': data = false; break;
			default: data = {};
		}
	}

	function enterRaw() {
		// Serialize non-string binds (hosts may store parsed objects) so the
		// raw textarea never shows "[object Object]".
		rawText = typeof value === 'string' ? value : JSON.stringify(value ?? null, null, 2);
		rawError = '';
		mode = 'raw';
	}

	function syncRaw() {
		// Emit the user's text verbatim — pages keep their own JSON validation
		// on submit — but only refresh the structured model when it parses.
		value = rawText;
		lastEmitted = value;
		onchange?.(value);
		try {
			const parsed = rawText.trim() === '' ? undefined : JSON.parse(rawText);
			const nextSig = JSON.stringify(parsed ?? null);
			if (nextSig !== lastSig) {
				data = parsed;
				lastSig = nextSig;
			}
			rawError = '';
		} catch (e) {
			rawError = e instanceof Error ? e.message : (labels?.invalidJson ?? 'Invalid JSON');
		}
	}

	function enterForm() {
		try {
			const parsed = rawText.trim() === '' ? undefined : JSON.parse(rawText);
			data = parsed;
			lastSig = JSON.stringify(parsed ?? null);
			value = rawText;
			lastEmitted = value;
			rawError = '';
			mode = 'form';
		} catch (e) {
			rawError = e instanceof Error ? e.message : (labels?.invalidJson ?? 'Invalid JSON');
		}
	}
</script>

<div class="jve" class:jve-disabled={disabled}>
	<div class="jve-toolbar">
		<span class="jve-toolbar-label">{labels?.formMode ?? 'Form'}</span>
		{#if !disabled}
			<button
				type="button"
				class="jve-raw-toggle"
				title={labels?.rawMode ?? 'Edit raw JSON'}
				onclick={mode === 'form' ? enterRaw : enterForm}
			>
				{#if mode === 'form'}
					<Pencil size={11} /> {labels?.rawMode ?? 'Raw JSON'}
				{:else}
					<LayoutList size={11} /> {labels?.formMode ?? 'Back to form'}
				{/if}
			</button>
		{/if}
	</div>

	{#if mode === 'raw'}
		<textarea
			{id}
			class="jve-raw"
			bind:value={rawText}
			oninput={syncRaw}
			{disabled}
			{placeholder}
			rows={8}
			spellcheck="false"
			aria-invalid={rawError ? 'true' : 'false'}
		></textarea>
		{#if rawError}
			<p class="jve-error">{rawError}</p>
		{/if}
	{:else if data === undefined}
		<div class="jve-empty">
			<span class="jve-empty-label">{labels?.empty ?? 'No value — start with:'}</span>
			{#if !disabled}
				<div class="jve-start">
					<button type="button" {id} onclick={() => startWith('object')}><Braces size={12} /> {labels?.startObject ?? 'Object'}</button>
					<button type="button" onclick={() => startWith('array')}><List size={12} /> {labels?.startArray ?? 'List'}</button>
					<button type="button" onclick={() => startWith('string')}><Type size={12} /> {labels?.startText ?? 'Text'}</button>
					<button type="button" onclick={() => startWith('number')}><Hash size={12} /> {labels?.startNumber ?? 'Number'}</button>
					<button type="button" onclick={() => startWith('boolean')}><ToggleLeft size={12} /> {labels?.startBoolean ?? 'On/off'}</button>
				</div>
			{/if}
		</div>
	{:else}
		<div class="jve-body">
			<JsonNode bind:node={data} {disabled} depth={0} {labels} inputId={id} />
		</div>
	{/if}
</div>

<style>
	.jve {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		min-width: 0;
		width: 100%;
	}
	.jve-toolbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.jve-toolbar-label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
	}
	.jve-raw-toggle {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		border: 1px solid var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.375rem;
		padding: 0.15rem 0.5rem;
		font-size: 0.65rem;
		font-weight: 500;
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
		transition: background-color 0.12s, color 0.12s;
	}
	.jve-raw-toggle:hover {
		background: var(--color-surface-subtle, var(--color-muted, #f1f5f9));
		color: var(--color-foreground, var(--color-text, inherit));
	}
	.jve-raw {
		width: 100%;
		border: 1px solid var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.5rem;
		padding: 0.5rem;
		font-size: 0.75rem;
		font-family: ui-monospace, monospace;
		resize: vertical;
		background: var(--color-surface, var(--color-background, transparent));
		color: var(--color-foreground, var(--color-text, inherit));
	}
	.jve-raw[aria-invalid='true'] {
		border-color: var(--color-danger, #dc2626);
	}
	.jve-error {
		font-size: 0.7rem;
		color: var(--color-danger, #dc2626);
	}
	.jve-empty {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		border: 1px dashed var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.5rem;
		padding: 0.625rem;
	}
	.jve-empty-label {
		font-size: 0.7rem;
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
	}
	.jve-start {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
	}
	.jve-start button {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		border: 1px solid var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.375rem;
		padding: 0.25rem 0.5rem;
		font-size: 0.7rem;
		font-weight: 500;
		background: var(--color-surface, var(--color-background, transparent));
		color: var(--color-foreground, var(--color-text, inherit));
		transition: border-color 0.12s;
	}
	.jve-start button:hover {
		border-color: var(--color-primary, var(--color-accent, #2563eb));
	}
	.jve-body {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}
	.jve-disabled .jve-body,
	.jve-disabled .jve-empty {
		opacity: 0.6;
	}
</style>
