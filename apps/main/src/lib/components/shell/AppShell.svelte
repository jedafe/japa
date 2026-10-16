<script lang="ts">
	import { type Snippet } from 'svelte';
	import { page } from '$app/state';
	import { Menu as MenuIcon, Sun, Moon } from 'lucide-svelte';
	import LeftNav from './LeftNav.svelte';
	import RightSidebar from './RightSidebar.svelte';
	import LeftRail from './LeftRail.svelte';
	import { routeSidebar } from './routeSidebar.svelte';
	import ComposeModal from './ComposeModal.svelte';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import { t } from '$lib/i18n';
	import { theme } from '$lib/stores/theme.svelte';
	import type { Menu, SessionUser, TrendingTag, SpaceSuggestion, UserSpaceRef } from '$lib/types';

	interface Props {
		menu: Menu | null;
		user: SessionUser | null;
		collapsed: boolean;
		onToggleCollapse: () => void;
		trendingTags?: TrendingTag[];
		suggestedSpaces?: SpaceSuggestion[];
		userSpaces?: UserSpaceRef[];
		/** Platform logo URL from appearance.logo_url (control panel). */
		logoUrl?: string;
		/** Platform dark-theme logo URL from appearance.logo_url_dark (control panel). */
		logoDarkUrl?: string;
		/** Comma-separated enabled languages from i18n.enabled_languages. */
		enabledLanguages?: string;
		children: Snippet;
		/** ADR-058 Exception routes suppress the right sidebar entirely. */
		hideRightSidebar?: boolean;
		/** Page Builder blocks for the right sidebar (sitewide). */
		rightSidebarWidgets?: unknown[];
		/** Admin-managed left-position widgets (Section 59) — rendered as a
		 *  rail that only appears while the side nav is collapsed. */
		leftSidebarWidgets?: unknown[];
		/** Whether the global Compose bottom-sheet is available on this route. */
		composeEnabled?: boolean;
		/** Feature flags from /feature-flags — passed to LeftNav to hide
		 *  nav items for disabled features. */
		features?: Record<string, boolean>;
	}

	let {
		menu,
		user,
		collapsed,
		onToggleCollapse,
		trendingTags = [],
		suggestedSpaces = [],
		userSpaces = [],
		logoUrl = '',
	logoDarkUrl = '',
		enabledLanguages = 'en,fr',
		children,
		hideRightSidebar = false,
		rightSidebarWidgets = [],
		leftSidebarWidgets = [],
		composeEnabled = false,
		features = {}
	}: Props = $props();

	let mobileNavOpen = $state(false);
	// Hover behavior on the collapsed rail: default = expand in place
	// (grid column grows, content shifts — same as clicking expand). On the
	// excepted utility surfaces it renders an overlay peek instead — no
	// layout reflow on jump-heavy pages. The persisted nav_collapsed
	// preference is untouched either way.
	let navHoverExpand = $state(false);
	let navHoverPeek = $state(false);
	const overlayRoutes = ['/ai', '/settings', '/dashboard', '/services', '/studio'];
	const overlayAllowed = $derived(
		overlayRoutes.some(
			(r) => page.url.pathname === r || page.url.pathname.startsWith(`${r}/`)
		)
	);
	$effect(() => {
		// Reset transient hover state on navigation and when the nav is
		// pinned open — otherwise a stale peek could re-render when
		// returning to an overlay route without touching the rail.
		page.url.pathname;
		if (!collapsed || !overlayAllowed) navHoverPeek = false;
		if (!collapsed) navHoverExpand = false;
	});
	const effectiveCollapsed = $derived(collapsed && !navHoverExpand);
	const navWidth = $derived(
		effectiveCollapsed ? 'var(--nav-width-collapsed)' : 'var(--nav-width-expanded)'
	);

	// Left rail (Section 59, ADR-066): an ad-only 160px column between the
	// collapsed nav and the content. It opens only while the nav is collapsed
	// AND at least one resolved left widget is an ad_slot — other widget
	// types assigned to the left position do not render here.
	const showLeftRail = $derived(
		collapsed &&
			(leftSidebarWidgets as { widget_type?: string }[]).some(
				(w) => w?.widget_type === 'ad_slot'
			)
	);

	// ADR-058: route-specific sidebar snippet is provided by child routes via
	// SetSidebar writing to the shared routeSidebar registry. Reset it at init —
	// AppShell initializes before children render on every SSR pass, so a stale
	// registration can never leak into a route that mounts no SetSidebar.
	routeSidebar.owner = null;
	routeSidebar.snippet = null;
</script>

<div class="shell" class:hide-sidebar={hideRightSidebar} class:show-left-rail={showLeftRail} style="--current-nav-width: {navWidth}" data-nav-collapsed={effectiveCollapsed}>
	<div
		class="shell-nav"
		role="presentation"
		onmouseenter={() => {
			if (!collapsed) return;
			if (overlayAllowed) navHoverPeek = true;
			else navHoverExpand = true;
		}}
		onmouseleave={() => (navHoverExpand = false)}
	>
		<LeftNav {menu} {user} collapsed={effectiveCollapsed} pinned={!collapsed} {userSpaces} {logoUrl} {logoDarkUrl} {features} onToggle={onToggleCollapse} />
	</div>

	{#if collapsed && navHoverPeek && overlayAllowed}
		<!-- Expanded overlay nav — covers the rail; closes on mouseleave. The
		     LeftNav toggle inside calls onToggleCollapse, pinning it open. -->
		<div class="nav-hover-peek" role="presentation" onmouseleave={() => (navHoverPeek = false)}>
			<LeftNav {menu} {user} collapsed={false} pinned={false} {userSpaces} {logoUrl} {logoDarkUrl} {features} onToggle={onToggleCollapse} />
		</div>
	{/if}

	{#if showLeftRail}
		<LeftRail widgets={leftSidebarWidgets} />
	{/if}

	<main class="shell-center">
		<div class="mobile-topbar">
			<a href="/" class="flex min-h-11 min-w-11 items-center justify-center gap-2 font-medium text-[var(--color-text)]">
				<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name')} class="h-9 w-auto max-w-[120px] object-contain" />
			<span class="hidden">{t('app.name')}</span>
			</a>
			<div class="flex items-center gap-1">
				<button
					type="button"
					onclick={() => theme.toggle()}
					class="grid h-11 w-11 place-items-center rounded-[var(--radius)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					aria-label={theme.current === 'dark' ? t('theme.toggle_light', 'Switch to light') : t('theme.toggle_dark', 'Switch to dark')}
				>
					{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
				</button>
				<button
					type="button"
					onclick={() => (mobileNavOpen = true)}
				class="grid h-11 w-11 place-items-center rounded-[var(--radius)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('a11y.open_menu')}
			>
				<MenuIcon size={20} />
			</button>
		</div>
	</div>
		{@render children()}
		{#if composeEnabled && user}
			<ComposeModal />
		{/if}
	</main>

	<!-- The rail stays mounted while hidden (hide-sidebar collapses the grid
	     column) — unmounting on /ai rail toggles or exception routes would
	     destroy every widget's loaded state and replay fetch + empty-state
	     flashes on the way back. -->
	<div class="shell-sidebar" class:hidden={hideRightSidebar} data-tour="sidebar" aria-hidden={hideRightSidebar || undefined}>
		<RightSidebar {user} {trendingTags} {suggestedSpaces} {enabledLanguages} {rightSidebarWidgets} />
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
		<!-- Drawer nav is transient (dismissed via backdrop/button), so the
		     toggle shows the unpinned outline state. -->
		<LeftNav {menu} {user} collapsed={false} pinned={false} {userSpaces} {logoUrl} {logoDarkUrl} {features} onToggle={() => (mobileNavOpen = false)} />
	</div>
{/if}

<style>
	.shell {
		display: grid;
		grid-template-columns: var(--current-nav-width) 1fr var(--sidebar-width);
		min-height: 100vh;
		background-color: var(--color-bg);
	}

	/* ADR-058 Exception routes: drop the right sidebar column entirely. */
	.shell.hide-sidebar {
		grid-template-columns: var(--current-nav-width) 1fr;
	}

	/* Left rail (Section 59): extra column between the collapsed nav and the
	   content, only while the side nav is collapsed. */
	.shell.show-left-rail {
		grid-template-columns: var(--current-nav-width) var(--left-rail-width) 1fr var(--sidebar-width);
	}
	.shell.show-left-rail.hide-sidebar {
		grid-template-columns: var(--current-nav-width) var(--left-rail-width) 1fr;
	}

	.shell-nav {
		position: sticky;
		top: 0;
		height: 100vh;
	}

	/* Collapsed nav: raise the rail's stacking context so the item-label
	   flyouts rendered inside LeftNav paint above the left rail and page
	   content (sticky siblings would otherwise overlay them in DOM order). */
	.shell[data-nav-collapsed='true'] .shell-nav {
		z-index: 30;
	}

	.shell-center {
		min-width: 0;
	}

	.shell-sidebar {
		position: sticky;
		top: 0;
		height: 100vh;
		padding-right: 1rem;
	}

	.mobile-topbar {
		display: none;
		position: sticky;
		top: 0;
		z-index: 20;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		min-height: 3.5rem;
		padding: env(safe-area-inset-top) 0.75rem 0.5rem;
		border-bottom: 1px solid var(--color-border);
		background-color: var(--color-surface);
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
	}

	/* Hover-peek overlay for the collapsed rail on the excepted utility
	   routes — slides over the content without a layout shift. */
	.nav-hover-peek {
		position: fixed;
		top: 0;
		left: 0;
		z-index: 50;
		width: var(--nav-width-expanded);
		height: 100vh;
		box-shadow: 8px 0 24px rgb(0 0 0 / 0.12);
		animation: nav-hover-peek-in 0.15s ease-out;
	}

	@keyframes nav-hover-peek-in {
		from {
			opacity: 0.4;
			transform: translateX(-8px);
		}
		to {
			opacity: 1;
			transform: translateX(0);
		}
	}

	/* No hover state on touch — the rail stays tap-to-expand there. */
	@media (max-width: 767px) {
		.nav-hover-peek {
			display: none;
		}
	}

	/* Below 1280px: drop the right sidebar and the left rail. */
	@media (max-width: 1279px) {
		.shell,
		.shell.hide-sidebar,
		.shell.show-left-rail,
		.shell.show-left-rail.hide-sidebar {
			grid-template-columns: var(--current-nav-width) 1fr;
		}
		.shell-sidebar {
			display: none;
		}
		.shell :global(.left-rail) {
			display: none;
		}
	}

	/* Below 768px: single column, nav becomes a drawer via the mobile top bar. */
	@media (max-width: 767px) {
		.shell,
		.shell.hide-sidebar,
		.shell.show-left-rail,
		.shell.show-left-rail.hide-sidebar {
			grid-template-columns: 1fr;
		}
		.shell-nav {
			display: none;
		}
		.mobile-topbar {
			display: flex;
		}
	}
</style>
