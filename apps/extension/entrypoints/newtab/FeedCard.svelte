<script lang="ts">
	import { contentHref, siteUrl } from '../../lib/links';
	import type { FeedItem } from '../../lib/types';

	let { item }: { item: FeedItem } = $props();
	const href = $derived(siteUrl(contentHref(item)));
	// Save-to-account deep link — extension tokens are read-only (Phase 3
	// spec), so the site performs the bookmark under the user's session.
	const saveUrl = $derived(
		siteUrl(
			`/save?content_id=${encodeURIComponent(item.id)}&return=${encodeURIComponent(contentHref(item))}`,
		),
	);
	const meta = $derived(
		[item.space_name, item.author_name].filter(Boolean).join(' · '),
	);
	// Some content types store ProseMirror JSON in excerpt — never render
	// the raw document blob as card text.
	const excerpt = $derived(
		item.excerpt && !item.excerpt.trimStart().startsWith('{') ? item.excerpt : null,
	);
</script>

<div class="card-wrap">
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
	<a
		class="card-save"
		href={saveUrl}
		target="_blank"
		rel="noopener"
		title="Save to your library"
		aria-label="Save to your library"
	>
		<svg
			viewBox="0 0 24 24"
			width="15"
			height="15"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z" />
		</svg>
	</a>
</div>
