import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	tagsAdminApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		create: vi.fn().mockResolvedValue({ ok: true }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		delete: vi.fn().mockResolvedValue({ ok: true }),
		getModerators: vi.fn().mockResolvedValue({ ok: true, data: [] }),
	},
	tagsModeratorsAdminApi: {
		add: vi.fn().mockResolvedValue({ ok: true }),
		remove: vi.fn().mockResolvedValue({ ok: true }),
	},
	api: vi.fn().mockResolvedValue({ ok: true, data: [] }),
}));
vi.mock('$app/state', () => ({
	page: { params: {}, url: new URL('http://localhost/') },
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({}),
}));

describe('/tags', () => {
	it('renders the Tags heading', () => {
		render(Page);
		expect(screen.getByText('Tags')).toBeTruthy();
	});

	it('shows empty state when no tags', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
