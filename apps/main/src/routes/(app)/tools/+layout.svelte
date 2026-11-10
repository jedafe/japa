<script lang="ts">
	// /tools layout — mounts the entity discussion block at the bottom of
	// tool detail pages whose registry entry opts in (discussion: 'bottom',
	// the default) and constrains those pages to the standard center-shell
	// width (max-w-3xl) used by every other page that renders with the right
	// sidebar (/thread, /wiki, /spaces). 'none' tools (JapaBot, readiness,
	// travel) are full-width workbenches that extend into the right shell —
	// they render unwrapped with no discussion block. The hub index and the
	// [tab] permalink redirect never reach this render path — the slug must
	// resolve to a registered tool anyway.
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

{#if toolSlug}
	{#key toolSlug}
		<div class="mx-auto w-full max-w-3xl">
			{@render children()}
			<!-- Tool pages pad their own content with px-4 — the discussion
			     block needs the same inset or it sits 16px proud each side. -->
			<div class="px-4">
				<EntityDiscussions entityType="tool" entityKey={toolSlug} authed={isAuthed()} />
			</div>
		</div>
	{/key}
{:else}
	{@render children()}
{/if}
