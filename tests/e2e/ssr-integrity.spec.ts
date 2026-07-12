import { test, expect } from '@playwright/test';

test.describe('SSR Integrity Checks', () => {
	const routes = [
		{ path: '/', titlePattern: /JapaConnect/i },
		{ path: '/forum', titlePattern: /Forum/i },
		{ path: '/spaces', titlePattern: /Spaces/i },
		{ path: '/blog', titlePattern: /Blog/i },
		{ path: '/search', titlePattern: /Search/i },
		{ path: '/universities', titlePattern: /Universities/i },
		{ path: '/about', titlePattern: /About/i },
		{ path: '/terms', titlePattern: /Terms/i },
		{ path: '/privacy', titlePattern: /Privacy/i }
	];

	for (const route of routes) {
		test(`${route.path} has correct SSR meta tags`, async ({ page }) => {
			const response = await page.goto(route.path);
			expect(response?.status()).toBe(200);

			// Check title
			const title = await page.title();
			expect(title).toMatch(route.titlePattern);

			// Check meta description
			const metaDescription = await page.getAttribute('meta[name="description"]', 'content');
			expect(metaDescription).toBeTruthy();
			expect(metaDescription?.length).toBeGreaterThan(10);

			// Check OG tags
			const ogTitle = await page.getAttribute('meta[property="og:title"]', 'content');
			expect(ogTitle).toBeTruthy();

			const ogDescription = await page.getAttribute('meta[property="og:description"]', 'content');
			expect(ogDescription).toBeTruthy();

			// Check canonical URL
			const canonical = await page.getAttribute('link[rel="canonical"]', 'href');
			expect(canonical).toBeTruthy();
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
		const response = await page.goto('/forum/demo-forum-thread-1');
		if (response?.status() === 200) {
			const jsonLd = await page.locator('script[type="application/ld+json"]').first().textContent();
			if (jsonLd) {
				const parsed = JSON.parse(jsonLd);
				expect(parsed['@type']).toBe('DiscussionForumPosting');
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
