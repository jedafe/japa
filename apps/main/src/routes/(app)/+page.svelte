<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { UrlTabs, type UrlTab } from '$lib/components/ui';
	import FeedGeoControl from '$lib/components/feed/FeedGeoControl.svelte';
	import FeedSortBar from '$lib/components/feed/FeedSortBar.svelte';
	import FeedList from '$lib/components/feed/FeedList.svelte';
	import FeedUpdatePill from '$lib/components/feed/FeedUpdatePill.svelte';
	import { LandingRenderer } from '@japaconnect/landing-blocks';
	import { t } from '$lib/i18n';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import GuestHomePage from '$lib/components/landing/GuestHomePage.svelte';

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
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

{#if data.guestHome}
	<!-- ADR-058 §5 Exception #3: new default guest homepage -->
	<GuestHomePage home={data.home} />
{:else if data.landing}
	<!-- ADR-058 §5 Exception #3: guest Page Builder landing page (or default seed) -->
	<LandingRenderer blocks={data.landing.blocks} geoCountry={data.contentCountry} />
{:else}
	<!-- Authenticated home timeline in the app shell -->
	<div class="mx-auto w-full max-w-full">
		<div
			class="sticky top-0 z-10 flex items-center border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] backdrop-blur"
			style="min-height: var(--nav-height)"
		>
			<UrlTabs
				{tabs}
				value={data.tab}
				onValueChange={(v) => {
					// Preserve non-tab query params (especially ?geo=) while switching tabs.
					const url = new URL(page.url);
					url.searchParams.set('tab', v);
					url.searchParams.set('sort', data.sort);
					goto(`${url.pathname}${url.search}`, { keepFocus: true });
				}}
				class="border-b-0"
			/>
			{#if data.authed}
				<div class="ml-auto pr-1">
					<FeedGeoControl />
				</div>
			{/if}
		</div>
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
				hasMore={data.hasMore}
			/>
		</div>
	</div>
{/if}
