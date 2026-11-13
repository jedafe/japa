<script lang="ts">
	/** Refine rail — search + counted category links + theme facet
	 *  checkboxes (mockup 2 left column). Mobile collapses behind a toggle;
	 *  desktop shows the full rail. The themes block only renders when the
	 *  caller supplies real facets — never hardcoded placeholders. */
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

	const VISIBLE_THEMES = 5;

	let {
		query = $bindable(''),
		categories,
		themes = [],
		onQuery,
		onToggleTheme
	}: {
		query?: string;
		categories: RailCategory[];
		themes?: RailTheme[];
		onQuery: (q: string) => void;
		onToggleTheme?: (slug: string) => void;
	} = $props();

	let open = $state(false);
	let categoriesOpen = $state(true);
	let themesExpanded = $state(false);
	const visibleThemes = $derived(themesExpanded ? themes : themes.slice(0, VISIBLE_THEMES));

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
			<button
				type="button"
				onclick={() => (categoriesOpen = !categoriesOpen)}
				aria-expanded={categoriesOpen}
				class="flex w-full items-center justify-between pb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]"
			>
				{t('jce.compare.category.categories', 'Categories')}
				{#if categoriesOpen}<ChevronUp size={13} />{:else}<ChevronDown size={13} />{/if}
			</button>
			{#if categoriesOpen}
				{#each categories as cat (cat.href)}
					<a
						href={cat.href}
						aria-current={cat.active ? 'page' : undefined}
						class="flex items-center justify-between rounded-[var(--radius)] px-3 py-2 text-sm transition-colors
							{cat.active
								? 'bg-[var(--color-primary)]/10 font-medium text-[var(--color-primary-text)]'
								: 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
					>
						<span>{cat.label}</span>
						{#if cat.count !== undefined}
							<span class="text-xs text-[var(--color-muted)]">{cat.count}</span>
						{/if}
					</a>
				{/each}
			{/if}
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
