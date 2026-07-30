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

	it('shows all category tabs after load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('General')).toBeTruthy();
		});
		// Only categories still present in settingCategoryOrder (features, email,
		// integrations, advanced, payment have been removed).
		const tabs = ['Appearance', 'Users', 'Content', 'Media', 'Seo', 'Security', 'Ai'];
		for (const tab of tabs) {
			expect(screen.getByText(tab)).toBeTruthy();
		}
	});

	it('shows empty state when no settings loaded', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Settings API not implemented yet. This section will be available when the backend settings endpoint is ready.')).toBeTruthy();
		});
	});
});
