import { describe, it, expect } from 'vitest';
import { feedPath, resolveTab, resolveSort, PAGE_SIZE } from './endpoint';

describe('feedPath', () => {
	it('maps tabs to bare backend paths (no /api/v1)', () => {
		expect(feedPath('following', 'relevant', 20, 0)).toBe('/feed?sort=relevant&page_size=20&page_offset=0');
		expect(feedPath('for_you', 'relevant', 20, 0)).toBe('/feed/for-you?sort=relevant&page_size=20&page_offset=0');
		expect(feedPath('explore', 'relevant', 20, 0)).toBe('/explore?sort=relevant&page_size=20&page_offset=0');
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
		expect(resolveTab('explore', true)).toBe('for_you'); // authed can't select explore
		expect(resolveTab('garbage', true)).toBe('for_you');
	});
});

describe('resolveSort', () => {
	it('accepts the four valid values', () => {
		for (const s of ['relevant', 'latest', 'top', 'rising'] as const) {
			expect(resolveSort(s)).toBe(s);
		}
	});
	it('defaults invalid/absent to latest', () => {
		expect(resolveSort(null)).toBe('latest');
		expect(resolveSort('popular')).toBe('latest'); // "popular" is a label, not a value
		expect(resolveSort('')).toBe('latest');
	});
});

it('PAGE_SIZE is 20', () => {
	expect(PAGE_SIZE).toBe(20);
});
