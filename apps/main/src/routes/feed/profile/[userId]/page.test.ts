import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

beforeEach(() => {
	vi.stubGlobal(
		'IntersectionObserver',
		class {
			observe() {}
			unobserve() {}
			disconnect() {}
		}
	);
});

const defaultData = {
	user: null,
	flags: {},
	notificationCount: 0,
	token: null,
	locale: 'en',
	translations: {},
	profile: { id: '1', username: 'testuser', display_name: 'Test User', bio: 'Hello' },
	feed: [],
	hasMore: false
};

describe('Feed Profile +page.svelte', () => {
	it('shows empty state when feed is empty', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('No posts yet')).toBeTruthy();
		});
	});

	it('shows the Profile link', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Profile')).toBeTruthy();
	});

	it('shows the empty state message', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText("This user hasn't posted anything yet.")).toBeTruthy();
		});
	});
});
