import type { DocNavItem } from '$lib/types';

/**
 * Category/subcategory grouping for docs nav trees and audience listings.
 * Ordering contract (§44 taxonomy amendment): ungrouped items first in input
 * order, then each category in first-appearance order; inside a category,
 * items with no subcategory first, then each subcategory in first-appearance
 * order.
 */

export interface DocSubcategoryGroup<T> {
	/** '' = items with no subcategory (rendered first, no header). */
	name: string;
	items: T[];
}

export interface DocCategoryGroup<T> {
	name: string;
	subcategories: DocSubcategoryGroup<T>[];
}

export interface GroupedDocItems<T> {
	ungrouped: T[];
	categories: DocCategoryGroup<T>[];
}

type Taxonomized = { category?: string | null; subcategory?: string | null };

export function groupByCategory<T extends Taxonomized>(items: T[]): GroupedDocItems<T> {
	const ungrouped: T[] = [];
	const categories: DocCategoryGroup<T>[] = [];
	// Case-folded keys — "Guides" and "guides" are the same group; the
	// first-seen casing becomes the display label (§44 amendment 2).
	const catIdx = new Map<string, DocCategoryGroup<T>>();

	for (const item of items) {
		const catName = item.category?.trim();
		if (!catName) {
			ungrouped.push(item);
			continue;
		}

		let cat = catIdx.get(catName.toLowerCase());
		if (!cat) {
			cat = { name: catName, subcategories: [{ name: '', items: [] }] };
			catIdx.set(catName.toLowerCase(), cat);
			categories.push(cat);
		}

		const subName = item.subcategory?.trim() ?? '';
		let sub = cat.subcategories.find(
			(s) => s.name.toLowerCase() === subName.toLowerCase()
		);
		if (!sub) {
			sub = { name: subName, items: [] };
			cat.subcategories.push(sub);
		}
		sub.items.push(item);
	}

	return { ungrouped, categories };
}

/** Convenience for DocNavItem lists (nav tree + audience listing). */
export function groupNavItems(items: DocNavItem[]): GroupedDocItems<DocNavItem> {
	return groupByCategory(items);
}
