import { describe, expect, it, vi } from 'vitest';
import {
	housingPlatforms,
	housingRentTiers,
	rentTierLabel,
	isStaleVerification
} from './housing.js';

describe('housingPlatforms', () => {
	it('normalizes the §96 [{name, url, description}] shape', () => {
		const out = housingPlatforms([
			{ name: 'Zillow', url: 'https://zillow.com', description: 'Largest US marketplace' },
			{ name: 'Craigslist' }
		]);
		expect(out).toEqual([
			{ name: 'Zillow', url: 'https://zillow.com', description: 'Largest US marketplace' },
			{ name: 'Craigslist', url: undefined, description: undefined }
		]);
	});

	it('tolerates plain string entries', () => {
		expect(housingPlatforms(['ImmobilienScout24', '  '])).toEqual([
			{ name: 'ImmobilienScout24', url: undefined, description: undefined }
		]);
	});

	it('tolerates the legacy key→url object map', () => {
		expect(housingPlatforms({ Idealista: 'https://idealista.com' })).toEqual([
			{ name: 'Idealista', url: 'https://idealista.com', description: undefined }
		]);
	});

	it('drops empty/malformed entries without failing', () => {
		expect(housingPlatforms([{ name: '' }, null, 42])).toEqual([]);
		expect(housingPlatforms(undefined)).toEqual([]);
		expect(housingPlatforms('not json')).toEqual([]);
	});
});

describe('housingRentTiers', () => {
	it('normalizes the §96 {tier: {studio, 1bed, 2bed, currency}} shape', () => {
		const out = housingRentTiers({
			city_centre: { studio: '€900–1,200', '1bed': '€1,200–1,800', '2bed': '€1,700–2,400', currency: 'EUR' },
			outer: { '1bed': '€700–1,000', currency: 'EUR' }
		});
		expect(out).toEqual([
			{ tier: 'city_centre', studio: '€900–1,200', oneBed: '€1,200–1,800', twoBed: '€1,700–2,400', currency: 'EUR', raw: undefined },
			{ tier: 'outer', studio: undefined, oneBed: '€700–1,000', twoBed: undefined, currency: 'EUR', raw: undefined }
		]);
	});

	it('keeps scalar community-overlay values as raw rows', () => {
		const out = housingRentTiers({ studio: '€400–700', bad: { not: 'rent' }, empty: {} });
		expect(out).toEqual([
			{ tier: 'studio', raw: '€400–700', studio: undefined, oneBed: undefined, twoBed: undefined, currency: undefined }
		]);
	});

	it('returns [] for non-object input', () => {
		expect(housingRentTiers(null)).toEqual([]);
		expect(housingRentTiers([1, 2])).toEqual([]);
		expect(housingRentTiers('x')).toEqual([]);
	});
});

describe('rentTierLabel', () => {
	it('maps known tiers and humanizes unknown keys', () => {
		expect(rentTierLabel('city_centre')).toBe('City centre');
		expect(rentTierLabel('outer')).toBe('Outer districts');
		expect(rentTierLabel('near_campus')).toBe('near campus');
	});
});

describe('isStaleVerification (ADR-104 90-day threshold)', () => {
	it('is false for a recent date', () => {
		expect(isStaleVerification(new Date().toISOString())).toBe(false);
	});

	it('is true for a date older than 90 days', () => {
		const old = new Date(Date.now() - 100 * 24 * 60 * 60 * 1000).toISOString();
		expect(isStaleVerification(old)).toBe(true);
	});

	it('is false for missing/unparseable dates', () => {
		expect(isStaleVerification(null)).toBe(false);
		expect(isStaleVerification(undefined)).toBe(false);
		expect(isStaleVerification('not-a-date')).toBe(false);
	});

	it('respects the 90-day boundary', () => {
		vi.useFakeTimers();
		try {
			vi.setSystemTime(new Date('2026-06-01T00:00:00Z'));
			expect(isStaleVerification('2026-03-04')).toBe(false); // 89 days
			expect(isStaleVerification('2026-03-02')).toBe(true); // 91 days
		} finally {
			vi.useRealTimers();
		}
	});
});
