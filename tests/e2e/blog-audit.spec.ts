import { test, expect } from '@playwright/test';

/**
 * Blog surface audit — /blog, /blog?tab=..., /blog/[slug], /blog/series.
 * Captures console errors and failing API/network responses.
 */
test.describe('Blog surface audit', () => {
	const pages = [
		{ path: '/blog', label: 'blog latest' },
		{ path: '/blog?tab=featured', label: 'blog featured' },
		{ path: '/blog?tab=series', label: 'blog series tab' },
		{ path: '/blog/series', label: 'blog series list' },
		{ path: '/blog/series/another-series', label: 'blog series detail' },
		{ path: '/blog/starting-a-business-in-canada-as-a-nigerian', label: 'blog single' }
	];

	for (const p of pages) {
		test(`${p.label} (${p.path}) renders without console/network errors`, async ({ page }) => {
			const consoleMessages: { type: string; text: string }[] = [];
			const failedResponses: { url: string; status: number }[] = [];

			page.on('console', (msg) => {
				if (msg.type() === 'error' || msg.type() === 'warning') {
					consoleMessages.push({ type: msg.type(), text: msg.text() });
				}
			});

			page.on('pageerror', (err) => {
				consoleMessages.push({ type: 'pageerror', text: err.message });
			});

			page.on('response', (res) => {
				if (res.status() >= 400) {
					failedResponses.push({ url: res.url(), status: res.status() });
				}
			});

			const response = await page.goto(p.path, { waitUntil: 'networkidle' });
			expect(response, `page ${p.path} did not respond`).not.toBeNull();
			expect(response?.status()).toBeLessThan(500);

			// Wait a beat for lazy client fetches (reaction types, share platforms, etc.)
			await page.waitForTimeout(1000);

			const failedApi = failedResponses.filter((r) => r.url.includes('/api/v1/'));
			if (failedApi.length) {
				console.log(`Failed API calls on ${p.path}:`, failedApi);
			}

			// Fail the test only on unexpected errors so we can see them in the report.
			expect(consoleMessages, `console errors on ${p.path}`).toEqual([]);
			expect(failedApi, `failing /api/v1 calls on ${p.path}`).toEqual([]);
		});
	}

	test('blog rss (/blog/rss.xml) returns a valid RSS feed', async ({ page }) => {
		const response = await page.goto('/blog/rss.xml');
		expect(response, 'RSS feed did not respond').not.toBeNull();
		expect(response?.status()).toBe(200);
		expect(response?.headers()['content-type']).toContain('application/rss+xml');
		const body = await response?.text() ?? '';
		expect(body).toContain('<rss');
		expect(body).toContain('<channel>');
	});
});
