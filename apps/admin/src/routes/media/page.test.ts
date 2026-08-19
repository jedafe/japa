import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	mediaAdminApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		listQuarantined: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		approve: vi.fn().mockResolvedValue({ ok: true }),
		reject: vi.fn().mockResolvedValue({ ok: true }),
		updateStatus: vi.fn().mockResolvedValue({ ok: true }),
		delete: vi.fn().mockResolvedValue({ ok: true }),
	},
	apiRaw: vi.fn().mockResolvedValue({ ok: true, data: { data: [], total: 0, total_pages: 1 } }),
	stockAdminApi: {
		listAvailableProviders: vi.fn().mockResolvedValue({ ok: true, data: { providers: [] } }),
		search: vi.fn().mockResolvedValue({ ok: true, data: { photos: [], has_next_page: false } }),
		import: vi.fn().mockResolvedValue({ ok: true }),
	},
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

describe('/media', () => {
	it('renders the Media Library heading', () => {
		render(Page);
		expect(screen.getByText('Media Library')).toBeTruthy();
	});

	it('shows empty state when no media', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data')).toBeTruthy();
		});
	});
});
