import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Admin +page.svelte', () => {
	it('renders the dashboard heading', () => {
		render(Page);
		expect(screen.getByText('Dashboard')).toBeTruthy();
	});

	it('shows key metric labels after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Signups Today')).toBeTruthy();
		});
	});

	it('shows top spaces section after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Top Spaces')).toBeTruthy();
		});
	});

	it('shows geo distribution section after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Geo Distribution')).toBeTruthy();
		});
	});

	it('shows moderation section after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Moderation')).toBeTruthy();
		});
	});
});
