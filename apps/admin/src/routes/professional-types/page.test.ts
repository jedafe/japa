import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	professionalAdminApi: {
		listTypes: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		createType: vi.fn().mockResolvedValue({ ok: true }),
		updateType: vi.fn().mockResolvedValue({ ok: true }),
		deleteType: vi.fn().mockResolvedValue({ ok: true }),
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

describe('/professional-types', () => {
	it('renders the Professional Types heading', () => {
		render(Page);
		expect(screen.getByText('Professional Types')).toBeTruthy();
	});

	it('shows empty state when no types', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
