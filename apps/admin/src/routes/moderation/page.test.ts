import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	moderationAdminApi: {
		getQueue: vi.fn().mockResolvedValue({ ok: true, data: { items: [], total: 0, total_pages: 1 } }),
		updateItem: vi.fn().mockResolvedValue({ ok: true }),
		bulkAction: vi.fn().mockResolvedValue({ ok: true }),
	},
}));
vi.mock('$app/state', () => ({
	page: { params: {}, url: new URL('http://localhost/') },
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({}),
}));

describe('/moderation', () => {
	it('renders the Moderation heading', () => {
		render(Page);
		expect(screen.getByText('Content Moderation')).toBeTruthy();
	});

	it('shows empty state when no items', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
