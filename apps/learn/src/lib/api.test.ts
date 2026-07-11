import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('$env/static/public', () => ({ PUBLIC_JC_API_URL: 'https://api.japaconnect.com' }));

describe('learn api client base URL', () => {
	const originalFetch = globalThis.fetch;

	beforeEach(() => {
		globalThis.fetch = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ data: [] }), { status: 200 })
		);
	});

	afterEach(() => {
		globalThis.fetch = originalFetch;
		vi.resetModules();
	});

	it('builds request URLs from PUBLIC_JC_API_URL, never localhost:8080', async () => {
		const { api } = await import('./api');
		await api.listTracks();

		const [calledUrl] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(calledUrl).toContain('https://api.japaconnect.com');
		expect(calledUrl).not.toContain('localhost:8080');
	});
});
