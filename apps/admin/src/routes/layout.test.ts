import { describe, it, expect, vi } from 'vitest';

vi.mock('$app/environment', () => ({ browser: false }));

import { load } from './+layout';

// The admin app is a pure SPA (ssr=false). The universal +layout.ts load
// resolves the locale from the jc_locale cookie (client-only) and initializes
// the i18n store, then returns an empty object — auth is handled client-side
// in +layout.svelte's onMount via /me + /me/permissions.
describe('admin root +layout load', () => {
	it('returns an empty object (SPA — auth is client-side in onMount)', async () => {
		const result = (await load({
			url: new URL('http://admin.japaconnect.com/users')
		} as any)) as any;

		expect(result).toEqual({});
	});

	it('does not throw for the login route', async () => {
		await expect(load({
			url: new URL('http://admin.japaconnect.com/login')
		} as any)).resolves.toEqual({});
	});
});
