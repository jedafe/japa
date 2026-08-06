import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	sidebarsAdminApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: { items: [] } }),
		get: vi.fn().mockResolvedValue({ ok: true, data: {} }),
		create: vi.fn().mockResolvedValue({ ok: true }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		delete: vi.fn().mockResolvedValue({ ok: true }),
		addWidget: vi.fn().mockResolvedValue({ ok: true }),
		updateWidget: vi.fn().mockResolvedValue({ ok: true }),
		deleteWidget: vi.fn().mockResolvedValue({ ok: true }),
		reorderWidgets: vi.fn().mockResolvedValue({ ok: true }),
		widgetTypes: vi.fn().mockResolvedValue({ ok: true, data: { widget_types: [] } }),
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

describe('/sidebars', () => {
	it('renders the Sidebar Manager heading', () => {
		render(Page);
		expect(screen.getByText('Sidebar Manager')).toBeTruthy();
	});

	it('shows empty state when no sidebars', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
