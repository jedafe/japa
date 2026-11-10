import { describe, it, expect } from 'vitest';
import { feedPath, resolveTab, resolveSort, PAGE_SIZE } from './endpoint';

describe('feedPath', () => {
	it('maps tabs to bare backend paths (no /api/v1)', () => {
		expect(feedPath('following', 'relevant', 20, 0)).toBe('/feed?sort=relevant&page_size=20&page_offset=0');
		expect(feedPath('for_you', 'relevant', 20, 0)).toBe('/feed/for-you?sort=relevant&page_size=20&page_offset=0');
		expect(feedPath('explore', 'relevant', 20, 0)).toBe('/explore?sort=relevant&page_size=20&page_offset=0');
		// The Thread tab lists discussions via the same /explore feed /thread does.
		expect(feedPath('thread', 'latest', 20, 0)).toBe('/explore?sort=latest&page_size=20&page_offset=0');
	});

	it('uses page_size/page_offset, never limit', () => {
		const p = feedPath('for_you', 'top', 20, 40);
		expect(p).toContain('page_size=20');
		expect(p).toContain('page_offset=40');
		expect(p).not.toContain('limit=');
	});

	it('carries the raw sort value including top for Popular', () => {
		expect(feedPath('for_you', 'top', 20, 0)).toContain('sort=top');
	});
});

describe('resolveTab', () => {
	it('forces guests to explore', () => {
		expect(resolveTab('for_you', false)).toBe('explore');
		expect(resolveTab(null, false)).toBe('explore');
	});
	it('defaults authed users to for_you and accepts valid tabs', () => {
		expect(resolveTab(null, true)).toBe('for_you');
		expect(resolveTab('following', true)).toBe('following');
		expect(resolveTab('for_you', true)).toBe('for_you');
		expect(resolveTab('thread', true)).toBe('thread');
		expect(resolveTab('explore', true)).toBe('for_you'); // authed can't select explore
		expect(resolveTab('garbage', true)).toBe('for_you');
	});
	it('never lets guests select the thread tab', () => {
		expect(resolveTab('thread', false)).toBe('explore');
	});
});

describe('resolveSort', () => {
	it('accepts the four valid values on any tab', () => {
		for (const s of ['relevant', 'latest', 'top', 'rising'] as const) {
			expect(resolveSort(s, 'for_you')).toBe(s);
			expect(resolveSort(s, 'following')).toBe(s);
			expect(resolveSort(s, 'explore')).toBe(s);
		}
	});
	it('defaults For You/Following to latest and explore to relevant', () => {
		expect(resolveSort(null, 'for_you')).toBe('latest');
		expect(resolveSort(null, 'following')).toBe('latest');
		expect(resolveSort(null, 'explore')).toBe('relevant');
		expect(resolveSort('', 'for_you')).toBe('latest');
		expect(resolveSort('popular', 'following')).toBe('latest'); // "popular" is a label, not a value
	});
	it('restricts the thread tab to latest/top like the /thread index', () => {
		expect(resolveSort('top', 'thread')).toBe('top');
		expect(resolveSort('latest', 'thread')).toBe('latest');
		expect(resolveSort(null, 'thread')).toBe('latest');
		// Carried-over feed sorts coerce instead of leaving no chip lit.
		expect(resolveSort('relevant', 'thread')).toBe('latest');
		expect(resolveSort('rising', 'thread')).toBe('latest');
	});
});

it('PAGE_SIZE is 20', () => {
	expect(PAGE_SIZE).toBe(20);
});
