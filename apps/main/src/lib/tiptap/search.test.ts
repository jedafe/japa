import { describe, it, expect } from 'vitest';
import { searchPath, mapUserHits, mapContentHits, mapTagHits, mapLiteralHits } from './search';

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
	it('builds a /literals path and allows empty queries', () => {
		expect(searchPath('literals', '')).toBe('/literals?q=');
		expect(searchPath('literals', 'Current')).toBe('/literals?q=Current');
	});
});

describe('hit mappers', () => {
	// §54: /search returns { type, item } where item holds the type-specific
	// fields — username/display_name/avatar_url for users, title/url/slug for
	// content. The mappers unwrap `item` before reading fields.
	it('maps user hits from the {type,item} §54 shape', () => {
		const raw = [
			{
				type: 'user',
				item: { id: 'u1', url: '/users/jane', username: 'jane', display_name: 'Jane D', avatar_url: '/a.jpg' }
			}
		];
		expect(mapUserHits(raw)).toEqual([
			{ id: 'u1', label: 'Jane D', handle: 'jane', avatar_url: '/a.jpg' }
		]);
	});
	it('maps user hits falling back to username when display_name is empty', () => {
		const raw = [{ type: 'user', item: { id: 'u2', username: 'jane', display_name: '' } }];
		expect(mapUserHits(raw)).toEqual([
			{ id: 'u2', label: 'jane', handle: 'jane', avatar_url: null }
		]);
	});
	it('maps content hits from the {type,item} §54 shape', () => {
		const raw = [
			{ type: 'content', item: { id: 'c1', title: 'UK Unis', url: '/blog/uk-unis', slug: 'uk-unis' } }
		];
		expect(mapContentHits(raw)).toEqual([{ id: 'c1', label: 'UK Unis', slug: 'uk-unis' }]);
	});
	it('maps content hits extracting slug from url when slug is missing', () => {
		const raw = [{ type: 'content', item: { id: 'c2', title: 'X', url: '/blog/some-post' } }];
		expect(mapContentHits(raw)).toEqual([{ id: 'c2', label: 'X', slug: 'some-post' }]);
	});
	it('tolerates legacy flat hits', () => {
		const raw = [{ id: 'c1', title: 'UK Unis', url: '/blog/uk-unis' }];
		expect(mapContentHits(raw)).toEqual([{ id: 'c1', label: 'UK Unis', slug: 'uk-unis' }]);
	});
	it('maps tag hits to {id,label,slug}', () => {
		const raw = [{ id: 't1', name: 'visa', slug: 'visa' }];
		expect(mapTagHits(raw)).toEqual([{ id: 't1', label: 'visa', slug: 'visa' }]);
	});
	it('maps literal hits with id = token key', () => {
		const raw = [
			{ key: 'Current_Year', kind: 'expression', label: 'Current Year', is_system: true },
			{ key: 'Promo_Name', kind: 'static' }
		];
		expect(mapLiteralHits(raw)).toEqual([
			{ id: 'Current_Year', label: 'Current Year', slug: 'Current_Year' },
			{ id: 'Promo_Name', label: 'Promo_Name', slug: 'Promo_Name' }
		]);
	});
	it('returns [] for non-array input', () => {
		expect(mapUserHits(null)).toEqual([]);
		expect(mapContentHits({})).toEqual([]);
	});
});
