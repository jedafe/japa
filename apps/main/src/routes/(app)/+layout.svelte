<script lang="ts">
	import { untrack, onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import GuestHeader from '$lib/components/shell/GuestHeader.svelte';
	import GuestFooter from '$lib/components/shell/GuestFooter.svelte';
	import MobileBottomTabBar from '$lib/components/shell/MobileBottomTabBar.svelte';
	import { isExceptionRoute } from '$lib/components/create/gate';
	import { sidebarStore } from '$lib/components/shell/sidebar.svelte';
	import { api } from '$lib/api/client';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// ADR-058 Exception #2: /create collapses the nav and hides the right sidebar.
	const exception = $derived(isExceptionRoute(page.route.id));

	// ADR-058 §5 Exception #3: guest landing page renders WITHOUT the app shell.
	// This covers both the Page Builder landing and the default GuestLanding fallback.
	const isGuestLanding = $derived(!!page.data.landing || !!page.data.guestLanding);

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

	// Route-specific right-sidebar snippet (set by child routes via setSidebar).
	const routeSidebar = $derived(sidebarStore.current);

	async function toggleCollapse() {
		const next = !collapsed;
		collapsed = next; // optimistic
		if (!data.user) return; // guests have nowhere to persist; keep it session-local
		const res = await api.patch('/me/account', { ui: { nav_collapsed: next } });
		if (res.error) collapsed = !next; // rollback on failure
	}

	// Phase 23 — Real-Time WebSocket: connect when an authenticated session is
	// present, disconnect on unmount or when the session is lost. The store is
	// a no-op during SSR; onMount guarantees a browser-only connect.
	onMount(() => {
		if (data.user) realtime.connect();
		return () => realtime.disconnect();
	});

	// Reconnect if the user logs in mid-session (e.g. after /login redirect).
	$effect(() => {
		if (data.user) realtime.connect();
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
	     The RightSidebar renders admin-managed widgets for all users. -->
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
		sidebar={routeSidebar ?? undefined}
	>
		{@render children()}
	</AppShell>
	{#if !exception}
		<MobileBottomTabBar user={data.user} />
	{/if}
{/if}

<style></style>
