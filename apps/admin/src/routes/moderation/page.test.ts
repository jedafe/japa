import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected')),
	moderationAdminApi: {
		getQueue: vi.fn().mockRejectedValue(new Error('not connected')),
		updateItem: vi.fn().mockRejectedValue(new Error('not connected')),
		bulkAction: vi.fn().mockRejectedValue(new Error('not connected')),
	}
}));

describe('Moderation +page.svelte', () => {
	it('renders the moderation heading', () => {
		render(Page);
		expect(screen.getByText('Content Moderation')).toBeTruthy();
	});

	it('shows filter controls', () => {
		render(Page);
		expect(screen.getByText('Apply')).toBeTruthy();
	});

	it('shows empty state when no items', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
