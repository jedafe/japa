import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Media Library +page.svelte', () => {
	const defaultData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {},
		media: []
	};

	it('renders the Media Library heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Media Library' })).toBeTruthy();
	});

	it('shows the Upload button', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Upload')).toBeTruthy();
	});

	it('shows empty state when no media', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('No media yet')).toBeTruthy();
		});
	});
});
