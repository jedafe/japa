<script lang="ts">
	import { contentHref, siteUrl } from '../../lib/links';
	import type { FeedItem } from '../../lib/types';

	let { item }: { item: FeedItem } = $props();
	const href = $derived(siteUrl(contentHref(item)));
	const meta = $derived(
		[item.space_name, item.author_name].filter(Boolean).join(' · '),
	);
</script>

<a class="card" {href}>
	<span class="card-title">{item.title}</span>
	{#if item.excerpt}
		<span class="card-excerpt">{item.excerpt}</span>
	{/if}
	<span class="card-meta">
		{#if meta}{meta}{/if}
		{#if item.comment_count > 0} · {item.comment_count} comments{/if}
	</span>
</a>
