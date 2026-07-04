import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Bot Management +page.svelte', () => {
	it('renders the Bot Management heading', () => {
		render(Page);
		expect(screen.getByText('Bot Management')).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Manage automated bots and their messaging capabilities')).toBeTruthy();
	});

	it('shows Create Bot link', () => {
		render(Page);
		expect(screen.getByText('Create Bot')).toBeTruthy();
	});

	it('shows empty state when no bots', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No bots created yet')).toBeTruthy();
		});
	});
});
