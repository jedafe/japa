import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/state', () => ({
	page: {
		params: { spaceId: 'test-space' },
		url: new URL('http://localhost/')
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	replaceState: vi.fn(),
}));

vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	permissions: [],
	getPermissionState: () => ({})
}));

describe('Space Forms +page.svelte', () => {
	it('renders the heading', () => {
		render(Page);
		expect(screen.getByText('Space Forms')).toBeTruthy();
	});

	it('shows Create Form link', () => {
		render(Page);
		expect(screen.getByText('+ Create Form')).toBeTruthy();
	});

	it('shows empty state when no forms', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText(/No forms yet/)).toBeTruthy();
		});
	});
});
