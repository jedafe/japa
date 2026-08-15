<script lang="ts">
	// Ref: japaconnect-engineering/sections/10-search-discover-trending.md (autocomplete)
	// Debounced search combobox. As the user types (≥2 chars), it queries
	// GET /search/autocomplete?q=... and shows a dropdown of suggestions.
	// Selecting a suggestion navigates to its URL; pressing Enter submits the
	// full search to /search?q=...
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Search } from 'lucide-svelte';
	import { goto } from '$app/navigation';

	interface AutocompleteSuggestion {
		type: string;
		id: string;
		title?: string;
		excerpt?: string;
		url?: string;
		content_type?: string;
		slug?: string;
		username?: string;
		display_name?: string;
		name?: string;
	}

	let query = $state('');
	let suggestions = $state<AutocompleteSuggestion[]>([]);
	let showDropdown = $state(false);
	let activeIndex = $state(-1);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;
	let loading = $state(false);

	function onInput(value: string) {
		query = value;
		if (debounceTimer) clearTimeout(debounceTimer);
		if (value.trim().length < 2) {
			suggestions = [];
			showDropdown = false;
			return;
		}
		debounceTimer = setTimeout(async () => {
			loading = true;
			try {
				const res = await api.get<{ data: { suggestions: AutocompleteSuggestion[] } }>(
					`/search/autocomplete?q=${encodeURIComponent(value.trim())}&limit=8`
				);
				suggestions = res.data?.data?.suggestions ?? [];
				showDropdown = suggestions.length > 0;
				activeIndex = -1;
			} catch {
				suggestions = [];
				showDropdown = false;
			} finally {
				loading = false;
			}
		}, 200);
	}

	function select(s: AutocompleteSuggestion) {
		showDropdown = false;
		if (s.url) {
			goto(s.url);
		} else if (s.slug) {
			goto(`/${s.type === 'user' ? 'u' : s.type === 'space' ? 'spaces' : s.type}/${s.slug}`);
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (!showDropdown) {
			if (e.key === 'Enter' && query.trim()) {
				goto(`/search?q=${encodeURIComponent(query.trim())}`);
			}
			return;
		}
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			activeIndex = Math.min(activeIndex + 1, suggestions.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			activeIndex = Math.max(activeIndex - 1, 0);
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (activeIndex >= 0 && suggestions[activeIndex]) {
				select(suggestions[activeIndex]);
			} else {
				goto(`/search?q=${encodeURIComponent(query.trim())}`);
			}
		} else if (e.key === 'Escape') {
			showDropdown = false;
		}
	}

	function onBlur() {
		// Delay to allow click on suggestion.
		setTimeout(() => (showDropdown = false), 150);
	}
</script>

<div class="relative flex-1">
	<form
		action="/search"
		method="GET"
		class="relative"
		onsubmit={(e) => {
			if (query.trim()) {
				e.preventDefault();
				goto(`/search?q=${encodeURIComponent(query.trim())}`);
			}
		}}
	>
		<Search
			size={16}
			class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
		/>
		<input
			type="search"
			name="q"
			value={query}
			oninput={(e) => onInput(e.currentTarget.value)}
			onkeydown={onKeydown}
			onblur={onBlur}
			onfocus={() => { if (suggestions.length > 0) showDropdown = true; }}
			placeholder={t('sidebar.search_placeholder')}
			class="w-full rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-9 pr-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			aria-expanded={showDropdown}
			aria-autocomplete="list"
			aria-controls="search-suggestions"
			role="combobox"
		/>
	</form>

	{#if loading && showDropdown}
		<!-- Loading indicator (§54: loading states) -->
		<div class="absolute left-0 right-0 top-full z-50 mt-1 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] py-3 text-center shadow-lg">
			<span class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-border)] border-t-[var(--color-primary)]"></span>
		</div>
	{/if}

	{#if showDropdown && suggestions.length > 0}
		<ul
			id="search-suggestions"
			class="absolute left-0 right-0 top-full z-50 mt-1 max-h-80 overflow-y-auto rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg"
			role="listbox"
		>
			{#each suggestions as s, i (s.id)}
				<li role="option" aria-selected={i === activeIndex}>
					<button
						type="button"
						onclick={() => select(s)}
						onmouseenter={() => (activeIndex = i)}
						class="flex w-full items-start gap-2 px-3 py-2 text-left transition {i === activeIndex ? 'bg-[var(--color-surface-subtle)]' : ''}"
					>
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{s.title ?? s.name ?? s.username ?? s.display_name ?? ''}</p>
							{#if s.excerpt}
								<p class="truncate text-xs text-[var(--color-muted)]">{s.excerpt}</p>
							{/if}
						</div>
						{#if s.content_type}
							<span class="shrink-0 rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] font-semibold uppercase text-[var(--color-muted)]">
								{s.content_type}
							</span>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
