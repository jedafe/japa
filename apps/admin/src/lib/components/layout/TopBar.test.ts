import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';

// vi.mock factories are hoisted above module init, so any mock that needs a
// shared spy reference must create it via vi.hoisted (which also hoists) —
// otherwise the factory runs before the plain `const` is initialized.
const { clearUserMock, permissionsSetMock } = vi.hoisted(() => ({
	clearUserMock: vi.fn(),
	permissionsSetMock: vi.fn()
}));

vi.mock('$env/static/public', () => ({ PUBLIC_JC_ENV: 'development' }));

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn().mockResolvedValue({ data: null, error: null, status: 200 })
}));

vi.mock('$lib/stores/auth', () => ({
	user: { subscribe: (fn: (v: unknown) => void) => (fn({ display_name: 'Ada', avatar_url: '' }), () => {}) },
	clearUser: clearUserMock
}));

vi.mock('$lib/stores/permissions', () => ({
	permissions: { set: permissionsSetMock }
}));

vi.mock('$lib/stores/theme', () => ({
	theme: { subscribe: (fn: (v: unknown) => void) => (fn('light'), () => {}), update: vi.fn() }
}));

import { apiFetch } from '$lib/api/client';
import TopBar from './TopBar.svelte';

describe('admin TopBar logout', () => {
	beforeEach(() => {
		vi.mocked(apiFetch).mockClear();
		clearUserMock.mockClear();
		permissionsSetMock.mockClear();
		// @ts-expect-error jsdom navigation stub
		delete window.location;
		// @ts-expect-error jsdom navigation stub
		window.location = { href: '' };
	});

	it('best-effort revokes the session server-side and clears in-memory state before redirecting', async () => {
		render(TopBar);

		await fireEvent.click(screen.getByRole('button', { name: /ada/i }));
		await fireEvent.click(screen.getByText('Logout'));

		expect(apiFetch).toHaveBeenCalledWith('/auth/logout', { method: 'POST' });
		expect(clearUserMock).toHaveBeenCalled();
		expect(permissionsSetMock).toHaveBeenCalledWith([]);
		expect(window.location.href).toBe('/login');
	});
});
