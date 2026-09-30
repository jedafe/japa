<script lang="ts">
	// Autocomplete — a debounced search dropdown with single-select support.
	// Ported from apps/admin/src/lib/components/ui/Autocomplete.svelte so
	// the control app can reuse the same live-debounced-search pattern without
	// cross-app imports (each SvelteKit app is a standalone module tree).
	// CSS tokens adapted to the control app's variable names.
	//
	// The dropdown is teleported to document.body via the `portal` action so
	// it is not clipped by ancestor overflow:hidden containers (e.g. the
	// settings card). It is positioned with position:fixed based on the
	// input wrapper's bounding rect.
	import { t } from '$lib/stores/i18n.svelte';
	import { untrack } from 'svelte';
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
	/** Optional blur callback for free-text mode — fires when the input
	 * loses focus with the current query text. Use this to commit the typed
	 * value to parent state without creating a reactive feedback loop. */
	onFreeTextBlur?: (q: string) => void;
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

		/** Accessible label for the input when no visible label element exists. */
		ariaLabel?: string;
		/** Disable interaction — renders dimmed and cannot be focused/opened. */
		disabled?: boolean;
		/** Extra classes on the outer wrapper. */
		class?: string;
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
		onFreeTextBlur,
		freeText = false,
		debounceMs = 300,
		id,
		inputClass = '',
		ariaLabel = undefined,
		disabled = false,
		class: wrapperClass = ''
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
	let dropdownEl = $state<HTMLDivElement | null>(null);
	let focused = $state(false);

	// Dropdown position (fixed, relative to viewport).
	let dropdownTop = $state(0);
	let dropdownLeft = $state(0);
	let dropdownWidth = $state(0);

	// Debounce timer
	let timer: ReturnType<typeof setTimeout> | null = null;
	// Request token — prevents stale responses from overwriting newer ones
	let requestToken = 0;

	// portal action — moves the element to document.body so it escapes
	// any ancestor overflow:hidden containers. On destroy, just remove
	// the node from the DOM entirely (Svelte is destroying the {#if}
	// block that contained it, so moving it back to a dying parent is
	// pointless and can orphan the element in document.body).
	//
	// When the Autocomplete is inside a bits-ui Dialog (aria-modal="true"),
	// the dialog's overlay intercepts pointer events on elements outside
	// the dialog content. Teleporting to document.body would place the
	// dropdown outside the dialog, making options unclickable. To fix
	// this, we portal into the closest [role=dialog] ancestor if one
	// exists, falling back to document.body otherwise.
	function portal(node: HTMLElement) {
		const dialog = fieldEl?.closest('[role="dialog"][aria-modal="true"]');
		const target = dialog ?? document.body;
		target.appendChild(node);
		return {
			destroy() {
				node.remove();
			}
		};
	}

	// Click-outside handler
	function onWindowClick(e: MouseEvent) {
		if (!focused) return;
		const target = e.target as Node;
		// Check if the click is inside the field or the teleported dropdown.
		if (fieldEl?.contains(target) || dropdownEl?.contains(target)) return;
		close();
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			close();
		}
	}

	const selectedDisplay = $derived(selectedLabel ?? '');
	// Unique per-instance id so multiple Autocompletes never share listbox ids.
	const listId = $props.id();

	// In free-text mode, seed the input with the current value whenever the
	// field is not focused so the typed text survives modal re-opens and
	// external value changes (e.g. editing an existing item).
	$effect(() => {
		if (freeText && !focused && selectedLabel) {
			query = selectedLabel;
		}
	});

	// updateDropdownPosition calculates the position for the dropdown
	// based on the input wrapper's bounding rect. When inside a dialog
	// (which uses CSS transform), position:fixed behaves like absolute
	// relative to the dialog, so we compute offsets relative to the
	// portal target (the dialog element) instead of the viewport.
	function updateDropdownPosition() {
		if (!fieldEl) return;
		const rect = fieldEl.getBoundingClientRect();
		dropdownWidth = rect.width;
		const portalParent = dropdownEl?.parentElement;
		if (portalParent && portalParent !== document.body) {
			// Inside a dialog — compute position relative to the dialog.
			const parentRect = portalParent.getBoundingClientRect();
			dropdownTop = rect.bottom - parentRect.top + 4;
			dropdownLeft = rect.left - parentRect.left;
		} else {
			// Default — viewport-relative fixed positioning.
			dropdownTop = rect.bottom + 4;
			dropdownLeft = rect.left;
		}
	}

	// Reposition the dropdown on scroll/resize while open.
	$effect(() => {
		if (!open) return;
		updateDropdownPosition();
		const onScroll = () => updateDropdownPosition();
		window.addEventListener('scroll', onScroll, true);
		window.addEventListener('resize', onScroll);
		return () => {
			window.removeEventListener('scroll', onScroll, true);
			window.removeEventListener('resize', onScroll);
		};
	});

	async function open_() {
		if (disabled) return;
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
		loadingInitial = false;
		initialResults = [];
		results = [];
		if (timer) { clearTimeout(timer); timer = null; }
	}

	function clearSelection() {
		if (disabled) return;
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
		// In free-text mode, commit the typed value to the parent on blur
		// via onFreeTextBlur. This avoids the infinite effect loop that
		// occurs when onInput updates parent state that feeds back into
		// selectedLabel (which re-seeds query via the $effect).
		// SKIP this if the blur was caused by pick() — otherwise the
		// typed query text overwrites the just-selected item id.
		if (freeText && onFreeTextBlur && !picking) {
			onFreeTextBlur(query);
		}
		// Close after focus transfer completes — prevents stale dropdowns
		// when clicking another Autocomplete instance.
		setTimeout(() => {
			if (!focused) close();
		}, 0);
	}

	// Debounced search — fires fetcher after the user stops typing.
	// Only `query` and `open` are tracked reactively. The fetcher/onInput
	// props are read via untrack() so that new arrow-function identities
	// created on parent re-render do NOT re-trigger this effect (which
	// would cause an infinite loop: effect sets loadingInitial → template
	// re-renders → new initialFetcher prop → effect re-runs → ...).
	$effect(() => {
		const q = query.trim();

		// Read props unreactively so their identity changes don't
		// re-trigger this effect.
		const onInputFn = untrack(() => onInput);
		const initialFetcherFn = untrack(() => initialFetcher);
		const fetcherFn = untrack(() => fetcher);

		// Notify parent of every keystroke so free-text fields (e.g.
		// permission_node) can sync the typed value to their own state.
		if (onInputFn) onInputFn(query);

		if (timer) clearTimeout(timer);
		if (q.length === 0) {
			results = [];
			loading = false;
			// Re-fetch initial results when query clears
			if (open && initialFetcherFn) {
				loadingInitial = true;
				const token = ++requestToken;
				initialFetcherFn().then((items) => {
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
				const items = await fetcherFn(q);
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

	// picking flag — set to true during pick() so onBlur knows NOT to
	// fire onFreeTextBlur (which would overwrite the just-selected value
	// with the typed query text). Without this, the blur event (which
	// may fire asynchronously after .blur()) runs after onSelect and
	// clobbers form.model with the partial search string.
	let picking = false;

	function pick(item: AutocompleteItem) {
		if (disabled) return;
		// Close the dropdown FIRST, before calling onSelect. onSelect
		// updates parent state which triggers a re-render and new prop
		// identities — closing first ensures the {#if showDropdown} block
		// is torn down cleanly before the parent re-render happens.
		picking = true;
		focused = false;
		if (!freeText) query = '';
		close();
		inputEl?.blur();
		onSelect(item);
		// Reset after a microtask to allow the blur handler to see it.
		queueMicrotask(() => { picking = false; });
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

<div class="autocomplete-field {wrapperClass}" bind:this={fieldEl}>
	<div
		class="autocomplete-input-wrapper {inputClass}"
		class:focused
		class:has-value={!!selectedId}
		class:disabled
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
			aria-label={ariaLabel ?? placeholder}
			{disabled}
			bind:this={inputEl}
			bind:value={query}
			onfocus={onFocus}
			onblur={onBlur}
			onkeydown={handleKeydown}
			role="combobox"
			aria-expanded={showDropdown}
			aria-autocomplete="list"
			aria-controls={listId}
			aria-haspopup="listbox"
		/>

		{#if loading || loadingInitial}
			<span class="autocomplete-spinner" aria-label={t('common.loading', 'Loading...')}></span>
		{/if}
	</div>
</div>

{#if showDropdown}
	<!-- The dropdown is teleported via the `portal` action so it is not
	     clipped by ancestor overflow:hidden containers. When inside a
	     bits-ui Dialog, it is portaled into the dialog content to avoid
	     the dialog overlay intercepting pointer events. -->
	<div
		bind:this={dropdownEl}
		use:portal
		class="autocomplete-dropdown"
		style="position: absolute; top: {dropdownTop}px; left: {dropdownLeft}px; width: {dropdownWidth}px; z-index: 99999;"
		role="listbox"
	>
		<div class="autocomplete-options" id={listId}>
			{#if loadingInitial}
				<div class="autocomplete-empty">{t('common.loading', 'Loading...')}</div>
			{:else if query.trim().length > 0 && !(freeText && query === selectedLabel)}
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
		border: 1px solid var(--color-border-input);
		border-radius: var(--radius-input, 9px);
		background: var(--background);
		color: var(--color-foreground);
		font-size: 0.875rem;
		cursor: text;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.autocomplete-input-wrapper:hover {
		border-color: var(--color-border-input-hover);
	}
	.autocomplete-input-wrapper.focused {
		border-color: var(--color-border-input-hover);
		box-shadow: var(--shadow-date-field-focus);
	}
	.autocomplete-input-wrapper.disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.autocomplete-input-wrapper.disabled .autocomplete-input {
		cursor: not-allowed;
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
		color: var(--color-muted-foreground);
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
		background: color-mix(in srgb, var(--accent) 10%, transparent);
		color: var(--color-accent-foreground);
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
		color: var(--color-accent-foreground);
		cursor: pointer;
		pointer-events: all;
		transition: background 0.1s, color 0.1s;
	}
	.autocomplete-chip-clear:hover {
		background: color-mix(in srgb, var(--accent) 20%, transparent);
		color: var(--color-foreground);
	}

	.autocomplete-spinner {
		width: 14px;
		height: 14px;
		border: 2px solid var(--color-border-input);
		border-top-color: var(--color-accent-foreground);
		border-radius: 50%;
		animation: autocomplete-spin 0.6s linear infinite;
	}
	@keyframes autocomplete-spin {
		to { transform: rotate(360deg); }
	}

	/* Dropdown styles are global because the dropdown is teleported to
	   document.body and is no longer a descendant of this component's
	   scoped style root. */
	:global(.autocomplete-dropdown) {
		background: var(--background);
		border: 1px solid var(--color-border-input);
		border-radius: var(--radius-input, 9px);
		box-shadow: var(--shadow-popover);
		overflow: hidden;
	}

	:global(.autocomplete-options) {
		max-height: 220px;
		overflow-y: auto;
		padding: 4px;
	}

	:global(.autocomplete-option) {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		width: 100%;
		padding: 8px 10px;
		border: none;
		border-radius: calc(var(--radius-input, 9px) - 2px);
		background: transparent;
		color: var(--color-foreground);
		font-size: 0.8125rem;
		text-align: left;
		cursor: pointer;
		transition: background 0.1s;
	}
	:global(.autocomplete-option:hover),
	:global(.autocomplete-option.active) {
		background: color-mix(in srgb, var(--muted) 15%, transparent);
	}
	:global(.autocomplete-option.selected) {
		background: color-mix(in srgb, var(--accent) 10%, transparent);
		color: var(--color-accent-foreground);
		font-weight: 500;
	}
	:global(.autocomplete-option-label) {
		display: flex;
		align-items: center;
		gap: 6px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	:global(.autocomplete-option-sub) {
		color: var(--color-muted-foreground);
		font-size: 0.75rem;
		font-weight: 400;
	}
	:global(.autocomplete-option .autocomplete-check) {
		flex-shrink: 0;
		color: var(--color-accent-foreground);
	}
	:global(.autocomplete-empty) {
		padding: 0.75rem;
		text-align: center;
		color: var(--color-muted-foreground);
		font-size: 0.8125rem;
	}
</style>
