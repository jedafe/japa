import { describe, it, expect } from 'vitest';
import { buildContentQuery } from './contentQuery';

describe('buildContentQuery', () => {
	it('defaults to published content with a page size', () => {
		expect(buildContentQuery({})).toBe('/me/content?page_size=20');
	});
	it('routes drafts and scheduled to their own endpoints', () => {
		expect(buildContentQuery({ status: 'draft' })).toBe('/me/drafts?page_size=20');
		expect(buildContentQuery({ status: 'scheduled' })).toBe('/me/scheduled?page_size=20');
	});
	it('adds a content-type filter and page offset for published', () => {
		expect(buildContentQuery({ type: 'blog_post', pageOffset: 20 })).toBe(
			'/me/content?page_size=20&type=blog_post&page_offset=20'
		);
	});
	it('carries type and sort filters for drafts and scheduled', () => {
		expect(buildContentQuery({ status: 'draft', type: 'blog_post', sort: 'most_viewed', q: 'hello' })).toBe(
			'/me/drafts?page_size=20&type=blog_post&q=hello&sort=most_viewed'
		);
		expect(buildContentQuery({ status: 'scheduled', type: 'blog_post', sort: 'oldest', q: 'hello' })).toBe(
			'/me/scheduled?page_size=20&type=blog_post&q=hello&sort=oldest'
		);
	});
	it('adds a search query param', () => {
		expect(buildContentQuery({ q: 'hello' })).toBe('/me/content?page_size=20&q=hello');
		expect(buildContentQuery({ status: 'draft', q: 'hello' })).toBe(
			'/me/drafts?page_size=20&q=hello'
		);
	});
	it('adds a sort query param for published', () => {
		expect(buildContentQuery({ sort: 'most_viewed' })).toBe('/me/content?page_size=20&sort=most_viewed');
		expect(buildContentQuery({ sort: 'oldest', pageOffset: 20 })).toBe(
			'/me/content?page_size=20&sort=oldest&page_offset=20'
		);
	});
});
