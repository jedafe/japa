import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Payments +page.svelte', () => {
	it('renders the Payments heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Payments' })).toBeTruthy();
	});

	it('shows the Payment History section', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Payment History')).toBeTruthy();
		});
	});

	it('shows empty state when no payments', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No payments yet')).toBeTruthy();
		});
	});
});
