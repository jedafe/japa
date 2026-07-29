<script lang="ts">
	import { page } from '$app/state';
	import FeedSortBar from '$lib/components/feed/FeedSortBar.svelte';
	import FeedList from '$lib/components/feed/FeedList.svelte';
	import { t } from '$lib/i18n';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });

	const title = $derived(t('explore.title', 'Explore — JapaConnect'));
	const description = $derived(t('explore.description', 'Discover stories, discussions, and guides from the Nigerian diaspora community.'));
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('explore.heading', 'Explore')}</h1>
	</div>
	<FeedSortBar tab="explore" sort={data.sort} />
	<div class="p-3">
		<FeedList
			items={data.items}
			tab="explore"
			sort={data.sort}
			pageSize={data.pageSize}
			{config}
			loadError={data.loadError}
		/>
	</div>
</div>
