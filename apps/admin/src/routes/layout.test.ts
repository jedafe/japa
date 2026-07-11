import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$app/environment', () => ({ browser: false }));

vi.mock('$lib/stores/permissions', () => ({
	fetchPermissions: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn()
}));

import { load } from './+layout';
import { fetchPermissions } from '$lib/stores/permissions';
import { apiFetch } from '$lib/api/client';

describe('admin root +layout load', () => {
	beforeEach(() => {
		vi.mocked(fetchPermissions).mockClear();
		vi.mocked(apiFetch).mockReset();
	});

	it('skips fetching permissions/user on /login to avoid a redirect loop', async () => {
		const result = (await load({ url: new URL('http://admin.japaconnect.com/login') } as any)) as any;

		expect(fetchPermissions).not.toHaveBeenCalled();
		expect(apiFetch).not.toHaveBeenCalled();
		expect(result.user).toBeNull();
		expect(result.token).toBeNull();
	});

	it('fetches permissions and the current user unconditionally on non-login routes', async () => {
		const fakeUser = {
			id: '1',
			display_name: 'Ada',
			avatar_url: '',
			permissions: ['admin.users.view']
		};
		vi.mocked(apiFetch).mockResolvedValue({ data: fakeUser, error: null, status: 200 });

		const result = (await load({ url: new URL('http://admin.japaconnect.com/users') } as any)) as any;

		expect(fetchPermissions).toHaveBeenCalledTimes(1);
		expect(apiFetch).toHaveBeenCalledWith('/me');
		expect(result.user).toEqual(fakeUser);
	});
});
