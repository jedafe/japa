<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { beforeNavigate } from '$app/navigation';
	import { Sparkles } from 'lucide-svelte';
	import { AiPanel, setAiPageContext } from '@japaconnect/ai-panel';
	import { pwaInstall, moduleServiceWorkerSupported } from '@japaconnect/ui/pwa-install';
	import { ToastHost, ConfirmHost, toast } from '@japaconnect/ui';
	import { setGlobalErrorHandler } from '#lib/api/client.js';
	import PwaInstallBanner from '#lib/pwa/PwaInstallBanner.svelte';
	import CookieConsentBanner from '#lib/components/CookieConsentBanner.svelte';
	import { consent, initConsent } from '#lib/consent.svelte.js';
	import { ensureAiPanelDeps } from '#lib/ai/panel.js';
	import { resolveAiPageContext } from '#lib/ai/pageContext.js';
	import NextActions from '#lib/components/shell/NextActions.svelte';
	import { theme } from '#lib/stores/theme.svelte.js';
	import { t } from '#lib/i18n/index.js';
	import { setRuntimePack } from '#lib/i18n/dict.js';
	import { isRtl } from '#lib/i18n/rtl.js';
	import { PUBLIC_JC_UMAMI_URL, PUBLIC_JC_UMAMI_SITE_ID_DOCS } from '$app/env/public';
	import ProductTour from '#lib/components/onboarding/ProductTour.svelte';
	import type { Snippet } from 'svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Section 09 Product Tour (docs surface): ?tour=replay always runs the
	// walkthrough; ?tour=1 runs it while the docs-surface dismissal is unset;
	// and it auto-starts once for authenticated users who have never
	// dismissed it (dismissal is persisted per surface server-side).
	const tourParam = $derived(page.url.searchParams.get('tour'));

	const tourActive = $derived(!!data.user && (tourParam === 'replay' || !data.tourDismissed && (tourParam === '1' || tourParam === null)));

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

	// Sync the shared consent state from jc_consent so consent-gated scripts
	// (Umami below) match the cookie the API gates on.
	onMount(initConsent);

	// Surface unhandled API failures (network loss, 5xx) as toasts — docs
	// forms keep their own inline errors for request-level problems.
	$effect(() => {
		setGlobalErrorHandler((err) => {
			toast.error(err.message || t('errors.generic', 'Something went wrong. Please try again.'));
		});
		return () => setGlobalErrorHandler(null);
	});

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
	let faviconUrl = $derived((theme.resolved === 'dark'
		? data.siteSettings?.['appearance.favicon_url_dark'] as string | undefined
		: undefined) ?? data.siteSettings?.['appearance.favicon_url'] as string | undefined ?? '');

	// Umami analytics — Docs surface (§40).
	// Ref: japaconnect-engineering/ADR-040-platform-analytics-dual-layer.md
	// §19 opt-in: inject only after jc_consent=all. Once injected the script
	// binds its own history listeners, so mid-session revocation applies
	// from the next full page load — the cookie already governs that.
	const umamiEnabled = $derived(
		!!PUBLIC_JC_UMAMI_URL && !!PUBLIC_JC_UMAMI_SITE_ID_DOCS && consent.analytics
	);

	// §115 AI Quick Panel: FAB toggles the page-aware slide-in panel
	// ("Open in Japa AI" links to the main app's /ai). Deps init on
	// every page.
	const showChatFab = $derived(data.features?.['feature.japabot'] === true);
	let aiPanelOpen = $state(false);

	$effect(() => {
		setAiPageContext(resolveAiPageContext(page.url.pathname, page.data));
	});

	beforeNavigate(({ shallow, type }) => {
		if (shallow && type === 'goto') return;

		return setAiPageContext(null);
	});

	onMount(() => ensureAiPanelDeps());

	onMount(() => {
		if (installPromptEnabled) pwaInstall.init();

		if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator && (// Firefox ≤146 / ESR 140 can't run module service workers, and the
		// emitted /service-worker.js IS an ES module in every build — SvelteKit
		// stopped shipping a classic IIFE, so a classic registration fails
		// script evaluation on every load (seen in PageSpeed console errors).
		// Browsers without module-SW support get no PWA worker at all, which
		// matches the previous dev-only gate.
		moduleServiceWorkerSupported())) {
			// Both dev and production service workers are ES modules — register
			// with the matching type so the browser can evaluate the script.
			const swType = 'module';
			const registerSw = (retried = false) => {
				navigator.serviceWorker.register('/service-worker.js', { type: swType }).catch((err) => {
					// A stale classic-registered worker can win Chrome's update
					// race and evaluate the dev ES-module stub as a classic
					// script — one unregister+retry clears it for good.
					if (!retried) {
						navigator.serviceWorker.getRegistration().then((reg) => {
							void (reg ? reg.unregister() : Promise.resolve())
								.finally(() => registerSw(true));
						});
						return;
					}
					// eslint-disable-next-line no-console
					console.error('Service worker registration failed:', err);
				});
			};
			if (import.meta.env.DEV) {
				// During dev, an existing classic-registered worker (from production
				// or an older dev build) would try to re-fetch the script as a
				// classic script and fail because it is now an ES module. Unregister
				// any stale worker first so the new registration can take over.
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
			src="{PUBLIC_JC_UMAMI_URL}/script.js"
			data-website-id={PUBLIC_JC_UMAMI_SITE_ID_DOCS}
			data-do-not-track="true"
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
		onclick={() => aiPanelOpen = !aiPanelOpen}
		aria-label={t('ai.panel.title', 'Japa AI')}
		title={t('ai.panel.title', 'Japa AI')}
		aria-expanded={aiPanelOpen}
		class="fixed bottom-5 end-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-transform hover:scale-105"
	><Sparkles size={22} /></button>

	<AiPanel
		open={aiPanelOpen}
		user={data.user ?? null}
		onclose={() => aiPanelOpen = false}
	/>
{/if}

{#if installPromptEnabled}
	<!-- §124 install prompt — renders only when the browser can install. -->
	<PwaInstallBanner />
{/if}

<!-- §19 cookie consent — self-hides once the jc_consent decision exists. -->
<CookieConsentBanner />

<!-- Global transient surfaces: error toasts + promise-based confirm dialogs. -->
<ToastHost />
<ConfirmHost />
