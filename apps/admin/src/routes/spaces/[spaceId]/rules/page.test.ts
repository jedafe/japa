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

vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	permissions: [],
	getPermissionState: () => ({})
}));

describe('Space Rules +page.svelte', () => {
	it('renders the heading', () => {
		render(Page);
		expect(screen.getByText('Space Rules')).toBeTruthy();
	});

	it('shows Add Rule button', () => {
		render(Page);
		expect(screen.getByText('+ Add Rule')).toBeTruthy();
	});

	it('shows empty state when no rules', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText(/No rules yet/)).toBeTruthy();
		});
	});
});
