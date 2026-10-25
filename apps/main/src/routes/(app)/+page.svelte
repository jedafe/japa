<script lang="ts">
	import { page } from '$app/state';
	import { browser } from '$app/environment';
	import type { UrlTab } from '$lib/components/ui';
	import { LandingRenderer, type Block } from '@japaconnect/landing-blocks';
	import { t } from '$lib/i18n';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { apiBaseUrl } from '$lib/api/client';
	import { getCsrfToken } from '$lib/api/csrf.svelte';

	let { data }: { data: PageData } = $props();

	// Tab bar values — labels match the old FeedTabs component. The active tab
	// comes from the server load (data.tab); switching calls goto() so SvelteKit
	// re-runs the load function for the new tab's per-tab feed data.
	const tabs = $derived<UrlTab[]>(
		data.authed
			? [
					{ value: 'for_you', label: t('feed.tab.for_you') },
					{ value: 'following', label: t('feed.tab.following') }
				]
			: [{ value: 'explore', label: t('feed.tab.explore', 'Explore') }]
	);

	// cardStyleConfig comes from the (app) layout loader (shared shell data).
	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });

	// SEO: title/description differ for the landing page vs the authed feed.
	const isLanding = $derived(!!data.landing);
	const title = $derived(
		isLanding
			? (data.landing?.title ?? t('app.name'))
			: t('home.title', '{site} — Explore')
	);
	const description = $derived(
		isLanding
			? (data.landing?.description ?? t('landing.description', 'Stories, discussions, and guides for Nigerians abroad.'))
			: t('home.description', 'Explore the Nigerian diaspora community: stories, discussions, comparisons, and guides.')
	);
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'WebSite',
			name: t('app.name'),
			url: page.url.origin
		})
	);

	// Fetch context for Page Builder blocks (newsletter, widgets, etc.).
	// Only pass in the browser — blocks like MapBlock call fetch during
	// component init, and SvelteKit forbids relative fetch() during SSR.
	const fetchCtx = $derived(
		browser
			? {
					fetch,
					apiBase: apiBaseUrl,
					csrfToken: data.csrfToken ?? getCsrfToken() ?? null,
					// X-JC-Surface for scoped feature-flag resolution on
					// block-level backend fetches.
					surface: 'main'
			  }
			: undefined
	);

	// Fallback guest homepage is a single native Page Builder block.
	const fallbackBlocks = $derived<Block[]>([
		{
			id: 'guest-home-fallback',
			type: 'guest_home',
			visible: true,
			label: 'Guest Home',
			props: {}
		}
	]);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

{#if data.guestHome}
	<!-- ADR-058 §5 Exception #3: new default guest homepage -->
	<LandingRenderer blocks={fallbackBlocks} {fetchCtx} geoCountry={data.contentCountry} guestHome={data.home} />
{:else if data.landing}
	<!-- ADR-058 §5 Exception #3: guest Page Builder landing page (or default seed) -->
	<LandingRenderer blocks={data.landing.blocks} {fetchCtx} geoCountry={data.contentCountry} guestHome={data.home} />
{:else}
	<!-- Authenticated home timeline — dynamic import keeps the feed stack out
	     of the guest landing bundle (PageSpeed). SSR still renders it. -->
	{#await import('$lib/components/home/AuthedHomeView.svelte') then { default: AuthedHomeView }}
		<AuthedHomeView {data} {tabs} {config} />
	{/await}
{/if}
