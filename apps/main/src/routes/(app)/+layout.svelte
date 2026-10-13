<script lang="ts">
	import { untrack, onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import GuestHeader from '$lib/components/shell/GuestHeader.svelte';
	import GuestFooter from '$lib/components/shell/GuestFooter.svelte';
	import MobileBottomTabBar from '$lib/components/shell/MobileBottomTabBar.svelte';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import { isExceptionRoute, isNoTabBarRoute } from '$lib/components/create/gate';
	import { api } from '$lib/api/client';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import ProductTour from '$lib/components/onboarding/ProductTour.svelte';
	import MediaLightbox from '$lib/components/ui/MediaLightbox.svelte';
	import StickyAnchorAd from '$lib/components/ads/StickyAnchorAd.svelte';
	import AdConversionTracker from '$lib/components/ads/AdConversionTracker.svelte';
	import { lightbox } from '$lib/lightbox.svelte';
	import { Sparkles } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// ADR-058 Exception #2: the /studio surface collapses the nav and hides
	// the right sidebar — every tab renders in composer chrome.
	const exception = $derived(isExceptionRoute(page.route.id));
	// Routes that render their own fixed mobile action bar instead of the
	// shell's tab bar (see gate.isNoTabBarRoute).
	const noTabBar = $derived(isNoTabBarRoute(page.route.id));

	// Section 09 Product Tour: ?tour=1 triggers the contextual walkthrough
	// after onboarding completion. The tour starts once the app shell renders.
	// Guard: if the user previously dismissed the tour (tour_dismissed=true),
	// ?tour=1 is suppressed so it never re-triggers automatically. The
	// explicit "Replay tour" link uses ?tour=replay to override the guard.
	const tourParam = $derived(page.url.searchParams.get('tour'));
	const tourRequested = $derived(
		tourParam === 'replay' || (tourParam === '1' && !data.tourDismissed)
	);

	// Latch: ProductTour strips the ?tour param once the tour starts, which
	// flips tourRequested back to false for replay+dismissed users — keep the
	// component mounted so its navigation guard can tear the driver down
	// cleanly instead of leaving an orphaned overlay.
	let tourMounted = $state(false);
	$effect(() => {
		if (tourRequested && data.user) tourMounted = true;
	});

	// ADR-058 §5 Exception #3: guest landing page renders WITHOUT the app shell.
	// The new default guest homepage (guestHome) also uses GuestHeader and a custom footer.
	const isGuestLanding = $derived(!!page.data.landing || !!page.data.guestHome);

	// ADR-127: "Back to JapaConnect" targets the apex domain — derived from the
	// request host by stripping the leading `roadmap.` label (port preserved for
	// dev). JC_ROADMAP_HOSTS custom domains without that prefix keep their host.
	const roadmapApexUrl = $derived(
		`${page.url.protocol}//${page.url.host.replace(/^roadmap\./i, '')}`
	);

	// Settings hub: auto-collapse the left nav to give settings panels more
	// room, but keep the right sidebar and let the user expand via the toggle.
	const isSettings = $derived(page.route.id?.startsWith('/(app)/settings') ?? false);

	// Dashboard hub: same treatment — the dashboard has its own HubRail
	// (secondary left nav for the dashboard views), so collapsing the main
	// LeftNav gives the dashboard content more room and avoids two stacked
	// left rails. The user can still expand via the toggle.
	const isDashboard = $derived(page.route.id?.startsWith('/(app)/dashboard') ?? false);

	// Roadmap board index: the §113 kanban needs horizontal room, so only
	// /roadmap is an auto-collapse zone (nav → 72px icons, left rail
	// hidden). Subpages (item detail, changelog, submit) keep the shell.
	const isRoadmap = $derived(page.route.id === '/(app)/roadmap');

	// ADR-058 Exception #1: content single pages (blog, content, guide,
	// story, comparisons) still render the unified shell's left ad rail. The
	// /studio surface is the exception exception: it owns its own layout
	// and hides the left rail. /create redirects to /studio/create.
	const isCreateRoute = $derived(
		(page.route.id?.startsWith('/(app)/studio') ?? false) || page.route.id === '/(app)/create'
	);

	// Seed from the server value once; thereafter this is user-controlled.
	let collapsed = $state(untrack(() => data.navCollapsed));

	// Auto-collapse when entering settings, dashboard or roadmap (once per
	// transition). The user can still expand via the toggle — we only
	// collapse on entry, not force.
	let prevWasCollapsedZone = $state(false);
	$effect(() => {
		const inCollapsedZone = isSettings || isDashboard || isRoadmap;
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
	// present. The /ws + /auth/ws-ticket transport is ungated server-side
	// (Section 35 — notifications, presence and feed updates share the socket
	// with DM events), so DM being off must not kill realtime notifications.
	// The store is a no-op during SSR; onMount guarantees a browser-only connect.
	// Ref: sections/10-realtime-websockets.md, sections/35-notification-system.md

	// §23 — JapaBot floating action button: links to the JapaBot chat tab on
	// /ai from every app-shell page, gated by the same feature.japabot flag.
	// Hidden on all /ai pages (self-link). Guests can click — the route
	// redirects to login.
	const showChatFab = $derived(
		data.features?.['feature.japabot'] === true &&
			!page.route.id?.startsWith('/(app)/ai')
	);
	onMount(() => {
		if (data.user) realtime.connect();
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
		realtime.setUnreadCount(data.unreadNotifications);
	});
</script>

{#if data.isRoadmapHost}
	<!-- ADR-127: roadmap.japaconnect.com renders a compact roadmap-branded
	     shell — no app sidebars, left nav, widgets, chat FAB, or product tour.
	     Voting/submission auth uses the shared session cookie on /login. -->
	<header class="roadmap-header">
		<div class="roadmap-header-inner">
			<a href="/roadmap" class="roadmap-brand" aria-label={t('roadmap.title', 'Product Roadmap')}>
				<ThemeLogo
					logoUrl={data.siteSettings?.['appearance.logo_url'] as string | undefined}
					logoDarkUrl={data.siteSettings?.['appearance.logo_url_dark'] as string | undefined}
					alt={t('app.name')}
					class="h-7 w-auto max-w-[120px] object-contain"
				/>
				<span class="roadmap-brand-title">{t('roadmap.title', 'Product Roadmap')}</span>
			</a>
			<nav class="roadmap-nav" aria-label={t('roadmap.nav', 'Roadmap navigation')}>
				<a href="/roadmap/changelog" class="roadmap-nav-link">{t('roadmap.changelog', 'Changelog')}</a>
				<a href={roadmapApexUrl} class="roadmap-nav-link roadmap-back-link">
					{t('roadmap.back_to_site', 'Back to JapaConnect')}
				</a>
			</nav>
		</div>
	</header>
	<main class="roadmap-main">
		{@render children()}
	</main>
	<footer class="roadmap-footer">
		<span>
			&copy; {new Date().getFullYear()} {t('app.name')}. {t('landing.footer_rights', 'All rights reserved.')}
		</span>
	</footer>
{:else if isGuestLanding}
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
		leftSidebarWidgets={isCreateRoute || isSettings || isDashboard || isRoadmap ? [] : data.leftSidebarWidgets}

		composeEnabled={!exception && !!data.user}
		features={data.features}
	>
		{@render children()}
	</AppShell>
	{#if !exception}
		<!-- §25: dismissible sticky anchor ad — suppressed on exception
		     surfaces (create/studio/settings/dashboard) like the sidebars. -->
		<StickyAnchorAd />
	{/if}
	{#if !exception && !noTabBar}
		<MobileBottomTabBar user={data.user} features={data.features} menu={data.mobileMenu} />
	{/if}
	{#if showChatFab}
		<!-- §23 JapaBot FAB — sits above the mobile tab bar (bottom-20),
		     flush to the right edge on desktop (bottom-5). -->
		<a
			href="/ai?mode=chat"
			aria-label={t('ai.chat.title') ?? 'JapaBot Chat'}
			title={t('ai.chat.title') ?? 'JapaBot Chat'}
			class="fixed bottom-20 right-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-transform hover:scale-105 sm:bottom-5"
		>
			<Sparkles size={22} />
		</a>
	{/if}
	{#if tourMounted}
		<!-- §09 Product Tour: steps come from onboarding.tour_steps (admin-managed) -->
		<ProductTour replay={true} steps={data.tourSteps} />
	{/if}
{/if}

<!-- Shared fullscreen media viewer — any surface opens it via openLightbox() -->
<MediaLightbox bind:open={lightbox.open} bind:index={lightbox.index} items={lightbox.items} />

<!-- §25: fires the conversion ad event when a navigation lands with the
     same-origin attribution param (jc_ad_cid). Covers guests and authed
     users — ad serving is optional-auth. -->
<AdConversionTracker />

<style>
	/* ADR-127 minimal roadmap shell */
	.roadmap-header {
		position: sticky;
		top: 0;
		z-index: 50;
		border-bottom: 1px solid var(--color-border);
		background: var(--color-surface);
	}
	.roadmap-header-inner {
		display: flex;
		align-items: center;
		gap: 1rem;
		height: var(--nav-height);
		padding: 0 1rem;
		max-width: 72rem;
		margin: 0 auto;
	}
	.roadmap-brand {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		font-weight: 600;
		color: var(--color-text);
		white-space: nowrap;
	}
	.roadmap-brand-title {
		font-size: 0.9375rem;
		font-weight: 600;
	}
	.roadmap-nav {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		margin-left: auto;
	}
	.roadmap-nav-link {
		padding: 0.375rem 0.75rem;
		border-radius: var(--radius);
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text-subtle);
		white-space: nowrap;
		transition: color 0.15s, background 0.15s;
	}
	.roadmap-nav-link:hover {
		color: var(--color-text);
		background: var(--color-surface-subtle);
	}
	.roadmap-main {
		max-width: 72rem;
		margin: 0 auto;
		padding: 2rem 1.5rem;
		min-height: 60vh;
	}
	.roadmap-footer {
		border-top: 1px solid var(--color-border);
		padding: 1.5rem;
		text-align: center;
		font-size: 0.875rem;
		color: var(--color-muted);
		background: var(--color-surface);
	}
</style>
