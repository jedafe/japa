import { describe, it, expect, vi, afterEach } from 'vitest';

// apiBaseUrl + the first-party host are captured at module scope, so each
// apiBaseUrl scenario needs a fresh module graph with a mocked client.
async function loadRemoteImage(apiBaseUrl = '/api/v1') {
	vi.resetModules();
	vi.doMock('#lib/api/client.js', () => ({ apiBaseUrl }));
	const mod = await import('./remoteImage.js');
	vi.doUnmock('#lib/api/client.js');
	return mod.remoteImageUrl;
}

describe('remoteImageUrl', () => {
	afterEach(() => {
		vi.doUnmock('#lib/api/client.js');
	});

	describe('relative apiBaseUrl (dev / proxied deployments)', () => {
		it('collapses absolute backend API URLs to same-origin paths', async () => {
			const remoteImageUrl = await loadRemoteImage();
			expect(remoteImageUrl('http://localhost:8080/api/v1/media/serve/b/x.webp')).toBe(
				'/api/v1/media/serve/b/x.webp'
			);
		});

		it('collapses https API URLs onto the relative base too', async () => {
			const remoteImageUrl = await loadRemoteImage();
			expect(remoteImageUrl('https://api.japa.to/api/v1/media/serve/b/x.webp')).toBe(
				'/api/v1/media/serve/b/x.webp'
			);
		});

		it('keeps query strings when collapsing', async () => {
			const remoteImageUrl = await loadRemoteImage();
			expect(remoteImageUrl('http://localhost:8080/api/v1/media/serve/x?sig=abc')).toBe(
				'/api/v1/media/serve/x?sig=abc'
			);
		});

		it('passes through relative non-API and API paths', async () => {
			const remoteImageUrl = await loadRemoteImage();
			expect(remoteImageUrl('/brand/logo.png')).toBe('/brand/logo.png');
			expect(remoteImageUrl('/api/v1/media/serve/x.webp')).toBe('/api/v1/media/serve/x.webp');
		});

		it('proxies genuinely external images through /media/proxy', async () => {
			const remoteImageUrl = await loadRemoteImage();
			expect(remoteImageUrl('https://cdn.example.com/pic.png')).toBe(
				`/api/v1/media/proxy?url=${encodeURIComponent('https://cdn.example.com/pic.png')}`
			);
		});

		it('returns empty for empty input', async () => {
			const remoteImageUrl = await loadRemoteImage();
			expect(remoteImageUrl('')).toBe('');
			expect(remoteImageUrl(null)).toBe('');
			expect(remoteImageUrl(undefined)).toBe('');
		});
	});

	describe('absolute apiBaseUrl (prod gateway)', () => {
		it('produces the identical URL for public-origin API URLs', async () => {
			const remoteImageUrl = await loadRemoteImage('https://api.japa.to/api/v1');
			expect(remoteImageUrl('https://api.japa.to/api/v1/media/serve/b/x.webp')).toBe(
				'https://api.japa.to/api/v1/media/serve/b/x.webp'
			);
		});

		it('rewrites internal-host API URLs onto the public origin', async () => {
			const remoteImageUrl = await loadRemoteImage('https://api.japa.to/api/v1');
			expect(remoteImageUrl('http://go-api:8080/api/v1/media/serve/b/x.webp')).toBe(
				'https://api.japa.to/api/v1/media/serve/b/x.webp'
			);
		});

		it('passes first-party media hosts through unchanged', async () => {
			const remoteImageUrl = await loadRemoteImage('https://api.japa.to/api/v1');
			expect(remoteImageUrl('https://img.japa.to/resize/x.webp')).toBe(
				'https://img.japa.to/resize/x.webp'
			);
		});

		it('proxies external images through the gateway proxy', async () => {
			const remoteImageUrl = await loadRemoteImage('https://api.japa.to/api/v1');
			expect(remoteImageUrl('https://cdn.example.com/pic.png')).toBe(
				`https://api.japa.to/api/v1/media/proxy?url=${encodeURIComponent('https://cdn.example.com/pic.png')}`
			);
		});
	});
});
