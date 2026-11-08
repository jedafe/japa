<script lang="ts">
	// /thread/{slug} dispatch — the server load resolves the slug against the
	// forum API first, then the content API (thread-rename spec):
	//   'forum' → §08 engine view, identical to the old /forum/{slug} page
	//   'embed' → entity embed card + ReplyThread (tool/wiki anchor threads,
	//             §49 linked discussions, other CPTs reached via /thread)
	import ForumThreadView from '#lib/components/thread/ForumThreadView.svelte';
	import ThreadEmbedView from '#lib/components/thread/ThreadEmbedView.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

{#if data.kind === 'embed'}
	<ThreadEmbedView {data} />
{:else}
	<ForumThreadView {data} />
{/if}
