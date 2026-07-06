import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Ad Campaigns +page.svelte', () => {
	it('renders the heading', () => {
		render(Page);
		expect(screen.getByText('Ad Campaigns')).toBeTruthy();
	});

	it('shows Create Campaign link', () => {
		render(Page);
		expect(screen.getByText('Create Campaign')).toBeTruthy();
	});

	it('shows empty state when no campaigns', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No campaigns created yet')).toBeTruthy();
		});
	});
});
