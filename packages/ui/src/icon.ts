/** lucide-svelte v1 icons are legacy SvelteComponentTyped classes — type
 *  them via the base Icon class (same convention as apps/main's
 *  lib/components/ui/icons.ts). */
export type IconComponent = typeof import('lucide-svelte').Icon;

/**
 * lucide-svelte ships every icon as its own module under ./icons/<kebab>.js.
 * Resolving names through a namespace import (`import * as LucideIcons`)
 * forces all ~1,600 icons (~850KB) into the eager bundle of any app that
 * pulls in @japaconnect/ui — a measurable PageSpeed regression. Lazy-glob
 * resolution keeps each icon a separate chunk fetched only when a string
 * icon name is actually rendered.
 */
const cache = new Map<string, Promise<IconComponent | undefined>>();

/**
 * Resolve a kebab-case Lucide icon name (e.g. "arrow-left-right") to the
 * component. Resolves to undefined for unknown names.
 */
export function resolveLucideIcon(name: string): Promise<IconComponent | undefined> {
	let pending = cache.get(name);
	if (!pending) {
		pending = import('./icon-map').then(({ iconLoaders }) => {
			const loader = iconLoaders[`../node_modules/lucide-svelte/dist/icons/${name}.js`];
			return loader ? loader().then((m) => m.default) : undefined;
		});
		cache.set(name, pending);
	}
	return pending;
}
