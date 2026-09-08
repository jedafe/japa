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

	// Mobile bottom tab bar is hidden only on the full-screen composer. It is
	// intentionally shown on reading/exception routes (e.g. blog single, story,
	// guide, content) so guests and logged-in users always have main navigation.
	const hideBottomNav = $derived(page.route.id === '/(app)/create');

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
	const isGuestLanding = $derived(!!page.data.landing);

	// Settings hub: auto-collapse the left nav to give settings panels more
	// room, but keep the right sidebar and let the user expand via the toggle.
	const isSettings = $derived(page.route.id?.startsWith('/(app)/settings') ?? false);

	// Dashboard hub: same treatment — the dashboard has its own HubRail
	// (secondary left nav for the dashboard views), so collapsing the main
	// LeftNav gives the dashboard content more room and avoids two stacked
	// left rails. The user can still expand via the toggle.
	const isDashboard = $derived(page.route.id?.startsWith('/(app)/dashboard') ?? false);

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
		logoUrl={data.siteSettings?.['appearance.logo_url'] as string | undefined}
		menu={data.menu}
	/>
	{@render children()}
	<GuestFooter menu={data.footerMenu} />
{:else}
	<!-- AppShell handles both authenticated and guest users. The LeftNav
	     shows a Login button for guests and filters menu items by visibility.
	     The RightSidebar renders admin-managed widgets for all users.
	     The left rail is suppressed on exception routes AND the auto-collapse
	     zones (/settings, /dashboard) — those already run with the nav
	     collapsed for their own rails, so a widget rail there would
	     compete for the same space. -->
	<AppShell
		menu={data.menu}
		user={data.user}
		collapsed={exception || collapsed}
		onToggleCollapse={toggleCollapse}
		trendingTags={data.trendingTags}
		suggestedSpaces={data.suggestedSpaces}
		userSpaces={data.userSpaces}
		logoUrl={data.siteSettings?.['appearance.logo_url'] as string | undefined}
		enabledLanguages={data.siteSettings?.['i18n.enabled_languages'] as string | undefined}
		hideRightSidebar={exception || !data.rightSidebarEnabled}
		rightSidebarWidgets={data.rightSidebarWidgets}
		leftSidebarWidgets={exception || isSettings || isDashboard ? [] : data.leftSidebarWidgets}

		composeEnabled={!exception && !!data.user}
		features={data.features}
	>
		{@render children()}
	</AppShell>
	{#if !hideBottomNav}
		<MobileBottomTabBar user={data.user} features={data.features} menu={data.mobileMenu} />
	{/if}
	{#if tourRequested && data.user}
		<!-- Section 09 Product Tour: triggered after onboarding completion -->
		<ProductTour replay={true} />
	{/if}
{/if}

<style></style>
