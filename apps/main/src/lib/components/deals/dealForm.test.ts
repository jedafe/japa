import { describe, it, expect } from 'vitest';
import {
	buildCouponPayload,
	emptyDealFormValues,
	emptyDetails,
	detailsFromCoupon,
	couponToFormValues,
	toDateTimeLocal,
	type DealFormValues
} from './dealForm';
import type { Coupon } from '$lib/types';

function baseValues(overrides: Partial<DealFormValues> = {}): DealFormValues {
	return { ...emptyDealFormValues(), title: '10% off luggage', ...overrides };
}

describe('buildCouponPayload', () => {
	it('emits the required staff fields and defaults coupon_type=external', () => {
		const payload = buildCouponPayload(baseValues(), 'staff');
		expect(payload.title).toBe('10% off luggage');
		expect(payload.coupon_type).toBe('external');
		expect(payload.status).toBe('active');
		expect(payload.is_verified).toBe(false);
	});

	it('omits details entirely when all detail fields are empty', () => {
		const payload = buildCouponPayload(baseValues(), 'staff');
		expect('details' in payload).toBe(false);
	});

	it('emits only the populated detail parts', () => {
		const v = baseValues();
		v.details.summary = '  Great offer  ';
		const payload = buildCouponPayload(v, 'staff');
		expect(payload.details).toEqual({ summary: 'Great offer' });
		expect((payload.details as Record<string, unknown>).sections).toBeUndefined();
		expect((payload.details as Record<string, unknown>).faq).toBeUndefined();
		expect((payload.details as Record<string, unknown>).trial).toBeUndefined();
	});

	it('nests populated sections, faq, and trial under details', () => {
		const v = baseValues();
		v.details.sections = [
			{ title: 'How to redeem', body: 'Click the link' },
			{ title: '', body: '' }
		];
		v.details.faq = [{ q: 'Who?', a: 'Students' }];
		v.details.trial = { title: 'Free trial', body: '', url: 'https://x.example' };
		const details = buildCouponPayload(v, 'staff').details as Record<string, unknown>;
		expect(details.sections).toEqual([{ title: 'How to redeem', body: 'Click the link' }]);
		expect(details.faq).toEqual([{ q: 'Who?', a: 'Students' }]);
		expect(details.trial).toEqual({ title: 'Free trial', url: 'https://x.example' });
	});

	it('drops detail rows where every field is blank', () => {
		const v = baseValues();
		v.details.sections = [{ title: '  ', body: '' }];
		v.details.faq = [{ q: '', a: '  ' }];
		const payload = buildCouponPayload(v, 'staff');
		expect('details' in payload).toBe(false);
	});

	it('converts expires_at datetime-local to the valid_until ISO key', () => {
		const payload = buildCouponPayload(
			baseValues({ expires_at: '2026-03-01T12:30', valid_from: '2026-01-01T00:00' }),
			'staff'
		);
		expect(payload.valid_until).toBe(new Date('2026-03-01T12:30').toISOString());
		expect(payload.valid_from).toBe(new Date('2026-01-01T00:00').toISOString());
		expect('expires_at' in payload).toBe(false);
	});

	it('omits empty optional strings and numbers', () => {
		const payload = buildCouponPayload(baseValues(), 'staff');
		for (const key of [
			'code',
			'slug',
			'description',
			'discount_type',
			'discount_value',
			'currency',
			'terms_url',
			'referral_url',
			'valid_from',
			'valid_until',
			'provider_id'
		]) {
			expect(key in payload).toBe(false);
		}
	});

	it('emits code, slug, urls, and discount fields when set', () => {
		const payload = buildCouponPayload(
			baseValues({
				code: 'SAVE10',
				slug: 'save-10',
				referral_url: 'https://partner.example/go',
				terms_url: 'https://partner.example/terms',
				description: 'A deal',
				discount_type: 'percent',
				discount_value: '10.5',
				currency: 'USD',
				provider_id: '11111111-1111-1111-1111-111111111111',
				is_verified: true
			}),
			'staff'
		);
		expect(payload.code).toBe('SAVE10');
		expect(payload.slug).toBe('save-10');
		expect(payload.referral_url).toBe('https://partner.example/go');
		expect(payload.terms_url).toBe('https://partner.example/terms');
		expect(payload.description).toBe('A deal');
		expect(payload.discount_type).toBe('percent');
		expect(payload.discount_value).toBe(10.5);
		expect(payload.currency).toBe('USD');
		expect(payload.provider_id).toBe('11111111-1111-1111-1111-111111111111');
		expect(payload.is_verified).toBe(true);
	});

	it('skips a non-numeric discount_value', () => {
		const payload = buildCouponPayload(baseValues({ discount_value: 'abc' }), 'staff');
		expect('discount_value' in payload).toBe(false);
	});

	it('strips staff-only keys for the org audience', () => {
		const payload = buildCouponPayload(
			baseValues({
				provider_id: '11111111-1111-1111-1111-111111111111',
				status: 'suspended',
				is_verified: true,
				code: 'X'
			}),
			'org'
		);
		expect('provider_id' in payload).toBe(false);
		expect('status' in payload).toBe(false);
		expect('is_verified' in payload).toBe(false);
		expect(payload.code).toBe('X');
	});
});

describe('emptyDetails', () => {
	it('returns a blank details editor state', () => {
		expect(emptyDetails()).toEqual({
			summary: '',
			trial: { title: '', body: '', url: '' },
			sections: [],
			faq: []
		});
	});
});

describe('detailsFromCoupon', () => {
	it('maps coupon.details back into editor values', () => {
		const coupon = {
			details: {
				summary: 'S',
				trial: { title: 'T', url: 'https://x' },
				sections: [{ title: 'Sec', body: 'B' }],
				faq: [{ q: 'Q', a: 'A' }]
			}
		} as Coupon;
		expect(detailsFromCoupon(coupon)).toEqual({
			summary: 'S',
			trial: { title: 'T', body: '', url: 'https://x' },
			sections: [{ title: 'Sec', body: 'B' }],
			faq: [{ q: 'Q', a: 'A' }]
		});
	});

	it('returns blanks when details are missing', () => {
		expect(detailsFromCoupon({} as Coupon)).toEqual(emptyDetails());
	});
});

describe('couponToFormValues', () => {
	it('seeds edit-mode values from a coupon', () => {
		const coupon = {
			id: 'c1',
			provider_id: 'p1',
			title: 'Deal',
			slug: 'deal',
			code: 'C1',
			status: 'suspended',
			description: 'D',
			discount_type: 'percent',
			discount_value: '12.50',
			currency: 'GBP',
			terms_url: 'https://t',
			referral_url: 'https://r',
			is_verified: true,
			valid_from: '2026-01-15T10:00:00Z',
			expires_at: '2026-02-15T10:00:00Z',
			details: { summary: 'Hi' }
		} as unknown as Coupon;
		const v = couponToFormValues(coupon);
		expect(v.provider_id).toBe('p1');
		expect(v.status).toBe('suspended');
		expect(v.is_verified).toBe(true);
		expect(v.discount_value).toBe('12.50');
		expect(v.valid_from).toBe(toDateTimeLocal('2026-01-15T10:00:00Z'));
		expect(v.expires_at).toBe(toDateTimeLocal('2026-02-15T10:00:00Z'));
		expect(v.details.summary).toBe('Hi');
	});
});

describe('toDateTimeLocal', () => {
	it('returns empty for missing input and formats ISO to datetime-local', () => {
		expect(toDateTimeLocal(null)).toBe('');
		expect(toDateTimeLocal(undefined)).toBe('');
		expect(toDateTimeLocal('not-a-date')).toBe('');
		expect(toDateTimeLocal('2026-02-15T10:00:00Z')).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);
	});
});
