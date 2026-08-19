import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	moderationAdminApi: {
		acknowledgeAlert: vi.fn().mockResolvedValue({ ok: true }),
		resolveAlert: vi.fn().mockResolvedValue({ ok: true }),
	},
	apiRaw: vi.fn().mockResolvedValue({ ok: true, data: { data: [], total: 0, total_pages: 1 } }),
	adminCatalogApi: {
		get: vi.fn().mockResolvedValue({ ok: true, data: [] }),
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

describe('/moderation/alerts', () => {
	it('renders the Moderation Alerts heading', () => {
		render(Page);
		expect(screen.getByText('Moderation Alerts')).toBeTruthy();
	});

	it('shows empty state when no alerts', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
