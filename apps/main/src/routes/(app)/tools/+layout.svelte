<script lang="ts">
	// /tools layout — mounts the entity discussion block at the bottom of
	// tool detail pages whose registry entry opts in (discussion: 'bottom',
	// the default). 'sidebar' tools mount EntityDiscussions in their own
	// page markup; 'none' (e.g. JapaBot) render no block. The hub index and
	// the [tab] permalink redirect never reach this render path — the slug
	// must resolve to a registered tool anyway.
	import { page } from '$app/state';
	import { isAuthed } from '#lib/auth/guest.js';
	import { MAIN_TOOLS } from '#lib/tools/registry.js';
	import EntityDiscussions from '#lib/components/thread/EntityDiscussions.svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	const toolSlug = $derived.by(() => {
		const segments = page.url.pathname.split('/').filter(Boolean);
		if (segments.length !== 2 || segments[0] !== 'tools') return '';
		const tool = MAIN_TOOLS.find((t) => t.href === `/tools/${segments[1]}`);
		return tool && (tool.discussion ?? 'bottom') === 'bottom' ? segments[1] : '';
	});
</script>

{@render children()}

{#if toolSlug}
	{#key toolSlug}
		<div class="tool-discussions-wrap">
			<EntityDiscussions entityType="tool" entityKey={toolSlug} authed={isAuthed()} />
		</div>
	{/key}
{/if}

<style>
	/* Tool pages each own their outer width/padding; the wrap matches the
	   common centered-column width so the block aligns with page content. */
	.tool-discussions-wrap {
		max-width: 72rem;
		margin: 0 auto;
		padding: 0 1rem 2rem;
	}
</style>
