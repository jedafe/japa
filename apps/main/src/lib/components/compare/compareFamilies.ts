/**
 * Comparison-type families — the hub's category model. `key` is the admin-set
 * `config.family` value on a comparison type. Family tiles on /compare enter
 * the picker workspace through the family's first member type (?type=), and
 * the in-workspace sibling switcher covers the rest.
 *
 * Mirrors FAMILY_ORDER in TypeCardGrid.svelte: a new admin family needs a
 * one-line entry here to get a landing tile.
 */
import type { Component } from 'svelte';
import {
	Globe,
	GraduationCap,
	Stamp,
	Landmark,
	Wallet,
	Building2,
	Plane,
	Briefcase,
	GitCompare,
	Store,
	Package
} from 'lucide-svelte';
import type { ComparisonType } from '#lib/types/jce.js';

// lucide-svelte ships legacy class-component typings; the family cards take
// Svelte 5 `Component` props, so cast at the seam (same convention as
// /compare's asComponent helper).
const asComponent = (icon: unknown): Component => icon as Component;

export interface CompareFamily {
	key: string;
	icon: Component;
	tint: string;
}

export const COMPARE_FAMILIES: CompareFamily[] = [
	{ key: 'location', icon: asComponent(Globe), tint: '#4f7cff' },
	{ key: 'education', icon: asComponent(GraduationCap), tint: '#22a06b' },
	{ key: 'visa_routes', icon: asComponent(Stamp), tint: '#8b5cf6' },
	{ key: 'banking', icon: asComponent(Landmark), tint: '#e8a13a' },
	{ key: 'cost-of-living', icon: asComponent(Wallet), tint: '#f97316' },
	{ key: 'embassies', icon: asComponent(Building2), tint: '#ec4899' },
	{ key: 'travel', icon: asComponent(Plane), tint: '#06b6d4' },
	{ key: 'jobs', icon: asComponent(Briefcase), tint: '#64748b' },
	{ key: 'business', icon: asComponent(Store), tint: '#0ea5e9' },
	{ key: 'products', icon: asComponent(Package), tint: '#f43f5e' }
];

export const FALLBACK_FAMILY_ICON: Component = asComponent(GitCompare);

export function familyForKey(key: string): CompareFamily | undefined {
	return COMPARE_FAMILIES.find((f) => f.key === key);
}

export interface FamilyGroup {
	family: CompareFamily;
	types: ComparisonType[];
}

/** Bucket types by config.family — groups in COMPARE_FAMILIES order, then
 *  ungrouped types (no family, or a family with no catalog entry). */
export function groupTypesByFamily(types: ComparisonType[]): {
	groups: FamilyGroup[];
	ungrouped: ComparisonType[];
} {
	const byKey = new Map<string, ComparisonType[]>();
	const ungrouped: ComparisonType[] = [];
	for (const cmpType of types) {
		const key = typeof cmpType.config?.family === 'string' ? cmpType.config.family : '';
		const family = key ? familyForKey(key) : undefined;
		if (!family) {
			ungrouped.push(cmpType);
			continue;
		}
		byKey.set(family.key, [...(byKey.get(family.key) ?? []), cmpType]);
	}
	const groups = COMPARE_FAMILIES.flatMap((family) => {
		const list = byKey.get(family.key);
		return list?.length ? [{ family, types: list }] : [];
	});
	return { groups, ungrouped };
}
