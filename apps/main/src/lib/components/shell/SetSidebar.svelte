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

	// Seed the registry at init when nothing is registered — covers SSR (the
	// rail renders after the center column in AppShell, so the snippet is set
	// by then) and first-paint hydration, so the route widgets render in the
	// initial HTML instead of popping in after mount. During client
	// navigations the outgoing page's registration is still in place here,
	// so this is a no-op and onMount below remains the commit-safe write.
	if (routeSidebar.snippet == null) {
		routeSidebar.owner = owner;
		routeSidebar.snippet = children;
	}

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
