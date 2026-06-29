import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Media Library +page.svelte', () => {
	it('renders the Media Library heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Media Library' })).toBeTruthy();
	});

	it('shows the Upload button', () => {
		render(Page);
		expect(screen.getByText('Upload')).toBeTruthy();
	});

	it('shows empty state when no media', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No media yet')).toBeTruthy();
		});
	});
});
