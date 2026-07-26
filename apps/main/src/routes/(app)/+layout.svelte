<script lang="ts">
	import { untrack, onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
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

	// Seed from the server value once; thereafter this is user-controlled.
	let collapsed = $state(untrack(() => data.navCollapsed));

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
	<!-- ADR-058 §5 Exception #3: guest landing page — no app shell -->
	{@render children()}
{:else}
	<AppShell
		menu={data.menu}
		user={data.user}
		collapsed={exception || collapsed}
		onToggleCollapse={toggleCollapse}
		trendingTags={data.trendingTags}
		suggestedSpaces={data.suggestedSpaces}
		userSpaces={data.userSpaces}
		hideRightSidebar={exception}
		sidebar={routeSidebar ?? undefined}
	>
		{@render children()}
	</AppShell>
	{#if !exception}
		<MobileBottomTabBar user={data.user} />
	{/if}
{/if}
