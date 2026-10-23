<script lang="ts">
	// SpaceAutocomplete — a debounced autocomplete dropdown for selecting
	// spaces. The search input lives inside the field; results appear in a
	// dropdown below. Selected spaces render as removable chips above the
	// input.
	//
	// Ref: japaconnect-engineering/sections/25-ad-placement.md —
	// target_space_ids on ad campaigns accept specific space UUIDs.
	// Ref: AGENTS.md Rule 16 — spaces come from the API, never hardcoded.
	import { spacesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Space = { id: string; name: string; slug: string; level?: string };

	let {
		selectedIds = $bindable<string[]>([]),
		placeholder = '',
		level = '',
		limit = 200,
		id = 'space-autocomplete-input'
	}: {
		selectedIds?: string[];
		placeholder?: string;
		level?: string;
		limit?: number;
		id?: string;
	} = $props();

	let allSpaces = $state<Space[]>([]);
	let query = $state('');
	let dropdownOpen = $state(false);
	let loading = $state(false);
	let activeIndex = $state(-1);
	let debounceTimer: ReturnType<typeof setTimeout> | undefined;

	// Load the full space list once on mount (filtered by level if given).
	// The admin spaces endpoint does not support server-side search, so we
	// fetch a generous page and filter client-side — this is the same
	// approach the previous inline search used, just debounced and inside
	// the field.
	// Note: the backend reads `page_size` (not `limit`) for pagination.
	$effect(() => {
		(async () => {
			loading = true;
			try {
				const params = new URLSearchParams();
				if (level) params.set('level', level);
				params.set('page_size', String(limit));
				const result = await spacesAdminApi.list(params.toString());
				if (result.ok) {
					const data = result.data as { items?: Space[]; data?: Space[] } | Space[];
					allSpaces = Array.isArray(data) ? data : (data.items ?? data.data ?? []);
				}
			} catch {
				allSpaces = [];
			} finally {
				loading = false;
			}
		})();
	});

	// Debounced filter — derived from the query string against the cached
	// space list. Excludes already-selected spaces from the results.
	// The admin spaces endpoint does not support server-side level
	// filtering, so level filtering (if needed) is done client-side.
	let filtered = $derived.by(() => {
		if (!allSpaces.length) return [];
		const q = query.trim().toLowerCase();
		let list = allSpaces.filter((s) => !selectedIds.includes(s.id));
		if (level) {
			list = list.filter((s) => s.level === level);
		}
		if (q) {
			list = list.filter(
				(s) =>
					s.name.toLowerCase().includes(q) ||
					s.slug.toLowerCase().includes(q)
			);
		}
		return list.slice(0, 20);
	});

	function handleInput(e: Event) {
		const target = e.target as HTMLInputElement;
		query = target.value;
		activeIndex = -1;
		if (debounceTimer) clearTimeout(debounceTimer);
		// Debounce the dropdown open so fast typing doesn't thrash the
		// derived filter on every keystroke. Open the dropdown whenever
		// there's a query — the "no results" message shows when filtered
		// is empty.
		debounceTimer = setTimeout(() => {
			dropdownOpen = query.trim().length > 0;
		}, 200);
	}

	// Clear any pending debounce timer when the component unmounts.
	$effect(() => {
		return () => {
			if (debounceTimer) clearTimeout(debounceTimer);
		};
	});

	function selectSpace(space: Space) {
		if (!selectedIds.includes(space.id)) {
			selectedIds = [...selectedIds, space.id];
		}
		query = '';
		dropdownOpen = false;
		activeIndex = -1;
	}

	function removeSpace(id: string) {
		selectedIds = selectedIds.filter((s) => s !== id);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!dropdownOpen) {
			if (e.key === 'ArrowDown' && filtered.length > 0) {
				dropdownOpen = true;
				activeIndex = 0;
				e.preventDefault();
			}
			return;
		}
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			activeIndex = Math.min(activeIndex + 1, filtered.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			activeIndex = Math.max(activeIndex - 1, 0);
		} else if (e.key === 'Enter' && activeIndex >= 0 && filtered[activeIndex]) {
			e.preventDefault();
			selectSpace(filtered[activeIndex]);
		} else if (e.key === 'Escape') {
			dropdownOpen = false;
			activeIndex = -1;
		}
	}

	function handleFocus() {
		if (query.trim()) {
			dropdownOpen = true;
		}
	}

	function handleBlur() {
		// Delay close so a click on a dropdown item registers first.
		setTimeout(() => {
			dropdownOpen = false;
		}, 200);
	}

	// Cleanup the debounce timer on destroy.
	$effect(() => {
		return () => {
			if (debounceTimer) clearTimeout(debounceTimer);
		};
	});

	let selectedSpaces = $derived(
		selectedIds
			.map((id) => allSpaces.find((s) => s.id === id))
			.filter((s): s is Space => s !== undefined)
	);
</script>

<div class="relative">
	<!-- Selected chips -->
	{#if selectedSpaces.length > 0}
		<div class="mb-2 flex flex-wrap gap-2">
			{#each selectedSpaces as space (space.id)}
				<span
					class="inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium"
					style="background-color: var(--color-info); color: white"
				>
					{space.name}
					<button
						type="button"
						onclick={() => removeSpace(space.id)}
						class="hover:opacity-70"
						aria-label={t('common.remove', 'Remove')}
					>
						×
					</button>
				</span>
			{/each}
		</div>
	{/if}

	<!-- Search input inside the field -->
	<input
		{id}
		type="text"
		{placeholder}
		value={query}
		oninput={handleInput}
		onkeydown={handleKeydown}
		onfocus={handleFocus}
		onblur={handleBlur}
		role="combobox"
		aria-expanded={dropdownOpen}
		aria-controls="space-autocomplete-list"
		aria-autocomplete="list"
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
	/>

	<!-- Loading indicator -->
	{#if loading}
		<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('common.loading')}</p>
	{/if}

	<!-- Dropdown results -->
	{#if dropdownOpen && filtered.length > 0}
		<ul
			id="space-autocomplete-list"
			role="listbox"
			class="absolute z-[60] mt-1 max-h-48 w-full overflow-auto rounded-md border shadow-lg"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			{#each filtered as space, i (space.id)}
				<li role="option" aria-selected={i === activeIndex}>
					<button
						type="button"
						class="flex w-full items-center justify-between px-3 py-2 text-start text-sm transition-colors"
						style={i === activeIndex
							? 'background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-foreground)'
							: 'color: var(--color-foreground)'}
						onmousedown={() => selectSpace(space)}
						onmouseenter={() => (activeIndex = i)}
					>
						<span>{space.name}</span>
						<span class="text-xs font-mono" style="color: var(--color-muted)">{space.slug}</span>
					</button>
				</li>
			{/each}
		</ul>
	{:else if dropdownOpen && query.trim() && !loading && filtered.length === 0}
		<p class="absolute z-[60] mt-1 rounded-md border px-3 py-2 text-xs shadow-lg" style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-muted)">
			{t('ads.no_spaces_found', 'No spaces found')}
		</p>
	{/if}
</div>
