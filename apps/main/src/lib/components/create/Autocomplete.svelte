<script lang="ts">
	import { t } from '$lib/i18n';
	import { ChevronDown, Search, Check } from 'lucide-svelte';

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
		/** Currently selected item id, or null. */
		selectedId: string | null;
		/** Label to display for the currently selected item (avoids re-fetch on render). */
		selectedLabel?: string | null;
		/** Optional badge/icon text shown next to the selected label (e.g. ★ for primary). */
		selectedBadge?: string | null;
		/** Callback when the user selects an item. */
		onSelect: (item: AutocompleteItem) => void;
		/** Optional focus callback. */
		onfocus?: () => void;
		/** Debounce delay in ms (default 300). */
		debounceMs?: number;
	}

	let {
		placeholder,
		emptyMessage,
		fetcher,
		selectedId,
		selectedLabel = null,
		selectedBadge = null,
		onSelect,
		onfocus,
		debounceMs = 300
	}: Props = $props();

	let open = $state(false);
	let query = $state('');
	let results = $state<AutocompleteItem[]>([]);
	let loading = $state(false);
	let activeIndex = $state(0);
	let triggerEl: HTMLElement | null = null;
	let searchInput = $state<HTMLInputElement | null>(null);

	// Debounce timer
	let timer: ReturnType<typeof setTimeout> | null = null;
	// Request token — prevents stale responses from overwriting newer ones
	let requestToken = 0;

	// Click-outside handler
	function onWindowClick(e: MouseEvent) {
		if (!open) return;
		const target = e.target as Node;
		if (triggerEl && !triggerEl.contains(target)) {
			const panel = document.querySelector('.autocomplete-dropdown');
			if (!panel || !panel.contains(target)) {
				close();
			}
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			close();
			triggerEl?.focus();
		}
	}

	const selectedDisplay = $derived(selectedLabel ?? '');

	function open_() {
		open = true;
		onfocus?.();
		query = '';
		results = [];
		activeIndex = 0;
		queueMicrotask(() => searchInput?.focus());
	}

	function close() {
		open = false;
	}

	function toggle() {
		if (open) close();
		else open_();
	}

	// Debounced search — fires fetcher after the user stops typing.
	$effect(() => {
		if (!open) return;
		const q = query.trim();

		if (timer) clearTimeout(timer);
		if (q.length === 0) {
			results = [];
			loading = false;
			return;
		}

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

	function pick(item: AutocompleteItem) {
		onSelect(item);
		close();
		triggerEl?.focus();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!open) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			activeIndex = Math.min(activeIndex + 1, results.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			activeIndex = Math.max(activeIndex - 1, 0);
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (results[activeIndex]) pick(results[activeIndex]);
		}
	}
</script>

<svelte:window onclick={onWindowClick} onkeydown={onKeydown} />

<div class="autocomplete-field">
	<button
		type="button"
		class="autocomplete-trigger"
		bind:this={triggerEl}
		onclick={toggle}
		aria-haspopup="listbox"
		aria-expanded={open}
		onkeydown={handleKeydown}
	>
		<span class="autocomplete-trigger-text" class:placeholder={!selectedDisplay}>
			{#if selectedBadge}<span class="autocomplete-badge">{selectedBadge}</span>{/if}
			{selectedDisplay || placeholder}
		</span>
		<ChevronDown size={16} class={'autocomplete-chevron' + (open ? ' is-open' : '')} />
	</button>

	{#if open}
		<div class="autocomplete-dropdown" role="listbox">
			<div class="autocomplete-search">
				<Search size={14} class="autocomplete-search-icon" />
				<input
					type="text"
					class="autocomplete-search-input"
					placeholder={placeholder}
					bind:this={searchInput}
					bind:value={query}
					onkeydown={handleKeydown}
					role="combobox"
					aria-expanded={open}
					aria-autocomplete="list"
					aria-controls="autocomplete-list"
				/>
				{#if loading}
					<span class="autocomplete-spinner" aria-label={t('common.loading')}></span>
				{/if}
			</div>
			<div class="autocomplete-options" id="autocomplete-list">
				{#if !loading && query.trim().length === 0}
					<div class="autocomplete-hint">{placeholder}</div>
				{:else if !loading && results.length === 0}
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
							onclick={() => pick(item)}
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

	.autocomplete-trigger {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
		text-align: left;
		cursor: pointer;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.autocomplete-trigger:hover {
		border-color: var(--color-text-subtle);
	}
	.autocomplete-trigger:focus,
	.autocomplete-trigger[aria-expanded='true'] {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
		outline: none;
	}
	.autocomplete-trigger-text {
		display: flex;
		align-items: center;
		gap: 4px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.autocomplete-trigger-text.placeholder {
		color: var(--color-muted);
		font-weight: 500;
	}
	.autocomplete-badge {
		color: var(--color-primary);
		font-weight: 700;
	}
	.autocomplete-trigger :global(.autocomplete-chevron) {
		flex-shrink: 0;
		color: var(--color-muted);
		transition: transform 0.15s;
	}
	.autocomplete-trigger :global(.autocomplete-chevron.is-open) {
		transform: rotate(180deg);
	}

	.autocomplete-dropdown {
		position: absolute;
		top: 100%;
		left: 0;
		right: 0;
		z-index: 40;
		margin-top: 4px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.12);
		overflow: hidden;
	}

	.autocomplete-search {
		position: relative;
		padding: 8px;
		border-bottom: 1px solid var(--color-border);
	}
	.autocomplete-search :global(.autocomplete-search-icon) {
		position: absolute;
		left: 16px;
		top: 50%;
		transform: translateY(-50%);
		color: var(--color-muted);
		pointer-events: none;
	}
	.autocomplete-search-input {
		width: 100%;
		padding: 6px 32px 6px 32px;
		border: 1px solid var(--color-border);
		border-radius: calc(var(--radius) - 2px);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.8125rem;
		outline: none;
		transition: border-color 0.15s;
	}
	.autocomplete-search-input:focus {
		border-color: var(--color-primary);
	}
	.autocomplete-search-input::placeholder {
		color: var(--color-muted);
	}

	.autocomplete-spinner {
		position: absolute;
		right: 16px;
		top: 50%;
		transform: translateY(-50%);
		width: 14px;
		height: 14px;
		border: 2px solid var(--color-border);
		border-top-color: var(--color-primary);
		border-radius: 50%;
		animation: autocomplete-spin 0.6s linear infinite;
	}
	@keyframes autocomplete-spin {
		to { transform: translateY(-50%) rotate(360deg); }
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
		border-radius: calc(var(--radius) - 2px);
		background: transparent;
		color: var(--color-text);
		font-size: 0.875rem;
		text-align: left;
		cursor: pointer;
		transition: background 0.1s;
	}
	.autocomplete-option:hover,
	.autocomplete-option.active {
		background: var(--color-surface-subtle);
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
	.autocomplete-empty,
	.autocomplete-hint {
		padding: 1rem;
		text-align: center;
		color: var(--color-muted);
		font-size: 0.8125rem;
	}
</style>
