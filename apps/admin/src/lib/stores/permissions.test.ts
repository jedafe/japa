import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn()
}));

import { apiFetch } from '$lib/api/client';
import { fetchPermissions, permissions, permissionsLoaded, hasPermission } from './permissions';

describe('fetchPermissions', () => {
	beforeEach(() => {
		permissions.set([]);
		permissionsLoaded.set(false);
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
		expect(get(permissions)).toEqual(['admin.users.view', 'admin.settings']);
		expect(get(permissionsLoaded)).toBe(true);
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

		expect(get(permissions)).toEqual([]);
		expect(get(permissionsLoaded)).toBe(true);
	});
});
