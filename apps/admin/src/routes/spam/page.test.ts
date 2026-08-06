import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	spamApi: {
		getIpBlocks: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		createIpBlock: vi.fn().mockResolvedValue({ ok: true }),
		updateIpBlock: vi.fn().mockResolvedValue({ ok: true }),
		deleteIpBlock: vi.fn().mockResolvedValue({ ok: true }),
		getCensoredWords: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		createCensoredWord: vi.fn().mockResolvedValue({ ok: true }),
		updateCensoredWord: vi.fn().mockResolvedValue({ ok: true }),
		deleteCensoredWord: vi.fn().mockResolvedValue({ ok: true }),
		getUserBlocks: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		deleteUserBlock: vi.fn().mockResolvedValue({ ok: true }),
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

describe('/spam', () => {
	it('renders the IP Blocks heading on the default tab', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'IP Blocks' })).toBeTruthy();
	});

	it('shows empty state when no IP blocks', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
