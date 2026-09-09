<script lang="ts">
	import { setContext, type Snippet } from 'svelte';
	import { Menu as MenuIcon } from 'lucide-svelte';
	import LeftNav from './LeftNav.svelte';
	import RightSidebar from './RightSidebar.svelte';
	import LeftRail from './LeftRail.svelte';
	import ComposeModal from './ComposeModal.svelte';
	import { t } from '$lib/i18n';
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
		enabledLanguages = 'en,fr',
		children,
		hideRightSidebar = false,
		rightSidebarWidgets = [],
		leftSidebarWidgets = [],
		composeEnabled = false,
		features = {}
	}: Props = $props();

	let mobileNavOpen = $state(false);
	const navWidth = $derived(
		collapsed ? 'var(--nav-width-collapsed)' : 'var(--nav-width-expanded)'
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

	// ADR-058: route-specific sidebar snippet is provided by child routes
	// via SetSidebar, using Svelte context instead of a module-level store so
	// the snippet is part of the component tree and hydrates correctly.
	const SIDEBAR_CONTEXT = 'appshell:sidebar';
	const sidebarContext = $state<{ current: Snippet | null }>({ current: null });
	setContext(SIDEBAR_CONTEXT, sidebarContext);
</script>

<div class="shell" class:hide-sidebar={hideRightSidebar} class:show-left-rail={showLeftRail} style="--current-nav-width: {navWidth}" data-nav-collapsed={collapsed}>
	<div class="shell-nav">
		<LeftNav {menu} {user} {collapsed} {userSpaces} {logoUrl} {features} onToggle={onToggleCollapse} />
	</div>

	{#if showLeftRail}
		<LeftRail widgets={leftSidebarWidgets} />
	{/if}

	<main class="shell-center">
		<div class="mobile-topbar">
			<a href="/" class="flex items-center gap-2 font-medium text-[var(--color-text)]">
				{#if logoUrl}
					<img src={logoUrl} alt={t('app.name')} class="h-6 w-auto max-w-[120px] object-contain" />
				{:else}
					<span
						class="grid h-6 w-6 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)] text-[10px] font-semibold text-[var(--color-primary-contrast)]"
						>JC</span
					>
				{/if}
				<span class="hidden md:inline">{t('app.name')}</span>
			</a>
			<button
				type="button"
				onclick={() => (mobileNavOpen = true)}
				class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('a11y.open_menu')}
			>
				<MenuIcon size={20} />
			</button>
		</div>
		{@render children()}
		{#if composeEnabled && user}
			<ComposeModal />
		{/if}
	</main>

	{#if !hideRightSidebar}
		<div class="shell-sidebar">
			<RightSidebar {user} {trendingTags} {suggestedSpaces} {enabledLanguages} {rightSidebarWidgets} />
		</div>
	{/if}
</div>

{#if mobileNavOpen}
	<button
		type="button"
		class="drawer-backdrop"
		aria-label={t('a11y.close_menu')}
		onclick={() => (mobileNavOpen = false)}
	></button>
	<div class="drawer">
		<LeftNav {menu} {user} collapsed={false} {userSpaces} {logoUrl} {features} onToggle={() => (mobileNavOpen = false)} />
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

	.shell-center {
		min-width: 0;
		border-right: 1px solid var(--color-border);
	}

	.shell-sidebar {
		position: sticky;
		top: 0;
		height: 100vh;
	}

	.mobile-topbar {
		display: none;
		position: sticky;
		top: 0;
		z-index: 20;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		height: 3rem;
		padding: env(safe-area-inset-top) 0.75rem 0;
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
		.shell-center {
			border-right: none;
		}
		.mobile-topbar {
			display: flex;
		}
	}
</style>
