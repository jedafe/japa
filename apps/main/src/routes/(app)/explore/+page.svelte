<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { page } from '$app/state';
	import FeedSortBar from '#lib/components/feed/FeedSortBar.svelte';
	import FeedList from '#lib/components/feed/FeedList.svelte';
	import JourneyContextBar from '#lib/components/feed/JourneyContextBar.svelte';
	import { t } from '#lib/i18n/index.js';
	import type { CardStyleConfig } from '#lib/feed/cardStyle.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });

	const title = $derived(t('explore.title', 'Explore — {site}'));
	const description = $derived(t('explore.description', 'Discover stories, discussions, and guides from the Nigerian diaspora community.'));
</script>

<SeoHead title={title} description={description} ogTitle={title} />

<div class="mx-auto w-full max-w-full">
	{#if !data.user}
		<!-- Guest journey context bar — scoped to /explore (the guest
		     discovery surface); the modal prompt fires site-wide from the
		     app layout. Self-gates for members and declared journeys. -->
		<JourneyContextBar />
	{/if}
	<div class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('explore.heading', 'Explore')}</h1>
	</div>
	<FeedSortBar tab="explore" sort={data.sort} />
	<div class="mx-auto w-full max-w-3xl p-4">
		<FeedList
			items={data.items}
			tab="explore"
			sort={data.sort}
			pageSize={data.pageSize}
			{config}
			loadError={data.loadError}
			hasMore={data.hasMore}
			adSlot="native_discover"
		/>
	</div>
</div>
