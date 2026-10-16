import type { Snippet } from 'svelte';

/**
 * ADR-058 — registry for the route-specific right-sidebar snippet.
 *
 * A module-level $state object is used instead of Svelte context: during
 * SvelteKit client-side navigations the incoming page tree can be staged
 * under a different component context than the mounted RightSidebar, so a
 * context-carried snippet could be written to a context object the rail
 * never reads — leaving the sidebar stuck empty until the next mount. One
 * shared record makes mount/unmount order the only thing that matters.
 *
 * `owner` must be a primitive (Symbol) — $state wraps stored plain objects
 * in proxies, so an object token would never survive an `===` comparison.
 * An outgoing page's destroy only clears the registry while it still owns
 * it, so the incoming page's registration wins regardless of cleanup order.
 */
export const routeSidebar = $state<{ owner: symbol | null; snippet: Snippet | null }>({
	owner: null,
	snippet: null
});

/**
 * Back-compat shim — the app layout reads `sidebarStore.current`.
 * Prefer `routeSidebar` for new code.
 */
export const sidebarStore = {
	get current(): Snippet | null {
		return routeSidebar.snippet;
	}
};
