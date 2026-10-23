<script lang="ts">
	// Ref: ADR-060 (Academy search), sections/10 (search autocomplete)
	// Debounced course search combobox. As the user types (≥2 chars), it queries
	// GET /academy/catalog?q=...&limit=8 and shows a dropdown of
	// course suggestions.
	// Selecting a suggestion navigates to the track detail page.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Search } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import type { TrackSummary } from '$lib/types';

	interface Props {
		/** Optional override for the empty-input hint (sidebar widget `placeholder` prop). */
		placeholder?: string;
	}
	let { placeholder = '' }: Props = $props();

	let query = $state('');
	let suggestions = $state<TrackSummary[]>([]);
	let showDropdown = $state(false);
	let activeIndex = $state(-1);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;
	let loading = $state(false);
	let requestGen = $state(0);

	const activeDescendantId = $derived(activeIndex >= 0 ? `search-suggestion-${activeIndex}` : undefined);

	function onInput(value: string) {
		query = value;
		if (debounceTimer) clearTimeout(debounceTimer);
		if (value.trim().length < 2) {
			suggestions = [];
			showDropdown = false;
			return;
		}
		debounceTimer = setTimeout(async () => {
			requestGen += 1;
			const myGen = requestGen;
			loading = true;
			try {
				const res = await api.get<TrackSummary[]>(
					`/academy/catalog?q=${encodeURIComponent(value.trim())}&limit=8`
				);
				// Drop stale responses so rapid typing never shows older results.
				if (myGen !== requestGen) return;
				suggestions = res.data ?? [];
				showDropdown = suggestions.length > 0;
				activeIndex = -1;
			} catch {
				if (myGen !== requestGen) return;
				suggestions = [];
				showDropdown = false;
			} finally {
				if (myGen === requestGen) loading = false;
			}
		}, 200);
	}

	function select(s: TrackSummary) {
		showDropdown = false;
		if (s.slug) goto(`/tracks/${s.slug}`);
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
			class="pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
		/>
		<input
			type="search"
			name="q"
			value={query}
			oninput={(e) => onInput(e.currentTarget.value)}
			onkeydown={onKeydown}
			onblur={onBlur}
			onfocus={() => { if (suggestions.length > 0) showDropdown = true; }}
			placeholder={placeholder || t('sidebar.search_placeholder')}
			class="w-full rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] py-2 ps-9 pe-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			aria-expanded={showDropdown}
			aria-autocomplete="list"
			aria-controls="search-suggestions"
			aria-activedescendant={activeDescendantId}
			role="combobox"
		/>
	</form>

	{#if loading && showDropdown}
		<!-- Loading indicator (§54: loading states) -->
		<div class="absolute start-0 end-0 top-full z-50 mt-1 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] py-3 text-center shadow-lg">
			<span class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-border)] border-t-[var(--color-primary)]"></span>
		</div>
	{/if}

	{#if showDropdown && suggestions.length > 0}
		<ul
			id="search-suggestions"
			class="absolute start-0 end-0 top-full z-50 mt-1 max-h-80 overflow-y-auto rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg"
			role="listbox"
		>
			{#each suggestions as s, i (s.id)}
				<li role="option" id="search-suggestion-{i}" aria-selected={i === activeIndex}>
					<button
						type="button"
						onclick={() => select(s)}
						onmouseenter={() => (activeIndex = i)}
						class="flex w-full items-start gap-2 px-3 py-2 text-start transition {i === activeIndex ? 'bg-[var(--color-surface-subtle)]' : ''}"
					>
						{#if s.thumbnail_url}
							<img src={s.thumbnail_url} alt="" class="h-10 w-10 shrink-0 rounded object-cover" />
						{/if}
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{s.title}</p>
							{#if s.instructor_name}
								<p class="truncate text-xs text-[var(--color-muted)]">{s.instructor_name}</p>
							{/if}
						</div>
						{#if s.level}
							<span class="shrink-0 rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] font-semibold uppercase text-[var(--color-muted)]">
								{s.level}
							</span>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
