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

/** Kebab candidates for an icon name, most likely first. Lucide also
 *  hyphenates letter→digit boundaries ("Building2" → "building-2") while
 *  keeping "NxN" units intact ("Grid2x2" → "grid-2x2") — no single transform
 *  recovers that split, so resolveIcon tries candidates in order. */
export function kebabCandidates(name: string): string[] {
	const base = name.includes('-') ? name.toLowerCase() : pascalToKebab(name);
	// Hyphenate a letter directly before a digit — unless the letter itself
	// follows a digit (keeps the "x" in "2x2"-style groups attached).
	const digitSplit = base.replace(/(?<![0-9])[a-z](?=[0-9])/g, '$&-');
	return digitSplit === base ? [base] : [base, digitSplit];
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
	const candidates = kebabCandidates(name);
	const cacheKey = candidates.join('|');
	let pending = cache.get(cacheKey);
	if (!pending) {
		pending = mapLoader().then(({ iconLoaders }) => {
			for (const kebab of candidates) {
				const loader = iconLoaders[`../../node_modules/lucide-svelte/dist/icons/${kebab}.js`];
				if (loader) return loader().then((m) => m.default);
			}
			return undefined;
		});
		cache.set(cacheKey, pending);
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
