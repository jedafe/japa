import { describe, it, expect, vi } from 'vitest';

// Pin the API base to a known non-localhost value so the assertion is hermetic
// (independent of whether PRIVATE_API_BASE_URL happens to be set in the
// ambient env). This proves the action builds its URL from the server
// api module's API_V1_BASE rather than a hardcoded (and undefined) Vite env var.
vi.mock('$lib/server/api', () => ({
	API_BASE: 'https://api.japaconnect.com',
	API_V1_BASE: 'https://api.japaconnect.com/api/v1'
}));

import { actions } from './+page.server';

describe('control login action', () => {
	it('posts to the configured API_BASE, not a hardcoded localhost fallback', async () => {
		const fetchMock = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ data: { access_token: 'abc123' } }), { status: 200 })
		);
		const cookies = { set: vi.fn() };
		const formData = new FormData();
		formData.set('email', 'admin@japaconnect.com');
		formData.set('password', 'correct-horse-battery-staple');
		const request = { formData: () => Promise.resolve(formData) };

		await expect(
			actions.default({ request, cookies, fetch: fetchMock } as any)
		).rejects.toMatchObject({ status: 302, location: '/' });

		const [calledUrl] = fetchMock.mock.calls[0];
		expect(calledUrl).not.toContain('localhost:8080');
		expect(calledUrl).toContain('api.japaconnect.com');
		expect(calledUrl).toContain('/auth/login');
	});
});
