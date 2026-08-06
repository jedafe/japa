import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	pageBuilderAdminApiExtended: {
		listBlocks: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		getBlock: vi.fn().mockResolvedValue({ ok: true, data: {} }),
		createBlock: vi.fn().mockResolvedValue({ ok: true }),
		updateBlock: vi.fn().mockResolvedValue({ ok: true }),
		deleteBlock: vi.fn().mockResolvedValue({ ok: true, data: { deleted: true } }),
	},
	pageBuilderApi: {
		getBlockTypes: vi.fn().mockResolvedValue({ ok: true, data: { block_types: [] } }),
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

describe('/page-builder', () => {
	it('renders the Blocks heading', () => {
		render(Page);
		expect(screen.getByText('Block Types')).toBeTruthy();
	});

	it('shows empty state when no blocks', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
