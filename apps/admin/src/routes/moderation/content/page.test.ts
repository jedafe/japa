import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('#lib/api.js', () => ({
	apiRaw: vi.fn().mockResolvedValue({ ok: true, data: { data: [], total: 0, total_pages: 1 } }),
	contentAdminApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: { items: [] } }),
		remove: vi.fn().mockResolvedValue({ ok: true }),
		restore: vi.fn().mockResolvedValue({ ok: true }),
		listContentTypes: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		getStatusOptions: vi.fn().mockResolvedValue({ ok: true, data: [] }),
	},
}));
vi.mock('$app/state', () => ({
	page: { params: {}, url: new URL('http://localhost/') },
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));
vi.mock('#lib/stores/permissions.svelte.js', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({}),
}));

describe('/moderation/content', () => {
	it('renders the Content Admin heading', () => {
		render(Page);
		expect(screen.getByText('Content Admin')).toBeTruthy();
	});

	it('shows empty state when no content', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
