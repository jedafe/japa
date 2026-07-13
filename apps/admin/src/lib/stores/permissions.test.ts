import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn()
}));

import { apiFetch } from '$lib/api/client';
import { fetchPermissions, permissions, permissionsLoaded, hasPermission } from './permissions.svelte';

describe('fetchPermissions', () => {
	beforeEach(async () => {
		vi.mocked(apiFetch).mockResolvedValue({
			data: { permissions: [] },
			error: null,
			status: 200
		});
		await fetchPermissions();
		vi.mocked(apiFetch).mockReset();
	});

	it('takes no arguments and populates the store from /me/permissions', async () => {
		vi.mocked(apiFetch).mockResolvedValue({
			data: { permissions: ['admin.users.view', 'admin.settings'] },
			error: null,
			status: 200
		});

		await fetchPermissions();

		expect(apiFetch).toHaveBeenCalledWith('/me/permissions');
		expect(permissions).toEqual(['admin.users.view', 'admin.settings']);
		expect(permissionsLoaded).toBe(true);
		expect(hasPermission('admin.users.view')).toBe(true);
		expect(hasPermission('admin.broadcast')).toBe(false);
	});

	it('sets an empty list (not an exception) when the API call fails', async () => {
		vi.mocked(apiFetch).mockResolvedValue({
			data: null,
			error: { code: 'server_error', message: 'boom', status: 500 },
			status: 500
		});

		await fetchPermissions();

		expect(permissions).toEqual([]);
		expect(permissionsLoaded).toBe(true);
	});
});
