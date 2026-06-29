import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Notifications +page.svelte', () => {
	it('renders the Notifications heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Notifications' })).toBeTruthy();
	});

	it('shows empty state after load with no notifications', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No notifications')).toBeTruthy();
		});
	});

	it('shows the empty state message', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText("You're all caught up! New notifications will appear here.")).toBeTruthy();
		});
	});
});
