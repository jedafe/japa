<script lang="ts">
	/** Refine rail — search + counted category links + theme facet
	 *  checkboxes (mockup 2 left column). Mobile collapses behind a toggle;
	 *  desktop shows the full rail. The themes block only renders when the
	 *  caller supplies real facets — never hardcoded placeholders. */
	import { untrack } from 'svelte';
	import { ChevronDown, ChevronUp, Info, Search } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';

	interface RailCategory {
		label: string;
		count?: number;
		href: string;
		active: boolean;
	}

	interface RailTheme {
		slug: string;
		label: string;
		count: number;
		active: boolean;
	}

	interface RailCategoryGroup {
		/** Stable key for accordion state (labels are localized). */
		key: string;
		label: string;
		categories: RailCategory[];
		/** Initial open state — only one group may carry this. */
		expanded?: boolean;
	}

	const VISIBLE_THEMES = 5;
	const VISIBLE_GROUP_CATS = 4;

	let {
		query = $bindable(''),
		categories,
		groups = [],
		themes = [],
		onQuery,
		onToggleTheme
	}: {
		query?: string;
		categories: RailCategory[];
		/** Optional grouped rendering — each entry becomes a labelled
		 *  sub-section under CATEGORIES (e.g. comparison families). */
		groups?: RailCategoryGroup[];
		themes?: RailTheme[];
		onQuery: (q: string) => void;
		onToggleTheme?: (slug: string) => void;
	} = $props();

	let open = $state(false);
	let themesExpanded = $state(false);
	const visibleThemes = $derived(themesExpanded ? themes : themes.slice(0, VISIBLE_THEMES));
	// Accordion — one family open at a time; groups arrive collapsed and
	// the caller marks at most one `expanded` (e.g. the active filter's
	// family, else a sensible default like Location). untrack: the prop
	// read is an intentional initial snapshot, not a subscription.
	const initialOpenKey = untrack(() => groups.find((g) => g.expanded)?.key ?? null);
	let openGroup = $state<string | null>(initialOpenKey);
	// Re-sync when the caller's expanded mark changes (URL-driven filter
	// moves to another family) — manual toggles don't touch the prop, so
	// they never fight this.
	let expandedPropKey = $state(initialOpenKey);
	$effect(() => {
		const next = groups.find((g) => g.expanded)?.key ?? null;
		if (next !== expandedPropKey) {
			expandedPropKey = next;
			openGroup = next;
		}
	});
	let overflowGroups = $state<Set<string>>(new Set());
	const groupCats = (g: RailCategoryGroup) =>
		overflowGroups.has(g.key) ? g.categories : g.categories.slice(0, VISIBLE_GROUP_CATS);

	function toggleGroup(key: string) {
		openGroup = openGroup === key ? null : key;
	}

	function toggleOverflow(key: string) {
		const next = new Set(overflowGroups);
		if (next.has(key)) next.delete(key);
		else next.add(key);
		overflowGroups = next;
	}

	let timeout: ReturnType<typeof setTimeout> | null = null;
	function input(e: Event) {
		query = (e.currentTarget as HTMLInputElement).value;
		if (timeout) clearTimeout(timeout);
		timeout = setTimeout(() => onQuery(query), 300);
	}
	// Don't fire a stale debounced query after the rail unmounts.
	$effect(() => () => {
		if (timeout) clearTimeout(timeout);
	});
</script>

<aside class="w-full shrink-0 lg:w-60">
	<!-- Mobile toggle: aria-expanded mirrors visual state; hidden on lg where
		 the rail body is always shown via lg:block below. -->
	<button
		type="button"
		aria-expanded={open}
		onclick={() => (open = !open)}
		class="flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-sm font-semibold text-[var(--color-text)] lg:hidden"
	>
		{t('jce.compare.category.refine', 'Refine results')}
		<ChevronDown size={16} class="motion-safe:transition-transform {open ? 'rotate-180' : ''}" />
	</button>

	<div class="{open ? 'block' : 'hidden'} space-y-5 pt-4 lg:block lg:pt-0">
		<h2 class="hidden text-sm font-semibold text-[var(--color-text)] lg:block">
			{t('jce.compare.category.refine', 'Refine results')}
		</h2>
		<div class="relative">
			<Search size={15} class="absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
			<input
				type="text"
				value={query}
				oninput={input}
				placeholder={t('jce.compare.category.search_placeholder', 'Search comparisons...')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 ps-9 pe-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		<nav class="space-y-1" aria-label={t('jce.compare.category.categories', 'Categories')}>
			{#each categories as cat (cat.href)}
				{@render catLink(cat)}
			{/each}
			{#each groups as group (group.key)}
				<div class="pt-1">
					<button
						type="button"
						onclick={() => toggleGroup(group.key)}
						aria-expanded={openGroup === group.key}
						class="flex w-full items-center justify-between px-3 py-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text)] transition-colors hover:text-[var(--color-muted)]"
					>
						{group.label}
						<ChevronDown size={12} class="motion-safe:transition-transform {openGroup === group.key ? 'rotate-180' : ''}" />
					</button>
					{#if openGroup === group.key}
						{#each groupCats(group) as cat (cat.href)}
							{@render catLink(cat)}
						{/each}
						{#if group.categories.length > VISIBLE_GROUP_CATS}
							<button
								type="button"
								onclick={() => toggleOverflow(group.key)}
								class="flex items-center gap-1 px-3 pt-0.5 text-xs font-medium text-[var(--color-primary-text)]"
							>
								{overflowGroups.has(group.key)
									? t('jce.compare.category.show_less', 'Show less')
									: t('jce.compare.category.show_more', 'Show more')}
								{#if overflowGroups.has(group.key)}<ChevronUp size={12} />{:else}<ChevronDown size={12} />{/if}
							</button>
						{/if}
					{/if}
				</div>
			{/each}
		</nav>

		{#if themes.length > 0 && onToggleTheme}
			<fieldset class="space-y-1">
				<legend class="flex w-full items-center gap-1 pb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{t('jce.compare.category.themes', 'Popular themes')}
					<span title={t('jce.compare.category.themes_info', 'Filter by the topics these comparisons cover')}>
						<Info size={12} class="cursor-help" />
					</span>
				</legend>
				{#each visibleThemes as theme (theme.slug)}
					<label class="flex cursor-pointer items-center justify-between gap-2 rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">
						<span class="flex items-center gap-2">
							<input
								type="checkbox"
								checked={theme.active}
								onchange={() => onToggleTheme(theme.slug)}
								class="h-3.5 w-3.5 rounded border-[var(--color-border)] accent-[var(--color-primary)]"
							/>
							{theme.label}
						</span>
						<span class="text-xs text-[var(--color-muted)]">{theme.count}</span>
					</label>
				{/each}
				{#if themes.length > VISIBLE_THEMES}
					<button
						type="button"
						onclick={() => (themesExpanded = !themesExpanded)}
						class="flex items-center gap-1 px-3 pt-1 text-xs font-medium text-[var(--color-primary-text)]"
					>
						{themesExpanded
							? t('jce.compare.category.show_less', 'Show less')
							: t('jce.compare.category.show_more', 'Show more')}
						{#if themesExpanded}<ChevronUp size={12} />{:else}<ChevronDown size={12} />{/if}
					</button>
				{/if}
			</fieldset>
		{/if}
	</div>
</aside>

{#snippet catLink(cat: RailCategory)}
	<a
		href={cat.href}
		aria-current={cat.active ? 'page' : undefined}
		class="flex items-center justify-between rounded-[var(--radius)] px-3 py-2 text-sm transition-colors
			{cat.active
				? 'bg-[var(--color-primary)]/10 font-medium text-[var(--color-primary-text)]'
				: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]'}"
	>
		<span>{cat.label}</span>
		{#if cat.count !== undefined}
			<span class="text-xs text-[var(--color-muted)]">{cat.count}</span>
		{/if}
	</a>
{/snippet}
