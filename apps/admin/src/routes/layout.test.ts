import { describe, it, expect, vi } from 'vitest';

vi.mock('$app/environment', () => ({ browser: false }));

import { load } from './+layout';

// The root layout no longer fetches auth data client-side: the server layout
// (+layout.server.ts) validates the token and resolves user/permissions, and
// the universal load passes that data through unchanged.
describe('admin root +layout load', () => {
	it('passes through server-validated auth data', async () => {
		const result = (await load({
			url: new URL('http://admin.japaconnect.com/users'),
			data: {
				token: 'jwt-token',
				user: { id: '1', display_name: 'Ada' },
				permissions: ['admin.users.view']
			}
		} as any)) as any;

		expect(result.token).toBe('jwt-token');
		expect(result.user).toEqual({ id: '1', display_name: 'Ada' });
		expect(result.permissions).toEqual(['admin.users.view']);
	});

	it('passes through null auth fields for unauthenticated visitors', async () => {
		const result = (await load({
			url: new URL('http://admin.japaconnect.com/login'),
			data: { token: null, user: null, permissions: [] }
		} as any)) as any;

		expect(result.user).toBeNull();
		expect(result.token).toBeNull();
		expect(result.permissions).toEqual([]);
	});
});
