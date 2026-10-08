import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import BookingDialog from './BookingDialog.svelte';
import type { ServiceListing } from '$lib/types';

vi.mock('$app/state', () => ({
	page: {
		url: new URL('http://localhost/services/test-pkg'),
		data: { features: { 'service_coupons.enabled': true } }
	}
}));

const getMock = vi.fn().mockResolvedValue({ data: [], error: null, status: 200 });
const postMock = vi.fn();
vi.mock('$lib/api/client', () => ({
	api: {
		get: (...args: unknown[]) => getMock(...args),
		post: (...args: unknown[]) => postMock(...args)
	}
}));

const tiers = [
	{
		id: 'tier-basic',
		name: 'Basic',
		badge: '',
		price_amount: 500000,
		price_currency: 'NGN',
		features: [],
		position: 0,
		is_featured: false
	},
	{
		id: 'tier-pro',
		name: 'Pro',
		badge: 'Popular',
		price_amount: 900000,
		price_currency: 'NGN',
		features: [],
		position: 1,
		is_featured: true
	}
];

const paidService = {
	id: 'svc-1',
	slug: 'test-pkg',
	title: 'Test Package',
	listing_kind: 'package',
	purchase_mode: 'paid',
	price_currency: 'NGN',
	org_id: 'org-1',
	tiers,
	departures: []
} as unknown as ServiceListing;

const inquiryService = {
	...paidService,
	purchase_mode: 'inquiry'
} as unknown as ServiceListing;

describe('BookingDialog', () => {
	beforeEach(() => {
		getMock.mockReset().mockResolvedValue({ data: [], error: null, status: 200 });
		postMock.mockReset();
	});

	it('shows a login prompt for guests instead of the form', () => {
		const { baseElement } = render(BookingDialog, {
			props: { service: inquiryService, currencies: [], authed: false, open: true }
		});
		const link = baseElement.querySelector('a[href^="/login?redirect="]');
		expect(link).toBeTruthy();
		expect(link?.getAttribute('href')).toContain(encodeURIComponent('/services/test-pkg'));
	});

	it('renders the tier picker with the initialTierId tier preselected', () => {
		const { baseElement } = render(BookingDialog, {
			props: {
				service: paidService,
				currencies: [],
				authed: true,
				initialTierId: 'tier-pro',
				open: true
			}
		});
		const radios = baseElement.querySelectorAll('input[name="bk-tier"]');
		expect(radios.length).toBe(2);
		expect((radios[1] as HTMLInputElement).checked).toBe(true);
	});

	it('maps preview error_code to the coupon error line', async () => {
		getMock.mockImplementation((path: string) => {
			if (path.includes('price-preview')) {
				return Promise.resolve({
					data: { coupon_valid: false, error_code: 'coupon_min_spend' },
					error: null,
					status: 200
				});
			}
			return Promise.resolve({ data: [], error: null, status: 200 });
		});
		const { baseElement, getByRole, container } = render(BookingDialog, {
			props: { service: paidService, currencies: [], authed: true, open: true }
		});
		const input = container.querySelector('#bk-coupon') as HTMLInputElement;
		// The dialog portals to document.body — the coupon field lives there.
		const field = (input ?? baseElement.querySelector('#bk-coupon')) as HTMLInputElement;
		expect(field).toBeTruthy();
		await fireEvent.input(field, { target: { value: 'SAVE10' } });
		await fireEvent.click(getByRole('button', { name: /apply/i }));
		await waitFor(() => {
			expect(baseElement.textContent).toContain('Minimum spend not met');
		});
		expect(getMock).toHaveBeenCalledWith(
			expect.stringContaining('/services/svc-1/price-preview?')
		);
	});

	it('renders the totals breakdown when the coupon is valid', async () => {
		getMock.mockImplementation((path: string) => {
			if (path.includes('price-preview')) {
				return Promise.resolve({
					data: {
						base: 500000,
						discount: 50000,
						total: 450000,
						fee: 22500,
						balance_due: 427500,
						currency: 'NGN',
						coupon_valid: true
					},
					error: null,
					status: 200
				});
			}
			return Promise.resolve({ data: [], error: null, status: 200 });
		});
		const { baseElement, getByRole } = render(BookingDialog, {
			props: { service: paidService, currencies: [], authed: true, open: true }
		});
		const field = baseElement.querySelector('#bk-coupon') as HTMLInputElement;
		await fireEvent.input(field, { target: { value: 'SAVE10' } });
		await fireEvent.click(getByRole('button', { name: /apply/i }));
		await waitFor(() => {
			expect(baseElement.textContent).toContain('Coupon applied');
			expect(baseElement.textContent).toContain('Subtotal');
			expect(baseElement.textContent).toContain('Discount');
			expect(baseElement.textContent).toContain('Fee due now');
			expect(baseElement.textContent).toContain('Balance to provider');
		});
	});

	it('hides the coupon field for inquiry intent', () => {
		const { baseElement } = render(BookingDialog, {
			props: { service: inquiryService, currencies: [], authed: true, open: true }
		});
		expect(baseElement.querySelector('#bk-coupon')).toBeNull();
	});
});
