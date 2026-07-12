import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

const mocks = vi.hoisted(() => ({
	listSpaces: vi.fn()
}));

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: { listSpaces: mocks.listSpaces }
}));

describe('Spaces +page.svelte', () => {
	beforeEach(() => {
		mocks.listSpaces.mockResolvedValue([]);
	});
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

	it('derives filter buttons from the loaded space types', async () => {
		mocks.listSpaces.mockResolvedValue([
			{ id: '1', name: 'Nigeria', slug: 'nigeria', description: '', space_type: 'country', member_count: 1 },
			{ id: '2', name: 'Tech', slug: 'tech', description: '', space_type: 'interest', member_count: 1 },
			{ id: '3', name: 'Nurses', slug: 'nurses', description: '', space_type: 'profession', member_count: 1 },
			{ id: '4', name: 'Oxford', slug: 'oxford', description: '', space_type: 'university', member_count: 1 }
		]);
		render(Page);
		const filters = ['All Spaces', 'Countries', 'Interests', 'Professions', 'Universities'];
		for (const label of filters) {
			await waitFor(() => {
				expect(screen.getByText(label)).toBeTruthy();
			});
		}
	});

	it('shows empty state after load with no spaces', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No spaces found')).toBeTruthy();
		});
	});
});
