import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected')),
	settingsAdminApi: {
		list: vi.fn().mockRejectedValue(new Error('not connected')),
		updateCategory: vi.fn().mockRejectedValue(new Error('not connected')),
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	beforeNavigate: vi.fn()
}));

describe('Settings +page.svelte', () => {
	it('renders the settings heading', () => {
		render(Page);
		expect(screen.getByText('Settings')).toBeTruthy();
	});

	it('shows loading state initially', () => {
		render(Page);
		expect(screen.getByText('Loading settings...')).toBeTruthy();
	});

	it('shows error state when API fails', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('not connected')).toBeTruthy();
		});
	});
});
