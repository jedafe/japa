<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Guide-style tab strip for a destination wiki: "Overview" links to the
	// space index (/wiki/{slug}); each other tab links to the first page in
	// that category. Tabs render at natural width — whatever doesn't fit the
	// bar collapses into a "More" dropdown (measured against the real nav
	// width via a hidden mirror row + ResizeObserver, so it adapts to
	// viewport, locale, and font changes).
	import { untrack } from 'svelte';
	import { Menu, MenuItem } from '$lib/components/ui';
	import { ChevronDown } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { categoryIcon, categoryIconResolved } from './icons';
	import type { WikiPageIndex } from '$lib/types';

	export interface WikiTabCategory {
		slug: string;
		label_key?: string;
		label: string;
		/** §80 lazy categories — the stub page slug (/wiki/{space}/{slug});
		 *  resolves without a wiki_pages row. */
		page_slug?: string;
		/** §80 directories — the stub IS the child listing; link it rather
		 *  than a sibling page that lacks the directory payload. */
		directory?: boolean;
		/** §80 directories — true when the space has zero matching
		 *  descendants; the tab (and its stub page) is hidden. */
		empty?: boolean;
		/** §80 category icon — admin-configured lucide name; overrides the
		 *  slug→icon map when recognized. */
		icon?: string;
	}

	interface Props {
		spaceSlug: string;
		/** Canonical categories from GET /wiki/categories (already ordered). */
		categories: WikiTabCategory[];
		pages: WikiPageIndex[];
		/** 'overview' on the index; the page's category on a detail page. */
		active?: string;
		/** Slug of the page currently being viewed (detail pages) — a tab only
		 *  gets aria-current="page" when its href targets this exact page. */
		currentPageSlug?: string;
	}

	let {
		spaceSlug,
		categories,
		pages,
		active = 'overview',
		currentPageSlug = ''
	}: Props = $props();

	interface Tab {
		slug: string;
		label: string;
		href: string;
		icon: ReturnType<typeof categoryIcon>;
	}

	function firstPage(catSlug: string): WikiPageIndex | undefined {
		return pages.find((p) => p.category === catSlug);
	}

	function catLabel(c: WikiTabCategory): string {
		return t(c.label_key ?? `wiki.category_${c.slug}`, c.label);
	}

	const tabs = $derived.by<Tab[]>(() => {
		const base = `/wiki/${encodeURIComponent(spaceSlug)}`;
		const list: Tab[] = [
			{
				slug: 'overview',
				label: t('wiki.category_overview', 'Overview'),
				href: base,
				icon: categoryIcon('overview')
			}
		];
		// §80 directories: a directory category with no matching children
		// (empty:true) is hidden — the stub would render a listing of
		// nothing.
		const hiddenCats = new Set(
			categories.filter((c) => c.directory && c.empty).map((c) => c.slug)
		);
		for (const c of categories) {
			if (c.slug === 'overview' || hiddenCats.has(c.slug)) continue;
			const p = firstPage(c.slug);
			// §80 lazy categories: category stubs resolve without a
			// wiki_pages row. Directory categories link the stub
			// unconditionally (the stub IS the listing — a sibling page
			// lacks the directory payload); other categories prefer a
			// materialized page and otherwise fall back to the stub —
			// every declared category is reachable.
			const href = c.directory && c.page_slug
				? `${base}/${encodeURIComponent(c.page_slug)}`
				: p
					? `${base}/${encodeURIComponent(p.slug)}`
					: c.page_slug
						? `${base}/${encodeURIComponent(c.page_slug)}`
						: null;
			if (!href) continue;
			list.push({
				slug: c.slug,
				label: catLabel(c),
				href,
				icon: categoryIconResolved(c.slug, c.icon)
			});
		}
		// Pages whose category isn't in the canonical set still get a tab —
		// otherwise they'd be unreachable from the strip (data drift between
		// the categories endpoint and stored page categories).
		const covered = new Set(list.map((tab) => tab.slug));
		for (const p of pages) {
			if (covered.has(p.category) || hiddenCats.has(p.category)) continue;
			covered.add(p.category);
			list.push({
				slug: p.category,
				label: t(`wiki.category_${p.category}`, p.category.replace(/_/g, ' ')),
				href: `${base}/${encodeURIComponent(p.slug)}`,
				icon: categoryIcon(p.category)
			});
		}
		return list;
	});

	// A tab is "current" when it's the active slot AND — on detail pages — its
	// href actually points at the page being read (the tab may link to a
	// sibling page that merely shares the category).
	function isCurrent(tab: Tab): boolean {
		if (tab.slug !== active) return false;
		if (!currentPageSlug) return true;
		return tab.href.endsWith(`/${encodeURIComponent(currentPageSlug)}`);
	}

	// --- Fit measurement ---------------------------------------------------
	// A hidden mirror row renders every tab at natural width; recalc() walks
	// it to find how many fit the real nav (reserving room for "More" unless
	// everything fits). SSR renders all tabs; hydration trims the excess.
	let navEl = $state<HTMLElement>();
	let measureEl = $state<HTMLElement>();
	let fitCount = $state(Number.MAX_SAFE_INTEGER);

	const visibleTabs = $derived(tabs.slice(0, fitCount));
	const overflowTabs = $derived(tabs.slice(fitCount));
	const overflowActive = $derived(overflowTabs.some((tab) => tab.slug === active));

	function recalc() {
		const nav = navEl;
		const meas = measureEl;
		if (!nav || !meas) return;
		const items = Array.from(meas.querySelectorAll('[data-wiki-tab]')) as HTMLElement[];
		const moreEl = meas.querySelector('[data-wiki-more]') as HTMLElement | null;
		const gap = 4; // gap-1
		const moreW = (moreEl?.offsetWidth ?? 0) + gap;
		const style = getComputedStyle(nav);
		const avail =
			nav.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
		let used = 0;
		let fit = 0;
		for (let i = 0; i < items.length; i++) {
			const w = items[i].offsetWidth;
			// If this isn't the last tab, More may still be needed after it —
			// reserve its width so the trigger never overflows the bar.
			const reserve = i < items.length - 1 ? moreW : 0;
			if (used + w + reserve <= avail) {
				used += w + gap;
				fit++;
			} else {
				break;
			}
		}
		fitCount = fit;
	}

	$effect(() => {
		void tabs; // re-measure when the tab set (or locale) changes
		const nav = navEl;
		if (!nav) return;
		untrack(recalc);
		const ro = new ResizeObserver(recalc);
		ro.observe(nav);
		if (measureEl) ro.observe(measureEl); // catches font-load reflow
		return () => ro.disconnect();
	});

	const TAB_CLASSES =
		'relative flex shrink-0 items-center gap-1.5 whitespace-nowrap px-3 py-2.5 text-sm font-medium transition-colors after:absolute after:inset-x-3 after:bottom-0 after:h-0.5 after:rounded-full after:transition-colors';
</script>

<nav
	bind:this={navEl}
	aria-label={t('wiki.heading', 'Wiki')}
	class="relative mt-4 flex items-center gap-1 overflow-hidden border-b border-[var(--color-border)] px-3"
>
	{#each visibleTabs as tab (tab.slug)}
		{@const Icon = tab.icon}
		<a
			href={tab.href}
			aria-current={isCurrent(tab) ? 'page' : undefined}
			class="{TAB_CLASSES} {tab.slug === active
				? 'text-[var(--color-primary)] after:bg-[var(--color-primary)]'
				: 'text-[var(--color-muted)] after:bg-transparent hover:text-[var(--color-text)]'}"
		>
			<Icon size={15} class="shrink-0" aria-hidden="true" />
			{tab.label}
		</a>
	{/each}

	{#if overflowTabs.length > 0}
		<Menu sideOffset={6}>
			{#snippet trigger(props)}
				<button
					type="button"
					{...props}
					class="{TAB_CLASSES} cursor-pointer {overflowActive
						? 'text-[var(--color-primary)] after:bg-[var(--color-primary)]'
						: 'text-[var(--color-muted)] after:bg-transparent hover:text-[var(--color-text)]'}"
				>
					{t('wiki.more_tab', 'More')}
					<ChevronDown size={14} aria-hidden="true" />
				</button>
			{/snippet}
			{#each overflowTabs as tab (tab.slug)}
				{@const Icon = tab.icon}
				<MenuItem
					href={tab.href}
					icon={Icon}
					class={tab.slug === active ? 'font-medium text-[var(--color-primary)]' : ''}
				>
					{tab.label}
				</MenuItem>
			{/each}
		</Menu>
	{/if}

	<!-- Hidden mirror row: all tabs + the More trigger at natural width,
	     used only for fit measurement. Removed from tab order and AT. -->
	<div
		bind:this={measureEl}
		aria-hidden="true"
		class="pointer-events-none invisible absolute left-0 top-0 flex w-max items-center gap-1"
	>
		{#each tabs as tab (tab.slug)}
			{@const Icon = tab.icon}
			<a data-wiki-tab href={tab.href} tabindex="-1" class={TAB_CLASSES}>
				<Icon size={15} class="shrink-0" aria-hidden="true" />
				{tab.label}
			</a>
		{/each}
		<span data-wiki-more class={TAB_CLASSES}>
			{t('wiki.more_tab', 'More')}
			<ChevronDown size={14} aria-hidden="true" />
		</span>
	</div>
</nav>
