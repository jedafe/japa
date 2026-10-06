import type { Snippet } from 'svelte';

/**
 * Reactive bridge for route-specific right-sidebar widgets.
 *
 * The (app) layout owns the AppShell and its right sidebar. Child routes
 * (e.g. /tracks/[slug], /my-learning) set a sidebar snippet here so the layout
 * can render it above the global defaults — without the layout needing to know
 * which route is active.
 *
 * SSR safety: the snippet is set during component initialisation (before the
 * first render), so SSR and CSR render the same sidebar content. The store is
 * cleared on unmount via the $effect cleanup in setSidebar().
 *
 * Ref: ADR-058 (right sidebar — route-aware widgets via `sidebar` slot)
 */
let current = $state<Snippet | null>(null);

export const sidebarStore = {
	get current(): Snippet | null {
		return current;
	},
	set(snippet: Snippet | null) {
		current = snippet;
	}
};

/**
 * Set the route-specific sidebar snippet for the lifetime of the calling
 * component. Clears on cleanup so navigating away restores the global widgets.
 */
export function setSidebar(snippet: Snippet): void {
	sidebarStore.set(snippet);
	$effect(() => {
		return () => sidebarStore.set(null);
	});
}
