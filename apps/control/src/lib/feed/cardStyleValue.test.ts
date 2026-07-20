import { describe, it, expect } from 'vitest';
import { parseCardStyleValue, buildCardStyleValue } from './cardStyleValue';

describe('parseCardStyleValue', () => {
	it('parses the object form', () => {
		expect(parseCardStyleValue('{"default":"reddit","by_type":{"blog_post":"grid"}}')).toEqual({
			default: 'reddit',
			byType: { blog_post: 'grid' }
		});
	});
	it('parses a legacy scalar', () => {
		expect(parseCardStyleValue('"magazine"')).toEqual({ default: 'magazine', byType: {} });
	});
	it('coerces invalid default and drops invalid overrides', () => {
		expect(parseCardStyleValue('{"default":"x","by_type":{"blog_post":"grid","d":"z"}}')).toEqual({
			default: 'reddit',
			byType: { blog_post: 'grid' }
		});
	});
	it('falls back for empty/garbage', () => {
		expect(parseCardStyleValue('')).toEqual({ default: 'reddit', byType: {} });
		expect(parseCardStyleValue('nonsense')).toEqual({ default: 'reddit', byType: {} });
	});
});

describe('buildCardStyleValue', () => {
	it('serializes default + by_type, dropping invalid entries', () => {
		expect(buildCardStyleValue('reddit', { blog_post: 'grid', d: 'bad' })).toBe(
			'{"default":"reddit","by_type":{"blog_post":"grid"}}'
		);
	});
	it('round-trips through parse', () => {
		const s = buildCardStyleValue('compact', { discussion: 'reddit' });
		expect(parseCardStyleValue(s)).toEqual({ default: 'compact', byType: { discussion: 'reddit' } });
	});
});
