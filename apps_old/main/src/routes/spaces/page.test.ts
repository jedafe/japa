import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Spaces +page.svelte', () => {
	it('renders the Spaces heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Spaces' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Join communities that matter to you.')).toBeTruthy();
	});

	it('renders the Create space button', () => {
		render(Page);
		expect(screen.getByText('Create space')).toBeTruthy();
	});

	it('shows all filter buttons', () => {
		render(Page);
		const filters = ['All Spaces', 'Countries', 'Interests', 'Professions', 'Universities'];
		for (const label of filters) {
			expect(screen.getByText(label)).toBeTruthy();
		}
	});

	it('shows empty state after load with no spaces', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No spaces found')).toBeTruthy();
		});
	});
});
