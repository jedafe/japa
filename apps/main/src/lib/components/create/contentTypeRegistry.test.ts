import { describe, it, expect } from 'vitest';
import { resolveType, availableTypes, createAvailableTypes, CREATE_ALLOWED_TYPES } from './contentTypeRegistry';

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
	it('maps list to the list form, public setting, and RBAC node', () => {
		const entry = resolveType('list');
		expect(entry.formKind).toBe('list');
		expect(entry.featureFlag).toBe('list.enabled');
		expect(entry.requiredPermission).toBe('list.create');
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

describe('createAvailableTypes', () => {
	const all = [
		{ value: 'discussion', label: 'Discussion' },
		{ value: 'blog_post', label: 'Blog Post' },
		{ value: 'page', label: 'Page' },
		{ value: 'comparison', label: 'Comparison' },
		{ value: 'guide', label: 'Guide' },
		{ value: 'story', label: 'Story' },
		{ value: 'video', label: 'Video' },
		{ value: 'list', label: 'List' },
		{ value: 'news', label: 'News' }
	];
	const listCaller = { permissions: ['list.create'] };

	it('exposes only the allowed /create types', () => {
		const vals = createAvailableTypes(
			all,
			{ 'feature.blog': true, 'feature.comparison': true, 'video.enabled': true },
			undefined,
			listCaller
		).map((t) => t.value);
		expect(vals).toEqual(CREATE_ALLOWED_TYPES);
	});

	it('excludes discussion, page, and unrecognised types', () => {
		const vals = createAvailableTypes(all, { 'video.enabled': true }).map((t) => t.value);
		expect(vals).not.toContain('discussion');
		expect(vals).not.toContain('page');
		expect(vals).not.toContain('question');
	});

	it('follows the CREATE_ALLOWED_TYPES order', () => {
		const vals = createAvailableTypes(
			all,
			{ 'feature.blog': true, 'feature.comparison': true, 'video.enabled': true },
			undefined,
			listCaller
		).map((t) => t.value);
		expect(vals).toEqual(['blog_post', 'comparison', 'guide', 'story', 'video', 'list', 'news']);
	});

	it('hides comparison when feature.comparison is off', () => {
		const vals = createAvailableTypes(
			all,
			{ 'feature.blog': true, 'feature.comparison': false, 'video.enabled': true },
			undefined,
			listCaller
		).map((t) => t.value);
		expect(vals).toEqual(['blog_post', 'guide', 'story', 'video', 'list', 'news']);
	});

	it('hides video when video.enabled is off', () => {
		const vals = createAvailableTypes(all, { 'feature.blog': true, 'feature.comparison': true, 'video.enabled': false }).map(
			(t) => t.value
		);
		expect(vals).not.toContain('video');
	});

	it('hides video when the video.enabled public setting is false', () => {
		const vals = createAvailableTypes(
			all,
			{ 'feature.blog': true, 'feature.comparison': true },
			{ 'video.enabled': false }
		).map((t) => t.value);
		expect(vals).not.toContain('video');
	});

	it('shows video when the video.enabled public setting is true', () => {
		const vals = createAvailableTypes(
			all,
			{ 'feature.blog': true, 'feature.comparison': true },
			{ 'video.enabled': true }
		).map((t) => t.value);
		expect(vals).toContain('video');
	});

	it('maps story to the standard composer and its feature flag', () => {
		expect(resolveType('story').formKind).toBe('standard');
		// Canonical flag key — matches routeFeatureFlags (/stories) and the
		// backend service gate.
		expect(resolveType('story').featureFlag).toBe('story.enabled');
	});

	it('maps video to the standard composer and its feature flag', () => {
		expect(resolveType('video').formKind).toBe('standard');
		expect(resolveType('video').featureFlag).toBe('video.enabled');
	});

	it('includes list when the caller holds the list.create node', () => {
		const vals = createAvailableTypes(all, {}, undefined, listCaller).map((t) => t.value);
		expect(vals).toContain('list');
	});

	it('hides list when the caller lacks the list.create node', () => {
		const vals = createAvailableTypes(all, {}, undefined, { permissions: ['blog.create'] }).map(
			(t) => t.value
		);
		expect(vals).not.toContain('list');
		// No caller at all behaves the same.
		expect(createAvailableTypes(all, {}).map((t) => t.value)).not.toContain('list');
	});

	it('shows list to a super admin without the explicit node', () => {
		const vals = createAvailableTypes(all, {}, undefined, { isSuperAdmin: true }).map((t) => t.value);
		expect(vals).toContain('list');
	});

	it('hides list when the list.enabled public setting is false, even with permission', () => {
		const vals = createAvailableTypes(all, {}, { 'list.enabled': false }, listCaller).map(
			(t) => t.value
		);
		expect(vals).not.toContain('list');
	});
});
