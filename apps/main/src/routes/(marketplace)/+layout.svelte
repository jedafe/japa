<script lang="ts">
	// ADR-058 shell exception: the marketplace group now renders with only
	// the standard LeftNav as a sticky sidebar at lg+ — MarketplaceHeader /
	// MarketplaceFooter chrome was dropped so /services sits inside the same
	// primary navigation as the rest of the app (services/mentorship
	// unification, Task 8). Authoring routes (/services/new,
	// /services/[slug]/edit) live in (app).
	// Ref: docs/superpowers/plans/2026-10-07-services-marketplace-frontend.md (Task 2)
	import { untrack, onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import LeftNav from '$lib/components/shell/LeftNav.svelte';
	import ComposeModal from '$lib/components/shell/ComposeModal.svelte';
	import ProductTour from '$lib/components/onboarding/ProductTour.svelte';
	import MediaLightbox from '$lib/components/ui/MediaLightbox.svelte';
	import AdConversionTracker from '$lib/components/ads/AdConversionTracker.svelte';
	import { lightbox } from '$lib/lightbox.svelte';
	import { realtime } from '$lib/realtime/realtime.svelte';
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

	// Section 09 Product Tour — same ?tour=1/?tour=replay handling and
	// latch as (app)/+layout.svelte so the walkthrough survives the shell
	// boundary when onboarding links land on /services.
	const tourParam = $derived(page.url.searchParams.get('tour'));
	const tourRequested = $derived(
		tourParam === 'replay' || (tourParam === '1' && !data.tourDismissed)
	);
	let tourMounted = $state(false);
	$effect(() => {
		if (tourRequested && data.user) tourMounted = true;
	});

	// Realtime socket + PWA service worker — identical mounting pattern to
	// (app)/+layout.svelte; notifications/DM must stay live while browsing
	// the marketplace group.
	// Ref: sections/10-realtime-websockets.md, sections/35-notification-system.md
	onMount(() => {
		if (data.user) realtime.connect();
		if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
			const swType = import.meta.env.DEV ? 'module' : 'classic';
			const registerSw = () => {
				navigator.serviceWorker.register('/service-worker.js', { type: swType }).catch((err) => {
					// eslint-disable-next-line no-console
					console.error('Service worker registration failed:', err);
				});
			};
			if (import.meta.env.DEV) {
				navigator.serviceWorker.getRegistration().then((reg) => {
					if (reg) {
						reg.unregister().then(registerSw).catch(registerSw);
					} else {
						registerSw();
					}
				});
			} else {
				registerSw();
			}
		}
		return () => realtime.disconnect();
	});

	// Reconnect on mid-session login, drop the socket on logout (mirrors
	// (app)/+layout.svelte).
	$effect(() => {
		if (data.user) realtime.connect();
		else realtime.disconnect();
	});

	// Seed the realtime unread-notification counter from the SSR value.
	$effect(() => {
		realtime.setUnreadCount(data.unreadNotifications);
	});
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

{#if tourMounted}
	<!-- §09 Product Tour: steps come from onboarding.tour_steps (admin-managed) -->
	<ProductTour replay={true} steps={data.tourSteps} />
{/if}

<!-- Shared fullscreen media viewer — any surface opens it via openLightbox() -->
<MediaLightbox bind:open={lightbox.open} bind:index={lightbox.index} items={lightbox.items} />

<!-- §25: fires the conversion ad event when a navigation lands with the
     same-origin attribution param (jc_ad_cid). Covers guests and authed
     users — ad serving is optional-auth. -->
<AdConversionTracker />

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
			top: 0;
			align-self: flex-start;
			flex-shrink: 0;
			width: var(--current-nav-width);
			height: 100dvh;
		}

		/* Raise the collapsed rail so its label flyouts paint above the
		   page content (same stacking fix as AppShell's shell-nav). */
		.marketplace-nav[data-nav-collapsed='true'] {
			z-index: 30;
		}
	}
</style>
