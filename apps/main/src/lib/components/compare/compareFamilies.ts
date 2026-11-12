/**
 * Comparison-type families — the hub's category model. `key` is the admin-set
 * `config.family` value on a comparison type; `slug` is its URL form for
 * /compare/<slug>. Note family keys mix conventions already in the data
 * (`visa_routes` vs `cost-of-living`) so slug ↔ key mapping always goes
 * through the catalog, never a string transform.
 *
 * Mirrors FAMILY_ORDER in TypeCardGrid.svelte: a new admin family needs a
 * one-line entry here to get a /compare/<slug> browse page.
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
	GitCompare
} from 'lucide-svelte';
import type { ComparisonType } from '#lib/types/jce.js';

// lucide-svelte ships legacy class-component typings; the family cards take
// Svelte 5 `Component` props, so cast at the seam (same convention as
// /compare's asComponent helper).
const asComponent = (icon: unknown): Component => icon as Component;

export interface CompareFamily {
	key: string;
	slug: string;
	icon: Component;
	tint: string;
}

export const COMPARE_FAMILIES: CompareFamily[] = [
	{ key: 'location', slug: 'location', icon: asComponent(Globe), tint: '#4f7cff' },
	{ key: 'education', slug: 'education', icon: asComponent(GraduationCap), tint: '#22a06b' },
	{ key: 'visa_routes', slug: 'visa-routes', icon: asComponent(Stamp), tint: '#8b5cf6' },
	{ key: 'banking', slug: 'banking', icon: asComponent(Landmark), tint: '#e8a13a' },
	{ key: 'cost-of-living', slug: 'cost-of-living', icon: asComponent(Wallet), tint: '#f97316' },
	{ key: 'embassies', slug: 'embassies', icon: asComponent(Building2), tint: '#ec4899' },
	{ key: 'travel', slug: 'travel', icon: asComponent(Plane), tint: '#06b6d4' },
	{ key: 'jobs', slug: 'jobs', icon: asComponent(Briefcase), tint: '#64748b' }
];

export const FALLBACK_FAMILY_ICON: Component = asComponent(GitCompare);

export function familySlug(key: string): string {
	return familyForKey(key)?.slug ?? key.replace(/_/g, '-');
}

export function familyKeyFromSlug(slug: string): string {
	return familyForSlug(slug)?.key ?? slug;
}

export function familyForKey(key: string): CompareFamily | undefined {
	return COMPARE_FAMILIES.find((f) => f.key === key);
}

export function familyForSlug(slug: string): CompareFamily | undefined {
	return COMPARE_FAMILIES.find((f) => f.slug === slug);
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
