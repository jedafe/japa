import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

const defaultData = { isAuthenticated: false, summary: null };

describe('Dashboard +page.svelte', () => {
	it('renders the Dashboard heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Dashboard' })).toBeTruthy();
	});

	it('shows sign in prompt when not authenticated', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('Sign in to view your analytics dashboard.')).toBeTruthy();
		});
	});

	it('renders the Sign in link when not authenticated', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('Sign in')).toBeTruthy();
		});
	});
});
