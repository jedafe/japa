<script lang="ts">
	// ADR-058 shell exception: the marketplace group now renders with only
	// the standard LeftNav as a sticky sidebar at lg+ — MarketplaceHeader /
	// MarketplaceFooter chrome was dropped so /services sits inside the same
	// primary navigation as the rest of the app (services/mentorship
	// unification, Task 8). Authoring routes (/services/new,
	// /services/[slug]/edit) live in (app).
	// Ref: docs/superpowers/plans/2026-10-07-services-marketplace-frontend.md (Task 2)
	import { untrack, type Snippet } from 'svelte';
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
			onToggle={toggleCollapse}
		/>
	</aside>
	<main class="marketplace-center min-h-[60vh] px-4 py-6 sm:px-6 lg:px-8">
		{@render children()}
		{#if data.user}
			<!-- LeftNav's compose button needs the modal in-tree — AppShell
			     mounts it inside shell-center; mirror that here. -->
			<ComposeModal />
		{/if}
	</main>
</div>

<style>
	.marketplace-body {
		display: flex;
		align-items: stretch;
	}

	.marketplace-center {
		flex: 1;
		min-width: 0;
	}

	/* Sidebar is desktop-only — with the header/footer chrome gone there
	   is no marketplace nav below lg; pages carry their own breadcrumbs. */
	.marketplace-nav {
		display: none;
	}

	@media (min-width: 1024px) {
		.marketplace-nav {
			display: block;
			position: sticky;
			top: 1rem;
			align-self: flex-start;
			flex-shrink: 0;
			width: var(--current-nav-width);
			height: calc(100dvh - 2rem);
		}

		/* Raise the collapsed rail so its label flyouts paint above the
		   page content (same stacking fix as AppShell's shell-nav). */
		.marketplace-nav[data-nav-collapsed='true'] {
			z-index: 30;
		}
	}
</style>
