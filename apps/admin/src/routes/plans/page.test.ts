import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	plansApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		create: vi.fn().mockResolvedValue({ ok: true }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		delete: vi.fn().mockResolvedValue({ ok: true }),
		listTracks: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		setTracks: vi.fn().mockResolvedValue({ ok: true }),
	},
	walletApi: {
		getCurrencies: vi.fn().mockResolvedValue({ ok: true, data: [] }),
	},
	api: vi.fn().mockResolvedValue({ ok: true, data: [] }),
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

describe('/plans', () => {
	it('renders the Subscription Plans heading', () => {
		render(Page);
		expect(screen.getByText('Subscription Plans')).toBeTruthy();
	});

	it('shows empty state when no plans', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
