<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import SidebarWidgetRenderer from './SidebarWidgetRenderer.svelte';
	import type { SessionUser, TagSuggestion, SpaceSuggestion } from '$lib/types';

	interface SidebarWidget {
		id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
	}

	interface Props {
		user: SessionUser | null;
		trendingTags?: TagSuggestion[];
		suggestedSpaces?: SpaceSuggestion[];
		/** Comma-separated list of enabled language codes from i18n.enabled_languages. */
		enabledLanguages?: string;
		/** Route-specific widgets rendered above the global defaults. */
		sidebar?: Snippet;
		/** Sidebar widgets from the Sidebar Management System (Section 59, ADR-066).
		 *  Resolved via /sidebars/resolve and rendered by SidebarWidgetRenderer. */
		rightSidebarWidgets?: unknown[];
	}

	let {
		user,
		trendingTags = [],
		suggestedSpaces = [],
		enabledLanguages = 'en,fr',
		sidebar,
		rightSidebarWidgets = []
	}: Props = $props();

	// Normalize incoming widgets to the SidebarWidget shape.
	const widgets = $derived(
		(rightSidebarWidgets as SidebarWidget[]).filter((w) => w && typeof w.widget_type === 'string')
	);

	// ADR-060: Dashboard is private — no ad slots on /dashboard/* routes.
	let isDashboard = $derived(page.url.pathname.startsWith('/dashboard'));

	// Split widgets into topbar (sticky) and content (scrollable) slots.
	// Topbar widgets: search, theme_toggle, language_toggle, user_dropdown.
	// Content widgets: everything else (ad_slot, trending_tags, suggested_spaces, etc.).
	const topbarWidgetTypes = new Set(['search', 'theme_toggle', 'language_toggle', 'user_dropdown']);
	let topbarWidgets = $derived(widgets.filter((w) => topbarWidgetTypes.has(w.widget_type)));
	let contentWidgets = $derived(widgets.filter((w) => !topbarWidgetTypes.has(w.widget_type)));
</script>

<aside class="flex h-full flex-col gap-4 overflow-y-auto px-4 py-3">
	<!-- Sticky top bar: rendered from topbar-slot widgets (search, theme toggle,
	     language toggle, user dropdown). These are managed via the Sidebar
	     Management System (Section 59) and can be reordered/toggled from admin. -->
	{#if topbarWidgets.length > 0}
		<div
			class="sticky top-0 z-10 -mx-4 flex items-center gap-2 bg-[var(--color-bg)]/90 px-4 py-2 backdrop-blur"
		>
			{#each topbarWidgets as widget (widget.id)}
				<SidebarWidgetRenderer
					{widget}
					{user}
					{trendingTags}
					{suggestedSpaces}
					{enabledLanguages}
					{isDashboard}
				/>
			{/each}
		</div>
	{/if}

	<!-- Route-specific widgets (via setSidebar snippet) -->
	{#if sidebar}
		{@render sidebar()}
	{/if}

	<!-- Content-slot widgets (ad slots, trending tags, suggested spaces, etc.)
	     managed via the Sidebar Management System (Section 59, ADR-066). -->
	{#each contentWidgets as widget (widget.id)}
		<SidebarWidgetRenderer
			{widget}
			{user}
			{trendingTags}
			{suggestedSpaces}
			{enabledLanguages}
			{isDashboard}
		/>
	{/each}
</aside>
