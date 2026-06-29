import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Wallet +page.svelte', () => {
	it('renders the Wallet heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Wallet' })).toBeTruthy();
	});

	it('shows the Points label', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Points')).toBeTruthy();
		});
	});

	it('shows the Credits label', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Credits')).toBeTruthy();
		});
	});
});
