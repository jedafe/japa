<script lang="ts">
	// Generic content single (fallback route family — ADR-058). Types with a
	// canonical route are 301-redirected by the loader; this renders the rest.
	import ContentArticlePage from '$lib/components/content/ContentArticlePage.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<ContentArticlePage {data} />
