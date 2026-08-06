import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	moderationAdminApi: {
		listAppeals: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		getAppeal: vi.fn().mockResolvedValue({ ok: true, data: {} }),
		updateAppeal: vi.fn().mockResolvedValue({ ok: true }),
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

describe('/moderation/appeals', () => {
	it('renders the Appeals heading', () => {
		render(Page);
		expect(screen.getByText('Appeals')).toBeTruthy();
	});

	it('shows empty state when no appeals', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
