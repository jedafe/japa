<script lang="ts">
	import { page } from '$app/state';
	import FeedTabs from '$lib/components/feed/FeedTabs.svelte';
	import FeedSortBar from '$lib/components/feed/FeedSortBar.svelte';
	import FeedList from '$lib/components/feed/FeedList.svelte';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// cardStyleConfig comes from the (app) layout loader (shared shell data).
	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });
</script>

<div class="mx-auto w-full max-w-2xl">
	<FeedTabs tab={data.tab} sort={data.sort} authed={data.authed} />
	<FeedSortBar tab={data.tab} sort={data.sort} />
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
