import { describe, it, expect } from 'vitest';
import { fieldCurrencyCode, formatPriceRange, formatWikiFieldValue, mediaFieldItems, priceRangeParts } from './wikiFieldValue';
import type { CurrencyInfo } from './currency';

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

	// §58 — when a currency_code + the managed currency list are given,
	// bounds render with the managed symbol.
	const ngn: CurrencyInfo[] = [{ code: 'NGN', name: 'Nigerian Naira', symbol: '₦' }];

	it('prefixes the managed symbol when currency context is known', () => {
		expect(formatPriceRange({ min: 800, max: 1600 }, 'NGN', ngn)).toBe('₦800 – ₦1,600');
		expect(formatPriceRange({ avg: 1200 }, 'NGN', ngn)).toBe('₦1,200');
	});

	it('falls back to "CODE n" when the currency list is empty', () => {
		expect(formatPriceRange({ avg: 1200 }, 'ZZZ', [])).toBe('ZZZ 1,200');
	});

	it('keeps bare numbers when no currency_code is bound', () => {
		expect(formatPriceRange({ avg: 1200 })).toBe('1,200');
	});
});

describe('fieldCurrencyCode', () => {
	it('extracts {currency_code} from object options', () => {
		expect(fieldCurrencyCode({ currency_code: 'USD' })).toBe('USD');
	});

	it('returns empty for arrays, null, and missing keys', () => {
		expect(fieldCurrencyCode([{ value: 'USD' }])).toBe('');
		expect(fieldCurrencyCode(null)).toBe('');
		expect(fieldCurrencyCode({})).toBe('');
	});
});

describe('formatWikiFieldValue', () => {
	const ngn = [{ code: 'NGN', name: 'Nigerian Naira', symbol: '₦' }];

	it('formats a currency field with its bound code + managed symbol', () => {
		expect(
			formatWikiFieldValue(
				{ id: 'f1', key: 'rent', label: 'Rent', field_type: 'currency', value: 1200, options: { currency_code: 'NGN' }, can_edit: true, visible: true },
				ngn
			)
		).toBe('₦1,200');
	});

	// §58 — country-inherited denomination: a currency field with no
	// options.currency_code adopts the entity's resolved country currency.
	it('falls back to the context currency when the field has no currency_code', () => {
		expect(
			formatWikiFieldValue(
				{ id: 'f1', key: 'rent', label: 'Rent', field_type: 'currency', value: 1200, can_edit: true, visible: true },
				ngn,
				'NGN'
			)
		).toBe('₦1,200');
	});

	it('lets an explicit field currency_code override the context currency', () => {
		const list = [
			{ code: 'NGN', name: 'Nigerian Naira', symbol: '₦' },
			{ code: 'USD', name: 'US Dollar', symbol: '$' }
		];
		expect(
			formatWikiFieldValue(
				{ id: 'f1', key: 'rent', label: 'Rent', field_type: 'currency', value: 1200, options: { currency_code: 'USD' }, can_edit: true, visible: true },
				list,
				'NGN'
			)
		).toBe('$1,200');
	});

	it('renders a bare number when neither field option nor context resolves', () => {
		expect(
			formatWikiFieldValue(
				{ id: 'f1', key: 'rent', label: 'Rent', field_type: 'currency', value: 1200, can_edit: true, visible: true },
				ngn,
				''
			)
		).toBe('1,200');
	});

	it('inherits the context currency for price_range fields', () => {
		expect(
			formatWikiFieldValue(
				{ id: 'f1', key: 'rent', label: 'Rent', field_type: 'price_range', value: { min: 800, max: 1600 }, can_edit: true, visible: true },
				ngn,
				'NGN'
			)
		).toBe('₦800 – ₦1,600');
	});

	// §80 — the entity-fields DTO emits the canonical select/multiselect
	// shape, but values written by other paths (cfv upsert, attraction
	// enrichment) may still carry a {"value"/"values"} envelope; the
	// formatter unwraps so option labels resolve instead of raw JSON.
	const kindField = (value: unknown) => ({
		id: 'f1',
		key: 'kind',
		label: 'Kind',
		field_type: 'select',
		value,
		options: [{ value: 'museum', label: 'Museum' }],
		can_edit: true,
		visible: true
	});

	it('resolves option labels for bare and wrapped select values', () => {
		expect(formatWikiFieldValue(kindField('museum'))).toBe('Museum');
		expect(formatWikiFieldValue(kindField({ value: 'museum' }))).toBe('Museum');
	});

	it('unwraps {values:[…]} envelopes on multiselect fields', () => {
		const f = {
			id: 'f1',
			key: 'tags',
			label: 'Tags',
			field_type: 'multiselect',
			value: { values: ['a', 'b'] },
			options: [
				{ value: 'a', label: 'Alpha' },
				{ value: 'b', label: 'Beta' }
			],
			can_edit: true,
			visible: true
		};
		expect(formatWikiFieldValue(f)).toBe('Alpha, Beta');
	});
});

// §80 attractions — resolved media field values.
describe('mediaFieldItems', () => {
	it('unwraps the single-object shape (image_single)', () => {
		expect(mediaFieldItems({ id: 'm1', url: 'https://x/img.webp', alt: 'Falls' })).toEqual([
			{ id: 'm1', url: 'https://x/img.webp', alt: 'Falls' }
		]);
	});

	it('unwraps the array shape (image_multi/media)', () => {
		const items = mediaFieldItems([
			{ id: 'm1', url: 'https://x/a.webp' },
			{ id: 'm2', url: 'https://x/b.webp', alt: 'B' }
		]);
		expect(items).toHaveLength(2);
		expect(items[1].alt).toBe('B');
	});

	it('returns [] for unresolved/raw values', () => {
		expect(mediaFieldItems('d9c9…')).toEqual([]);
		expect(mediaFieldItems(['d9c9…'])).toEqual([]);
		expect(mediaFieldItems({ value: 'd9c9…' })).toEqual([]);
		expect(mediaFieldItems(null)).toEqual([]);
	});
});
