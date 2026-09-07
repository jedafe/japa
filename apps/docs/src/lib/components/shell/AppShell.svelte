<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Menu as MenuIcon, PanelLeftClose, PanelLeft } from 'lucide-svelte';
	import DocNavTree from './DocNavTree.svelte';
	import TableOfContents from './TableOfContents.svelte';
	import SearchLauncher from './SearchLauncher.svelte';
	import LeftRail from './LeftRail.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import SidebarWidgetRenderer from './SidebarWidgetRenderer.svelte';
	import GuestFooter from './GuestFooter.svelte';
	import GuestAuthButtons from './GuestAuthButtons.svelte';
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
		/** Footer menu from GET /menus/docs_footer — ADR-054. */
		footerMenu?: Menu | null;
		/** Sidebar widgets from the Sidebar Management System (Section 59, ADR-066).
		 *  Resolved via /sidebars/resolve and rendered by SidebarWidgetRenderer. */
		rightSidebarWidgets?: unknown[];
		/** Admin-managed left-position widgets — only `ad_slot` widgets render,
		 *  as a 160×600 rail while the nav is collapsed (Section 59, ADR-066). */
		leftSidebarWidgets?: unknown[];
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
		footerMenu = null,
		rightSidebarWidgets = [],
		leftSidebarWidgets = [],
		children
	}: Props = $props();

	// Normalize incoming widgets to the SidebarWidget shape.
	const sidebarWidgets = $derived(
		(rightSidebarWidgets as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number; slot?: string }[]).filter(
			(w) => w && typeof w.widget_type === 'string'
		)
	);

	// Split widgets into topbar (sticky) and content (scrollable) slots using
	// the `slot` field from the widget type registry (Section 59 §6, BE-S3).
	// Falls back to a hardcoded topbar type set for backward compat with older
	// cached payloads that do not include `slot`. This replaces the previously
	// hardcoded sidebar-topbar (SearchLauncher + UserDropdown), which
	// duplicated the admin-managed topbar widgets seeded by migration 000223.
	const topbarWidgetTypes = new Set(['search', 'search_launcher', 'theme_toggle', 'language_toggle', 'user_dropdown']);
	function isTopbar(w: { slot?: string; widget_type: string; props?: Record<string, unknown> }): boolean {
		// Explicit slot field on the widget (from the sidebar API)
		if (w.slot === 'topbar') return true;
		if (w.slot === 'content') return false;
		// Check props.slot for widgets that store slot in their props (e.g., ad_slot)
		const propsSlot = w.props?.slot;
		if (propsSlot === 'topbar') return true;
		if (propsSlot === 'content' || propsSlot === 'sidebar_top' || propsSlot === 'sidebar_bottom') return false;
		// Fallback: known topbar widget types
		return topbarWidgetTypes.has(w.widget_type);
	}
	let topbarWidgets = $derived(sidebarWidgets.filter((w) => isTopbar(w)));
	let contentWidgets = $derived(sidebarWidgets.filter((w) => !isTopbar(w)));

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
</script>

{#if isFull}
	<!-- ADR-060 full-width exception: the API playground breaks the shell. -->
	<div class="shell-full">
		<div class="full-topbar">
			<a href="/docs" class="full-brand">
				{#if logoUrl}
					<img src={logoUrl} alt={t('app.name')} class="full-brand-logo" />
				{:else}
					<span class="brand-mark">JC</span>
				{/if}
				<span>{t('app.name')}</span>
			</a>
			<div class="full-topbar-actions">
				<SearchLauncher />
				<UserDropdown {user} />
			</div>
		</div>
		<main class="full-main">
			{@render children()}
		</main>
	</div>
{:else if !user}
	<!-- Guest layout: same 3-column shell as authed users. The DocNavTree
	     doesn't depend on user; the sidebar shows admin-managed widgets.
	     UserDropdown is hidden for guests (no avatar to show). -->
	<div
		class="shell"
		class:show-left-rail={showLeftRail}
		style="--current-nav-width: {navWidth}; --current-right-width: {rightWidth};"
	>
		<div class="shell-nav">
			<DocNavTree
				{nav}
				{accessibleAudiences}
				{activeAudience}
				{collapsed}
				{onToggleCollapse}
				{logoUrl}
			/>
		</div>

		{#if showLeftRail}
			<LeftRail widgets={leftSidebarWidgets} />
		{/if}

		<main class="shell-center">
			<!-- Mobile topbar (hidden on desktop) -->
			<div class="mobile-topbar">
				<button
					type="button"
					onclick={() => (mobileNavOpen = true)}
					class="grid h-9 w-9 place-items-center rounded-[var(--radius)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					aria-label={t('a11y.open_menu')}
				>
					<MenuIcon size={20} />
				</button>
				<a href="/docs" class="flex items-center gap-2 font-medium text-[var(--color-text)]">
					{#if logoUrl}
						<img src={logoUrl} alt={t('app.name')} class="h-6 w-auto object-contain" />
					{:else}
						<span class="brand-mark">JC</span>
					{/if}
					<span>{t('app.name')}</span>
				</a>
				<div class="ml-auto"><SearchLauncher keyboardShortcut={false} /></div>
				{#if !user}<GuestAuthButtons compact />{/if}
			</div>
			{@render children()}
		</main>

		<div class="shell-sidebar">
			<div class="sidebar-topbar">
				{#each topbarWidgets as widget (widget.id)}
					<SidebarWidgetRenderer {widget} {user} />
				{/each}
				{#if !user}
					<GuestAuthButtons />
				{/if}
			</div>
			<div class="sidebar-body">
				{#if variant === 'api' && apiPane}
					{@render apiPane()}
				{:else if sidebar}
					{@render sidebar()}
				{:else}
					<div class="sidebar-empty"></div>
				{/if}

				{#each contentWidgets as widget (widget.id)}
					<div class="mt-4">
						<SidebarWidgetRenderer {widget} {user} />
					</div>
				{/each}
			</div>
		</div>
	</div>

	<GuestFooter menu={footerMenu} {user} />

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
			/>
		</div>
	{/if}
{:else}
	<div
		class="shell"
		class:variant-api={variant === 'api'}
		class:show-left-rail={showLeftRail}
		style="--current-nav-width: {navWidth}; --current-right-width: {rightWidth};"
	>
		<div class="shell-nav">
			<DocNavTree
				{nav}
				{accessibleAudiences}
				{activeAudience}
				{collapsed}
				{onToggleCollapse}
				{logoUrl}
			/>
		</div>

		{#if showLeftRail}
			<LeftRail widgets={leftSidebarWidgets} />
		{/if}

		<main class="shell-center">
			<!-- Mobile topbar (hidden on desktop) -->
			<div class="mobile-topbar">
				<button
					type="button"
					onclick={() => (mobileNavOpen = true)}
					class="grid h-9 w-9 place-items-center rounded-[var(--radius)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					aria-label={t('a11y.open_menu')}
				>
					<MenuIcon size={20} />
				</button>
				<a href="/docs" class="flex items-center gap-2 font-medium text-[var(--color-text)]">
					{#if logoUrl}
						<img src={logoUrl} alt={t('app.name')} class="h-6 w-auto object-contain" />
					{:else}
						<span class="brand-mark">JC</span>
					{/if}
					<span>{t('app.name')}</span>
				</a>
				<div class="ml-auto"><SearchLauncher keyboardShortcut={false} /></div>
			</div>
			{@render children()}
		</main>

		<div class="shell-sidebar">
			<div class="sidebar-topbar">
				{#each topbarWidgets as widget (widget.id)}
					<SidebarWidgetRenderer {widget} {user} />
				{/each}
			</div>
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
			</div>
		</div>
	</div>

	<GuestFooter menu={footerMenu} {user} />

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
			/>
		</div>
	{/if}
{/if}

<style>
	/* ---- Standard shell (doc + api variants) ---- */
	.shell {
		display: grid;
		grid-template-columns: var(--current-nav-width) 1fr var(--current-right-width);
		min-height: 100vh;
		background-color: var(--color-bg);
	}
	.shell-nav {
		position: sticky;
		top: 0;
		height: 100vh;
	}
	.shell-center {
		min-width: 0;
		border-right: 1px solid var(--color-border);
	}
	.shell-sidebar {
		position: sticky;
		top: 0;
		height: 100vh;
		display: flex;
		flex-direction: column;
		border-left: 1px solid var(--color-border);
		background: var(--color-surface);
	}
	.sidebar-topbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		height: var(--nav-height);
		padding: 0 0.75rem;
		border-bottom: 1px solid var(--color-border);
		flex-shrink: 0;
	}
	.sidebar-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 1rem 0.75rem 2rem;
	}
	.sidebar-empty {
		height: 100%;
	}

	.mobile-topbar {
		display: none;
		position: sticky;
		top: 0;
		z-index: 20;
		align-items: center;
		gap: 0.5rem;
		height: var(--nav-height);
		padding: 0 0.75rem;
		border-bottom: 1px solid var(--color-border);
		background-color: var(--color-surface);
	}

	.brand-mark {
		display: inline-grid;
		place-items: center;
		height: 1.5rem;
		width: 1.5rem;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.6875rem;
		font-weight: 700;
		font-family: var(--font-sans);
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
		left: 0;
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
	.full-topbar {
		display: flex;
		align-items: center;
		gap: 1rem;
		height: var(--nav-height);
		padding: 0 1rem;
		border-bottom: 1px solid var(--color-border);
		background: var(--color-surface);
		position: sticky;
		top: 0;
		z-index: 20;
	}
	.full-topbar-actions {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.full-brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 600;
		color: var(--color-text);
		flex-shrink: 0;
	}
	.full-brand-logo {
		height: 1.75rem;
		width: auto;
		max-width: 140px;
		object-fit: contain;
	}
	.full-main {
		min-width: 0;
	}

	/* ---- Desktop topbar ---- */
	/* ---- Responsive ---- */
	/* Below 1280px: drop the right sidebar (TOC/code pane) and the left rail. */
	@media (max-width: 1279px) {
		.shell,
		.shell.show-left-rail {
			grid-template-columns: var(--current-nav-width) 1fr;
		}
		.shell-sidebar {
			display: none;
		}
		.shell :global(.left-rail) {
			display: none;
		}
	}
	/* Below 768px: single column, nav becomes a drawer. */
	@media (max-width: 767px) {
		.shell,
		.shell.show-left-rail {
			grid-template-columns: 1fr;
		}
		.shell-nav {
			display: none;
		}
		.shell-center {
			border-right: none;
		}
		.mobile-topbar {
			display: flex;
		}
	}
</style>
