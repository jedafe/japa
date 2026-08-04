<script lang="ts">
	import { page } from '$app/state';
	import FeedTabs from '$lib/components/feed/FeedTabs.svelte';
	import FeedSortBar from '$lib/components/feed/FeedSortBar.svelte';
	import FeedList from '$lib/components/feed/FeedList.svelte';
	import FeedUpdatePill from '$lib/components/feed/FeedUpdatePill.svelte';
	import { LandingRenderer } from '@japaconnect/landing-blocks';
	import { t } from '$lib/i18n';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// cardStyleConfig comes from the (app) layout loader (shared shell data).
	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });

	// SEO: title/description differ for the landing page vs the authed feed.
	const isLanding = $derived(!!data.landing);
	const title = $derived(
		isLanding
			? (data.landing?.title ?? t('app.name'))
			: t('home.title', 'JapaConnect — Explore')
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
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

{#if data.landing}
	<!-- ADR-058 §5 Exception #3: guest Page Builder landing page (or default seed) -->
	<LandingRenderer blocks={data.landing.blocks} geoCountry={data.contentCountry} />
{:else}
	<!-- Authenticated home timeline in the app shell -->
	<div class="mx-auto w-full max-w-full">
		<FeedTabs tab={data.tab} sort={data.sort} authed={data.authed} />
		<FeedSortBar tab={data.tab} sort={data.sort} />
		<!-- Phase 23 — live feed update pill (driven by WS feed_update events) -->
		<FeedUpdatePill />
		<div class="p-3">
			<FeedList
				items={data.items}
				tab={data.tab}
				sort={data.sort}
				pageSize={data.pageSize}
				{config}
				loadError={data.loadError}
			/>
		</div>
	</div>
{/if}
