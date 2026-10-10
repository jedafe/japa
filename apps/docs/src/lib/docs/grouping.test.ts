import { describe, it, expect } from 'vitest';
import { groupByCategory } from './grouping';
import type { DocNavItem } from '$lib/types';

function item(id: string, category?: string, subcategory?: string): DocNavItem {
	return {
		id,
		title: id,
		slug: id,
		audience: 'users',
		position: 0,
		category: category ?? null,
		subcategory: subcategory ?? null
	};
}

describe('groupByCategory', () => {
	it('puts ungrouped items first and preserves input order', () => {
		const out = groupByCategory([
			item('a', 'Guides'),
			item('b'),
			item('c', 'Guides'),
			item('d')
		]);
		expect(out.ungrouped.map((i) => i.id)).toEqual(['b', 'd']);
		expect(out.categories).toHaveLength(1);
		expect(out.categories[0].name).toBe('Guides');
		expect(out.categories[0].subcategories[0].items.map((i) => i.id)).toEqual(['a', 'c']);
	});

	it('orders categories by first appearance, not alphabetically', () => {
		const out = groupByCategory([item('a', 'Zeta'), item('b', 'Alpha'), item('c', 'Zeta')]);
		expect(out.categories.map((c) => c.name)).toEqual(['Zeta', 'Alpha']);
	});

	it('groups subcategories with empty subcategory first', () => {
		const out = groupByCategory([
			item('a', 'Guides', 'Advanced'),
			item('b', 'Guides'),
			item('c', 'Guides', 'Advanced'),
			item('d', 'Guides', 'Basics')
		]);
		const subs = out.categories[0].subcategories;
		expect(subs.map((s) => s.name)).toEqual(['', 'Advanced', 'Basics']);
		expect(subs[0].items.map((i) => i.id)).toEqual(['b']);
		expect(subs[1].items.map((i) => i.id)).toEqual(['a', 'c']);
	});

	it('treats whitespace-only labels as ungrouped', () => {
		const out = groupByCategory([item('a', '  '), item('b', 'Guides', '   ')]);
		expect(out.ungrouped.map((i) => i.id)).toEqual(['a']);
		expect(out.categories[0].subcategories.map((s) => s.name)).toEqual(['']);
	});

	it('returns empty groups for empty input', () => {
		expect(groupByCategory([])).toEqual({ ungrouped: [], categories: [] });
	});
});
