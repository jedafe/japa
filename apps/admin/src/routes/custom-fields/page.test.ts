import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockResolvedValue({ ok: true, data: { group: null, fields: [] } }),
	fieldGroupsAdminApiExtended: {
		list: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		create: vi.fn().mockResolvedValue({ ok: true }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		delete: vi.fn().mockResolvedValue({ ok: true }),
		addField: vi.fn().mockResolvedValue({ ok: true }),
		updateField: vi.fn().mockResolvedValue({ ok: true }),
		deleteField: vi.fn().mockResolvedValue({ ok: true }),
		reorderFields: vi.fn().mockResolvedValue({ ok: true }),
	},
	userManagementApi: {
		list: vi.fn().mockRejectedValue(new Error('not connected')),
		listCustomFieldEntityTypes: vi.fn().mockResolvedValue({ ok: true, data: [] }),
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

describe('/custom-fields', () => {
	it('renders the Field Groups heading', () => {
		render(Page);
		expect(screen.getByText('Field Groups')).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Manage custom field groups')).toBeTruthy();
	});

	it('shows the Create Group action', () => {
		render(Page);
		expect(screen.getByText('Create Group')).toBeTruthy();
	});

	it('shows empty state when no field groups', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
