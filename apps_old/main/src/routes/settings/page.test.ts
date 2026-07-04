import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Settings +page.svelte', () => {
	it('renders the Settings heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Settings' })).toBeTruthy();
	});

	it('shows Profile Settings tab content by default', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Profile Settings')).toBeTruthy();
		});
	});

	it('shows the Display Name label', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Display Name')).toBeTruthy();
		});
	});

	it('shows the Bio label', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Bio')).toBeTruthy();
		});
	});

	it('shows the Location label', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Location')).toBeTruthy();
		});
	});

	it('shows the Save changes button', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Save changes')).toBeTruthy();
		});
	});

	it('renders tab navigation buttons (desktop + mobile)', () => {
		render(Page);
		// Each tab label appears twice: once in the desktop sidebar and once in the mobile tab bar
		expect(screen.getAllByText('Profile').length).toBeGreaterThanOrEqual(2);
		expect(screen.getAllByText('Notifications').length).toBeGreaterThanOrEqual(2);
		expect(screen.getAllByText('Account').length).toBeGreaterThanOrEqual(2);
	});
});
