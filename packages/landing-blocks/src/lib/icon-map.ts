import type { Component } from 'svelte';

/** See packages/ui/src/icon-map.ts — the glob map is its own lazily-loaded
 * module so pages that render no dynamic icons never pay for it. */
export const iconLoaders = import.meta.glob<{ default: Component }>(
	'../../node_modules/lucide-svelte/dist/icons/*.js'
);
