import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';

vi.mock('$app/environment', () => ({ browser: false }));

vi.mock('$env/static/public', () => ({ PUBLIC_JC_ENV: 'development' }));

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn().mockResolvedValue({ data: null, error: null, status: 200 })
}));

vi.mock('$lib/stores/auth.svelte', () => ({
	user: { current: { username: 'Ada', avatar_url: '' } },
	clearUser: vi.fn()
}));

vi.mock('$lib/stores/permissions.svelte', () => ({
	permissions: [],
	hasPermission: vi.fn().mockReturnValue(false),
	getPermissionState: vi.fn().mockReturnValue([])
}));

vi.mock('$lib/stores/theme.svelte', () => ({
	theme: 'light'
}));

vi.mock('$lib/stores/notifications.svelte', () => ({
	getNotifications: () => [],
	getUnreadCount: () => 0,
	markRead: vi.fn(),
	markAllRead: vi.fn(),
	initNotifications: () => () => {}
}));

import { load } from './+layout';
import TopBar from '$lib/components/layout/TopBar.svelte';

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

describe('TopBar collapse toggle', () => {
	it('renders a desktop collapse button that fires onCollapseToggle', async () => {
		let toggled = false;
		const { container } = render(TopBar, { onCollapseToggle: () => (toggled = true) });
		const btn = container.querySelector('[data-sidebar-collapse]');
		expect(btn).toBeTruthy();
		await fireEvent.click(btn!);
		expect(toggled).toBe(true);
	});
});
