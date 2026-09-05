import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockResolvedValue({ ok: true, data: null }),
	apiRaw: vi.fn().mockResolvedValue({
		ok: true,
		data: { data: [], total: 0, total_pages: 0 }
	})
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

describe('/literals', () => {
	it('renders the Template Literals heading', () => {
		render(Page);
		expect(screen.getByText('Template Literals')).toBeTruthy();
	});

	it('shows empty state when no literals', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
