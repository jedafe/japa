<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { theme } from '#lib/stores/theme.svelte.js';
	import { t, DEFAULT_SITE_NAME, isRtl } from '#lib/i18n/index.js';
	import { seedCsrfToken } from '#lib/api/csrf.svelte.js';
	import { seedAccessToken, clearAccessToken } from '#lib/api/token.svelte.js';
	import { realtime } from '#lib/realtime/realtime.svelte.js';
	import { pwaInstall, moduleServiceWorkerSupported } from '@japaconnect/ui/pwa-install';
	import { ToastHost, ConfirmHost, toast } from '@japaconnect/ui';
	import { setGlobalErrorHandler } from '#lib/api/client.js';
	import PwaInstallBanner from '#lib/pwa/PwaInstallBanner.svelte';
	import CookieConsentBanner from '#lib/components/CookieConsentBanner.svelte';
	import { consent, initConsent } from '#lib/consent.js';
	import { syncPushSubscription } from '#lib/pwa/push-sync.js';

	import {
		PUBLIC_JC_UMAMI_URL,
		PUBLIC_JC_UMAMI_SITE_ID,
		PUBLIC_JC_API_URL
	} from '$app/env/public';

	import { resolveString, GLOBAL_TOKENS } from '@japaconnect/landing-blocks';
	import type { Snippet } from 'svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Sync the client theme store with the server-resolved value once, after
	// hydration. The initial attribute is already on <html>, so there is no flash.
	onMount(() => theme.init(data.theme));

	// Sync the shared consent state from jc_consent so consent-gated scripts
	// (Umami below) match the cookie the API gates on.
	onMount(initConsent);

	// Surface unhandled API failures (network loss, 5xx, mutating 403s) as
	// toasts — callers keep their own inline errors for page-level problems.
	$effect(() => {
		setGlobalErrorHandler((err) => {
			toast.error(err.message || t('errors.generic', 'Something went wrong. Please try again.'));
		});
		return () => setGlobalErrorHandler(null);
	});

	// Seed the browser-side CSRF token from server-rendered page data so the
	// first client mutation already carries X-CSRF-Token without a boot fetch.
	// Re-seeds whenever navigation refreshes the layout data (token may rotate).
	// Ref: japa-bugs-hunters.md §5.2 — audit finding C1.
	$effect(() => {
		seedCsrfToken(data.csrfToken ?? null);
	});

	// Seed the browser-side Bearer store for authed api calls; clear when the
	// session is gone so stale tokens can't linger after logout.
	$effect(() => {
		if (data.accessToken) seedAccessToken(data.accessToken);
		else clearAccessToken();
	});

	// The i18n locale is derived from `page.data.locale` (request-scoped during
	// SSR, reactive on the client) inside the i18n store itself — no manual
	// sync needed here. When the `jc_locale` cookie changes (e.g. after visiting
	// /fr or saving a language preference + invalidateAll), `page.data.locale`
	// updates and `i18n.locale` reactively follows.
	//
	// <html lang/dir> is stamped by hooks on full loads; client-side locale
	// switches never re-render the html element — sync both attributes here.
	$effect(() => {
		document.documentElement.lang = data.locale || 'en';
		document.documentElement.dir = isRtl(data.locale || 'en') ? 'rtl' : 'ltr';
	});

	// Admin appearance theming (appearance.* → CSS vars) is injected server-side
	// into the static <style id="jc-theme-overrides"> tag in app.html by
	// hooks.server.ts transformPageChunk — nothing to do here.
	// Ref: conventions/system-settings-catalog.md (Appearance / Branding)

	// Apply the custom favicon from the control panel appearance settings.
	// When appearance.favicon_url is set, we override the default favicon.svg
	// link tag with the uploaded favicon. Ref: conventions/system-settings-catalog.md
	let faviconUrl = $derived((theme.resolved === 'dark'
		? data.siteSettings?.['appearance.favicon_url_dark'] as string | undefined
		: undefined) ?? data.siteSettings?.['appearance.favicon_url'] as string | undefined ?? '');

	$effect(() => {
		if (!faviconUrl) return;
		// Update every icon link — the app.html fallbacks carry
		// prefers-color-scheme media attributes, so mutating only the
		// first link would leave the bundled icon winning in dark mode.
		const links = document.querySelectorAll<HTMLLinkElement>('link[rel="icon"]');
		const ext = faviconUrl.split('.').pop()?.toLowerCase();
		const mime = ext === 'svg' ? 'image/svg+xml' : ext === 'ico' ? 'image/x-icon' : 'image/png';
		if (links.length === 0) {
			const link = document.createElement('link');
			link.rel = 'icon';
			link.href = faviconUrl;
			link.type = mime;
			document.head.appendChild(link);
			return;
		}
		links.forEach((link) => { link.href = faviconUrl; link.type = mime; });
	});

	// hreflang alternates are injected server-side by hooks.server.ts with
	// absolute URLs (PageSpeed/Google require absolute hreflang hrefs).
	// enabledLanguages is read from i18n.enabled_languages (system_settings
	// catalog, Public: true) so the og:locale:alternate set matches the active
	// language selector rather than a hardcoded en/fr list. Falls back to
	// en,fr when the setting is absent (Rule 3 — no hardcoded config in the
	// steady state). Ref: japaconnect-engineering/sections/26-seo-performance.md
	//      conventions/system-settings-catalog.md (i18n.enabled_languages)
	let enabledLanguages = $derived((data.siteSettings?.['i18n.enabled_languages'] as string | undefined ?? 'en,fr').split(',').map((l) => l.trim()).filter(Boolean));

	let defaultLanguage = $derived(data.siteSettings?.['i18n.default_language'] as string | undefined ?? 'en');

	// og:locale mirrors the active locale; alternates cover the other enabled
	// languages (same source as the server-side hreflang set so they never drift).
	let ogLocale = $derived(data.locale as string | undefined ?? defaultLanguage);

	// Site-level settings from GET /site/settings/public (general.*).
	// Used for SEO meta tags and the maintenance banner.
	let siteName = $derived(resolveString(data.siteSettings?.['general.site_name'] as string | undefined ?? DEFAULT_SITE_NAME, GLOBAL_TOKENS));

	// The admin-managed description may carry {{country}}/{{country_adjective}}
	// geo tokens (Page Builder copy syntax). The layout has no per-visitor geo
	// context, so resolve against GLOBAL_TOKENS — never emit a raw {{token}}.
	let siteDescription = $derived(resolveString(data.siteSettings?.['general.site_description'] as string | undefined ?? '', GLOBAL_TOKENS));

	let maintenanceMode = $derived(data.siteSettings?.['general.maintenance_mode'] as boolean | undefined ?? false);

	// Mirror the SSR dedupeFallbackMeta on client navigations: the layout's
	// fallback metas persist across nav, so when the incoming page supplies
	// its own description the marked fallbacks are removed, and restored when
	// the page has none (Svelte won't recreate them — this effect owns their
	// post-mount lifecycle).
	$effect(() => {
		void page.url.pathname;
		const marked = [...document.head.querySelectorAll('meta[data-jc-fallback]')];
		const pageOwns = document.head.querySelector(
			'meta[name="description"]:not([data-jc-fallback]), meta[property="og:description"]:not([data-jc-fallback])'
		);
		if (pageOwns) {
			for (const m of marked) m.remove();
		} else if (siteDescription && marked.length === 0) {
			for (const [attr, val] of [
				['name', 'description'],
				['property', 'og:description'],
				['name', 'twitter:description']
			] as const) {
				const m = document.createElement('meta');
				m.setAttribute(attr, val);
				m.setAttribute('content', siteDescription);
				m.setAttribute('data-jc-fallback', '');
				document.head.appendChild(m);
			}
		}
	});
	// Admins bypass the maintenance banner.
	let showMaintenanceBanner = $derived(
		maintenanceMode && !(data.user?.groups?.some((g: string) => g === 'Super Admin' || g === 'Admin') ?? false)
	);

	// Custom code injection (§57). Loaded from GET /site/settings/public
	// and rendered server-side in the root layout so it applies to every route.
	//
	// Staff-only settings with a server-side audit trail (admin settings
	// writes are audit-logged) — but the payload still reaches every visitor,
	// so three gates apply:
	//  1. CSP nonce — both blocks carry nonce={cspNonce} and the response
	//     carries a nonce-based style-src/script-src policy (hooks.server.ts).
	//  2. Custom CSS passes through the sanitizeCustomCss blocklist —
	//     url(/expression(/@import/javascript:) payloads are stripped.
	//  3. Custom JS renders ONLY when the super_admin-approved
	//     `site.custom_js_approved` flag is set (fail-closed default);
	//     the approval itself is audit-logged server-side.
	// site.custom_css is injected server-side by hooks.server.ts into the
	// static <style id="jc-custom-css"> tag in app.html (sanitized there) —
	// {@html} inside a <style> element does not compile, so it cannot live here.
	let customJsRaw = $derived(data.siteSettings?.['site.custom_js'] as string | undefined ?? '');

	let customJsApproved = $derived(data.siteSettings?.['site.custom_js_approved'] as boolean | undefined ?? false);
	let customJs = $derived(customJsApproved ? customJsRaw : '');
	let cspNonce = $derived(data.cspNonce ?? '');

	// Umami analytics — Main surface (§40, ROADMAP-MAIN 01.10).
	// Ref: japaconnect-engineering/ADR-040-platform-analytics-dual-layer.md
	// §19 opt-in: inject only after jc_consent=all. Once injected the script
	// binds its own history listeners, so mid-session revocation applies
	// from the next full page load — the cookie already governs that.
	const umamiEnabled = $derived(
		!!PUBLIC_JC_UMAMI_URL && !!PUBLIC_JC_UMAMI_SITE_ID && consent.analytics
	);

	// Cross-origin API host (logos, avatars, media). Preconnecting saves the
	// DNS+TLS handshake on the first media request — a real LCP factor on
	// media-heavy pages (§26 performance).
	const apiOrigin = $derived.by(() => {
		try {
			const u = new URL(PUBLIC_JC_API_URL ?? '');
			return u.origin;
		} catch {
			return '';
		}
	});
	const preconnectApi = $derived(apiOrigin !== '' && apiOrigin !== page.url.origin);

	// §124 — PWA install prompt, gated by pwa.install_prompt_enabled.
	// init() only registers the beforeinstallprompt listener (idempotent);
	// the banner self-hides when the browser can't install or the app is
	// already running standalone.
	const installPromptEnabled = $derived(
		data.siteSettings?.['pwa.install_prompt_enabled'] !== false
	);

	onMount(() => {
		if (installPromptEnabled) pwaInstall.init();
		// Register the service worker at the root so EVERY surface — auth,
		// onboarding, marketplace, public pages — gets installability,
		// offline caching, and push delivery, not just the (app) group.
		// Ref: .jeos/sections/124-progressive-web-app.md
		if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator && (// Firefox ≤146 / ESR 140 can't run module service workers — it
		// falls back to classic evaluation of the ES-module dev stub and
		// throws "script threw an exception during script evaluation" on
		// every load. Production serves a classic IIFE, so only dev gates.
		!import.meta.env.DEV || moduleServiceWorkerSupported())) {
			// SvelteKit/Vite serves the source service worker as an ES module in dev,
			// but compiles it to a classic IIFE for production. Use the matching
			// registration type so the browser does not fail to evaluate the script.
			const swType = import.meta.env.DEV ? 'module' : 'classic';
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
			// §35 — the worker broadcasts jc:push-subscription-changed when the
			// OS rotates/revokes the push subscription; reconcile the new
			// endpoint with the backend (only a page has the access token).
			navigator.serviceWorker.addEventListener('message', (e) => {
				if ((e.data as { type?: string } | undefined)?.type === 'jc:push-subscription-changed') {
					void syncPushSubscription();
				}
			});
		}
	});

	// §124 — report auth state to the service worker so its caches only ever
	// store guest content. reportAuth also re-sends on controllerchange, so
	// this is safe even before the worker finishes claiming the page. Reads
	// isAuthed (never overridden) rather than data.user — page-level loads
	// can shadow the layout's `user` key (e.g. a viewed profile's record).
	$effect(() => {
		pwaInstall.reportAuth(data.isAuthed ?? !!data.user);
	});

	// §35/§124 — reconcile the browser's push subscription with the backend
	// token list once per authenticated user (keyed on viewerId, which is
	// never shadowed by page-level `user` data — profile pages return the
	// viewed profile under `user`). Covers rotations that ran while no
	// window was open; mid-session rotations re-sync via the worker's
	// jc:push-subscription-changed broadcast.
	let pushSyncedFor = $state<string | null>(null);
	$effect(() => {
		const uid = data.viewerId ?? null;
		if (uid && uid !== pushSyncedFor) {
			pushSyncedFor = uid;
			void syncPushSubscription();
		}
	});

	// §124 — mirror the LIVE unread count onto the installed app icon badge
	// (Chromium only; no-op elsewhere). Gated by pwa.badge_enabled. Reads the
	// realtime store so WebSocket-delivered notifications update the badge
	// without waiting for a navigation/invalidation.
	$effect(() => {
		const enabled = data.siteSettings?.['pwa.badge_enabled'] !== false;
		const nav = navigator as Navigator & {
			setAppBadge?: (n: number) => Promise<void>;
			clearAppBadge?: () => Promise<void>;
		};
		if (!enabled || !pwaInstall.standalone || !nav.setAppBadge) return;
		const count = realtime.unreadNotifications;
		if (count > 0) void nav.setAppBadge(count).catch(() => {});
		else void nav.clearAppBadge?.().catch(() => {});
	});
</script>

<svelte:head>
	{#if preconnectApi}<link rel="preconnect" href={apiOrigin} crossorigin="anonymous" />{/if}
	<meta property="og:locale" content={ogLocale} />
	{#each enabledLanguages.filter((l) => l !== ogLocale) as alt}
		<meta property="og:locale:alternate" content={alt} />
	{/each}
	<link
		rel="alternate"
		type="application/rss+xml"
		title={t('feed.rss_title', '{site} Feed')}
		href="/feed.xml"
	/>

	<link
		rel="alternate"
		type="application/rss+xml"
		title={t('blog.rss_title', '{site} Blog')}
		href="/blog/feed.xml"
	/>

	<link
		rel="alternate"
		type="application/rss+xml"
		title={t('forum.rss_title', '{site} Forum')}
		href="/thread/feed.xml"
	/>
	<!-- Site-level fallback description metas are injected by hooks.server.ts
	     (marked data-jc-fallback) when the page supplies no description —
	     they are NOT rendered here: post-render removal of Svelte-claimed
	     head nodes breaks hydration. The effect below owns their client-nav
	     lifecycle (remove when a page owns its description, restore when not). -->
	<meta property="og:site_name" content={siteName} />
	{#if faviconUrl}
		<!-- Override the default favicon with the control-panel uploaded favicon.
		Ref: conventions/system-settings-catalog.md (appearance.favicon_url) -->
		<link rel="icon" href={faviconUrl} type="image/png" />
	{/if}
	{#if umamiEnabled}
		<!-- Umami page-view analytics (§40, ADR-040). Cookieless, privacy-first. -->
		<script
			defer
			src="{PUBLIC_JC_UMAMI_URL}/script.js"
			data-website-id={PUBLIC_JC_UMAMI_SITE_ID}
			data-do-not-track="true"
		></script>
	{/if}
</svelte:head>

{#if showMaintenanceBanner}
	<div
		class="fixed inset-x-0 top-0 z-[200] bg-amber-600 px-4 py-2 text-center text-sm font-medium text-white shadow-lg"
		role="alert"
	>
		{t('maintenance.banner', 'This site is undergoing maintenance. Some features may be unavailable.')}
	</div>
{/if}

<a
	href="#main-content"
	class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:start-2 focus:z-[100] focus:inline-flex focus:min-h-11 focus:min-w-11 focus:items-center focus:justify-center focus:rounded-[var(--radius)] focus:bg-[var(--color-primary)] focus:px-4 focus:py-3 focus:text-white"
>
	{t('a11y.skip_to_content', 'Skip to content')}
</a>

<div id="main-content">
	{@render children()}
</div>

{#if installPromptEnabled}
	<!-- §124 install prompt — renders only when the browser can install
	     (beforeinstallprompt fired or manual iOS flow) and not standalone. -->
	<PwaInstallBanner />
{/if}

<!-- §19 cookie consent — self-hides once the jc_consent decision cookie
     exists. The cookie gates non-essential identifiers (jc_guest) on the
     API side; the POST is the audit trail. -->
<CookieConsentBanner />

{#if customJs}
	<!-- Admin custom JavaScript (§57). Renders only when the super_admin-approved
	site.custom_js_approved flag is set (audit-logged server-side); gated behind
	the CSP nonce. Without approval the setting is inert. -->
	{@html `<script id="jc-custom-js" nonce="${cspNonce}">` + customJs + `</script>`}
{/if}

<!-- Global transient surfaces: API/network error toasts + promise-based
     confirm dialogs (replaces window.confirm). -->
<ToastHost />
<ConfirmHost />
