import { describe, it, expect } from 'vitest';
import {
	categorySlug,
	collectCategories,
	findCategory,
	parsePinnedPages,
	resolvePinnedPages,
	parseVersions,
	parseCsvSetting,
	parseIntSetting,
	parseLinkList,
	parseLinkCards,
	parseManagedCategories,
	mergeManagedCategories,
	collectChangelog,
	audienceStats,
	popularPages
} from './catalog';
import type { DocNavItem, DocNavTree, DocPageSummary } from '$lib/types';

function item(id: string, category?: string, subcategory?: string, children?: DocNavItem[]): DocNavItem {
	return {
		id,
		title: id,
		slug: id,
		audience: 'users',
		position: 0,
		category: category ?? null,
		subcategory: subcategory ?? null,
		children
	};
}

function tree(audience: DocNavTree['audience'], items: DocNavItem[]): DocNavTree {
	return { audience, items };
}

describe('collectCategories', () => {
	it('aggregates categories across audiences, case-folded, first-seen order', () => {
		const nav = [
			tree('users', [item('a', 'Guides', 'Basics'), item('b', 'guides', 'Advanced'), item('c')]),
			tree('api', [item('d', 'API Reference'), item('e', 'GUIDES')])
		];
		const cats = collectCategories(nav);
		expect(cats.map((c) => c.name)).toEqual(['Guides', 'API Reference']);
		expect(cats[0].items.map((i) => i.slug)).toEqual(['a', 'b', 'e']);
		expect(cats[0].subcategories).toEqual(['Basics', 'Advanced']);
		expect(cats[1].items).toHaveLength(1);
	});

	it('skips uncategorized items and does not scan children', () => {
		const nav = [tree('users', [item('a', undefined, undefined, [item('kid', 'Guides')])])];
		expect(collectCategories(nav)).toEqual([]);
	});
});

describe('categorySlug / findCategory', () => {
	it('slugifies names and resolves them back', () => {
		expect(categorySlug('API Reference')).toBe('api-reference');
		const nav = [tree('users', [item('a', 'API Reference')])];
		expect(findCategory(nav, 'api-reference')?.name).toBe('API Reference');
		expect(findCategory(nav, 'API%20Reference')?.name).toBe('API Reference');
		expect(findCategory(nav, 'nope')).toBeNull();
	});
});

describe('pinned pages', () => {
	const nav = [tree('users', [item('quick-start')]), tree('admin', [item('internal')])];

	it('parses object and shorthand pin shapes', () => {
		expect(parsePinnedPages([{ audience: 'users', slug: 'a', label: 'A' }, 'users/b', { slug: 'x' }])).toEqual([
			{ audience: 'users', slug: 'a', label: 'A' },
			{ audience: 'users', slug: 'b' }
		]);
		expect(parsePinnedPages('[{"audience":"users","slug":"a"}]')).toHaveLength(1);
		expect(parsePinnedPages('not json')).toEqual([]);
		expect(parsePinnedPages(undefined)).toEqual([]);
	});

	it('resolves pins to hrefs, skipping inaccessible audiences and missing slugs', () => {
		const pins = parsePinnedPages([
			{ audience: 'users', slug: 'quick-start' },
			{ audience: 'admin', slug: 'internal' }, // not accessible for a guest
			{ audience: 'users', slug: 'missing' }
		]);
		const out = resolvePinnedPages(pins, nav, ['users', 'api']);
		expect(out).toEqual([{ href: '/docs/users/quick-start', label: 'quick-start' }]);
	});
});

describe('versions', () => {
	it('parses object and string shapes', () => {
		expect(parseVersions([{ label: 'v1.0', url: '/docs' }, 'v2.0-beta'])).toEqual([
			{ label: 'v1.0', url: '/docs' },
			{ label: 'v2.0-beta' }
		]);
		expect(parseVersions('[]')).toEqual([]);
		expect(parseVersions(null)).toEqual([]);
	});
});

describe('csv / int settings', () => {
	it('parses csv settings from string or array', () => {
		expect(parseCsvSetting('Changelog, Release Notes')).toEqual(['Changelog', 'Release Notes']);
		expect(parseCsvSetting(['a', 'b'])).toEqual(['a', 'b']);
		expect(parseCsvSetting('', ['X'])).toEqual(['X']);
	});
	it('parses int settings with fallback', () => {
		expect(parseIntSetting(30, 14)).toBe(30);
		expect(parseIntSetting('7', 14)).toBe(7);
		expect(parseIntSetting('nope', 14)).toBe(14);
		expect(parseIntSetting(undefined, 14)).toBe(14);
	});
});

describe('collectChangelog', () => {
	const summary = (slug: string, category: string | null, created_at: string): DocPageSummary => ({
		id: slug,
		title: slug,
		slug,
		audience: 'users',
		review_status: 'published',
		position: 0,
		category,
		created_at
	});
	const now = new Date('2026-10-11T00:00:00Z');

	it('filters to changelog categories, newest first, with isNew relative to window', () => {
		const out = collectChangelog(
			[
				summary('old', 'Changelog', '2026-01-01T00:00:00Z'),
				summary('fresh', 'Changelog', '2026-10-05T00:00:00Z'),
				summary('guide', 'Guides', '2026-10-10T00:00:00Z'),
				summary('other-cat', 'changelog', '2026-09-20T00:00:00Z')
			],
			['changelog'],
			14,
			now
		);
		expect(out.map((e) => e.slug)).toEqual(['fresh', 'other-cat', 'old']);
		expect(out[0].isNew).toBe(true);
		expect(out[1].isNew).toBe(false);
	});
});

describe('parseLinkList', () => {
	it('keeps entries with non-empty label and url', () => {
		expect(
			parseLinkList([
				{ label: 'Visit Help Center', url: 'https://help.example.com' },
				{ label: 'No url' },
				{ label: '', url: '/x' },
				'junk',
				{ label: ' Contact Support ', url: ' /support ' }
			])
		).toEqual([
			{ label: 'Visit Help Center', url: 'https://help.example.com' },
			{ label: 'Contact Support', url: '/support' }
		]);
	});

	it('accepts a JSON string and returns [] for non-arrays', () => {
		expect(parseLinkList('[{"label":"A","url":"/a"}]')).toEqual([{ label: 'A', url: '/a' }]);
		expect(parseLinkList('{"label":"A"}')).toEqual([]);
		expect(parseLinkList(undefined)).toEqual([]);
		expect(parseLinkList(null)).toEqual([]);
	});
});

describe('parseLinkCards', () => {
	it('keeps optional description and icon', () => {
		expect(
			parseLinkCards([
				{ label: 'API Playground', url: '/docs/api/playground', description: 'Try it', icon: 'play' },
				{ label: 'Keys', url: '/docs/api/keys', icon: '  ' }
			])
		).toEqual([
			{ label: 'API Playground', url: '/docs/api/playground', description: 'Try it', icon: 'play' },
			{ label: 'Keys', url: '/docs/api/keys', description: undefined, icon: undefined }
		]);
	});
});

describe('managed categories', () => {
	it('parses the docs.categories catalog tolerantly', () => {
		expect(
			parseManagedCategories([
				{ name: 'Guides', description: 'How-tos', icon: 'Book', subcategories: ['Basics', ' '] },
				{ name: '  ' },
				{ slug: 'no-name' },
				{ name: 'API' }
			])
		).toEqual([
			{ name: 'Guides', description: 'How-tos', icon: 'book', subcategories: ['Basics'] },
			{ name: 'API', description: undefined, icon: undefined, subcategories: [] }
		]);
		expect(parseManagedCategories('[{"name":"Guides"}]')).toHaveLength(1);
		expect(parseManagedCategories('nope')).toEqual([]);
		expect(parseManagedCategories(undefined)).toEqual([]);
	});

	it('merges managed order first, then derived extras, inheriting page items', () => {
		const derived = collectCategories([
			tree('users', [
				item('a', 'Guides', 'Basics'),
				item('b', 'Guides', 'Advanced'),
				item('c', 'Reference')
			])
		]);
		const merged = mergeManagedCategories(derived, [
			{ name: 'API', description: 'Endpoints', icon: 'api', subcategories: ['Auth'] },
			{ name: 'guides', subcategories: ['Quickstart'] }
		]);
		expect(merged.map((c) => c.name)).toEqual(['API', 'guides', 'Reference']);
		// Managed-only entry: own slug, zero items, managed metadata.
		expect(merged[0].slug).toBe('api');
		expect(merged[0].items).toEqual([]);
		expect(merged[0].description).toBe('Endpoints');
		expect(merged[0].icon).toBe('api');
		// Managed subs first, then derived extras; page items inherited.
		expect(merged[1].subcategories).toEqual(['Quickstart', 'Basics', 'Advanced']);
		expect(merged[1].items.map((i) => i.slug)).toEqual(['a', 'b']);
	});

	it('returns derived unchanged when the catalog is empty', () => {
		const derived = collectCategories([tree('users', [item('a', 'Guides')])]);
		expect(mergeManagedCategories(derived, [])).toBe(derived);
	});
});

describe('audienceStats / popularPages', () => {
	const nav = [
		tree('users', [item('a', undefined, undefined, [item('a1'), item('a2')]), item('b')]),
		tree('api', [item('c'), item('d'), item('e')])
	];

	it('counts top-level guides and total pages including children', () => {
		expect(audienceStats(nav[0])).toEqual({ guides: 2, pages: 4 });
		expect(audienceStats(nav[1])).toEqual({ guides: 3, pages: 3 });
		expect(audienceStats(undefined)).toEqual({ guides: 0, pages: 0 });
	});

	it('takes leading items per audience capped at total', () => {
		const out = popularPages(nav, 2, 3);
		expect(out.map((p) => p.slug)).toEqual(['a', 'b', 'c']);
	});
});
