import { test, expect } from '@playwright/test';

/**
 * Blog surface audit — /blog, /blog?tab=..., /blog/[slug], /series.
 * Captures console errors and failing API/network responses.
 */
test.describe('Blog surface audit', () => {
	const pages = [
		{ path: '/blog', label: 'blog latest' },
		{ path: '/blog?tab=featured', label: 'blog featured' },
		{ path: '/blog?tab=series', label: 'blog series tab' },
		{ path: '/series', label: 'series directory' },
		{ path: '/series/another-series', label: 'series detail' },
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

	test('series nav updates after client-side navigation', async ({ page }) => {
		await page.goto('/blog/this-is-an-example-post', { waitUntil: 'networkidle' });
		await page.waitForTimeout(1000);

		const nav = page.locator('nav[aria-label="Series navigation"]').first();
		await expect(nav).toContainText('How to Get a UK Driving License as a New Resident');
		await expect(nav).toContainText('This is an Example Post');
		await expect(nav).toContainText('Starting a Business in Canada as a Nigerian Immigrant');
		await expect(nav).toContainText('4 more parts');

		// Click the part 5 link within the series card (client nav)
		const part5Link = nav.locator('a', { hasText: 'Starting a Business in Canada as a Nigerian Immigrant' }).first();
		await part5Link.click();
		await page.waitForURL('/blog/demo-blog-post-11-business-canada', { waitUntil: 'networkidle' });
		await page.waitForTimeout(1000);

		// After navigation the visible list should update to 1, 2, 5 and stay folded
		const navAfter = page.locator('nav[aria-label="Series navigation"]').first();
		await expect(navAfter).toContainText('How to Get a UK Driving License as a New Resident');
		await expect(navAfter).toContainText('This is an Example Post');
		await expect(navAfter).toContainText('Starting a Business in Canada as a Nigerian Immigrant');
		await expect(navAfter).toContainText('4 more parts');
	});
});
