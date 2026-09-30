import * as LucideIcons from 'lucide-svelte';

/** lucide-svelte v1 icons are legacy SvelteComponentTyped classes — type
 *  them via the base Icon class (same convention as apps/main's
 *  lib/components/ui/icons.ts). */
export type IconComponent = typeof import('lucide-svelte').Icon;

const cache = new Map<string, IconComponent | undefined>();

/**
 * Resolve a kebab-case Lucide icon name (e.g. "arrow-left-right") to the
 * component. Returns undefined for unknown names.
 */
export function resolveLucideIcon(name: string): IconComponent | undefined {
	if (cache.has(name)) return cache.get(name);
	const pascal = name
		.split('-')
		.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
		.join('');
	const icon = (LucideIcons as Record<string, unknown>)[pascal] as IconComponent | undefined;
	cache.set(name, icon);
	return icon;
}
