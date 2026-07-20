import { describe, it, expect } from 'vitest';
import { normalizeConfig, cardStyleFor, gridMode } from './cardStyle';

describe('normalizeConfig', () => {
	it('accepts the {default, by_type} object', () => {
		expect(normalizeConfig({ default: 'reddit', by_type: { blog_post: 'grid' } })).toEqual({
			default: 'reddit',
			byType: { blog_post: 'grid' }
		});
	});
	it('accepts a legacy scalar', () => {
		expect(normalizeConfig('magazine')).toEqual({ default: 'magazine', byType: {} });
	});
	it('coerces invalid default and drops invalid overrides', () => {
		expect(normalizeConfig({ default: 'bogus', by_type: { blog_post: 'grid', x: 'nope' } })).toEqual({
			default: 'reddit',
			byType: { blog_post: 'grid' }
		});
	});
	it('falls back for null/garbage', () => {
		expect(normalizeConfig(null)).toEqual({ default: 'reddit', byType: {} });
		expect(normalizeConfig(42)).toEqual({ default: 'reddit', byType: {} });
	});
});

describe('cardStyleFor', () => {
	const cfg = { default: 'reddit', byType: { blog_post: 'grid' } } as const;
	it('returns the per-type override', () => {
		expect(cardStyleFor('blog_post', cfg)).toBe('grid');
	});
	it('falls back to default for unmapped types', () => {
		expect(cardStyleFor('discussion', cfg)).toBe('reddit');
	});
});

describe('gridMode', () => {
	it('is true only when every type resolves to grid', () => {
		const allGrid = { default: 'grid', byType: {} } as const;
		expect(gridMode(['blog_post', 'discussion'], allGrid)).toBe(true);
	});
	it('is false for a mixed set', () => {
		const cfg = { default: 'reddit', byType: { blog_post: 'grid' } } as const;
		expect(gridMode(['blog_post', 'discussion'], cfg)).toBe(false);
	});
	it('is false for an empty page', () => {
		expect(gridMode([], { default: 'grid', byType: {} })).toBe(false);
	});
});
