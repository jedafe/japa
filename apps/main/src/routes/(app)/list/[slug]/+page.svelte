<script lang="ts">
	// List single page at /list/{slug} — canonical route for content_type
	// 'list' (§14 list amendment, ADR-058). Shares the generic article
	// layout; list item markup renders verbatim from body_html and the
	// content-embed/link-preview/video placeholders hydrate inside it.
	import ContentArticlePage from '$lib/components/content/ContentArticlePage.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<ContentArticlePage {data} />
