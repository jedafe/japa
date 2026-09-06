<script lang="ts">
	// Ref: japaconnect-engineering/sections/05-geo-first-space-system.md (lines 252-267)
	// Reusable space selector backed by GET /spaces/search (Meilisearch).
	// On focus: shows user's joined spaces + primary spaces first. On keystroke
	// (debounced 200ms): searches via the API. Returns selected space IDs to the
	// parent via the onSelect callback.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Search, Loader2, X, Plus, Check } from 'lucide-svelte';

	interface SpaceOption {
		id: string;
		name: string;
		slug: string;
		level?: string;
	}

	interface Props {
		/** When true, multiple spaces can be selected (chips). */
		multiple?: boolean;
		/** Currently selected space IDs (for multiple mode). */
		selectedIds?: string[];
		/** Currently selected single space (for single mode). */
		selected?: SpaceOption | null;
		/** Filter param appended to the search query (e.g. 'level:country'). */
		filter?: string;
		/** Placeholder text for the search input. */
		placeholder?: string;
		/** Called with the full selected space object(s). */
		onSelect?: (space: SpaceOption) => void;
		/** Called when a space is removed (multiple mode). */
		onRemove?: (spaceId: string) => void;
		/** Disabled state. */
		disabled?: boolean;
		/** Exclude these space IDs from results (e.g. the current space). */
		excludeIds?: string[];
		/**
		 * When true, use GET /spaces/suggest-parents instead of /spaces/search.
		 * This endpoint returns primary spaces eligible to be parents of a
		 * custom space (Section 05 lines 252-267, BUG-012). Ignored when a
		 * query is typed — in that case /spaces/search is used so the user
		 * can still search by name.
		 */
		parentMode?: boolean;
	}

	let {
		multiple = false,
		selectedIds = [],
		selected = null,
		filter = '',
		placeholder,
		onSelect,
		onRemove,
		disabled = false,
		excludeIds = [],
		parentMode = false
	}: Props = $props();

	let query = $state('');
	let results = $state<SpaceOption[]>([]);
	let loading = $state(false);
	let open = $state(false);
	let timer: ReturnType<typeof setTimeout> | null = null;
	// Names of selected options — the results list is cleared on select, so
	// chip labels would otherwise fall back to rendering the raw UUID.
	let selectedLabels = $state<Record<string, string>>({});

	const resolvedPlaceholder = $derived(
		placeholder ?? t('space.combobox.placeholder', 'Search spaces…')
	);

	function doSearch(q: string) {
		query = q;
		if (timer) clearTimeout(timer);
		// BUG-012: In parentMode, always use /spaces/suggest-parents — it
		// accepts a query and only returns primary (country/state/city) spaces
		// eligible to be parents. Otherwise fall back to /spaces/search and map
		// the SearchResponse envelope to SpaceOption[].
		if (q.trim().length < 1 && !parentMode) {
			results = [];
			return;
		}
		loading = true;
		timer = setTimeout(async () => {
			try {
				let items: SpaceOption[] = [];
				if (parentMode) {
					const res = await api.get<SpaceOption[]>(`/spaces/suggest-parents?q=${encodeURIComponent(q.trim())}`);
					items = res.data ?? [];
				} else {
					const res = await api.get<{
						results: { type: string; item: { id: string; name: string; slug?: string; level?: string; url?: string; excerpt?: string } }[]
					}>(`/spaces/search?q=${encodeURIComponent(q.trim())}`);
					items = (res.data?.results ?? []).map((r) => ({
						id: r.item.id,
						name: r.item.name,
						slug: r.item.slug ?? r.item.url?.split('/').pop() ?? '',
						level: r.item.level
					}));
				}
				if (excludeIds.length > 0) {
					items = items.filter((s) => !excludeIds.includes(s.id));
				}
				results = items;
			} finally {
				loading = false;
			}
		}, parentMode && q.trim().length < 1 ? 100 : 200);
	}

	function select(space: SpaceOption) {
		selectedLabels = { ...selectedLabels, [space.id]: space.name };
		if (multiple) {
			if (!selectedIds.includes(space.id)) {
				onSelect?.(space);
			}
		} else {
			onSelect?.(space);
		}
		query = '';
		results = [];
		open = false;
	}

	function remove(spaceId: string) {
		const { [spaceId]: _removed, ...rest } = selectedLabels;
		selectedLabels = rest;
		onRemove?.(spaceId);
	}

	function onWindowClick(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('.space-combobox')) {
			open = false;
		}
	}
</script>

<svelte:window onclick={onWindowClick} />

<div class="space-combobox relative">
	{#if multiple && selectedIds.length > 0}
		<div class="mb-2 flex flex-wrap gap-1.5">
			{#each selectedIds as id (id)}
				{@const space = results.find((r) => r.id === id)}
				<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-xs text-[var(--color-primary)]">
					<Check size={11} />
					{space?.name ?? selectedLabels[id] ?? id}
					<button
						type="button"
						onclick={() => remove(id)}
						class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
						aria-label={t('common.remove', 'Remove')}
					>
						<X size={11} />
					</button>
				</span>
			{/each}
		</div>
	{/if}

	{#if !multiple && selected}
		<div class="flex items-center justify-between rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2">
			<span class="text-sm text-[var(--color-text)]">{selected.name}</span>
			<button
				type="button"
				onclick={() => remove(selected.id)}
				class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
				aria-label={t('common.remove', 'Remove')}
			>
				<X size={14} />
			</button>
		</div>
	{:else}
		<div class="relative">
			<Search size={15} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
			<input
				type="text"
				class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-8 pr-3 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] disabled:opacity-50"
				placeholder={resolvedPlaceholder}
				value={query}
				disabled={disabled}
				oninput={(e) => doSearch(e.currentTarget.value)}
				onfocus={() => { open = true; if (parentMode && query.trim().length < 1) doSearch(''); }}
			/>
			{#if loading}
				<div class="absolute right-2.5 top-1/2 -translate-y-1/2">
					<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
				</div>
			{/if}
		</div>
	{/if}

	{#if open && results.length > 0}
		<div class="absolute z-30 mt-1 w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg max-h-52 overflow-y-auto">
			{#each results as space (space.id)}
				<button
					type="button"
					class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					onclick={() => select(space)}
				>
					<span class="min-w-0">
						<span class="block truncate">{space.name}</span>
						{#if space.level}
							<span class="block text-xs capitalize text-[var(--color-muted)]">{space.level}</span>
						{/if}
					</span>
					<Plus size={14} class="shrink-0 text-[var(--color-muted)]" />
				</button>
			{/each}
		</div>
	{/if}
</div>
