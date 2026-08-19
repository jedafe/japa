import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	moderationAdminApi: {
		getQueue: vi.fn().mockResolvedValue({ ok: true, data: { items: [], total: 0, total_pages: 1 } }),
		getAlerts: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		getOptions: vi.fn().mockResolvedValue({ ok: true, data: { statuses: [], priorities: [], reasons: [] } }),
		assignItem: vi.fn().mockResolvedValue({ ok: true }),
		bulkAction: vi.fn().mockResolvedValue({ ok: true }),
		acknowledgeAlert: vi.fn().mockResolvedValue({ ok: true }),
		resolveAlert: vi.fn().mockResolvedValue({ ok: true }),
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
vi.mock('$lib/stores/auth.svelte', () => ({
	user: { current: { id: 'admin-1', username: 'admin', email: 'admin@example.com' } },
}));

describe('/moderation-queue', () => {
	it('renders the Moderation Queue heading on the default (queue) tab', () => {
		render(Page);
		expect(screen.getByText('Moderation Queue')).toBeTruthy();
	});

	it('shows empty state when the queue has no items', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
