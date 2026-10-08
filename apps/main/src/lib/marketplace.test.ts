import { describe, it, expect } from 'vitest';
import {
	accentClasses,
	discountLabel,
	priceLabel,
	serviceCategoryInfo
} from './marketplace';
import type { CurrencyInfo } from '$lib/utils/currency';
import type { ListingCoupon, ServiceCategoryInfo, ServiceListing } from '$lib/types';

// §114 services marketplace — pure display helpers shared by the list-card,
// detail hero, and sticky CTA surfaces. These guard the money/discount/
// accent contracts against the backend DTO shapes (Option, ListingCoupon,
// listing card fields).
const ngn: CurrencyInfo[] = [{ code: 'NGN', name: 'Nigerian Naira', symbol: '₦' }];
const both: CurrencyInfo[] = [
	{ code: 'NGN', name: 'Nigerian Naira', symbol: '₦' },
	{ code: 'USD', name: 'US Dollar', symbol: '$' }
];

function listing(patch: Partial<ServiceListing> = {}): ServiceListing {
	return {
		id: 'l1',
		title: 'Express Entry package',
		slug: 'express-entry',
		body: '',
		body_html: '',
		status: 'published',
		visibility: 'public',
		view_count: 0,
		comment_count: 0,
		published_at: '',
		created_at: '',
		updated_at: '',
		is_remote: false,
		is_verified: false,
		...patch
	};
}

function coupon(patch: Partial<ListingCoupon> = {}): ListingCoupon {
	return {
		id: 'c1',
		title: 'Launch deal',
		code: 'SAVE10',
		discount_type: 'percent',
		discount_value: 10,
		...patch
	};
}

describe('priceLabel', () => {
	it('prefers min_price over price_amount (the "From" card price)', () => {
		expect(
			priceLabel(
				listing({ price_amount: 140000, min_price: 90000, price_currency: 'NGN' }),
				ngn
			)
		).toBe('₦900');
	});

	it('falls back to price_amount when min_price is absent or null', () => {
		expect(priceLabel(listing({ price_amount: 140000, price_currency: 'NGN' }), ngn)).toBe(
			'₦1,400'
		);
		expect(
			priceLabel(listing({ price_amount: 140000, min_price: null, price_currency: 'NGN' }), ngn)
		).toBe('₦1,400');
	});

	it('returns null when neither price field is present', () => {
		expect(priceLabel(listing({ price_currency: 'NGN' }), ngn)).toBeNull();
		expect(priceLabel(listing({ min_price: null }), ngn)).toBeNull();
	});

	it('appends the translated per-unit suffix for package listings', () => {
		expect(
			priceLabel(
				listing({ price_amount: 140000, price_currency: 'NGN', price_per: 'person' }),
				ngn
			)
		).toBe('₦1,400 per person');
		expect(
			priceLabel(
				listing({ price_amount: 140000, price_currency: 'NGN', price_per: 'package' }),
				ngn
			)
		).toBe('₦1,400 per package');
		expect(
			priceLabel(
				listing({ price_amount: 140000, price_currency: 'NGN', price_per: 'group' }),
				ngn
			)
		).toBe('₦1,400 per group');
	});

	it('omits the suffix for empty/unknown price_per values', () => {
		expect(
			priceLabel(listing({ price_amount: 140000, price_currency: 'NGN', price_per: '' }), ngn)
		).toBe('₦1,400');
	});

	it('renders the ISO code prefix when the currency list is unknown', () => {
		expect(priceLabel(listing({ price_amount: 140000, price_currency: 'ZZZ' }), [])).toBe(
			'ZZZ 1,400'
		);
	});

	// §114: the detail payload omits min_price — a tiers-only listing
	// would render "Price on request" without the caller-derived override.
	it('uses minOverride when the listing carries no price fields', () => {
		expect(priceLabel(listing({ price_currency: 'NGN' }), ngn, 90000)).toBe('₦900');
	});

	it('minOverride wins over min_price and price_amount', () => {
		expect(
			priceLabel(
				listing({ price_amount: 140000, min_price: 120000, price_currency: 'NGN' }),
				ngn,
				90000
			)
		).toBe('₦900');
	});

	it('falls back to listing fields when minOverride is null', () => {
		expect(priceLabel(listing({ price_amount: 140000, price_currency: 'NGN' }), ngn, null)).toBe(
			'₦1,400'
		);
	});
});

describe('discountLabel', () => {
	it('renders percent discounts as -N%', () => {
		expect(discountLabel(coupon({ discount_type: 'percent', discount_value: 10 }), ngn)).toBe(
			'-10%'
		);
		expect(discountLabel(coupon({ discount_type: 'percent', discount_value: 12.5 }), ngn)).toBe(
			'-12.5%'
		);
	});

	it('renders fixed discounts as -amount in the coupon currency', () => {
		expect(
			discountLabel(
				coupon({ discount_type: 'fixed', discount_value: 25000, currency: 'NGN' }),
				ngn
			)
		).toBe('-₦250');
		expect(
			discountLabel(
				coupon({ discount_type: 'fixed', discount_value: 5000, currency: 'USD' }),
				both
			)
		).toBe('-$50');
	});

	it('returns the empty string for unknown/other discount types', () => {
		expect(discountLabel(coupon({ discount_type: 'free_shipping' }), ngn)).toBe('');
		expect(discountLabel(coupon({ discount_type: 'bogus' }), ngn)).toBe('');
		expect(discountLabel(coupon({ discount_type: '' }), ngn)).toBe('');
	});
});

describe('accentClasses', () => {
	it('maps known palette accents to chip + icon classes', () => {
		const cyan = accentClasses('cyan');
		expect(cyan.chip).toContain('bg-cyan-500/10');
		expect(cyan.chip).toContain('text-cyan-600');
		expect(cyan.icon).toBe('text-cyan-600');

		const violet = accentClasses('violet');
		expect(violet.chip).toContain('bg-violet-500/10');
		expect(violet.icon).toBe('text-violet-600');
	});

	it('covers every palette accent defined for admin category options', () => {
		for (const a of [
			'violet',
			'green',
			'amber',
			'blue',
			'sky',
			'emerald',
			'rose',
			'orange',
			'slate',
			'indigo',
			'gray',
			'cyan'
		]) {
			const cls = accentClasses(a);
			expect(cls.chip.length).toBeGreaterThan(0);
			expect(cls.icon.length).toBeGreaterThan(0);
		}
	});

	it('falls back to slate for unknown or absent accents', () => {
		const slate = accentClasses('slate');
		expect(accentClasses('hotpink')).toEqual(slate);
		expect(accentClasses(undefined)).toEqual(slate);
		expect(accentClasses('')).toEqual(slate);
	});
});

describe('serviceCategoryInfo', () => {
	const cats: ServiceCategoryInfo[] = [
		{ value: 'relocation', label: 'Relocation', icon: 'plane', accent_color: 'cyan' },
		{ value: 'finance', label: 'Finance', applies_to: 'package' }
	];

	it('resolves a category record by slug', () => {
		expect(serviceCategoryInfo(cats, 'relocation')).toEqual(cats[0]);
	});

	it('returns undefined for missing or empty slugs', () => {
		expect(serviceCategoryInfo(cats, 'health')).toBeUndefined();
		expect(serviceCategoryInfo(cats, undefined)).toBeUndefined();
		expect(serviceCategoryInfo(cats, '')).toBeUndefined();
		expect(serviceCategoryInfo([], 'relocation')).toBeUndefined();
	});
});
