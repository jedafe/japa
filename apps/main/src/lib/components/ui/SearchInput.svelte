<script lang="ts">
	// Reusable search input with debounced autocomplete dropdown.
	// As the user types (>=2 chars), it calls the provided fetcher and shows
	// a dropdown of suggestions. Clicking a suggestion navigates to its URL;
	// pressing Enter with no highlighted suggestion lets the enclosing form
	// submit naturally (filter behaviour).
	//
	// Ref: japaconnect-engineering/sections/10-search-discover-trending.md
	import { Search, Loader2 } from 'lucide-svelte';
	import { goto } from '$app/navigation';

	export interface SearchSuggestion {
		id: string;
		title: string;
		url: string;
		excerpt?: string;
		type?: string;
	}

	interface Props {
		/** Bindable query string. */
		value: string;
		/** Input placeholder. */
		placeholder: string;
		/** Accessibility label. */
		ariaLabel?: string;
		/** HTML name attribute for the input (defaults to "q"). */
		name?: string;
		/** Fetcher called with the debounced query; returns suggestions. */
		fetcher: (q: string) => Promise<SearchSuggestion[]>;
		/** Debounce delay in ms (default 250). */
		debounceMs?: number;
		/** Extra classes for the input element. */
		inputClass?: string;
		/** Minimum query length before fetching (default 2). */
		minQuery?: number;
		/** Optional callback fired on every input event (for live URL updates). */
		onInput?: (value: string) => void;
	}

	let {
		value = $bindable(),
		placeholder,
		ariaLabel,
		name = 'q',
		fetcher,
		debounceMs = 250,
		inputClass = '',
		minQuery = 2,
		onInput
	}: Props = $props();

	let suggestions = $state<SearchSuggestion[]>([]);
	let showDropdown = $state(false);
	let activeIndex = $state(-1);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;
	let loading = $state(false);
	let requestToken = 0;

	function onInputEvent(e: Event) {
		value = (e.currentTarget as HTMLInputElement).value;
		onInput?.(value);
		if (debounceTimer) clearTimeout(debounceTimer);
		if (value.trim().length < minQuery) {
			suggestions = [];
			showDropdown = false;
			return;
		}
		const token = ++requestToken;
		debounceTimer = setTimeout(async () => {
			loading = true;
			try {
				const items = await fetcher(value.trim());
				if (token === requestToken) {
					suggestions = items;
					showDropdown = items.length > 0;
					activeIndex = -1;
				}
			} catch {
				if (token === requestToken) {
					suggestions = [];
					showDropdown = false;
				}
			} finally {
				if (token === requestToken) loading = false;
			}
		}, debounceMs);
	}

	function select(s: SearchSuggestion) {
		showDropdown = false;
		if (s.url) goto(s.url);
	}

	function onKeydown(e: KeyboardEvent) {
		if (!showDropdown || suggestions.length === 0) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			activeIndex = Math.min(activeIndex + 1, suggestions.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			activeIndex = Math.max(activeIndex - 1, 0);
		} else if (e.key === 'Enter' && activeIndex >= 0) {
			e.preventDefault();
			select(suggestions[activeIndex]);
		} else if (e.key === 'Escape') {
			showDropdown = false;
		}
	}

	function onBlur() {
		// Delay to allow click on suggestion.
		setTimeout(() => (showDropdown = false), 150);
	}
</script>

<div class="relative">
	<Search
		size={16}
		class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
	/>
	<input
		type="search"
		{name}
		value={value}
		oninput={onInputEvent}
		onkeydown={onKeydown}
		onblur={onBlur}
		onfocus={() => { if (suggestions.length > 0) showDropdown = true; }}
		{placeholder}
		aria-label={ariaLabel}
		aria-expanded={showDropdown}
		aria-autocomplete="list"
		aria-controls="search-input-suggestions"
		role="combobox"
		class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-9 pr-9 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none {inputClass}"
	/>
	{#if loading}
		<Loader2
			size={14}
			class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 animate-spin text-[var(--color-muted)]"
		/>
	{/if}
	{#if showDropdown && suggestions.length > 0}
		<ul
			id="search-input-suggestions"
			class="absolute left-0 right-0 top-full z-50 mt-1 max-h-80 overflow-y-auto rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg"
			role="listbox"
		>
			{#each suggestions as s, i (s.id)}
				<li role="option" aria-selected={i === activeIndex}>
					<button
						type="button"
						onclick={() => select(s)}
						onmouseenter={() => (activeIndex = i)}
						class="flex w-full items-start gap-2 px-3 py-2 text-left transition {i === activeIndex
							? 'bg-[var(--color-surface-subtle)]'
							: ''}"
					>
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{s.title}</p>
							{#if s.excerpt}
								<p class="truncate text-xs text-[var(--color-muted)]">{s.excerpt}</p>
							{/if}
						</div>
						{#if s.type}
							<span
								class="shrink-0 rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] font-semibold uppercase text-[var(--color-muted)]"
							>
								{s.type}
							</span>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
