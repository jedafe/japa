<script lang="ts">
	import { untrack, type Snippet } from 'svelte';
	import { setSidebar } from '$lib/components/shell/sidebar.svelte';

	/**
	 * Wrapper that registers its children as the route-specific right-sidebar
	 * snippet for the AppShell. Place this at the top of a page component so
	 * the sidebar renders during SSR (the snippet is set during initialisation,
	 * before the first paint).
	 *
	 * Ref: ADR-058 (right sidebar — route-aware widgets via `sidebar` slot)
	 */
	let { children }: { children: Snippet } = $props();
	untrack(() => setSidebar(children));
</script>
