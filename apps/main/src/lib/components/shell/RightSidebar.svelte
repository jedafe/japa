<script lang="ts">
	import { type Snippet } from 'svelte';
	import { page, navigating } from '$app/state';
	import SidebarWidgetRenderer from './SidebarWidgetRenderer.svelte';
	import { routeSidebar } from './routeSidebar.svelte';
	import type { SessionUser, TrendingTag, SpaceSuggestion } from '$lib/types';

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
		trendingTags?: TrendingTag[];
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
		sidebar: sidebarProp,
		rightSidebarWidgets = []
	}: Props = $props();

	// Prefer the registered route snippet (set by SetSidebar) over the direct
	// prop.
	const sidebar = $derived(routeSidebar.snippet ?? sidebarProp);

	// Normalize incoming widgets to the SidebarWidget shape.
	const widgets = $derived(
		(rightSidebarWidgets as SidebarWidget[]).filter((w) => w && typeof w.widget_type === 'string')
	);

	// ADR-060: Dashboard is private — no ad slots on /dashboard/* routes.
	let isDashboard = $derived(page.url.pathname.startsWith('/dashboard'));

	// Split widgets into topbar (sticky) and content (scrollable) slots using
	// the `slot` field from the widget type registry (Section 59 §6, BE-S3).
	// Falls back to a hardcoded topbar type set for backward compat with older
	// cached payloads that do not include `slot`.
	const topbarWidgetTypes = new Set([
		'search',
		'theme_toggle',
		'language_toggle',
		'user_dropdown',
		'notification_centre'
	]);
	function isTopbar(w: SidebarWidget): boolean {
		if (w.slot === 'topbar') return true;
		if (w.slot === 'content') return false;
		return topbarWidgetTypes.has(w.widget_type);
	}
	let topbarWidgets = $derived(widgets.filter((w) => isTopbar(w)));
	let contentWidgets = $derived(widgets.filter((w) => !isTopbar(w)));

	// §09 Product Tour: exactly one element may carry the search target.
	// Tag only the first search-typed widget in DOM order (topbar slot first,
	// then content) — a second configured search widget must not double-tag
	// the attribute.
	const searchTourWidgetId = $derived(
		[...topbarWidgets, ...contentWidgets].find((w) => w.widget_type === 'search')?.id
	);
</script>

<aside class="flex h-full flex-col">
	<!-- Pinned top bar at top:0 of the column — outside the scrollable body so
	     it never scrolls away. Rendered from topbar-slot widgets (search,
	     theme toggle, language toggle, user dropdown, notification centre),
	     managed via the Sidebar Management System (Section 59). -->
	{#if topbarWidgets.length > 0}
		<div
			class="flex h-[var(--nav-height)] shrink-0 items-center justify-end gap-2 border-b border-[var(--color-border)] bg-[var(--color-bg)]/90 px-4 backdrop-blur"
		>
			{#each topbarWidgets as widget (widget.id)}
				<SidebarWidgetRenderer
					{widget}
					{user}
					{trendingTags}
					{suggestedSpaces}
					{enabledLanguages}
					{isDashboard}
					searchTourTarget={widget.id === searchTourWidgetId}
				/>
			{/each}
		</div>
	{/if}

	<!-- Scrollable sidebar body: route-specific widgets + content-slot
	     widgets (ad slots, trending tags, suggested spaces, etc.) managed
	     via the Sidebar Management System (Section 59, ADR-066). The bottom
	     fade signals more content when the rail overflows — overlay
	     scrollbars are zero-width, so clipped cards otherwise read as
	     broken widgets rather than scrollable content. -->
	<div
		class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 py-3 [mask-image:linear-gradient(to_bottom,black_0%,black_calc(100%-28px),transparent_100%)]"
	>
		{#if sidebar}
			{@render sidebar()}
		{:else if navigating?.to}
			<!-- Client navigation in flight — the incoming route's SetSidebar
			     hasn't mounted yet, so pulse placeholders in place of the
			     route widgets until it registers. -->
			{#each [0, 1] as i (i)}
				<div class="animate-pulse rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="mb-3 h-4 w-1/3 rounded bg-[var(--color-surface-subtle)]"></div>
					<div class="mb-2 h-3 w-full rounded bg-[var(--color-surface-subtle)]"></div>
					<div class="h-3 w-2/3 rounded bg-[var(--color-surface-subtle)]"></div>
				</div>
			{/each}
		{/if}

		{#each contentWidgets as widget (widget.id)}
			<SidebarWidgetRenderer
				{widget}
				{user}
				{trendingTags}
				{suggestedSpaces}
				{enabledLanguages}
				{isDashboard}
				searchTourTarget={widget.id === searchTourWidgetId}
			/>
		{/each}
	</div>
</aside>
