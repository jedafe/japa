import type { Component } from 'svelte';

/**
 * Lazy lucide resolution — mirror of @japaconnect/ui's icon.ts. A namespace
 * import (`import * as LucideIcons`) puts all ~1,600 icons (~850KB) into any
 * bundle containing landing blocks; lazy-glob keeps each icon a separate
 * chunk fetched only when a name is actually rendered.
 */
const mapLoader = () => import('./icon-map');

function pascalToKebab(name: string): string {
	return name.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase();
}

function kebabToPascal(name: string): string {
	return name
		.split('-')
		.map((p) => p.charAt(0).toUpperCase() + p.slice(1))
		.join('');
}

const cache = new Map<string, Promise<Component | undefined>>();

/** Resolve a kebab-case or PascalCase lucide name to its component. */
export function resolveIcon(name: string): Promise<Component | undefined> {
	const kebab = name.includes('-') ? name : pascalToKebab(name);
	let pending = cache.get(kebab);
	if (!pending) {
		pending = mapLoader().then(({ iconLoaders }) => {
			const loader = iconLoaders[`../../node_modules/lucide-svelte/dist/icons/${kebab}.js`];
			return loader ? loader().then((m) => m.default) : undefined;
		});
		cache.set(kebab, pending);
	}
	return pending;
}

/** All icon names (PascalCase) — enumerated from glob keys without loading
 * any icon module. For pickers and admin UIs. */
export async function iconNames(): Promise<string[]> {
	const { iconLoaders } = await mapLoader();
	return Object.keys(iconLoaders)
		.map((k) => k.slice(k.lastIndexOf('/') + 1, -3))
		.map(kebabToPascal);
}
