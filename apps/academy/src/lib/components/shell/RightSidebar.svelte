<script lang="ts">
	import type { Snippet } from 'svelte';
	import ContinueAcademyCard from './ContinueAcademyCard.svelte';
	import SidebarWidgetRenderer from './SidebarWidgetRenderer.svelte';
	import GuestFooter from './GuestFooter.svelte';
	import type { Menu, SessionUser } from '$lib/types';

	interface SidebarWidget {
		id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
		/** Render slot from the widget type registry (BE-S3). Falls back to a
		 *  hardcoded topbar set for backward compat with older cached payloads. */
		slot?: string;
	}

	interface Props {
		user: SessionUser | null;
		/** Route-specific widgets rendered above the global defaults. */
		sidebar?: Snippet;
		/** Sidebar widgets from the Sidebar Management System (Section 59, ADR-066).
		 *  Resolved via /sidebars/resolve and rendered by SidebarWidgetRenderer. */
		rightSidebarWidgets?: unknown[];
		/** Suppress ad slots — ADR-060: "No ads on the lesson player". */
		suppressAds?: boolean;
		/** academy_footer menu — rendered pinned to the bottom of the rail. */
		footerMenu?: Menu | null;
	}

	let {
		user,
		sidebar,
		rightSidebarWidgets = [],
		suppressAds = false,
		footerMenu = null
	}: Props = $props();

	// Normalize incoming widgets to the SidebarWidget shape. On the lesson
	// player, ad_slot widgets are filtered out (ADR-060: no ads on the lesson
	// player — focused reading/watching).
	const widgets = $derived(
		(rightSidebarWidgets as SidebarWidget[])
			.filter((w) => w && typeof w.widget_type === 'string')
			.filter((w) => !(suppressAds && w.widget_type === 'ad_slot'))
	);

	// Split widgets into topbar (sticky) and content (scrollable) slots using
	// the `slot` field from the widget type registry (Section 59 §6, BE-S3).
	// Falls back to a hardcoded topbar type set for backward compat with older
	// cached payloads that do not include `slot`. This replaces the previously
	// hardcoded topbar (SearchAutocomplete + theme + language + UserDropdown),
	// which duplicated the admin-managed topbar widgets seeded by migration
	// 000223.
	const topbarWidgetTypes = new Set(['search', 'theme_toggle', 'language_toggle', 'user_dropdown']);
	function isTopbar(w: SidebarWidget): boolean {
		if (w.slot === 'topbar') return true;
		if (w.slot === 'content') return false;
		return topbarWidgetTypes.has(w.widget_type);
	}
	let topbarWidgets = $derived(widgets.filter((w) => isTopbar(w)));
	let contentWidgets = $derived(widgets.filter((w) => !isTopbar(w)));

	// §09 Product Tour: exactly one element may carry the academy-search target.
	// Tag only the first search-typed widget in DOM order (topbar slot first,
	// then content) — a second configured search widget must not double-tag
	// the attribute.
	const searchTourWidgetId = $derived(
		[...topbarWidgets, ...contentWidgets].find(
			(w) => w.widget_type === 'search' || w.widget_type === 'search_launcher'
		)?.id
	);
</script>

<aside class="flex h-full flex-col gap-4 overflow-y-auto px-4 py-3">
	<!-- Sticky top bar: rendered from topbar-slot widgets (search, theme toggle,
	     language toggle, user dropdown) managed via the Sidebar Management
	     System (Section 59, ADR-066). -->
	{#if topbarWidgets.length > 0}
		<div
			class="sticky top-0 z-10 -mx-4 flex items-center gap-2 bg-[var(--color-bg)]/90 px-4 py-2 backdrop-blur"
		>
			{#each topbarWidgets as widget (widget.id)}
				<SidebarWidgetRenderer {widget} {user} searchTourTarget={widget.id === searchTourWidgetId} />
			{/each}
		</div>
	{/if}

	<!-- Route-specific widgets -->
	{#if sidebar}
		{@render sidebar()}
	{/if}

	<!-- Global widget: continue-learning (CSR, session-guarded) -->
	{#if user}
		<ContinueAcademyCard {user} />
	{/if}

	<!-- Admin-managed content-slot sidebar widgets (Section 59, ADR-066).
	     Rendered below the route-specific snippet and ContinueAcademyCard. -->
	{#each contentWidgets as widget (widget.id)}
		<SidebarWidgetRenderer {widget} {user} searchTourTarget={widget.id === searchTourWidgetId} />
	{/each}

	<!-- academy_footer menu pinned to the bottom of the rail (mt-auto pushes
	     it to the viewport bottom when content is short; scrolls below the
	     widgets otherwise). -->
	{#if footerMenu}
		<div class="mt-auto shrink-0 border-t border-[var(--color-border)] pt-3">
			<GuestFooter menu={footerMenu} />
		</div>
	{/if}
</aside>
