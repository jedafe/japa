<script lang="ts">
	import { getContext, type Snippet } from 'svelte';

	/**
	 * Wrapper that registers its children as the route-specific right-sidebar
	 * snippet for the AppShell. It writes the snippet into Svelte context set by
	 * AppShell, so the snippet is part of the component tree and hydrates
	 * correctly (HYDRATION-001).
	 *
	 * Ref: ADR-058 (right sidebar — route-aware widgets via `sidebar` slot)
	 */
	const SIDEBAR_CONTEXT = 'appshell:sidebar';
	const ctx = getContext<{ current: Snippet | null }>(SIDEBAR_CONTEXT);

	let { children }: { children: Snippet } = $props();

	$effect.pre(() => {
		if (ctx) ctx.current = children;
		// On route transitions SvelteKit can mount the incoming page before
		// the outgoing page's cleanup runs — an unconditional reset would
		// null out the NEW page's snippet, blanking the sidebar on every
		// in-app navigation between sidebar routes (e.g. wiki index → page).
		return () => {
			if (ctx && ctx.current === children) ctx.current = null;
		};
	});
</script>
