<script lang="ts">
	// Autocomplete — a debounced search dropdown with single-select support.
	// Ported from apps/main/src/lib/components/create/Autocomplete.svelte so
	// the admin app can reuse the same live-debounced-search pattern without
	// cross-app imports (each SvelteKit app is a standalone module tree).
	// CSS tokens adapted to the admin app's variable names.
	import { t } from '$lib/stores/i18n.svelte';
	import { Check, X } from 'lucide-svelte';

	export interface AutocompleteItem {
		id: string;
		label: string;
		sublabel?: string;
	}

	interface Props {
		/** Placeholder text for the search input. */
		placeholder: string;
		/** Empty-state message when no results match. */
		emptyMessage: string;
		/** Fetcher — called with the debounced query string. */
		fetcher: (q: string) => Promise<AutocompleteItem[]>;
		/** Optional fetcher for initial suggestions when focused with empty query. */
		initialFetcher?: () => Promise<AutocompleteItem[]>;
		/** Currently selected item id, or null. */
		selectedId: string | null;
		/** Label to display for the currently selected item (avoids re-fetch on render). */
		selectedLabel?: string | null;
		/** Optional badge/icon text shown next to the selected label (e.g. ★ for primary). */
		selectedBadge?: string | null;
		/** Callback when the user selects an item. */
		onSelect: (item: AutocompleteItem) => void;
		/** Callback when the user clears the selection. */
		onClear?: () => void;
		/** Optional focus callback. */
		onfocus?: () => void;
		/** Optional input callback — fires on every keystroke with the raw
		 * query text. Use this for free-text fields that also offer suggestions
		 * (e.g. permission_node) so the parent can sync the typed value. */
		onInput?: (q: string) => void;
		/** Free-text mode: the input keeps its value when focused (instead of
		 * clearing to show a blank search). Use for fields that accept any
		 * typed value but also offer catalog suggestions. Implies no chip. */
		freeText?: boolean;
		/** Debounce delay in ms (default 300). */
		debounceMs?: number;
		/** Input id (for label association). */
		id?: string;
		/** Extra class on the input wrapper for font tweaks (e.g. 'font-mono'). */
		inputClass?: string;
	}

	let {
		placeholder,
		emptyMessage,
		fetcher,
		initialFetcher,
		selectedId,
		selectedLabel = null,
		selectedBadge = null,
		onSelect,
		onClear,
		onfocus,
		onInput,
		freeText = false,
		debounceMs = 300,
		id,
		inputClass = ''
	}: Props = $props();

	let open = $state(false);
	let query = $state('');
	let results = $state<AutocompleteItem[]>([]);
	let initialResults = $state<AutocompleteItem[]>([]);
	let loading = $state(false);
	let loadingInitial = $state(false);
	let activeIndex = $state(0);
	let inputEl: HTMLInputElement | null = null;
	let fieldEl: HTMLDivElement | null = null;
	let focused = $state(false);

	// Debounce timer
	let timer: ReturnType<typeof setTimeout> | null = null;
	// Request token — prevents stale responses from overwriting newer ones
	let requestToken = 0;

	// Click-outside handler
	function onWindowClick(e: MouseEvent) {
		if (!focused) return;
		const target = e.target as Node;
		if (!fieldEl || !fieldEl.contains(target)) {
			close();
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			close();
		}
	}

	const selectedDisplay = $derived(selectedLabel ?? '');

	// In free-text mode, seed the input with the current value whenever the
	// field is not focused so the typed text survives modal re-opens and
	// external value changes (e.g. editing an existing item).
	$effect(() => {
		if (freeText && !focused && selectedLabel) {
			query = selectedLabel;
		}
	});

	async function open_() {
		open = true;
		onfocus?.();
		// In free-text mode, keep the current query so the user can continue
		// editing; in selection mode, clear it to start a fresh search.
		if (!freeText) {
			query = '';
		}
		results = [];
		initialResults = [];
		activeIndex = 0;

		// Load initial suggestions
		if (initialFetcher) {
			loadingInitial = true;
			try {
				initialResults = await initialFetcher();
			} catch {
				initialResults = [];
			} finally {
				loadingInitial = false;
			}
		}
	}

	function close() {
		open = false;
		loading = false;
		initialResults = [];
		if (timer) clearTimeout(timer);
	}

	function clearSelection() {
		onClear?.();
		query = '';
		results = [];
		initialResults = [];
		open = false;
		queueMicrotask(() => inputEl?.focus());
	}

	function onFocus() {
		focused = true;
		open_();
	}

	function onBlur() {
		focused = false;
		// Close after focus transfer completes — prevents stale dropdowns
		// when clicking another Autocomplete instance.
		setTimeout(() => {
			if (!focused) close();
		}, 0);
	}

	// Debounced search — fires fetcher after the user stops typing.
	$effect(() => {
		const q = query.trim();

		// Notify parent of every keystroke so free-text fields (e.g.
		// permission_node) can sync the typed value to their own state.
		if (onInput) onInput(query);

		if (timer) clearTimeout(timer);
		if (q.length === 0) {
			results = [];
			loading = false;
			// Re-fetch initial results when query clears
			if (open && initialFetcher) {
				loadingInitial = true;
				const token = ++requestToken;
				initialFetcher().then((items) => {
					if (token === requestToken) {
						initialResults = items;
						activeIndex = 0;
					}
				}).catch(() => {
					if (token === requestToken) initialResults = [];
				}).finally(() => {
					if (token === requestToken) loadingInitial = false;
				});
			}
			return;
		}

		// Clear initial results when user starts typing
		initialResults = [];
		loadingInitial = false;

		loading = true;
		const token = ++requestToken;
		timer = setTimeout(async () => {
			try {
				const items = await fetcher(q);
				// Only apply if this is still the latest request
				if (token === requestToken) {
					results = items;
					activeIndex = 0;
				}
			} catch {
				if (token === requestToken) results = [];
			} finally {
				if (token === requestToken) loading = false;
			}
		}, debounceMs);
	});

	// Clear any pending debounce timer when the component unmounts.
	$effect(() => {
		return () => {
			if (timer) clearTimeout(timer);
		};
	});

	function pick(item: AutocompleteItem) {
		// In free-text mode, keep the picked value visible in the input;
		// in selection mode, clear the query (the chip shows the selection).
		if (!freeText) query = '';
		onSelect(item);
		focused = false;
		inputEl?.blur();
		close();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!open) return;
		const items = query.trim().length > 0 ? results : initialResults;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			activeIndex = Math.min(activeIndex + 1, items.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			activeIndex = Math.max(activeIndex - 1, 0);
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (items[activeIndex]) pick(items[activeIndex]);
		} else if (e.key === 'Backspace' && query.length === 0 && selectedId && onClear) {
			e.preventDefault();
			clearSelection();
		}
	}

	const showDropdown = $derived(open && (results.length > 0 || initialResults.length > 0 || loading || loadingInitial || query.trim().length > 0));
</script>

<svelte:window onclick={onWindowClick} onkeydown={onKeydown} />

<div class="autocomplete-field" bind:this={fieldEl}>
	<div
		class="autocomplete-input-wrapper {inputClass}"
		class:focused
		class:has-value={!!selectedId}
	>
		{#if selectedId && !focused && !freeText}
			<div class="autocomplete-chip">
				{#if selectedBadge}<span class="autocomplete-badge">{selectedBadge}</span>{/if}
				<span class="autocomplete-chip-label">{selectedDisplay}</span>
				<button
					type="button"
					class="autocomplete-chip-clear"
					onmousedown={(e) => { e.preventDefault(); clearSelection(); }}
					aria-label={t('common.clear', 'Clear')}
				>
					<X size={12} />
				</button>
			</div>
		{/if}

		<input
			{id}
			type="text"
			class="autocomplete-input"
			placeholder={selectedId && !focused ? '' : placeholder}
			bind:this={inputEl}
			bind:value={query}
			onfocus={onFocus}
			onblur={onBlur}
			onkeydown={handleKeydown}
			role="combobox"
			aria-expanded={showDropdown}
			aria-autocomplete="list"
			aria-controls={id ? `${id}-list` : 'autocomplete-list'}
			aria-haspopup="listbox"
		/>

		{#if loading || loadingInitial}
			<span class="autocomplete-spinner" aria-label={t('common.loading', 'Loading...')}></span>
		{/if}
	</div>

	{#if showDropdown}
		<div class="autocomplete-dropdown" role="listbox">
			<div class="autocomplete-options" id={id ? `${id}-list` : 'autocomplete-list'}>
				{#if loadingInitial}
					<div class="autocomplete-empty">{t('common.loading', 'Loading...')}</div>
				{:else if query.trim().length > 0}
					<!-- Search results -->
					{#if !loading && results.length === 0}
						<div class="autocomplete-empty">{emptyMessage}</div>
					{:else}
						{#each results as item, i (item.id)}
							<button
								type="button"
								class="autocomplete-option"
								class:selected={item.id === selectedId}
								class:active={i === activeIndex}
								role="option"
								aria-selected={item.id === selectedId}
								onmousedown={(e) => { e.preventDefault(); pick(item); }}
								onmouseenter={() => (activeIndex = i)}
							>
								<span class="autocomplete-option-label">
									{item.label}
									{#if item.sublabel}
										<span class="autocomplete-option-sub">{item.sublabel}</span>
									{/if}
								</span>
								{#if item.id === selectedId}
									<Check size={14} class="autocomplete-check" />
								{/if}
							</button>
						{/each}
					{/if}
				{:else if initialResults.length > 0}
					<!-- Initial suggestions (no query typed) -->
					{#each initialResults as item, i (item.id)}
						<button
							type="button"
							class="autocomplete-option"
							class:selected={item.id === selectedId}
							class:active={i === activeIndex}
							role="option"
							aria-selected={item.id === selectedId}
							onmousedown={(e) => { e.preventDefault(); pick(item); }}
							onmouseenter={() => (activeIndex = i)}
						>
							<span class="autocomplete-option-label">
								{item.label}
								{#if item.sublabel}
									<span class="autocomplete-option-sub">{item.sublabel}</span>
								{/if}
							</span>
							{#if item.id === selectedId}
								<Check size={14} class="autocomplete-check" />
							{/if}
						</button>
					{/each}
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.autocomplete-field {
		position: relative;
	}

	.autocomplete-input-wrapper {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		height: 2.5rem;
		padding: 0 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-md);
		background: var(--color-surface);
		color: var(--color-foreground);
		font-size: 0.875rem;
		cursor: text;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.autocomplete-input-wrapper:hover {
		border-color: var(--color-muted);
	}
	.autocomplete-input-wrapper.focused {
		border-color: var(--color-primary);
	}

	.autocomplete-input {
		flex: 1;
		min-width: 0;
		padding: 0;
		border: none;
		background: transparent;
		color: inherit;
		font-size: inherit;
		outline: none;
		caret-color: var(--color-foreground);
	}
	.autocomplete-input:focus,
	.autocomplete-input:focus-visible {
		outline: none;
		box-shadow: none;
	}
	.autocomplete-input::placeholder {
		color: var(--color-muted);
	}
	/* Hide placeholder when chip is visible */
	.autocomplete-input-wrapper.has-value .autocomplete-input:placeholder-shown {
		opacity: 0;
	}

	.autocomplete-chip {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 2px 6px;
		border-radius: 9999px;
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
		font-size: 0.8125rem;
		font-weight: 500;
		white-space: nowrap;
		pointer-events: none;
	}
	.autocomplete-chip-label {
		max-width: 160px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.autocomplete-badge {
		font-weight: 700;
	}
	.autocomplete-chip-clear {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		height: 16px;
		padding: 0;
		border: none;
		border-radius: 50%;
		background: transparent;
		color: var(--color-primary);
		cursor: pointer;
		pointer-events: all;
		transition: background 0.1s, color 0.1s;
	}
	.autocomplete-chip-clear:hover {
		background: color-mix(in srgb, var(--color-primary) 20%, transparent);
		color: var(--color-foreground);
	}

	.autocomplete-spinner {
		width: 14px;
		height: 14px;
		border: 2px solid var(--color-border);
		border-top-color: var(--color-primary);
		border-radius: 50%;
		animation: autocomplete-spin 0.6s linear infinite;
	}
	@keyframes autocomplete-spin {
		to { transform: rotate(360deg); }
	}

	.autocomplete-dropdown {
		position: absolute;
		top: 100%;
		left: 0;
		right: 0;
		z-index: 60;
		margin-top: 4px;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-md);
		overflow: hidden;
	}

	.autocomplete-options {
		max-height: 220px;
		overflow-y: auto;
		padding: 4px;
	}
	.autocomplete-option {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		width: 100%;
		padding: 8px 10px;
		border: none;
		border-radius: calc(var(--radius-md) - 2px);
		background: transparent;
		color: var(--color-foreground);
		font-size: 0.8125rem;
		text-align: left;
		cursor: pointer;
		transition: background 0.1s;
	}
	.autocomplete-option:hover,
	.autocomplete-option.active {
		background: color-mix(in srgb, var(--color-muted) 15%, transparent);
	}
	.autocomplete-option.selected {
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
		font-weight: 500;
	}
	.autocomplete-option-label {
		display: flex;
		align-items: center;
		gap: 6px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.autocomplete-option-sub {
		color: var(--color-muted);
		font-size: 0.75rem;
		font-weight: 400;
	}
	.autocomplete-option :global(.autocomplete-check) {
		flex-shrink: 0;
		color: var(--color-primary);
	}
	.autocomplete-empty {
		padding: 0.75rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.8125rem;
	}
</style>
