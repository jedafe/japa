<script lang="ts">
	import PublishedPageView from '$lib/components/published-page/PublishedPageView.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<PublishedPageView page={data.page} />
