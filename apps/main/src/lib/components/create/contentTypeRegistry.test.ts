import { describe, it, expect } from 'vitest';
import { resolveType, availableTypes } from './contentTypeRegistry';

describe('resolveType', () => {
	it('maps blog_post/article/guide to the standard form', () => {
		for (const v of ['blog_post', 'article', 'guide']) {
			expect(resolveType(v).formKind).toBe('standard');
		}
	});
	it('maps comparison to the comparison form and its feature flag', () => {
		expect(resolveType('comparison').formKind).toBe('comparison');
		expect(resolveType('comparison').featureFlag).toBe('feature.comparison');
	});
	it('uses sensible defaults for unknown types', () => {
		expect(resolveType('nonsense').value).toBe('nonsense');
		expect(resolveType('nonsense').formKind).toBe('standard');
	});
	it('only standard blog-family types support series', () => {
		expect(resolveType('blog_post').supportsSeries).toBe(true);
		expect(resolveType('comparison').supportsSeries).toBe(false);
	});
});

describe('availableTypes', () => {
	const all = [
		{ value: 'discussion', label: 'Discussion' },
		{ value: 'blog_post', label: 'Blog Post' },
		{ value: 'page', label: 'Page' },
		{ value: 'comparison', label: 'Comparison' },
		{ value: 'question', label: 'Question' },
		{ value: 'article', label: 'Article' },
		{ value: 'guide', label: 'Guide' }
	];
	it('includes all content types the backend advertises (Rule 16)', () => {
		const vals = availableTypes(all, { 'feature.blog': true, 'feature.comparison': true }).map(
			(t) => t.value
		);
		expect(vals).toEqual(['discussion', 'blog_post', 'page', 'comparison', 'question', 'article', 'guide']);
	});
	it('hides comparison when feature.comparison is off', () => {
		const vals = availableTypes(all, { 'feature.blog': true, 'feature.comparison': false }).map(
			(t) => t.value
		);
		expect(vals).not.toContain('comparison');
	});
	it('hides blog_post when feature.blog is off (respecting intent)', () => {
		const vals = availableTypes(all, { 'feature.blog': false, 'feature.comparison': true }).map(
			(t) => t.value
		);
		expect(vals).not.toContain('blog_post');
	});
	it('treats a missing feature flag as enabled', () => {
		const vals = availableTypes(all, {}).map((t) => t.value);
		expect(vals).toContain('article');
		expect(vals).toContain('guide');
		expect(vals).toContain('page');
	});
});
