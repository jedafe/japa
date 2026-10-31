<script lang="ts">
	import { untrack, onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import { beforeNavigate } from '$app/navigation';
	import { Sparkles } from 'lucide-svelte';
	import { t, isRtl } from '$lib/i18n';
	import { AiPanel, setAiPageContext } from '@japaconnect/ai-panel';
	import { ensureAiPanelDeps } from '$lib/ai/panel';
	import { resolveAiPageContext } from '$lib/ai/pageContext';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import NextActions from '$lib/components/shell/NextActions.svelte';
	import MobileBottomTabBar from '$lib/components/shell/MobileBottomTabBar.svelte';
	import ProductTour from '$lib/components/onboarding/ProductTour.svelte';

	import { sidebarStore } from '$lib/components/shell/sidebar.svelte';
	import { api } from '$lib/api/client';
	import { pwaInstall } from '@japaconnect/ui/pwa-install';
	import PwaInstallBanner from '$lib/pwa/PwaInstallBanner.svelte';
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

	// <html lang/dir> is stamped by hooks on full loads; client-side locale
	// switches never re-render the html element — sync both attributes here.
	$effect(() => {
		document.documentElement.lang = data.locale || 'en';
		document.documentElement.dir = isRtl(data.locale || 'en') ? 'rtl' : 'ltr';
	});

	// Seed the browser-side CSRF token from server-rendered page data so the
	// first client mutation already carries X-CSRF-Token without a boot fetch.
	// Re-seeds whenever navigation refreshes the layout data (token may rotate).
	// Ref: japa-bugs-hunters.md §5.2 — audit finding C1.
	$effect(() => {
		seedCsrfToken(data.csrfToken ?? null);
	});

	// §124 — report auth state to the service worker so its caches only ever
	// store guest content (re-sent on controllerchange inside init()).
	$effect(() => {
		pwaInstall.reportAuth((data as { isAuthed?: boolean }).isAuthed ?? !!data.user);
	});

	// Seed from the server value once; thereafter this is user-controlled.
	let collapsed = $state(untrack(() => data.navCollapsed));

	// §115 AI Quick Panel: FAB toggles the page-aware slide-in panel
	// ("Open in Japa AI" links to the main app's /ai). Deps init on
	// every page.
	const showChatFab = $derived(data.features?.['feature.japabot'] === true);
	let aiPanelOpen = $state(false);

	// §124 — install prompt gated by pwa.install_prompt_enabled; init() only
	// registers the beforeinstallprompt listener, the banner self-hides when
	// the browser can't install or the app is already standalone.
	const installPromptEnabled = $derived(
		data.siteSettings?.['pwa.install_prompt_enabled'] !== false
	);

	$effect(() => {
		setAiPageContext(resolveAiPageContext(page.url.pathname, page.data));
	});
	beforeNavigate(() => setAiPageContext(null));

	onMount(() => ensureAiPanelDeps());

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
		// §124 — capture beforeinstallprompt + register the PWA service
		// worker (precache, runtime caching, offline fallback). Without
		// registration Chromium never marks the app installable and the
		// offline layer is dead code.
		// Ref: .jeos/sections/124-progressive-web-app.md
		if (installPromptEnabled) pwaInstall.init();
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
				// Unregister any stale classic-registered worker before
				// re-registering as a module (see main +layout.svelte).
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
			: undefined) ??
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
	<NextActions />
	{@render children()}
</AppShell>

<MobileBottomTabBar user={data.user} menu={data.bottomMenu} />

{#if tourMounted}
	<!-- §09 Product Tour: steps come from onboarding.tour_steps (admin-managed) -->
	<ProductTour steps={data.tourSteps} />
{/if}

{#if showChatFab}
	<!-- §115 Japa AI FAB — opens the page-aware quick panel. -->
	<button
		type="button"
		onclick={() => (aiPanelOpen = !aiPanelOpen)}
		aria-label={t('ai.panel.title', 'Japa AI')}
		title={t('ai.panel.title', 'Japa AI')}
		aria-expanded={aiPanelOpen}
		class="fixed bottom-20 end-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-transform hover:scale-105 sm:bottom-5"
	>
		<Sparkles size={22} />
	</button>
	<AiPanel open={aiPanelOpen} user={data.user ?? null} onclose={() => (aiPanelOpen = false)} />
{/if}

{#if installPromptEnabled}
	<!-- §124 install prompt — renders only when the browser can install. -->
	<PwaInstallBanner />
{/if}
