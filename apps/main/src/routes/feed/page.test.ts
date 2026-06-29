import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

beforeEach(() => {
	vi.stubGlobal(
		'IntersectionObserver',
		class {
			observe() {}
			unobserve() {}
			disconnect() {}
		}
	);
});

const defaultData = { feed: [], hasMore: false };

describe('Feed +page.svelte', () => {
	it('renders the Feed heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Feed' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Stay updated with the latest from your community.')).toBeTruthy();
	});

	it('shows empty state when feed is empty', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('Your feed is empty')).toBeTruthy();
		});
	});
});
