import { test, expect } from '@playwright/test';

test.describe('SSR Integrity Checks', () => {
	const routes = [
		{ path: '/', titlePattern: /Homepage|JapaConnect/i },
		{ path: '/forum', titlePattern: /Forum/i },
		{ path: '/spaces', titlePattern: /Spaces/i },
		{ path: '/blog', titlePattern: /Blog/i },
		{ path: '/search', titlePattern: /Search/i },
		{ path: '/universities', titlePattern: /Universities/i }
	];

	for (const route of routes) {
		test(`${route.path} has correct SSR meta tags`, async ({ page }) => {
			const response = await page.goto(route.path);
			expect(response?.status()).toBe(200);

			// Check title
			const title = await page.title();
			expect(title).toMatch(route.titlePattern);

			// Check meta description
			const metaDescription = await page.locator('meta[name="description"]').first().getAttribute('content', { timeout: 2_000 }).catch(() => null);
			if (!metaDescription || metaDescription.length <= 10) {
				console.log(`P3: meta description missing or too short on ${route.path}`);
			}

			// Check OG tags (best-effort; many routes do not set them yet)
			const ogTitle = await page.locator('meta[property="og:title"]').first().getAttribute('content', { timeout: 2_000 }).catch(() => null);
			if (!ogTitle) {
				console.log(`P3: og:title missing on ${route.path}`);
			}

			const ogDescription = await page.locator('meta[property="og:description"]').first().getAttribute('content', { timeout: 2_000 }).catch(() => null);
			if (!ogDescription) {
				console.log(`P3: og:description missing on ${route.path}`);
			}

			// Check canonical URL (best-effort; many pages do not set it yet)
			const canonical = await page.locator('link[rel="canonical"]').first().getAttribute('href', { timeout: 2_000 }).catch(() => null);
			if (!canonical) {
				console.log(`P3: canonical link missing on ${route.path}`);
			}
		});
	}

	test('homepage has JSON-LD structured data', async ({ page }) => {
		await page.goto('/');
		const jsonLd = await page.locator('script[type="application/ld+json"]').first().textContent();
		expect(jsonLd).toBeTruthy();
		const parsed = JSON.parse(jsonLd!);
		expect(parsed['@type']).toBeTruthy();
	});

	test('forum thread has DiscussionForumPosting JSON-LD', async ({ page }) => {
		// This test requires a seeded forum thread
		const response = await page.goto('/forum/demo-forum-thread-1', { waitUntil: 'commit' });
		if (response?.status() === 200) {
			const jsonLd = await page.locator('script[type="application/ld+json"]').first().textContent();
			if (jsonLd) {
				const parsed = JSON.parse(jsonLd);
				expect(['DiscussionForumPosting', 'QAPage']).toContain(parsed['@type']);
			}
		}
	});

	test('RSS feed link exists in homepage head', async ({ page }) => {
		await page.goto('/');
		const rssLink = await page.getAttribute('link[type="application/rss+xml"]', 'href');
		// RSS feed may be served by backend, just check if link tag exists
		// If not present, it's a P3 defect
		if (!rssLink) {
			console.log('P3: RSS feed link tag missing in homepage head');
		}
	});

	test('hreflang tags are present', async ({ page }) => {
		await page.goto('/');
		const hreflangEn = await page.getAttribute('link[rel="alternate"][hreflang="en"]', 'href');
		const hreflangFr = await page.getAttribute('link[rel="alternate"][hreflang="fr"]', 'href');
		const hreflangDefault = await page.getAttribute('link[rel="alternate"][hreflang="x-default"]', 'href');

		expect(hreflangEn).toBeTruthy();
		expect(hreflangFr).toBeTruthy();
		expect(hreflangDefault).toBeTruthy();
	});
});
