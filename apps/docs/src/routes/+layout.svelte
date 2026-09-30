<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { theme } from '$lib/stores/theme.svelte';
	import { t } from '$lib/i18n';
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

	// Apply the primary brand color from the control panel appearance settings.
	let primaryColor = $derived(
		(data.siteSettings?.['appearance.primary_color'] as string | undefined) ?? ''
	);
	$effect(() => {
		if (primaryColor && /^#[0-9a-fA-F]{3,8}$/.test(primaryColor)) {
			document.documentElement.style.setProperty('--color-primary', primaryColor);
		}
	});

	// Umami analytics — Docs surface (§40).
	// Ref: japaconnect-engineering/ADR-040-platform-analytics-dual-layer.md
	const umamiEnabled = $derived(!!env.PUBLIC_JC_UMAMI_URL && !!env.PUBLIC_JC_UMAMI_SITE_ID_DOCS);
</script>

<a
	href="#main-content"
	class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-[100] focus:rounded-[var(--radius)] focus:bg-[var(--color-primary)] focus:px-3 focus:py-1.5 focus:text-white"
>
	{t('a11y.skip_to_content')}
</a>

<svelte:head>
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
	{@render children()}
</div>

{#if tourMounted}
	<!-- Section 09 Product Tour: docs-surface contextual walkthrough -->
	<ProductTour steps={data.tourSteps} />
{/if}
