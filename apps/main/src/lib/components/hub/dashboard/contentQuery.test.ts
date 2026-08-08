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
	it('adds a content-type filter and page offset', () => {
		expect(buildContentQuery({ type: 'blog_post', pageOffset: 20 })).toBe(
			'/me/content?page_size=20&type=blog_post&page_offset=20'
		);
	});
	it('adds a search query param', () => {
		expect(buildContentQuery({ q: 'hello' })).toBe('/me/content?page_size=20&q=hello');
		expect(buildContentQuery({ status: 'draft', q: 'hello' })).toBe(
			'/me/drafts?page_size=20&q=hello'
		);
	});
});
