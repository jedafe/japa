import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	userManagementApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: { items: [], total: 0, page: 1, total_pages: 1 } }),
		getModerationConfig: vi.fn().mockResolvedValue({ ok: true, data: { statuses: [] } }),
		listGroups: vi.fn().mockResolvedValue({ ok: true, data: { groups: [] } }),
		issueWarning: vi.fn(),
		banUser: vi.fn(),
		unbanUser: vi.fn(),
		suspendUser: vi.fn(),
		deleteUser: vi.fn(),
		hardDeleteUser: vi.fn(),
		createUser: vi.fn(),
	},
	impersonationApi: {
		status: vi.fn().mockResolvedValue({ ok: true, data: { impersonating: false } }),
		start: vi.fn(),
		end: vi.fn(),
	},
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	replaceState: vi.fn(),
	invalidateAll: vi.fn(),
}));

vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: vi.fn().mockReturnValue(false),
}));

vi.mock('$lib/stores/i18n.svelte', () => ({
	t: vi.fn((key: string, fallback: string) => fallback),
}));

describe('Users +page.svelte', () => {
	it('renders the users heading', () => {
		render(Page);
		expect(screen.getByText('Users')).toBeTruthy();
	});

	it('shows filter controls', async () => {
		render(Page);
		// EntityPage renders the DataTable FilterPanel (accordion trigger labelled "Filters").
		await waitFor(() => {
			expect(screen.getByText('Filters')).toBeTruthy();
		});
	});

	it('has search input', async () => {
		render(Page);
		// DataTable's search box uses the i18n fallback "Search" as its placeholder.
		await waitFor(() => {
			expect(screen.getByPlaceholderText('Search')).toBeTruthy();
		});
	});

	it('renders an action menu trigger per row when rows are present', async () => {
		// Override the list mock for this test to return one row.
		const { userManagementApi } = await import('$lib/api');
		const listMock = userManagementApi.list as ReturnType<typeof vi.fn>;
		listMock.mockResolvedValue({
			ok: true,
			data: { items: [{ id: 'u1', username: 'alice', email: 'a@x.io', profile_type: 'user', status: 'active', follower_count: 0, groups: [], created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' }], total: 1, page: 1, total_pages: 1 },
		});
		const { container } = render(Page);
		await waitFor(() => {
			expect(container.querySelectorAll('[data-action-menu-trigger]').length).toBeGreaterThan(0);
		});
		// Restore the default empty-list mock for subsequent tests.
		listMock.mockResolvedValue({ ok: true, data: { items: [], total: 0, page: 1, total_pages: 1 } });
	});
});
