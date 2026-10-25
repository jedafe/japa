<script lang="ts">
	// Authenticated home timeline. Imported dynamically from (app)/+page.svelte
	// so the feed stack (cards, vote rail, hover chips, tiptap serializers,
	// ads — ~250KB transfer) stays out of the guest landing bundle. SSR still
	// renders it for signed-in users; the split only changes client loading.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { UrlTabs, type UrlTab } from '$lib/components/ui';
	import FeedGeoControl from '$lib/components/feed/FeedGeoControl.svelte';
	import FeedSortBar from '$lib/components/feed/FeedSortBar.svelte';
	import FeedList from '$lib/components/feed/FeedList.svelte';
	import FeedUpdatePill from '$lib/components/feed/FeedUpdatePill.svelte';
	import OnboardingSetupCard from '$lib/components/onboarding/OnboardingSetupCard.svelte';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import type { PageData } from '../../../routes/(app)/$types';

	let {
		data,
		tabs,
		config
	}: {
		data: PageData;
		tabs: UrlTab[];
		config: CardStyleConfig;
	} = $props();
</script>

<!-- Authenticated home timeline in the app shell -->
<div class="mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<div
		class="sticky top-0 z-10 flex items-center border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] backdrop-blur"
		style="margin-inline: max(1rem, calc((100% - 48rem)/2)); min-height: var(--nav-height)"
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
			<div class="ms-auto pe-1">
				<FeedGeoControl />
			</div>
		{/if}
	</div>
	<FeedSortBar tab={data.tab} sort={data.sort} stickyTop="var(--nav-height)" />
	<!-- Phase 23 — live feed update pill (driven by WS feed_update events) -->
	<FeedUpdatePill />
	<div class="mx-auto w-full max-w-3xl p-4">
		<!-- §09: finish-setup reminder — self-gating on page.data.onboardingCompleted -->
		<OnboardingSetupCard />
		<FeedList
			items={data.items}
			tab={data.tab}
			sort={data.sort}
			pageSize={data.pageSize}
			{config}
			loadError={data.loadError}
			hasMore={data.hasMore}
			adSlot="native_feed"
		/>
	</div>
</div>
