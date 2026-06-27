import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Tickets +page.svelte', () => {
	it('renders the tickets heading', () => {
		render(Page);
		expect(screen.getByText('Ticket Queue')).toBeTruthy();
	});

	it('shows filter controls', () => {
		render(Page);
		expect(screen.getByText('Apply')).toBeTruthy();
	});

	it('shows empty state when no tickets', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No tickets found')).toBeTruthy();
		});
	});
});
