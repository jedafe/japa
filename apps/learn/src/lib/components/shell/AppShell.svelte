<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Menu as MenuIcon } from 'lucide-svelte';
	import LeftNav from './LeftNav.svelte';
	import RightSidebar from './RightSidebar.svelte';
	import GuestHeader from './GuestHeader.svelte';
	import GuestFooter from './GuestFooter.svelte';
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
		/** True on the Japa University host → branded header variant (ADR-063). */
		isUniversity?: boolean;
		children: Snippet;
		/** Route-specific right-sidebar widgets. */
		sidebar?: Snippet;
		/** ADR-058 Exception routes suppress the right sidebar entirely. */
		hideRightSidebar?: boolean;
		/** Lesson player: collapse the left nav to 72px (ADR-060 reading exception). */
		lessonPlayer?: boolean;
		/** Sidebar widgets from the Sidebar Management System (Section 59, ADR-066). */
		rightSidebarWidgets?: unknown[];
	}

	let {
		menu,
		footerMenu = null,
		user,
		collapsed,
		onToggleCollapse,
		logoUrl = '',
		isUniversity = false,
		children,
		sidebar,
		hideRightSidebar = false,
		lessonPlayer = false,
		rightSidebarWidgets = []
	}: Props = $props();

	let mobileNavOpen = $state(false);
	// The lesson player auto-collapses the nav to 72px regardless of the user's
	// preference (ADR-060 reading exception). The collapsed prop from the layout
	// is overridden here for the grid width only — the user's persisted preference
	// is restored when they leave the player.
	const navWidth = $derived(
		lessonPlayer || collapsed
			? 'var(--nav-width-collapsed)'
			: 'var(--nav-width-expanded)'
	);
</script>

{#if !user}
	<!-- Guest layout: same 3-column shell as authed users. LeftNav shows
	     a Login button for guests; RightSidebar renders admin-managed widgets. -->
	<div
		class="shell"
		class:university={isUniversity}
		style="--current-nav-width: {navWidth}"
	>
		<div class="shell-nav">
			<LeftNav {menu} {user} {collapsed} {logoUrl} {isUniversity} onToggle={onToggleCollapse} />
		</div>

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
						<img src={logoUrl} alt={t('app.name')} class="h-7 w-auto max-w-[120px] object-contain" />
					{:else}
						<span
							class="grid h-7 w-7 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)] text-xs font-semibold text-[var(--color-primary-contrast)]"
							>JC</span
						>
					{/if}
					<span>{isUniversity ? t('university.name') : t('app.name')}</span>
				</a>
			</div>
			{@render children()}
		</main>

		{#if !hideRightSidebar}
			<div class="shell-sidebar">
				<RightSidebar {user} {isUniversity} rightSidebarWidgets={rightSidebarWidgets} />
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
			<LeftNav {menu} {user} collapsed={false} {logoUrl} {isUniversity} onToggle={() => (mobileNavOpen = false)} />
		</div>
	{/if}
{:else}
	<div
		class="shell"
		class:hide-sidebar={hideRightSidebar}
		class:university={isUniversity}
		style="--current-nav-width: {navWidth}"
	>
		<div class="shell-nav">
			<LeftNav {menu} {user} {collapsed} {logoUrl} {isUniversity} onToggle={onToggleCollapse} />
		</div>

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
						<img src={logoUrl} alt={t('app.name')} class="h-7 w-auto max-w-[120px] object-contain" />
					{:else}
						<span
							class="grid h-7 w-7 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)] text-xs font-semibold text-[var(--color-primary-contrast)]"
							>JC</span
						>
					{/if}
					<span>{isUniversity ? t('university.name') : t('app.name')}</span>
				</a>
			</div>
			{@render children()}
		</main>

		{#if !hideRightSidebar}
			<div class="shell-sidebar">
				<RightSidebar {user} {isUniversity} {sidebar} {rightSidebarWidgets} />
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
			<LeftNav {menu} {user} collapsed={false} {logoUrl} {isUniversity} onToggle={() => (mobileNavOpen = false)} />
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

	/* Below 1280px: drop the right sidebar. */
	@media (max-width: 1279px) {
		.shell {
			grid-template-columns: var(--current-nav-width) 1fr;
		}
		.shell-sidebar {
			display: none;
		}
	}

	/* Below 768px: single column, nav becomes a drawer via the mobile top bar. */
	@media (max-width: 767px) {
		.shell {
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
