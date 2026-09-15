<script lang="ts">
	import { untrack, onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import GuestHeader from '$lib/components/shell/GuestHeader.svelte';
	import GuestFooter from '$lib/components/shell/GuestFooter.svelte';
	import MobileBottomTabBar from '$lib/components/shell/MobileBottomTabBar.svelte';
	import { isExceptionRoute } from '$lib/components/create/gate';
	import { api } from '$lib/api/client';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import ProductTour from '$lib/components/onboarding/ProductTour.svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// ADR-058 Exception #2: /create collapses the nav and hides the right sidebar.
	const exception = $derived(isExceptionRoute(page.route.id));

	// Section 09 Product Tour: ?tour=1 triggers the contextual walkthrough
	// after onboarding completion. The tour starts once the app shell renders.
	// Guard: if the user previously dismissed the tour (tour_dismissed=true),
	// ?tour=1 is suppressed so it never re-triggers automatically. The
	// explicit "Replay tour" link uses ?tour=replay to override the guard.
	const tourParam = $derived(page.url.searchParams.get('tour'));
	const tourRequested = $derived(
		tourParam === 'replay' || (tourParam === '1' && !data.tourDismissed)
	);

	// ADR-058 §5 Exception #3: guest landing page renders WITHOUT the app shell.
	// The new default guest homepage (guestHome) also uses GuestHeader and a custom footer.
	const isGuestLanding = $derived(!!page.data.landing || !!page.data.guestHome);

	// Settings hub: auto-collapse the left nav to give settings panels more
	// room, but keep the right sidebar and let the user expand via the toggle.
	const isSettings = $derived(page.route.id?.startsWith('/(app)/settings') ?? false);

	// Dashboard hub: same treatment — the dashboard has its own HubRail
	// (secondary left nav for the dashboard views), so collapsing the main
	// LeftNav gives the dashboard content more room and avoids two stacked
	// left rails. The user can still expand via the toggle.
	const isDashboard = $derived(page.route.id?.startsWith('/(app)/dashboard') ?? false);

	// ADR-058 Exception #1: content single pages (blog, content, guide,
	// story, comparisons) still render the unified shell's left ad rail. /create
	// is the exception exception: it owns its own layout and hides the left rail.
	const isCreateRoute = $derived(page.route.id === '/(app)/create');

	// Seed from the server value once; thereafter this is user-controlled.
	let collapsed = $state(untrack(() => data.navCollapsed));

	// Auto-collapse when entering settings or dashboard (once per transition).
	// The user can still expand via the toggle — we only collapse on entry,
	// not force.
	let prevWasCollapsedZone = $state(false);
	$effect(() => {
		const inCollapsedZone = isSettings || isDashboard;
		if (inCollapsedZone && !prevWasCollapsedZone) {
			collapsed = true;
		}
		prevWasCollapsedZone = inCollapsedZone;
	});


	async function toggleCollapse() {
		const next = !collapsed;
		collapsed = next; // optimistic
		if (!data.user) return; // guests have nowhere to persist; keep it session-local
		const res = await api.patch('/me/account', { ui: { nav_collapsed: next } });
		if (res.error) collapsed = !next; // rollback on failure
	}

	// Phase 23 — Real-Time WebSocket: connect when an authenticated session is
	// present AND the feature.dm flag is enabled. The backend gates the
	// /auth/ws-ticket route behind feature.dm, so without this guard the client
	// would fire a doomed POST (404 when off, 502 when the backend is down) and
	// enter an infinite reconnect loop. The store is a no-op during SSR;
	// onMount guarantees a browser-only connect.
	// Ref: sections/10-realtime-websockets.md, sections/42-feature-flags.md
	const dmEnabled = $derived(data.features?.['feature.dm'] === true);
	onMount(() => {
		if (data.user && dmEnabled) realtime.connect();
		// Register the PWA service worker so push subscriptions can be created.
		// Ref: japaconnect-engineering/sections/35-notification-system.md
		if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
			// SvelteKit/Vite serves the source service worker as an ES module in dev,
			// but compiles it to a classic IIFE for production. Use the matching
			// registration type so the browser does not fail to evaluate the script.
			const swType = import.meta.env.DEV ? 'module' : 'classic';
			const registerSw = () => {
				navigator.serviceWorker.register('/service-worker.js', { type: swType }).catch((err) => {
					// eslint-disable-next-line no-console
					console.error('Service worker registration failed:', err);
				});
			};
			if (import.meta.env.DEV) {
				// During dev, an existing classic-registered worker (from before this
				// fix or from production) would try to re-fetch the script as a classic
				// script and fail because it is now an ES module. Unregister any stale
				// worker first so the new module registration can take over.
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

	// Reconnect if the user logs in mid-session (e.g. after /login redirect)
	// or if the feature flag is toggled on while the page is open.
	$effect(() => {
		if (data.user && dmEnabled) realtime.connect();
	});

	// Seed the realtime unread-notification counter from the SSR value, and
	// re-sync whenever the server payload changes (e.g. after invalidateAll).
	$effect(() => {
		realtime.setUnreadCount(data.unreadNotifications);
	});
</script>

{#if isGuestLanding}
	<!-- ADR-058 §5 Exception #3: guest landing page — GuestHeader + content + GuestFooter -->
	<GuestHeader
		logoUrl={page.data.guestHome
			? '/logos/homepage-logo-light.png'
			: (data.siteSettings?.['appearance.logo_url'] as string | undefined)}
		logoDarkUrl={page.data.guestHome
			? '/logos/homepage-logo-dark.png'
			: (data.siteSettings?.['appearance.logo_url_dark'] as string | undefined)}
		menu={data.menu}
	/>
	{@render children()}
	{#if !page.data.guestHome && !page.data.hasGuestHomeBlock}
		<GuestFooter menu={data.footerMenu} />
	{/if}
{:else}
	<!-- AppShell handles both authenticated and guest users. The LeftNav
	     shows a Login button for guests and filters menu items by visibility.
	     The RightSidebar renders admin-managed widgets for all users.
	     The left rail is suppressed on /create and the auto-collapse zones
	     (/settings, /dashboard). Content single pages (exception #1) keep the
	     left ad rail so the article body can devote its full center column to
	     the reading layout. -->
	<AppShell
		menu={data.menu}
		user={data.user}
		collapsed={exception || collapsed}
		onToggleCollapse={toggleCollapse}
		trendingTags={data.trendingTags}
		suggestedSpaces={data.suggestedSpaces}
		userSpaces={data.userSpaces}
		logoUrl={data.siteSettings?.['appearance.logo_url'] as string | undefined}
		logoDarkUrl={data.siteSettings?.['appearance.logo_url_dark'] as string | undefined}
		enabledLanguages={data.siteSettings?.['i18n.enabled_languages'] as string | undefined}
		hideRightSidebar={exception || !data.rightSidebarEnabled}
		rightSidebarWidgets={data.rightSidebarWidgets}
		leftSidebarWidgets={isCreateRoute || isSettings || isDashboard ? [] : data.leftSidebarWidgets}

		composeEnabled={!exception && !!data.user}
		features={data.features}
	>
		{@render children()}
	</AppShell>
	{#if !exception}
		<MobileBottomTabBar user={data.user} features={data.features} menu={data.mobileMenu} />
	{/if}
	{#if tourRequested && data.user}
		<!-- Section 09 Product Tour: triggered after onboarding completion -->
		<ProductTour replay={true} />
	{/if}
{/if}

<style></style>
