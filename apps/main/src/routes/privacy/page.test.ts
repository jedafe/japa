import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

const defaultData = {
	user: null,
	flags: {},
	notificationCount: 0,
	token: null,
	locale: 'en',
	translations: {},
	isAuthenticated: false,
	consents: [],
	sources: [],
	bans: null
};

describe('Privacy & Data +page.svelte', () => {
	it('renders the Privacy & Data heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Privacy & Data' })).toBeTruthy();
	});

	it('shows sign in prompt when not authenticated', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('Sign in to manage your privacy settings and data.')).toBeTruthy();
		});
	});

	it('renders the Sign in link when not authenticated', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('Sign in')).toBeTruthy();
		});
	});
});
