import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	pagesApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		create: vi.fn().mockResolvedValue({ ok: true }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		delete: vi.fn().mockResolvedValue({ ok: true }),
		publish: vi.fn().mockResolvedValue({ ok: true }),
		archive: vi.fn().mockResolvedValue({ ok: true })
	},
	pageTemplatesAdminApiExtended: {
		list: vi.fn().mockResolvedValue({ ok: true, data: [] })
	},
	pageBuilderAdminApiExtended: {
		listThemes: vi.fn().mockResolvedValue({ ok: true, data: [] })
	}
}));
vi.mock('$app/state', () => ({
	page: { params: {}, url: new URL('http://localhost/') }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({})
}));

describe('/page-builder/pages', () => {
	it('renders the Pages heading', () => {
		render(Page);
		expect(screen.getByText('Pages')).toBeTruthy();
	});

	it('shows empty state when no pages', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
