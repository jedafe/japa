import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Settings +page.svelte', () => {
	it('renders the settings heading', () => {
		render(Page);
		expect(screen.getByText('Settings')).toBeTruthy();
	});

	it('shows all 13 category tabs after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('General')).toBeTruthy();
		});
		const tabs = ['Appearance', 'Features', 'Users', 'Content', 'Email', 'Payment', 'Media', 'Seo', 'Security', 'Integrations', 'Ai', 'Advanced'];
		for (const tab of tabs) {
			expect(screen.getByText(tab)).toBeTruthy();
		}
	});

	it('shows empty state when no settings loaded', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No settings available. Connect to the API to load configuration.')).toBeTruthy();
		});
	});
});
