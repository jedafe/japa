<script lang="ts">
	// {{ token picker for plain-text composers (DMs, short-form fields) that
	// don't use the TipTap editor. Typing "{{" opens a suggestion list of
	// active template literals (GET /literals?q=); picking one inserts the
	// raw {{Token}} source — the server resolves it at the response boundary.
	import { api } from '$lib/api/client';
	import { mapLiteralHits, searchPath } from '$lib/tiptap/search';
	import type { SearchHit } from '$lib/types';

	let {
		value = $bindable(''),
		textarea = false,
		placeholder = '',
		class: cls = '',
		wrapperClass = 'relative flex-1',
		disabled = false,
		rows = undefined as number | undefined,
		onkeydown = undefined as ((e: KeyboardEvent) => void) | undefined,
		onvalue = undefined as ((v: string) => void) | undefined
	}: {
		value?: string;
		textarea?: boolean;
		placeholder?: string;
		class?: string;
		wrapperClass?: string;
		disabled?: boolean;
		rows?: number | undefined;
		onkeydown?: ((e: KeyboardEvent) => void) | undefined;
		// Fires after every value change (typing and token picks) so callers
		// can keep one-way data flow without a two-way bind.
		onvalue?: ((v: string) => void) | undefined;
	} = $props();

	let fieldEl: HTMLInputElement | HTMLTextAreaElement | undefined = $state();
	let items = $state<SearchHit[]>([]);
	let activeIndex = $state(0);
	let triggerStart = $state(-1); // index of the "{{" that opened the picker
	let debounce: ReturnType<typeof setTimeout> | undefined;

	const open = $derived(triggerStart >= 0 && items.length > 0);

	// Locate the live "{{query" span ending at the caret. Returns null when
	// the braces are already closed ({{Token}}|) or no trigger exists.
	function activeQuery(): { start: number; query: string } | null {
		const el = fieldEl;
		if (!el) return null;

		const caret = el.selectionStart ?? value.length;
		const head = value.slice(0, caret);
		const start = head.lastIndexOf('{{');
		if (start < 0) return null;

		const query = head.slice(start + 2);
		if (query.includes('}}') || query.includes('{{')) return null;

		return { start, query };
	}

	async function fetchItems(query: string) {
		const path = searchPath('literals', query);
		if (!path) return;

		const res = await api.get<Record<string, unknown>[]>(path);
		if (res.data) {
			items = mapLiteralHits(res.data).slice(0, 8);
			activeIndex = 0;
		}
	}

	function onInput() {
		onvalue?.(value);
		const hit = activeQuery();
		if (!hit) {
			triggerStart = -1;
			items = [];
			return;
		}

		triggerStart = hit.start;
		clearTimeout(debounce);
		debounce = setTimeout(() => fetchItems(hit.query), 150);
	}

	function pick(hit: SearchHit) {
		const el = fieldEl;
		const key = hit.slug ?? hit.id;
		const start = triggerStart;
		if (!el || start < 0) return;

		const caret = el.selectionStart ?? value.length;
		const inserted = `{{${key}}}`;
		value = value.slice(0, start) + inserted + value.slice(caret);
		onvalue?.(value);
		triggerStart = -1;
		items = [];

		// Restore focus and move the caret just past the inserted token.
		queueMicrotask(() => {
			el.focus();
			const pos = start + inserted.length;
			el.setSelectionRange(pos, pos);
		});
	}

	function onKeydown(e: KeyboardEvent) {
		if (open) {
			if (e.key === 'ArrowDown') {
				e.preventDefault();
				activeIndex = (activeIndex + 1) % items.length;
				return;
			}
			if (e.key === 'ArrowUp') {
				e.preventDefault();
				activeIndex = (activeIndex - 1 + items.length) % items.length;
				return;
			}
			if (e.key === 'Enter' || e.key === 'Tab') {
				e.preventDefault();
				pick(items[activeIndex]);
				return;
			}
			if (e.key === 'Escape') {
				triggerStart = -1;
				items = [];
				return;
			}
		}

		onkeydown?.(e);
	}
</script>

<div class={wrapperClass}>
	{#if textarea}
		<textarea
			bind:this={fieldEl}
			bind:value
			{placeholder}
			{disabled}
			{rows}
			class={cls}
			oninput={onInput}
			onkeydown={onKeydown}
			onblur={() => setTimeout(() => (items = []), 150)}
		></textarea>
	{:else}
		<input
			bind:this={fieldEl}
			type="text"
			bind:value
			{placeholder}
			{disabled}
			class={cls}
			oninput={onInput}
			onkeydown={onKeydown}
			onblur={() => setTimeout(() => (items = []), 150)}
		/>
	{/if}

	{#if open}
		<ul
			class="absolute bottom-full left-0 z-30 mb-1 w-64 overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg"
			role="listbox"
		>
			{#each items as hit, i (hit.id)}
				<li role="option" aria-selected={i === activeIndex}>
					<button
						type="button"
						class="flex w-full items-baseline gap-2 px-3 py-2 text-left text-sm {i === activeIndex
							? 'bg-[var(--color-surface-subtle)]'
							: ''} hover:bg-[var(--color-surface-subtle)]"
						onmousedown={(e) => e.preventDefault()}
						onclick={() => pick(hit)}
					>
						<code class="text-xs text-[var(--color-primary)]">{'{{'}{hit.slug ?? hit.id}{'}}'}</code>
						<span class="truncate text-[var(--color-muted)]">{hit.label}</span>
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
