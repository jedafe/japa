<script lang="ts">
	import { untrack, onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import ProductTour from '$lib/components/onboarding/ProductTour.svelte';
	import MobileBottomTabBar from '$lib/components/shell/MobileBottomTabBar.svelte';
	import { sidebarStore } from '$lib/components/shell/sidebar.svelte';
	import { api } from '$lib/api/client';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { seedCsrfToken } from '$lib/api/csrf.svelte';
	import { env } from '$env/dynamic/public';
	import '../app.css';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Seed the client stores from the SSR-resolved values once, after hydration.
	// This prevents flash-of-wrong-theme and ensures the toggle starts in sync.
	// untrack: theme.init reads reactive store state (current/resolved) — without
	// it those reads become effect dependencies, so every user toggle re-fires
	// the effect and init() snaps the theme back to the stale SSR value.
	$effect(() => {
		const serverTheme = data.theme;
		untrack(() => theme.init(serverTheme));
	});
	// The i18n locale is derived from `page.data.locale` in the store — no
	// manual setLocale() call needed (SSR-safe, no cross-request leakage).

	// Seed the browser-side CSRF token from server-rendered page data so the
	// first client mutation already carries X-CSRF-Token without a boot fetch.
	// Re-seeds whenever navigation refreshes the layout data (token may rotate).
	// Ref: japa-bugs-hunters.md §5.2 — audit finding C1.
	$effect(() => {
		seedCsrfToken(data.csrfToken ?? null);
	});

	// Seed from the server value once; thereafter this is user-controlled.
	let collapsed = $state(untrack(() => data.navCollapsed));

	// Route-specific right-sidebar snippet (set by child routes via setSidebar).
	const routeSidebar = $derived(sidebarStore.current);

	// Section 09 Product Tour (academy surface): `?tour=1` starts the tour once
	// for signed-in users until dismissed; `?tour=replay` always replays it
	// (account dropdown replay link). Authed users who have not dismissed it
	// get one auto-run.
	const tourParam = $derived(page.url.searchParams.get('tour'));
	const tourActive = $derived(
		!!data.user &&
			(tourParam === 'replay' ||
				(!data.tourDismissed && (tourParam === '1' || tourParam === null)))
	);

	// Latch: ProductTour strips the ?tour param once the tour starts, which
	// flips tourActive back to false for replay+dismissed users — keep the
	// component mounted so its navigation guard can tear the driver down
	// cleanly instead of leaving an orphaned overlay.
	let tourMounted = $state(false);
	$effect(() => {
		if (tourActive) tourMounted = true;
	});

	// Lesson player route: the reading exception (ADR-060). Auto-collapses the
	// nav to 72px and is detected by route id so the user's persisted preference
	// is restored when they leave the player.
	const isLessonPlayer = $derived(
		typeof page.route.id === 'string' &&
			page.route.id.includes('/tracks/[slug]/[module]/[lesson]')
	);

	async function toggleCollapse() {
		const next = !collapsed;
		collapsed = next; // optimistic
		if (!data.user) return; // guests have nowhere to persist; keep it session-local
		const res = await api.patch('/me/account', { ui: { nav_collapsed: next } });
		if (res.error) collapsed = !next; // rollback on failure
	}

	// Real-Time WebSocket: connect when an authenticated session is present,
	// disconnect on unmount or when the session is lost. The store is a no-op
	// during SSR; onMount guarantees a browser-only connect.
	onMount(() => {
		if (data.user) realtime.connect();
		return () => realtime.disconnect();
	});

	// Reconnect if the user logs in mid-session (e.g. after /login redirect),
	// and drop the socket on logout so a stale connection can't keep
	// delivering notifications for the previous session.
	$effect(() => {
		if (data.user) realtime.connect();
		else realtime.disconnect();
	});

	// Seed the realtime unread-notification counter from the SSR value, and
	// re-sync whenever the server payload changes (e.g. after invalidateAll).
	$effect(() => {
		realtime.setUnread(data.unreadNotifications);
	});

	// Admin appearance overrides are injected server-side by hooks.server.ts
	// into the static <style id="jc-theme-overrides"> tag in app.html.

	// Custom favicon from appearance.favicon_url — overrides the default
	// favicon links in app.html when set.
	// Ref: conventions/system-settings-catalog.md (appearance.favicon_url)
	let faviconUrl = $derived(
		((theme.resolved === 'dark'
			? (data.siteSettings?.['appearance.favicon_url_dark'] as string | undefined)
			: (data.siteSettings?.['appearance.favicon_url_light'] as string | undefined)) ??
			(data.siteSettings?.['appearance.favicon_url'] as string | undefined) ?? '')
	);

	// Umami analytics — Academy surface (§40, ROADMAP-ACADEMY 01.08).
	// Ref: japaconnect-engineering/ADR-040-platform-analytics-dual-layer.md
	const umamiEnabled = $derived(!!env.PUBLIC_JC_UMAMI_URL && !!env.PUBLIC_JC_UMAMI_SITE_ID_ACADEMY);
</script>

<svelte:head>
	{#if faviconUrl}
		<link rel="icon" href={faviconUrl} type="image/png" />
	{/if}
	{#if umamiEnabled}
		<!-- Umami page-view analytics (§40, ADR-040). Cookieless, privacy-first. -->
		<script
			defer
			src="{env.PUBLIC_JC_UMAMI_URL}/script.js"
			data-website-id={env.PUBLIC_JC_UMAMI_SITE_ID_ACADEMY}
		></script>
	{/if}
</svelte:head>

<AppShell
	menu={data.menu}
	footerMenu={data.footerMenu}
	user={data.user}
	{collapsed}
	onToggleCollapse={toggleCollapse}
	logoUrl={data.logoUrl}
	logoDarkUrl={data.logoDarkUrl}
	sidebar={routeSidebar ?? undefined}
	rightSidebarWidgets={data.rightSidebarWidgets}
	leftSidebarWidgets={isLessonPlayer ? [] : data.leftSidebarWidgets}
	lessonPlayer={isLessonPlayer}
>
	{@render children()}
</AppShell>

<MobileBottomTabBar user={data.user} menu={data.bottomMenu} />

{#if tourMounted}
	<!-- §09 Product Tour: steps come from onboarding.tour_steps (admin-managed) -->
	<ProductTour steps={data.tourSteps} />
{/if}
