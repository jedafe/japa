import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	pageBuilderAdminApiExtended: {
		listThemes: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		createTheme: vi.fn().mockResolvedValue({ ok: true }),
		updateTheme: vi.fn().mockResolvedValue({ ok: true }),
		setDefaultTheme: vi.fn().mockResolvedValue({ ok: true }),
		deleteTheme: vi.fn().mockResolvedValue({ ok: true }),
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

describe('/page-builder/themes', () => {
	it('renders the Page Themes heading', () => {
		render(Page);
		expect(screen.getByText('Page Themes')).toBeTruthy();
	});

	it('shows empty state when no themes', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
