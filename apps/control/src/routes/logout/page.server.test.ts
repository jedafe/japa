import { describe, it, expect, vi } from 'vitest';
import { actions } from './+page.server';

function makeCookies(initial: Record<string, string> = {}) {
	const store = { ...initial };
	return {
		get: (name: string) => store[name],
		delete: vi.fn((name: string) => {
			delete store[name];
		})
	};
}

describe('control /logout action', () => {
	it('best-effort calls the backend logout, deletes jc_token with the correct domain/path, and redirects to /login', async () => {
		const cookies = makeCookies({ jc_token: 'abc123' });
		const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200 }));

		await expect(actions.default({ cookies, fetch: fetchMock } as any)).rejects.toMatchObject({
			status: 303,
			location: '/login'
		});

		expect(fetchMock).toHaveBeenCalledWith(
			expect.stringContaining('/auth/logout'),
			expect.objectContaining({ method: 'POST', headers: { Authorization: 'Bearer abc123' } })
		);
		expect(cookies.delete).toHaveBeenCalledWith('jc_token', {
			path: '/',
			domain: '.japaconnect.com'
		});
	});

	it('still clears the cookie and redirects even if the backend call throws', async () => {
		const cookies = makeCookies({ jc_token: 'abc123' });
		const fetchMock = vi.fn().mockRejectedValue(new Error('network down'));

		await expect(actions.default({ cookies, fetch: fetchMock } as any)).rejects.toMatchObject({
			status: 303,
			location: '/login'
		});

		expect(cookies.delete).toHaveBeenCalledWith('jc_token', {
			path: '/',
			domain: '.japaconnect.com'
		});
	});

	it('redirects without a backend call when there is no session cookie', async () => {
		const cookies = makeCookies({});
		const fetchMock = vi.fn();

		await expect(actions.default({ cookies, fetch: fetchMock } as any)).rejects.toMatchObject({
			status: 303,
			location: '/login'
		});

		expect(fetchMock).not.toHaveBeenCalled();
		expect(cookies.delete).toHaveBeenCalledWith('jc_token', {
			path: '/',
			domain: '.japaconnect.com'
		});
	});
});
