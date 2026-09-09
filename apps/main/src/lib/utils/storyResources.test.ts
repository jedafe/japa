import { describe, it, expect } from 'vitest';
import { parseMostUsefulResource } from './storyResources';

describe('parseMostUsefulResource', () => {
	it('returns an empty array for nullish values', () => {
		expect(parseMostUsefulResource(null)).toEqual([]);
		expect(parseMostUsefulResource(undefined)).toEqual([]);
		expect(parseMostUsefulResource('')).toEqual([]);
	});

	it('splits name — link pairs joined by semicolons', () => {
		const raw = 'Mexico temporary resident visa guide — https://embamex.sre.gob.mx; CDMX expat community — https://example.org/cdmx-expats';
		expect(parseMostUsefulResource(raw)).toEqual([
			{
				name: 'Mexico temporary resident visa guide',
				url: 'https://embamex.sre.gob.mx'
			},
			{ name: 'CDMX expat community', url: 'https://example.org/cdmx-expats' }
		]);
	});

	it('prettifies bare domains with https://', () => {
		expect(parseMostUsefulResource('example.com')).toEqual([
			{ name: 'example.com', url: 'https://example.com' }
		]);
	});

	it('keeps fully qualified URLs as-is', () => {
		expect(parseMostUsefulResource('https://example.org; mailto:test@example.org; /relative-path')).toEqual([
			{ name: 'https://example.org', url: 'https://example.org' },
			{ name: 'mailto:test@example.org', url: 'mailto:test@example.org' },
			{ name: '/relative-path', url: '/relative-path' }
		]);
	});

	it('treats plain names without URLs as text-only entries', () => {
		expect(parseMostUsefulResource('A helpful friend; Another community')).toEqual([
			{ name: 'A helpful friend', url: '' },
			{ name: 'Another community', url: '' }
		]);
	});
});
