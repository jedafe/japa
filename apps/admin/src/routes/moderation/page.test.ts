import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('#lib/api.js', () => ({
	moderationAdminApi: {
		getQueue: vi.fn().mockResolvedValue({ ok: true, data: { items: [], total: 0, total_pages: 1 } }),
		updateItem: vi.fn().mockResolvedValue({ ok: true }),
		assignItem: vi.fn().mockResolvedValue({ ok: true }),
		bulkAction: vi.fn().mockResolvedValue({ ok: true }),
		getOptions: vi.fn().mockResolvedValue({ ok: true, data: { statuses: [], content_types: [] } }),
	},
	adsAdminApi: {
		activateCampaign: vi.fn().mockResolvedValue({ ok: true }),
		rejectCampaign: vi.fn().mockResolvedValue({ ok: true }),
	},
}));
vi.mock('$app/state', () => ({
	page: { params: {}, url: new URL('http://localhost/') },
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));
vi.mock('#lib/stores/permissions.svelte.js', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({}),
}));
vi.mock('#lib/stores/auth.svelte.js', () => ({
	user: { current: { id: 'admin-1', username: 'admin', email: 'admin@example.com' } },
}));

describe('/moderation', () => {
	it('renders the Moderation Queue heading', () => {
		render(Page);
		expect(screen.getByText('Moderation Queue')).toBeTruthy();
	});

	it('shows empty state when no items', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
