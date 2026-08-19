import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	contentAdminApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		get: vi.fn().mockResolvedValue({ ok: true, data: {} }),
		remove: vi.fn().mockResolvedValue({ ok: true }),
		restore: vi.fn().mockResolvedValue({ ok: true }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		feature: vi.fn().mockResolvedValue({ ok: true }),
		unfeature: vi.fn().mockResolvedValue({ ok: true }),
		archive: vi.fn().mockResolvedValue({ ok: true }),
		listContentTypes: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		getStatusOptions: vi.fn().mockResolvedValue({ ok: true, data: [] }),
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

describe('/content', () => {
	it('renders the All Content heading', () => {
		render(Page);
		expect(screen.getByText('All Content')).toBeTruthy();
	});

	it('shows empty state when no content', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
