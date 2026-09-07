<script lang="ts">
	// Guide single page at /guide/{slug} — canonical route for content_type
	// 'guide' (§14, ADR-058). Shares the generic article layout: the guide
	// category badge, journey chips, custom-field details card, series nav,
	// owner stats, and admin JSON-LD all render conditionally inside it.
	import ContentArticlePage from '$lib/components/content/ContentArticlePage.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<ContentArticlePage {data} />
