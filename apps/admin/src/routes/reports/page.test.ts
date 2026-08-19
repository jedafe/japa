import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	reportsAdminApiExtended: {
		list: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		get: vi.fn().mockResolvedValue({ ok: true, data: {} }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		delete: vi.fn().mockResolvedValue({ ok: true }),
	},
	apiRaw: vi.fn().mockResolvedValue({ ok: true, data: { data: { items: [], total: 0, total_pages: 1 } } }),
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

describe('/reports', () => {
	it('renders the Reports heading', () => {
		render(Page);
		expect(screen.getByText('Reports')).toBeTruthy();
	});

	it('shows empty state when no reports', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
