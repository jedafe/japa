<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Menu as MenuIcon } from 'lucide-svelte';
	import LeftNav from './LeftNav.svelte';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import RightSidebar from './RightSidebar.svelte';
	import GuestFooter from './GuestFooter.svelte';
	import LeftRail from './LeftRail.svelte';
	import { t } from '$lib/i18n';
	import type { Menu, SessionUser } from '$lib/types';

	interface Props {
		menu: Menu | null;
		/** Footer menu from GET /menus/learn_footer — ADR-054. */
		footerMenu?: Menu | null;
		user: SessionUser | null;
		collapsed: boolean;
		onToggleCollapse: () => void;
		/** Platform logo URL from appearance.logo_url (control panel). */
		logoUrl?: string;
		/** Platform dark-theme logo URL from appearance.logo_url_dark (control panel). */
		logoDarkUrl?: string;
		children: Snippet;
		/** Route-specific right-sidebar widgets. */
		sidebar?: Snippet;
		/** ADR-058 Exception routes suppress the right sidebar entirely. */
		hideRightSidebar?: boolean;
		/** Lesson player: collapse the left nav to 72px (ADR-060 reading exception). */
		lessonPlayer?: boolean;
		/** Sidebar widgets from the Sidebar Management System (Section 59, ADR-066). */
		rightSidebarWidgets?: unknown[];
		/** Admin-managed left-position widgets — only `ad_slot` widgets render,
		 *  as a 160×600 rail while the nav is collapsed (Section 59, ADR-066). */
		leftSidebarWidgets?: unknown[];
	}

	let {
		menu,
		footerMenu = null,
		user,
		collapsed,
		onToggleCollapse,
		logoUrl = '',
		logoDarkUrl = '',
		children,
		sidebar,
		hideRightSidebar = false,
		lessonPlayer = false,
		rightSidebarWidgets = [],
		leftSidebarWidgets = []
	}: Props = $props();

	let mobileNavOpen = $state(false);
	// The lesson player auto-collapses the nav to 72px regardless of the user's
	// preference (ADR-060 reading exception). The user can still manually expand
	// via the toggle button — `userExpanded` overrides the auto-collapse until
	// they navigate away.
	let userExpanded = $state(false);
	const autoCollapsed = $derived(lessonPlayer && !userExpanded);
	const effectiveCollapsed = $derived(autoCollapsed || collapsed);
	const navWidth = $derived(
		effectiveCollapsed
			? 'var(--nav-width-collapsed)'
			: 'var(--nav-width-expanded)'
	);
	// Left rail (Section 59): ad-only 160px column between the collapsed nav
	// and the content, only while the nav is collapsed and an ad_slot widget
	// is assigned to the left position.
	const showLeftRail = $derived(
		effectiveCollapsed &&
			(leftSidebarWidgets as { widget_type?: string }[]).some(
				(w) => w?.widget_type === 'ad_slot'
			)
	);

	function onNavToggle() {
		if (autoCollapsed) {
			// Auto-collapsed: first click expands (overrides auto-collapse),
			// second click re-collapses (back to auto-collapsed state).
			userExpanded = !userExpanded;
		} else {
			onToggleCollapse();
		}
	}
</script>

{#if !user}
	<!-- Guest layout: same 3-column shell as authed users. LeftNav shows
	     a Login button for guests; RightSidebar renders admin-managed widgets. -->
	<div
		class="shell university"
		class:hide-sidebar={hideRightSidebar}
		class:show-left-rail={showLeftRail}
		style="--current-nav-width: {navWidth}"
	>
		<div class="shell-nav">
			<LeftNav {menu} {user} collapsed={effectiveCollapsed} {logoUrl} {logoDarkUrl} onToggle={onNavToggle} />
		</div>

		{#if showLeftRail}
			<LeftRail widgets={leftSidebarWidgets} />
		{/if}

		<main class="shell-center">
			<div class="mobile-topbar">
				<button
					type="button"
					onclick={() => (mobileNavOpen = true)}
					class="grid h-9 w-9 place-items-center rounded-[var(--radius)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					aria-label={t('a11y.open_menu')}
				>
					<MenuIcon size={20} />
				</button>
				<a href="/" class="flex items-center gap-2 font-medium text-[var(--color-text)]">
					{#if logoUrl}
						<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('university.name')} class="h-7 w-auto max-w-[120px] object-contain" />
					{:else}
						<span
							class="grid h-7 w-7 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)] text-xs font-semibold text-[var(--color-primary-contrast)]"
							>JC</span
						>
					{/if}
					<span>{t('university.name')}</span>
				</a>
			</div>
			{@render children()}
			<!-- ADR-054: guest footer from the admin-managed learn_footer menu. -->
			<GuestFooter menu={footerMenu} />
		</main>

		{#if !hideRightSidebar}
			<div class="shell-sidebar">
				<RightSidebar {user} {sidebar} rightSidebarWidgets={rightSidebarWidgets} suppressAds={lessonPlayer} />
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
			<LeftNav {menu} {user} collapsed={false} {logoUrl} {logoDarkUrl} onToggle={() => (mobileNavOpen = false)} />
		</div>
	{/if}
{:else}
	<div
		class="shell university"
		class:hide-sidebar={hideRightSidebar}
		class:show-left-rail={showLeftRail}
		style="--current-nav-width: {navWidth}"
	>
		<div class="shell-nav">
			<LeftNav {menu} {user} collapsed={effectiveCollapsed} {logoUrl} {logoDarkUrl} onToggle={onNavToggle} />
		</div>

		{#if showLeftRail}
			<LeftRail widgets={leftSidebarWidgets} />
		{/if}

		<main class="shell-center">
			<div class="mobile-topbar">
				<button
					type="button"
					onclick={() => (mobileNavOpen = true)}
					class="grid h-9 w-9 place-items-center rounded-[var(--radius)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					aria-label={t('a11y.open_menu')}
				>
					<MenuIcon size={20} />
				</button>
				<a href="/" class="flex items-center gap-2 font-medium text-[var(--color-text)]">
					{#if logoUrl}
						<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('university.name')} class="h-7 w-auto max-w-[120px] object-contain" />
					{:else}
						<span
							class="grid h-7 w-7 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)] text-xs font-semibold text-[var(--color-primary-contrast)]"
							>JC</span
						>
					{/if}
					<span>{t('university.name')}</span>
				</a>
			</div>
			{@render children()}
		</main>

		{#if !hideRightSidebar}
			<div class="shell-sidebar">
				<RightSidebar {user} {sidebar} {rightSidebarWidgets} suppressAds={lessonPlayer} />
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
			<LeftNav {menu} {user} collapsed={false} {logoUrl} {logoDarkUrl} onToggle={() => (mobileNavOpen = false)} />
		</div>
	{/if}
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
		gap: 0.5rem;
		height: var(--nav-height);
		padding: 0 0.75rem;
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
