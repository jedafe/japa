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

const defaultData = {
	space: { id: '1', name: 'Test Space', slug: 'test-space', description: 'A test space' },
	feed: [],
	hasMore: false
};

describe('Feed Space +page.svelte', () => {
	it('renders the New post button', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('New post')).toBeTruthy();
	});

	it('shows empty state when feed is empty', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('No posts yet')).toBeTruthy();
		});
	});

	it('shows the About link', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('About')).toBeTruthy();
	});
});
