import { describe, it, expect } from 'vitest';
import { searchPath, mapUserHits, mapContentHits, mapTagHits } from './search';

describe('searchPath', () => {
	it('returns null below the 2-char minimum', () => {
		expect(searchPath('users', 'a')).toBeNull();
		expect(searchPath('users', '')).toBeNull();
	});
	it('builds an encoded /search path for users and content', () => {
		expect(searchPath('users', 'jane d')).toBe('/search?type=user&q=jane%20d');
		expect(searchPath('content', 'uk unis')).toBe('/search?type=content&q=uk%20unis');
	});
	it('builds a /tags path for tag search', () => {
		expect(searchPath('tags', 'visa')).toBe('/tags?q=visa');
	});
});

describe('hit mappers', () => {
	it('maps user hits preferring excerpt (display_name) over title (username)', () => {
		const raw = [{ id: 'u1', title: 'jane', excerpt: 'Jane D', url: '/users/jane' }];
		expect(mapUserHits(raw)).toEqual([
			{ id: 'u1', label: 'Jane D', handle: 'jane', avatar_url: null }
		]);
	});
	it('maps content hits extracting slug from url', () => {
		const raw = [{ id: 'c1', title: 'UK Unis', url: '/blog/uk-unis' }];
		expect(mapContentHits(raw)).toEqual([{ id: 'c1', label: 'UK Unis', slug: 'uk-unis' }]);
	});
	it('maps tag hits to {id,label,slug}', () => {
		const raw = [{ id: 't1', name: 'visa', slug: 'visa' }];
		expect(mapTagHits(raw)).toEqual([{ id: 't1', label: 'visa', slug: 'visa' }]);
	});
	it('returns [] for non-array input', () => {
		expect(mapUserHits(null)).toEqual([]);
		expect(mapContentHits({})).toEqual([]);
	});
});
