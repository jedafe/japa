<script lang="ts">
	// ADR-058 shell exception: public marketplace chrome. The /services
	// directory leaves the AppShell — public browsing pages render under
	// MarketplaceHeader + MarketplaceFooter instead of the full three-column
	// shell, while authoring routes (/services/new, /services/[slug]/edit)
	// stay in (app). Same pattern as the GuestHeader/GuestFooter exceptions
	// in (app)/+layout.svelte.
	//
	// The standard LeftNav still mounts as a sticky sidebar at lg+ so the
	// marketplace keeps the same primary navigation as the rest of the app.
	// showBrand=false because MarketplaceHeader already renders the logo;
	// the right sidebar, left ad rail and mobile bottom tab bar stay out —
	// the detail page owns the mobile bottom edge via StickyBookBar.
	// Ref: docs/superpowers/plans/2026-10-07-services-marketplace-frontend.md (Task 2)
	import { untrack, type Snippet } from 'svelte';
	import MarketplaceHeader from '$lib/components/marketplace/MarketplaceHeader.svelte';
	import MarketplaceFooter from '$lib/components/marketplace/MarketplaceFooter.svelte';
	import LeftNav from '$lib/components/shell/LeftNav.svelte';
	import ComposeModal from '$lib/components/shell/ComposeModal.svelte';
	import { api } from '$lib/api/client';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Nav collapse — same seed/persist pattern as (app)/+layout.svelte so
	// the rail state follows the user across the shell boundary.
	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		const next = !collapsed;
		collapsed = next; // optimistic
		if (!data.user) return; // guests have nowhere to persist; keep it session-local
		const res = await api.patch('/me/account', { ui: { nav_collapsed: next } });
		if (res.error) collapsed = !next; // rollback on failure
	}
</script>

<MarketplaceHeader
	user={data.user}
	unreadCount={data.unreadNotifications}
	logoUrl={data.siteSettings?.['appearance.logo_url'] as string | undefined}
	logoDarkUrl={data.siteSettings?.['appearance.logo_url_dark'] as string | undefined}
/>
<div
	class="marketplace-body"
	style="--current-nav-width: {collapsed
		? 'var(--nav-width-collapsed)'
		: 'var(--nav-width-expanded)'}"
>
	<aside class="marketplace-nav" data-nav-collapsed={collapsed}>
		<LeftNav
			menu={data.menu}
			user={data.user}
			{collapsed}
			userSpaces={data.userSpaces}
			logoUrl={data.siteSettings?.['appearance.logo_url'] as string | undefined}
			logoDarkUrl={data.siteSettings?.['appearance.logo_url_dark'] as string | undefined}
			features={data.features}
			showBrand={false}
			onToggle={toggleCollapse}
		/>
	</aside>
	<main class="marketplace-center min-h-[60vh]">
		{@render children()}
		{#if data.user}
			<!-- LeftNav's compose button needs the modal in-tree — AppShell
			     mounts it inside shell-center; mirror that here. -->
			<ComposeModal />
		{/if}
	</main>
</div>
<MarketplaceFooter
	menu={data.footerMenu}
	newsletterEnabled={data.features?.['feature.newsletter'] !== false}
/>

<style>
	.marketplace-body {
		display: flex;
		align-items: stretch;
	}

	.marketplace-center {
		flex: 1;
		min-width: 0;
	}

	/* Sidebar is desktop-only — below lg the MarketplaceHeader hamburger
	   drawer carries navigation, and the service detail page's
	   StickyBookBar owns the mobile bottom edge. */
	.marketplace-nav {
		display: none;
	}

	@media (min-width: 1024px) {
		.marketplace-nav {
			display: block;
			position: sticky;
			top: var(--nav-height);
			align-self: flex-start;
			flex-shrink: 0;
			width: var(--current-nav-width);
			height: calc(100dvh - var(--nav-height));
		}

		/* Raise the collapsed rail so its label flyouts paint above the
		   page content (same stacking fix as AppShell's shell-nav). */
		.marketplace-nav[data-nav-collapsed='true'] {
			z-index: 30;
		}
	}
</style>
