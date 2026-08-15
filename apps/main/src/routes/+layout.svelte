<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { theme } from '$lib/stores/theme.svelte';
	import { t } from '$lib/i18n';
	import { seedCsrfToken } from '$lib/api/csrf.svelte';
	import type { Snippet } from 'svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Sync the client theme store with the server-resolved value once, after
	// hydration. The initial attribute is already on <html>, so there is no flash.
	onMount(() => theme.init(data.theme));

	// Seed the browser-side CSRF token from server-rendered page data so the
	// first client mutation already carries X-CSRF-Token without a boot fetch.
	// Re-seeds whenever navigation refreshes the layout data (token may rotate).
	// Ref: japa-bugs-hunters.md §5.2 — audit finding C1.
	$effect(() => {
		seedCsrfToken(data.csrfToken ?? null);
	});

	// The i18n locale is derived from `page.data.locale` (request-scoped during
	// SSR, reactive on the client) inside the i18n store itself — no manual
	// sync needed here. When the `jc_locale` cookie changes (e.g. after visiting
	// /fr or saving a language preference + invalidateAll), `page.data.locale`
	// updates and `i18n.locale` reactively follows.

	// Apply the primary brand color from the control panel appearance settings.
	// The value comes from GET /site/settings/public (appearance.primary_color).
	// We override the --color-primary CSS variable on :root so every component
	// that references var(--color-primary) picks up the brand color. This runs
	// both during SSR (inline style on <html>) and reactively after hydration.
	let primaryColor = $derived(
		(data.siteSettings?.['appearance.primary_color'] as string | undefined) ?? ''
	);
	$effect(() => {
		if (primaryColor && /^#[0-9a-fA-F]{3,8}$/.test(primaryColor)) {
			document.documentElement.style.setProperty('--color-primary', primaryColor);
		}
	});

	// Apply the custom favicon from the control panel appearance settings.
	// When appearance.favicon_url is set, we override the default favicon.svg
	// link tag with the uploaded favicon. Ref: conventions/system-settings-catalog.md
	let faviconUrl = $derived(
		(data.siteSettings?.['appearance.favicon_url'] as string | undefined) ?? ''
	);
	$effect(() => {
		if (faviconUrl) {
			let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
			if (!link) {
				link = document.createElement('link');
				link.rel = 'icon';
				document.head.appendChild(link);
			}
			link.href = faviconUrl;
			// Infer type from extension; default to image/png for uploaded favicons.
			const ext = faviconUrl.split('.').pop()?.toLowerCase();
			link.type =
				ext === 'svg' ? 'image/svg+xml' :
				ext === 'ico' ? 'image/x-icon' :
				ext === 'png' ? 'image/png' :
				'image/png';
		}
	});

	// Canonical URLs for hreflang — pathname only, host is implicit.
	// Languages are read from i18n.enabled_languages (system_settings catalog,
	// Public: true) so the hreflang set matches the active language selector
	// rather than a hardcoded en/fr list. Falls back to en,fr when the
	// setting is absent (Rule 3 — no hardcoded config in the steady state).
	// The default language gets the unprefixed URL (e.g. /blog); all others
	// get the prefixed URL (e.g. /fr/blog). Ref: §18 (English unprefixed).
	// Ref: japaconnect-engineering/sections/26-seo-performance.md
	//      conventions/system-settings-catalog.md (i18n.enabled_languages)
	let pathname = $derived(page.url.pathname);
	let enabledLanguages = $derived(
		((data.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? 'en,fr')
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean)
	);
	let defaultLanguage = $derived(
		(data.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en'
	);
	let hreflangLinks = $derived(
		enabledLanguages.map((lang) => ({
			lang,
			href: lang === defaultLanguage
				? (pathname === '/' ? '/' : pathname)
				: `/${lang}${pathname === '/' ? '' : pathname}`
		}))
	);
	let xDefaultHref = $derived(
		pathname === '/' ? '/' : pathname
	);

	// Site-level settings from GET /site/settings/public (general.*).
	// Used for SEO meta tags and the maintenance banner.
	let siteName = $derived(
		(data.siteSettings?.['general.site_name'] as string | undefined) ?? 'JapaConnect'
	);
	let siteDescription = $derived(
		(data.siteSettings?.['general.site_description'] as string | undefined) ?? ''
	);
	let maintenanceMode = $derived(
		(data.siteSettings?.['general.maintenance_mode'] as boolean | undefined) ?? false
	);
	// Admins bypass the maintenance banner.
	let showMaintenanceBanner = $derived(
		maintenanceMode && !(data.user?.groups?.some((g) => g === 'Super Admin' || g === 'Admin') ?? false)
	);
</script>

<svelte:head>
	{#each hreflangLinks as link}<link rel="alternate" hreflang={link.lang} href={link.href} />{/each}
	<link rel="alternate" hreflang="x-default" href={xDefaultHref} />
	<link rel="alternate" type="application/rss+xml" title={t('blog.rss_title', 'JapaConnect Blog')} href="/blog/rss.xml" />
	<link
		rel="alternate"
		type="application/rss+xml"
		title={t('forum.rss_title', 'JapaConnect Forum')}
		href="/forum/rss.xml"
	/>
	{#if siteDescription}
		<meta name="description" content={siteDescription} />
		<meta property="og:description" content={siteDescription} />
		<meta name="twitter:description" content={siteDescription} />
	{/if}
	<meta property="og:site_name" content={siteName} />
	{#if primaryColor && /^#[0-9a-fA-F]{3,8}$/.test(primaryColor)}
		<!-- Apply the control-panel primary brand color during SSR to prevent
		a flash of the default color. Uses html:root for higher specificity than
		the app.css :root rules so the brand color always wins.
		Ref: conventions/system-settings-catalog.md -->
		{@html `<style>html:root, html:root[data-theme='dark'] { --color-primary: ${primaryColor}; }</style>`}
	{/if}
	{#if faviconUrl}
		<!-- Override the default favicon with the control-panel uploaded favicon.
		Ref: conventions/system-settings-catalog.md (appearance.favicon_url) -->
		<link rel="icon" href={faviconUrl} type="image/png" />
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
	class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-[100] focus:rounded-[var(--radius)] focus:bg-[var(--color-primary)] focus:px-3 focus:py-1.5 focus:text-white"
>
	Skip to content
</a>

<div id="main-content">
	{@render children()}
</div>
