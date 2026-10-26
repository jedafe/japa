<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { beforeNavigate } from '$app/navigation';
	import { Sparkles } from 'lucide-svelte';
	import { AiPanel, setAiPageContext } from '@japaconnect/ai-panel';
	import { pwaInstall } from '@japaconnect/ui/pwa-install';
	import PwaInstallBanner from '$lib/pwa/PwaInstallBanner.svelte';
	import { ensureAiPanelDeps } from '$lib/ai/panel';
	import { resolveAiPageContext } from '$lib/ai/pageContext';
	import NextActions from '$lib/components/shell/NextActions.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { t } from '$lib/i18n';
	import { setRuntimePack } from '$lib/i18n/dict';
	import { isRtl } from '$lib/i18n/rtl';
	import { env } from '$env/dynamic/public';
	import ProductTour from '$lib/components/onboarding/ProductTour.svelte';
	import type { Snippet } from 'svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Section 09 Product Tour (docs surface): ?tour=replay always runs the
	// walkthrough; ?tour=1 runs it while the docs-surface dismissal is unset;
	// and it auto-starts once for authenticated users who have never
	// dismissed it (dismissal is persisted per surface server-side).
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

	// Sync the client theme store with the server-resolved value once, after
	// hydration. The initial attribute is already on <html>, so there is no flash.
	onMount(() => theme.init(data.theme));

	// The i18n locale is derived from `page.data.locale` in the store — no
	// manual setLocale() call needed (SSR-safe, no cross-request leakage).
	// <html lang> is stamped by hooks on full loads, but client-side locale
	// switches (switchLanguage → goto) never re-render the html element —
	// sync it here so a11y/SEO lang stays correct after in-app switches.
	$effect(() => {
		document.documentElement.lang = data.locale || 'en';
		document.documentElement.dir = isRtl(data.locale || 'en') ? 'rtl' : 'ltr';
	});

	// Non-bundled locales are served by the DB pack fetched in
	// +layout.server.ts — register it so the client-side t() sees the same
	// strings the server rendered with.
	$effect(() => {
		if (data.i18nPack && data.locale) setRuntimePack(data.locale, data.i18nPack);
	});

	// §124 — report auth state to the service worker so its caches only ever
	// store guest content (re-sent on controllerchange inside init()).
	$effect(() => {
		pwaInstall.reportAuth((data as { isAuthed?: boolean }).isAuthed ?? !!data.user);
	});

	// §124 — install prompt gated by pwa.install_prompt_enabled.
	const installPromptEnabled = $derived(
		data.siteSettings?.['pwa.install_prompt_enabled'] !== false
	);

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

	// Umami analytics — Docs surface (§40).
	// Ref: japaconnect-engineering/ADR-040-platform-analytics-dual-layer.md
	const umamiEnabled = $derived(!!env.PUBLIC_JC_UMAMI_URL && !!env.PUBLIC_JC_UMAMI_SITE_ID_DOCS);

	// §115 AI Quick Panel: FAB toggles the page-aware slide-in panel
	// ("Open in Japa AI" links to the main app's /ai). Deps init on
	// every page.
	const showChatFab = $derived(data.features?.['feature.japabot'] === true);
	let aiPanelOpen = $state(false);

	$effect(() => {
		setAiPageContext(resolveAiPageContext(page.url.pathname, page.data));
	});
	beforeNavigate(() => setAiPageContext(null));

	onMount(() => ensureAiPanelDeps());

	// §124 — capture beforeinstallprompt + register the PWA service worker
	// (precache, runtime caching, offline fallback). Without registration
	// Chromium never marks the app installable and the offline layer is
	// dead code.
	// Ref: .jeos/sections/124-progressive-web-app.md
	onMount(() => {
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
	});
</script>

<a
	href="#main-content"
	class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:start-2 focus:z-[100] focus:rounded-[var(--radius)] focus:bg-[var(--color-primary)] focus:px-3 focus:py-1.5 focus:text-white"
>
	{t('a11y.skip_to_content')}
</a>

<svelte:head>
	{#if faviconUrl}
		<link rel="icon" href={faviconUrl} type="image/png" />
	{/if}
	{#if umamiEnabled}
		<!-- Umami page-view analytics (§40, ADR-040). Cookieless, privacy-first. -->
		<script
			defer
			src="{env.PUBLIC_JC_UMAMI_URL}/script.js"
			data-website-id={env.PUBLIC_JC_UMAMI_SITE_ID_DOCS}
		></script>
	{/if}
</svelte:head>

<div id="main-content">
	<NextActions inline />
	{@render children()}
</div>

{#if tourMounted}
	<!-- Section 09 Product Tour: docs-surface contextual walkthrough -->
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
		class="fixed bottom-5 end-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-transform hover:scale-105"
	>
		<Sparkles size={22} />
	</button>
	<AiPanel open={aiPanelOpen} user={data.user ?? null} onclose={() => (aiPanelOpen = false)} />
{/if}

{#if installPromptEnabled}
	<!-- §124 install prompt — renders only when the browser can install. -->
	<PwaInstallBanner />
{/if}
