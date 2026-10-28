<script lang="ts">
	// Recursive node renderer for JsonValueEditor — objects render as editable
	// key/value rows, arrays as a reorderable repeater, and scalars as
	// type-appropriate inputs (text/number/checkbox) with a type switcher.
	import JsonNode from './JsonNode.svelte';
	import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-svelte';
	import type { JsonEditorLabels } from './types';

	let {
		node = $bindable(),
		disabled = false,
		depth = 0,
		labels,
		inputId
	}: {
		node: any;
		disabled?: boolean;
		depth?: number;
		labels?: JsonEditorLabels;
		/** Applied to the first focusable control so an external
		 *  <label for={inputId}> lands on a real labelable element. */
		inputId?: string;
	} = $props();

	const isObject = (v: unknown): v is Record<string, unknown> =>
		v !== null && typeof v === 'object' && !Array.isArray(v);

	function nodeType(v: unknown): string {
		if (v === null) return 'null';
		if (Array.isArray(v)) return 'array';
		return typeof v; // 'object' | 'string' | 'number' | 'boolean' | 'undefined'
	}

	function defaultFor(t: string): unknown {
		switch (t) {
			case 'number': return 0;
			case 'boolean': return false;
			case 'null': return null;
			case 'object': return {};
			case 'array': return [];
			default: return '';
		}
	}

	// emptyClone copies the shape of a value but resets leaves so a new
	// repeater item starts blank rather than duplicating the last item.
	function emptyClone(v: unknown): unknown {
		if (Array.isArray(v)) return v.map(emptyClone);
		if (isObject(v)) return Object.fromEntries(Object.entries(v).map(([k, val]) => [k, emptyClone(val)]));
		if (typeof v === 'number') return 0;
		if (typeof v === 'boolean') return false;
		if (v === null) return null;
		return '';
	}

	function renameKey(oldKey: string, newKey: string): boolean {
		if (!isObject(node) || newKey === oldKey) return true;
		if (newKey in node) return false; // refuse to clobber an existing sibling
		const entries = Object.entries(node);
		const idx = entries.findIndex(([k]) => k === oldKey);
		if (idx === -1) return false;
		entries[idx] = [newKey, entries[idx][1]];
		node = Object.fromEntries(entries);
		return true;
	}

	function removeKey(key: string) {
		if (!isObject(node)) return;
		const { [key]: _drop, ...rest } = node;
		node = rest;
	}

	function addField() {
		if (!isObject(node)) return;
		let key = 'new_key';
		let i = 1;
		while (key in node) key = `new_key_${i++}`;
		node = { ...node, [key]: '' };
	}

	function moveItem(i: number, dir: -1 | 1) {
		if (!Array.isArray(node)) return;
		const j = i + dir;
		if (j < 0 || j >= node.length) return;
		const next = [...node];
		[next[i], next[j]] = [next[j], next[i]];
		node = next;
	}

	function removeItem(i: number) {
		if (!Array.isArray(node)) return;
		node = node.filter((_: unknown, idx: number) => idx !== i);
	}

	function addItem() {
		if (!Array.isArray(node)) return;
		const last = node[node.length - 1];
		node = [...node, node.length ? emptyClone(last) : ''];
	}

	const typeOptions = ['string', 'number', 'boolean', 'null', 'object', 'array'];
	const typeLabel = (t: string) =>
		t === 'string' ? (labels?.typeString ?? 'Text')
		: t === 'number' ? (labels?.typeNumber ?? 'Number')
		: t === 'boolean' ? (labels?.typeBoolean ?? 'On/off')
		: t === 'null' ? (labels?.typeNull ?? 'Empty')
		: t === 'object' ? (labels?.typeObject ?? 'Group')
		: (labels?.typeArray ?? 'List');
</script>

{#if isObject(node)}
	<!-- depth indents nested containers with a subtle guide rail. -->
	<div class="jve-children" class:jve-nested={depth > 0}>
		{#each Object.keys(node) as k, i (k)}
			<div class="jve-row">
				<input
					type="text"
					class="jve-key"
					id={i === 0 ? inputId : undefined}
					value={k}
					{disabled}
					aria-label={labels?.key ?? 'Key'}
					onchange={(e) => {
						// Revert the visible text when the rename is refused
						// (duplicate or missing key) so input and state stay in sync.
						if (!renameKey(k, e.currentTarget.value.trim() || k)) e.currentTarget.value = k;
					}}
				/>
				<span class="jve-sep" aria-hidden="true">:</span>
				<div class="jve-val">
					<JsonNode bind:node={node[k]} {disabled} depth={depth + 1} {labels} />
				</div>
				<!-- Container children get the row-level type switcher; leaf
				     children already render their own inside JsonNode. -->
				{#if isObject(node[k]) || Array.isArray(node[k])}
					<select
						class="jve-type"
						value={nodeType(node[k])}
						{disabled}
						aria-label="Type"
						onchange={(e) => (node[k] = defaultFor(e.currentTarget.value))}
					>
						{#each typeOptions as opt (opt)}
							<option value={opt}>{typeLabel(opt)}</option>
						{/each}
					</select>
				{/if}
				<button
					type="button"
					class="jve-icon-btn"
					{disabled}
					aria-label={labels?.remove ?? 'Remove'}
					title={labels?.remove ?? 'Remove'}
					onclick={() => removeKey(k)}
				>
					<Trash2 size={13} />
				</button>
			</div>
		{/each}
		{#if !disabled}
			<button type="button" class="jve-add" id={Object.keys(node).length === 0 ? inputId : undefined} onclick={addField}>
				<Plus size={12} /> {labels?.addField ?? 'Add field'}
			</button>
		{/if}
	</div>
{:else if Array.isArray(node)}
	<div class="jve-children" class:jve-nested={depth > 0}>
		{#each node as _item, i (i)}
			<div class="jve-row">
				<span class="jve-index" aria-hidden="true">{i + 1}</span>
				<div class="jve-val">
					<JsonNode bind:node={node[i]} {disabled} depth={depth + 1} {labels} inputId={i === 0 ? inputId : undefined} />
				</div>
				{#if isObject(node[i]) || Array.isArray(node[i])}
					<select
						class="jve-type"
						value={nodeType(node[i])}
						{disabled}
						aria-label="Type"
						onchange={(e) => (node[i] = defaultFor(e.currentTarget.value))}
					>
						{#each typeOptions as opt (opt)}
							<option value={opt}>{typeLabel(opt)}</option>
						{/each}
					</select>
				{/if}
				<div class="jve-actions">
					<button
						type="button"
						class="jve-icon-btn"
						disabled={disabled || i === 0}
						aria-label={labels?.moveUp ?? 'Move up'}
						title={labels?.moveUp ?? 'Move up'}
						onclick={() => moveItem(i, -1)}
					>
						<ChevronUp size={13} />
					</button>
					<button
						type="button"
						class="jve-icon-btn"
						disabled={disabled || i === node.length - 1}
						aria-label={labels?.moveDown ?? 'Move down'}
						title={labels?.moveDown ?? 'Move down'}
						onclick={() => moveItem(i, 1)}
					>
						<ChevronDown size={13} />
					</button>
					<button
						type="button"
						class="jve-icon-btn"
						{disabled}
						aria-label={labels?.remove ?? 'Remove'}
						title={labels?.remove ?? 'Remove'}
						onclick={() => removeItem(i)}
					>
						<Trash2 size={13} />
					</button>
				</div>
			</div>
		{/each}
		{#if !disabled}
			<button type="button" class="jve-add" id={node.length === 0 ? inputId : undefined} onclick={addItem}>
				<Plus size={12} /> {labels?.addItem ?? 'Add item'}
			</button>
		{/if}
	</div>
{:else}
	<div class="jve-leaf">
		{#if typeof node === 'boolean'}
			<label class="jve-check">
				<input
					type="checkbox"
					id={inputId}
					checked={node}
					{disabled}
					onchange={(e) => (node = e.currentTarget.checked)}
				/>
				<span class="jve-check-label">{node ? 'true' : 'false'}</span>
			</label>
		{:else if typeof node === 'number'}
			<input
				type="number"
				class="jve-input"
				id={inputId}
				value={node}
				{disabled}
				onchange={(e) => (node = e.currentTarget.value === '' ? 0 : Number(e.currentTarget.value))}
			/>
		{:else if node === null || node === undefined}
			<span class="jve-null">null</span>
		{:else}
			<input
				type="text"
				class="jve-input"
				id={inputId}
				value={node}
				{disabled}
				aria-label={labels?.value ?? 'Value'}
				oninput={(e) => (node = e.currentTarget.value)}
			/>
		{/if}
		<select
			class="jve-type"
			id={node === null || node === undefined ? inputId : undefined}
			value={nodeType(node)}
			{disabled}
			aria-label="Type"
			onchange={(e) => (node = defaultFor(e.currentTarget.value))}
		>
			{#each typeOptions as opt (opt)}
				<option value={opt}>{typeLabel(opt)}</option>
			{/each}
		</select>
	</div>
{/if}

<style>
	.jve-children {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}
	.jve-nested {
		margin-inline-start: 0.75rem;
		padding-inline-start: 0.5rem;
		border-inline-start: 1px solid var(--color-border-input, var(--color-border, #e2e8f0));
	}
	.jve-row {
		display: flex;
		align-items: flex-start;
		gap: 0.375rem;
		padding: 0.375rem;
		border: 1px solid var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.5rem;
		background: var(--color-surface, var(--color-background, transparent));
	}
	.jve-key {
		width: 34%;
		min-width: 6rem;
		flex-shrink: 0;
		border: 1px solid var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.375rem;
		padding: 0.3rem 0.5rem;
		font-size: 0.75rem;
		font-family: ui-monospace, monospace;
		background: var(--color-surface, var(--color-background, transparent));
		color: var(--color-foreground, var(--color-text, inherit));
	}
	.jve-sep {
		padding-top: 0.35rem;
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
		font-size: 0.75rem;
	}
	.jve-index {
		flex-shrink: 0;
		min-width: 1.5rem;
		padding-top: 0.35rem;
		text-align: center;
		font-size: 0.7rem;
		font-family: ui-monospace, monospace;
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
	}
	.jve-val {
		flex: 1;
		min-width: 0;
	}
	.jve-leaf {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}
	.jve-leaf .jve-input {
		flex: 1;
		min-width: 0;
	}
	.jve-input {
		width: 100%;
		border: 1px solid var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.375rem;
		padding: 0.3rem 0.5rem;
		font-size: 0.75rem;
		background: var(--color-surface, var(--color-background, transparent));
		color: var(--color-foreground, var(--color-text, inherit));
	}
	.jve-type {
		flex-shrink: 0;
		max-width: 5.5rem;
		border: 1px solid var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.375rem;
		padding: 0.2rem 0.25rem;
		font-size: 0.65rem;
		background: var(--color-surface, var(--color-background, transparent));
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
	}
	.jve-check {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		flex: 1;
	}
	.jve-check input {
		width: 1rem;
		height: 1rem;
		accent-color: var(--color-primary, var(--color-accent, #2563eb));
	}
	.jve-check-label {
		font-size: 0.75rem;
		font-family: ui-monospace, monospace;
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
	}
	.jve-null {
		flex: 1;
		font-size: 0.75rem;
		font-family: ui-monospace, monospace;
		font-style: italic;
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
	}
	.jve-actions {
		display: flex;
		flex-shrink: 0;
		gap: 0.125rem;
	}
	.jve-icon-btn {
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 1.5rem;
		height: 1.5rem;
		margin-top: 0.15rem;
		border-radius: 0.375rem;
		color: var(--color-muted, var(--color-muted-foreground, #64748b));
		transition: background-color 0.12s;
	}
	.jve-icon-btn:hover:not(:disabled) {
		background: var(--color-surface-subtle, var(--color-muted, #f1f5f9));
		color: var(--color-foreground, var(--color-text, inherit));
	}
	.jve-icon-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.jve-add {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		align-self: flex-start;
		border: 1px dashed var(--color-border-input, var(--color-border, #e2e8f0));
		border-radius: 0.5rem;
		padding: 0.3rem 0.625rem;
		font-size: 0.7rem;
		font-weight: 500;
		color: var(--color-primary, var(--color-accent, #2563eb));
		transition: background-color 0.12s;
	}
	.jve-add:hover {
		background: var(--color-surface-subtle, var(--color-muted, #f1f5f9));
	}
</style>
