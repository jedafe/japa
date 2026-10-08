import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import ServiceCard from './ServiceCard.svelte';
import type { ServiceListing } from '$lib/types';

const features: Record<string, boolean> = { 'feature.bookmarks': true };

vi.mock('$app/state', () => ({
	page: {
		url: new URL('http://localhost/services'),
		get data() {
			return { features };
		}
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/api/client', () => ({
	api: {
		post: vi.fn().mockResolvedValue({ data: {}, error: null, status: 200 }),
		delete: vi.fn().mockResolvedValue({ data: {}, error: null, status: 200 })
	}
}));

const getViewerStateMock = vi.fn().mockResolvedValue({ bookmarked: false });
const updateViewerStateMock = vi.fn();
vi.mock('$lib/api/viewerState', () => ({
	getViewerState: (...args: unknown[]) => getViewerStateMock(...args),
	updateViewerState: (...args: unknown[]) => updateViewerStateMock(...args)
}));

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
	} as ServiceListing;
}

describe('ServiceCard', () => {
	beforeEach(() => {
		features['feature.bookmarks'] = true;
		getViewerStateMock.mockClear();
	});

	it('renders a stretched link to the detail page with the title as accessible name', () => {
		const { getByRole } = render(ServiceCard, {
			props: { listing: listing(), currencies: [], categories: [], authed: false }
		});
		const link = getByRole('link', { name: 'Express Entry package' });
		expect(link.getAttribute('href')).toBe('/services/express-entry');
	});

	it('shows the "From" price when the listing carries a numeric price', () => {
		const { baseElement } = render(ServiceCard, {
			props: {
				listing: listing({ price_amount: 140000, price_currency: 'NGN' }),
				currencies: [{ code: 'NGN', name: 'Nigerian Naira', symbol: '₦' }],
				categories: [],
				authed: false
			}
		});
		expect(baseElement.textContent).toContain('₦1,400');
	});

	it('renders the Deal badge only when has_coupon is set', () => {
		const { baseElement } = render(ServiceCard, {
			props: {
				listing: listing({ has_coupon: true }),
				currencies: [],
				categories: [],
				authed: false
			}
		});
		expect(baseElement.textContent).toContain('Deal');
	});

	it('hides the bookmark button entirely when feature.bookmarks is off', () => {
		features['feature.bookmarks'] = false;
		const { baseElement } = render(ServiceCard, {
			props: { listing: listing(), currencies: [], categories: [], authed: true }
		});
		expect(baseElement.querySelector('button[aria-pressed]')).toBeNull();
	});

	it('renders the bookmark button with an aria-pressed state for authed users', async () => {
		const { baseElement } = render(ServiceCard, {
			props: { listing: listing(), currencies: [], categories: [], authed: true }
		});
		const btn = baseElement.querySelector('button[aria-pressed]');
		expect(btn).toBeTruthy();
		expect(btn?.getAttribute('aria-label')).toBeTruthy();
	});
});
