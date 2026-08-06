import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	menusAdminApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: { items: [] } }),
		get: vi.fn().mockResolvedValue({ ok: true, data: { items: [] } }),
		create: vi.fn().mockResolvedValue({ ok: true }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		delete: vi.fn().mockResolvedValue({ ok: true }),
		addItem: vi.fn().mockResolvedValue({ ok: true }),
		updateItem: vi.fn().mockResolvedValue({ ok: true }),
		deleteItem: vi.fn().mockResolvedValue({ ok: true }),
		reorderItems: vi.fn().mockResolvedValue({ ok: true }),
	},
	userManagementApi: {
		listGroups: vi.fn().mockResolvedValue({ ok: true, data: { groups: [] } }),
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

describe('/menus', () => {
	it('renders the Menus heading', () => {
		render(Page);
		expect(screen.getByText('Menus')).toBeTruthy();
	});

	it('shows empty state when no menus', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
