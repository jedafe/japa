<script lang="ts">
	// ADR-058 shell exception: public marketplace chrome. The /services
	// directory leaves the AppShell — public browsing pages render under
	// MarketplaceHeader + MarketplaceFooter instead of the left-nav shell,
	// while authoring routes (/services/new, /services/[slug]/edit) stay in
	// (app). Same pattern as the GuestHeader/GuestFooter exceptions in
	// (app)/+layout.svelte.
	// Ref: docs/superpowers/plans/2026-10-07-services-marketplace-frontend.md (Task 2)
	import type { Snippet } from 'svelte';
	import MarketplaceHeader from '$lib/components/marketplace/MarketplaceHeader.svelte';
	import MarketplaceFooter from '$lib/components/marketplace/MarketplaceFooter.svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();
</script>

<MarketplaceHeader
	user={data.user}
	unreadCount={data.unreadNotifications}
	logoUrl={data.siteSettings?.['appearance.logo_url'] as string | undefined}
	logoDarkUrl={data.siteSettings?.['appearance.logo_url_dark'] as string | undefined}
/>
<main class="min-h-[60vh]">
	{@render children()}
</main>
<MarketplaceFooter
	menu={data.footerMenu}
	newsletterEnabled={data.features?.['feature.newsletter'] !== false}
/>
