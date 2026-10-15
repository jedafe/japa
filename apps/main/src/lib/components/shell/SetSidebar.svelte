<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { routeSidebar } from './routeSidebar.svelte';

	/**
	 * Wrapper that registers its children as the route-specific right-sidebar
	 * snippet for the AppShell rail (ADR-058).
	 *
	 * Registration happens in onMount — writes made during component init or
	 * pre-effects are discarded when SvelteKit renders the incoming page in a
	 * staged/pending tree before commit, which left the rail stuck on the
	 * outgoing page's (or no) snippet after tab switches. onMount/onDestroy
	 * run against the committed tree, so ordering is stable; the cleanup is
	 * guarded by an ownership token so whichever page mounts last keeps the
	 * slot regardless of unmount order.
	 */
	let { children }: { children: Snippet } = $props();

	const owner = Symbol('set-sidebar');

	onMount(() => {
		routeSidebar.owner = owner;
		routeSidebar.snippet = children;
		return () => {
			if (routeSidebar.owner === owner) {
				routeSidebar.owner = null;
				routeSidebar.snippet = null;
			}
		};
	});
</script>
