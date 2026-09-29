import { describe, it, expect } from 'vitest';
import { formatPriceRange, priceRangeParts } from './wikiFieldValue';

// §80 Itemized Pricing — the composite {avg?,min?,max?} value must survive
// every partial shape the backend accepts (at least one bound, malformed
// extras tolerated). The SuggestEdit editor emits these objects from three
// bound inputs; this suite guards the display/read side.
describe('priceRangeParts', () => {
	it('parses a full {min,avg,max} object', () => {
		expect(priceRangeParts({ min: 1, avg: 2, max: 3 })).toEqual({ min: 1, avg: 2, max: 3 });
	});

	it('accepts partial bounds — {avg}-only and {min,max}-only', () => {
		expect(priceRangeParts({ avg: 2.29 })).toEqual({ avg: 2.29 });
		expect(priceRangeParts({ min: 800, max: 1600 })).toEqual({ min: 800, max: 1600 });
		expect(priceRangeParts({ max: 10 })).toEqual({ max: 10 });
	});

	it('rejects empty objects, non-objects, and non-finite numbers', () => {
		expect(priceRangeParts({})).toBeNull();
		expect(priceRangeParts('1,200')).toBeNull();
		expect(priceRangeParts(null)).toBeNull();
		expect(priceRangeParts([1, 2])).toBeNull();
		expect(priceRangeParts({ avg: NaN })).toBeNull();
		expect(priceRangeParts({ min: Infinity })).toBeNull();
	});
});

describe('formatPriceRange', () => {
	it('renders a lone avg as a single value', () => {
		expect(formatPriceRange({ avg: 2.29 })).toBe('2.29');
	});

	it('renders min–max as a range', () => {
		expect(formatPriceRange({ min: 800, max: 1600, avg: 1200 })).toBe('800 – 1,600');
	});

	it('renders equal bounds as a single value', () => {
		expect(formatPriceRange({ min: 5, max: 5 })).toBe('5');
	});

	it('renders a lone bound without collapsing to a range', () => {
		expect(formatPriceRange({ min: 800 })).toBe('800');
		expect(formatPriceRange({ max: 10 })).toBe('10');
	});

	it('returns the empty marker for malformed values', () => {
		expect(formatPriceRange('[object Object]')).toBe('—');
		expect(formatPriceRange({})).toBe('—');
	});
});
