import { describe, expect, it } from 'vitest';
import { badgeFor } from './compareBadge.js';
import type { JCEComparisonSummary } from '#lib/types/jce.js';

const NOW = Date.parse('2026-11-11T00:00:00Z');
const item = (o: Partial<JCEComparisonSummary>): JCEComparisonSummary =>
	({
		id: 'x',
		title: 't',
		slug: 's',
		status: 'published',
		visibility: 'public',
		depth: 'detailed',
		...o
	}) as JCEComparisonSummary;

describe('badgeFor', () => {
	it('top-3 popularity position wins popular', () => {
		expect(badgeFor(item({ view_count: 10 }), { popularityIndex: 0, now: NOW })).toBe('popular');
		expect(badgeFor(item({ view_count: 999999 }), { popularityIndex: 2, now: NOW })).toBe(
			'popular'
		);
	});
	it('high views without a top slot is hot', () => {
		expect(badgeFor(item({ view_count: 6000 }), { popularityIndex: 5, now: NOW })).toBe('hot');
		expect(badgeFor(item({ view_count: 6000 }), { now: NOW })).toBe('hot');
	});
	it('recent comparisons are trending', () => {
		expect(
			badgeFor(item({ created_at: '2026-11-05T00:00:00Z', view_count: 3 }), { now: NOW })
		).toBe('trending');
	});
	it('old low-view items get no badge', () => {
		expect(
			badgeFor(item({ created_at: '2026-01-01T00:00:00Z', view_count: 2 }), { now: NOW })
		).toBe('');
	});
});
