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
			expect(screen.getByText('Open Tickets')).toBeTruthy();
		});
	});

	it('shows platform overview section after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Platform Overview')).toBeTruthy();
		});
	});

	it('shows support queue section after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Support Queue')).toBeTruthy();
		});
	});
});
