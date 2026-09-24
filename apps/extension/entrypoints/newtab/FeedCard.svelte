<script lang="ts">
	import { contentHref, siteUrl } from '../../lib/links';
	import type { FeedItem } from '../../lib/types';

	let { item }: { item: FeedItem } = $props();
	const href = $derived(siteUrl(contentHref(item)));
	const meta = $derived(
		[item.space_name, item.author_name].filter(Boolean).join(' · '),
	);
	// Some content types store ProseMirror JSON in excerpt — never render
	// the raw document blob as card text.
	const excerpt = $derived(
		item.excerpt && !item.excerpt.trimStart().startsWith('{') ? item.excerpt : null,
	);
</script>

<a class="card" {href}>
	<span class="card-title">{item.title}</span>
	{#if excerpt}
		<span class="card-excerpt">{excerpt}</span>
	{/if}
	<span class="card-meta">
		{#if meta}{meta}{/if}
		{#if item.comment_count > 0} · {item.comment_count} comments{/if}
	</span>
</a>
