import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/state', () => ({
	page: {
		params: { username: 'testuser' }
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Profile +page.svelte', () => {
	it('shows loading state initially', () => {
		render(Page);
		expect(document.querySelector('.animate-pulse')).toBeTruthy();
	});

	it('shows error state when fetch fails', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText(/Failed to load profile|User not found/i)).toBeTruthy();
		});
	});
});
