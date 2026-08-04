import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';

// vi.mock factories are hoisted above module init, so any mock that needs a
// shared spy reference must create it via vi.hoisted (which also hoists) —
// otherwise the factory runs before the plain `const` is initialized.
const { clearUserMock } = vi.hoisted(() => ({
	clearUserMock: vi.fn()
}));

vi.mock('$env/static/public', () => ({ PUBLIC_JC_ENV: 'development' }));

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn().mockResolvedValue({ data: null, error: null, status: 200 })
}));

vi.mock('$lib/stores/auth.svelte', () => ({
	user: { current: { username: 'Ada', avatar_url: '' } },
	clearUser: clearUserMock
}));

vi.mock('$lib/stores/permissions.svelte', () => ({
	permissions: [],
	hasPermission: vi.fn().mockReturnValue(false)
}));

vi.mock('$lib/stores/theme.svelte', () => ({
	theme: 'light'
}));

import { apiFetch } from '$lib/api/client';
import TopBar from './TopBar.svelte';

describe('admin TopBar logout', () => {
	beforeEach(() => {
		vi.mocked(apiFetch).mockClear();
		clearUserMock.mockClear();
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
		expect(window.location.href).toBe('/login');
	});
});
