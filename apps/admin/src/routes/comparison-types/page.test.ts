import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	comparisonAdminApi: {
		listTypes: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		createType: vi.fn().mockResolvedValue({ ok: true }),
		updateType: vi.fn().mockResolvedValue({ ok: true }),
		deleteType: vi.fn().mockResolvedValue({ ok: true }),
		getType: vi.fn().mockResolvedValue({ ok: true, data: { dimensions: [], item_count: 0 } }),
		getTypeAccess: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		setTypeAccess: vi.fn().mockResolvedValue({ ok: true }),
		listFieldGroups: vi.fn().mockResolvedValue({ ok: true, data: [] }),
	},
	userManagementApi: {
		listGroups: vi.fn().mockResolvedValue({ ok: true, data: { groups: [], total: 0 } }),
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

describe('/comparison-types', () => {
	it('renders the Comparison Types heading', () => {
		render(Page);
		expect(screen.getByText('Comparison Types')).toBeTruthy();
	});

	it('shows empty state when no comparison types', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
