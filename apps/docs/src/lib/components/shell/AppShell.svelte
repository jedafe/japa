<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import DocNavTree from './DocNavTree.svelte';
	import LeftRail from './LeftRail.svelte';
	import DocsHeader from './DocsHeader.svelte';
	import GuestFooter from './GuestFooter.svelte';
	import SidebarWidgetRenderer from './SidebarWidgetRenderer.svelte';
	import { t } from '$lib/i18n';
	import type { DocNavTree as DocNavTreeType, DocAudience, SessionUser, Menu } from '$lib/types';

	interface Props {
		nav: DocNavTreeType[];
		accessibleAudiences: DocAudience[];
		activeAudience: DocAudience | null;
		user: SessionUser | null;
		collapsed: boolean;
		onToggleCollapse: () => void;
		/** Route-specific right-sidebar content (the in-page TOC for doc pages). */
		sidebar?: Snippet;
		/**
		 * ADR-060 variants:
		 *   - 'doc'  : nav · prose · TOC (default)
		 *   - 'api'  : nav · prose · code-sample pane (API reference)
		 *   - 'full' : breaks the shell entirely (playground exception)
		 */
		variant?: 'doc' | 'api' | 'full';
		/** API reference: the right code-sample pane snippet (variant='api'). */
		apiPane?: Snippet;
		/** Platform logo URL from appearance.logo_url (control panel). */
		logoUrl?: string;
		/** Platform dark-theme logo URL from appearance.logo_url_dark (control panel). */
		logoDarkUrl?: string;
		/** Footer menu from GET /menus/docs_footer — ADR-054. */
		footerMenu?: Menu | null;
		/** Sidebar widgets from the Sidebar Management System (Section 59, ADR-066).
		 *  Resolved via /sidebars/resolve and rendered by SidebarWidgetRenderer. */
		rightSidebarWidgets?: unknown[];
		/** Admin-managed left-position widgets — only `ad_slot` widgets render,
		 *  as a 160×600 rail while the nav is collapsed (Section 59, ADR-066). */
		leftSidebarWidgets?: unknown[];
		/** Portal landing mode: content spans the full center width — no right
		 *  sidebar column at all (the mockup homepage has none). */
		hideRightSidebar?: boolean;
		children: Snippet;
	}

	let {
		nav,
		accessibleAudiences,
		activeAudience,
		user,
		collapsed,
		onToggleCollapse,
		sidebar,
		variant = 'doc',
		apiPane,
		logoUrl = '',
		logoDarkUrl = '',
		footerMenu = null,
		rightSidebarWidgets = [],
		leftSidebarWidgets = [],
		hideRightSidebar = false,
		children
	}: Props = $props();

	// Normalize incoming widgets to the SidebarWidget shape.
	const sidebarWidgets = $derived(
		(rightSidebarWidgets as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number; slot?: string }[]).filter(
			(w) => w && typeof w.widget_type === 'string'
		)
	);

	// Split widgets into topbar (header) and content (sidebar body) slots using
	// the `slot` field from the widget type registry (Section 59 §6, BE-S3).
	// Falls back to a hardcoded topbar type set for backward compat with older
	// cached payloads that do not include `slot`. Topbar widgets render inside
	// the global header — the header deduplicates types it provides natively.
	const topbarWidgetTypes = new Set(['search', 'search_launcher', 'theme_toggle', 'language_toggle', 'user_dropdown']);
	function isTopbar(w: { slot?: string; widget_type: string; props?: Record<string, unknown> }): boolean {
		// Explicit slot field on the widget (from the sidebar API)
		if (w.slot === 'topbar') return true;
		if (w.slot === 'content') return false;
		// NOTE: props.slot is the ad-slot name for ad_slot widgets (e.g.
		// 'sidebar', 'sidebar_sticky'), not a render position — it must NOT
		// be consulted here, or an ad slot literally named 'topbar' would
		// render in the header. Fallback: known topbar widget types.
		return topbarWidgetTypes.has(w.widget_type);
	}
	let topbarWidgets = $derived(sidebarWidgets.filter((w) => isTopbar(w)));
	let contentWidgets = $derived(sidebarWidgets.filter((w) => !isTopbar(w)));

	// Render the right rail only when it actually carries content — a route TOC
	// snippet, the API code pane, or admin-managed content-slot widgets.
	// Listing pages that pass nothing get a full-width center column instead
	// of a bordered empty column. ad_slot widgets count only while the ads
	// feature is enabled — when off they can never render (AdSlot skips its
	// fetch entirely), so they must not hold the rail open.
	const adsEnabled = $derived(
		(page.data?.features as Record<string, boolean> | undefined)?.['feature.ads'] !== false
	);
	const visibleContentWidgets = $derived(
		contentWidgets.filter((w) => w.widget_type !== 'ad_slot' || adsEnabled)
	);
	const hasRightSidebar = $derived(
		!hideRightSidebar &&
			Boolean(
				sidebar || (variant === 'api' && apiPane) || visibleContentWidgets.length > 0
			)
	);

	// Left rail (Section 59): ad-only 160px column between the collapsed nav
	// and the content, only while the nav is collapsed and an ad_slot widget
	// is assigned to the left position.
	const showLeftRail = $derived(
		collapsed &&
			(leftSidebarWidgets as { widget_type?: string }[]).some(
				(w) => w?.widget_type === 'ad_slot'
			)
	);

	let mobileNavOpen = $state(false);
	const navWidth = $derived(
		collapsed ? 'var(--nav-width-collapsed)' : 'var(--nav-width-expanded)'
	);
	const rightWidth = $derived(variant === 'api' ? 'var(--api-pane-width)' : 'var(--sidebar-width)');
	const isFull = $derived(variant === 'full');

	// Pages pass `logoUrl` from siteSettings; use the explicit `logoDarkUrl` prop
	// or fall back to the layout's public site settings so every route gets both.
	const resolvedLogoDarkUrl = $derived(
		logoDarkUrl ||
			((page.data.siteSettings?.['appearance.logo_url_dark'] as string | undefined) ?? '')
	);

	import AdConversionTracker from './AdConversionTracker.svelte';
</script>

{#if isFull}
	<!-- ADR-060 full-width exception: the API playground breaks the shell. -->
	<div class="shell-full">
		<DocsHeader
			{user}
			{logoUrl}
			logoDarkUrl={resolvedLogoDarkUrl}
			{topbarWidgets}
			showMenuButton={false}
		/>
		<main class="full-main">
			{@render children()}
		</main>
	</div>
	<GuestFooter menu={footerMenu} {user} />
{:else}
	<div
		class="shell"
		class:variant-api={variant === 'api'}
		class:show-left-rail={showLeftRail}
		class:no-right-sidebar={!hasRightSidebar}
		style="--current-nav-width: {navWidth}; --current-right-width: {rightWidth};"
	>
		<DocsHeader
			{user}
			{logoUrl}
			logoDarkUrl={resolvedLogoDarkUrl}
			{topbarWidgets}
			onOpenMenu={() => (mobileNavOpen = true)}
		/>

		<div class="shell-grid">
			<div class="shell-nav">
				<DocNavTree
					{nav}
					{accessibleAudiences}
					{activeAudience}
					{collapsed}
					{onToggleCollapse}
					{logoUrl}
					logoDarkUrl={resolvedLogoDarkUrl}
				/>
			</div>

			{#if showLeftRail}
				<LeftRail widgets={leftSidebarWidgets} />
			{/if}

			<main class="shell-center">
				{@render children()}
			</main>

			{#if hasRightSidebar}
			<div class="shell-sidebar" data-tour="docs-sidebar">
				<div class="sidebar-body">
					{#if variant === 'api' && apiPane}
						{@render apiPane()}
					{:else if sidebar}
						{@render sidebar()}
					{:else}
						<!-- No route-specific sidebar: show a minimal spacer so the grid
						stays consistent on landing/listing pages. -->
						<div class="sidebar-empty"></div>
					{/if}

					<!-- Admin-managed content-slot sidebar widgets (Section 59,
					     ADR-066). Rendered below the route-specific snippet (TOC). -->
					{#each contentWidgets as widget (widget.id)}
						<div class="mt-4">
							<SidebarWidgetRenderer {widget} {user} />
						</div>
					{/each}

					<!-- docs_footer menu pinned to the bottom of the right rail
					     (mt-auto → viewport bottom when content is short, scrolls
					     below the widgets otherwise). -->
					{#if footerMenu}
						<div class="mt-auto shrink-0 border-t border-[var(--color-border)] pt-3">
							<GuestFooter menu={footerMenu} {user} rail />
						</div>
					{/if}
				</div>
			</div>
			{/if}
		</div>
	</div>

	{#if mobileNavOpen}
		<button
			type="button"
			class="drawer-backdrop"
			aria-label={t('a11y.close_menu')}
			onclick={() => (mobileNavOpen = false)}
		></button>
		<div class="drawer">
			<DocNavTree
				{nav}
				{accessibleAudiences}
				{activeAudience}
				collapsed={false}
				onToggleCollapse={() => (mobileNavOpen = false)}
				{logoUrl}
				logoDarkUrl={resolvedLogoDarkUrl}
			/>
		</div>
	{/if}
{/if}

<AdConversionTracker />

<style>
	/* ---- Standard shell (doc + api variants) ---- */
	/* The header sits in normal flow so position:sticky can pin it for the
	   whole document scroll; the three columns live in .shell-grid below. */
	.shell {
		min-height: 100vh;
		background-color: var(--color-bg);
	}
	.shell-grid {
		display: grid;
		grid-template-columns: var(--current-nav-width) 1fr var(--current-right-width);
		min-height: calc(100vh - var(--nav-height));
	}
	/* The left ad rail inserts a fixed-width column between nav and center. */
	.show-left-rail .shell-grid {
		grid-template-columns:
			var(--current-nav-width) var(--left-rail-width) 1fr var(--current-right-width);
	}
	/* Portal landing: content spans full width — no right sidebar column. */
	.no-right-sidebar .shell-grid {
		grid-template-columns: var(--current-nav-width) 1fr;
	}
	.no-right-sidebar.show-left-rail .shell-grid {
		grid-template-columns:
			var(--current-nav-width) var(--left-rail-width) 1fr;
	}
	.no-right-sidebar .shell-center {
		border-inline-end: none;
	}
	.shell-nav {
		position: sticky;
		top: var(--nav-height);
		height: calc(100vh - var(--nav-height));
	}
	.shell-center {
		min-width: 0;
		border-inline-end: 1px solid var(--color-border);
	}
	.shell-sidebar {
		position: sticky;
		top: var(--nav-height);
		height: calc(100vh - var(--nav-height));
		display: flex;
		flex-direction: column;
		border-inline-start: 1px solid var(--color-border);
		background: var(--color-surface);
	}
	.shell :global(.left-rail) {
		top: var(--nav-height);
		height: calc(100vh - var(--nav-height));
	}
	.sidebar-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 1rem 0.75rem 2rem;
		display: flex;
		flex-direction: column;
	}
	.sidebar-empty {
		height: 100%;
	}

	.drawer-backdrop {
		position: fixed;
		inset: 0;
		z-index: 40;
		background-color: rgb(0 0 0 / 0.4);
	}
	.drawer {
		position: fixed;
		top: 0;
		inset-inline-start: 0;
		z-index: 50;
		width: var(--nav-width-expanded);
		height: 100vh;
		box-shadow: 0 0 40px rgb(0 0 0 / 0.25);
		background: var(--color-surface);
	}

	/* ---- Full-width exception (playground) ---- */
	.shell-full {
		min-height: 100vh;
		background: var(--color-bg);
	}
	.full-main {
		min-width: 0;
	}

	/* ---- Responsive ---- */
	/* Below 1280px: drop the right sidebar (TOC/code pane) and the left rail. */
	@media (max-width: 1279px) {
		.shell-grid,
		.show-left-rail .shell-grid,
		.no-right-sidebar .shell-grid,
		.no-right-sidebar.show-left-rail .shell-grid {
			grid-template-columns: var(--current-nav-width) 1fr;
		}
		.shell-sidebar {
			display: none;
		}
		.shell :global(.left-rail) {
			display: none;
		}
	}
	/* Below 768px: single column, nav becomes a drawer. The
	   .no-right-sidebar variants must be listed explicitly — their two-class
	   selectors outrank the one-class rules here and would otherwise keep a
	   phantom nav-width track next to the (hidden) drawer nav. */
	@media (max-width: 767px) {
		.shell-grid,
		.show-left-rail .shell-grid,
		.no-right-sidebar .shell-grid,
		.no-right-sidebar.show-left-rail .shell-grid {
			grid-template-columns: 1fr;
		}
		.shell-nav {
			display: none;
		}
		.shell-center {
			border-inline-end: none;
		}
	}
</style>
