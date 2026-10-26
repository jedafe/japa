import type { IconComponent } from './icon';

/**
 * The ~1,600-entry glob map lives in its own module so it stays out of the
 * eager bundle — icon.ts dynamic-imports this on first resolution, keeping
 * both the map (~1900 loader refs) and every icon chunk off the critical
 * path of pages that only render pre-selected icons.
 */
export const iconLoaders = import.meta.glob<{ default: IconComponent }>(
	'../node_modules/lucide-svelte/dist/icons/*.js'
);
