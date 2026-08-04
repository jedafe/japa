import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected')),
	botsAdminApiExtended: {
		list: vi.fn().mockRejectedValue(new Error('not connected')),
		create: vi.fn().mockRejectedValue(new Error('not connected')),
		update: vi.fn().mockRejectedValue(new Error('not connected')),
		delete: vi.fn().mockRejectedValue(new Error('not connected')),
	}
}));

vi.mock('$lib/stores/permissions.svelte', () => ({
	permissions: [],
	hasPermission: vi.fn().mockReturnValue(true)
}));

describe('Bot Management +page.svelte', () => {
	it('renders the Bot Management heading', () => {
		render(Page);
		expect(screen.getByText('Bots')).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Manage chatbots, moderation bots, and automated agents')).toBeTruthy();
	});

	it('shows Add Bot button', () => {
		render(Page);
		expect(screen.getByText('Add Bot')).toBeTruthy();
	});

	it('shows empty state when no bots', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No bots configured')).toBeTruthy();
		});
	});
});
