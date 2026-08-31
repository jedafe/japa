import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api/comparison-data-sources', () => ({
	listDataSources: vi.fn().mockResolvedValue([]),
	createDataSource: vi.fn().mockResolvedValue({ ok: true }),
	updateDataSource: vi.fn().mockResolvedValue({ ok: true }),
	deleteDataSource: vi.fn().mockResolvedValue({ ok: true }),
	getDataSource: vi.fn().mockResolvedValue(null)
}));

vi.mock('$app/state', () => ({
	page: { params: {}, url: new URL('http://localhost/comparison-data-sources') }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({})
}));

describe('/comparison-data-sources', () => {
	it('renders the Comparison data sources heading', () => {
		render(Page);
		expect(screen.getByText('Comparison data sources')).toBeTruthy();
	});

	it('shows empty state when no data sources', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
