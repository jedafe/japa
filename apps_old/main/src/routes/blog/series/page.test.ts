import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Blog Series +page.svelte', () => {
	const defaultData = { user: null, flags: {}, notificationCount: 0, token: null, locale: 'en', translations: {}, series: [] };

	it('renders the Blog Series heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Blog Series' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Dive deeper with multi-part guides and stories.')).toBeTruthy();
	});

	it('shows empty state when no series exist', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('No series yet')).toBeTruthy();
	});

	it('renders Back to blog link', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText(/Back to blog/)).toBeTruthy();
	});

	it('renders series cards when series exist', () => {
		const dataWithSeries = {
			...defaultData,
			series: [
				{ id: '1', title: 'My Series', slug: 'my-series', description: 'A series', status: 'published', created_at: '2026-01-01T00:00:00Z' }
			]
		};
		render(Page, { props: { data: dataWithSeries } });
		expect(screen.getByText('My Series')).toBeTruthy();
		expect(screen.getByText('A series')).toBeTruthy();
	});
});
