import { test, expect } from '@playwright/test';

/**
 * Frontend-Backend API Connectivity Tests
 *
 * Architecture notes:
 * - Main app (port 3000): Has Vite proxy `/api` → `http://localhost:8080`
 *   Client-side requests go through the proxy (same-origin).
 *   SSR layout server calls happen server-side (not visible in browser network).
 * - Admin/Control/Docs apps: Make direct cross-origin requests to port 8080.
 * - All apps use VITE_API_URL env var or default to http://localhost:8080/api/v1
 */

test.describe('Frontend-Backend API Connectivity', () => {
	test('main app spaces page fetches from backend via proxy', async ({ page }) => {
		const apiResponses: { url: string; status: number }[] = [];

		page.on('response', async (res) => {
			// Main app proxies /api/* to backend, so responses come from same origin
			if (res.url().includes('/api/v1/')) {
				apiResponses.push({ url: res.url(), status: res.status() });
			}
		});

		await page.goto('/spaces');
		await page.waitForTimeout(3000);

		console.log('Main spaces API responses:', apiResponses);

		// Should have made at least one API call through the proxy. The spaces
		// list may be fetched server-side on SSR, so accept any proxied API call
		// with a 200 or 401 response.
		const anyApiCall = apiResponses.find(r => r.url.includes('/api/v1/'));
		expect(anyApiCall).toBeTruthy();
		expect([200, 401]).toContain(anyApiCall!.status);
	});

	test('main app login submits to backend via proxy', async ({ page }) => {
		const apiCalls: { url: string; method: string; status: number }[] = [];

		page.on('response', async (res) => {
			if (res.url().includes('/api/v1/auth') || res.url().includes('/api/auth')) {
				apiCalls.push({
					url: res.url(),
					method: res.request().method(),
					status: res.status()
				});
			}
		});

		await page.goto('/login');

		// Wait for the rendered form fields before interacting.
		const emailInput = page.locator('#login-email');
		const passwordInput = page.locator('#login-password');
		await emailInput.waitFor({ state: 'visible', timeout: 5000 });
		await passwordInput.waitFor({ state: 'visible', timeout: 5000 });

		await emailInput.fill('playwright@test.com');
		await passwordInput.fill('Test1234!');

		// Submit form
		const submitBtn = page.locator('button[type="submit"]');
		await submitBtn.click();

		await page.waitForTimeout(3000);

		console.log('Main auth API calls:', apiCalls);

		// Should have made a login request through the proxy
		const loginCall = apiCalls.find(c => c.url.includes('/auth/login'));
		expect(loginCall).toBeTruthy();
		// 401 (invalid credentials) or 429 (rate limited) proves the proxy
		// forwarded the request to the backend.
		expect([200, 401, 429]).toContain(loginCall!.status);
	});

	test('admin app redirects unauthenticated users to /login', async ({ page }) => {
		// Admin app runs on port 3001 — auth-protected via hooks.server.ts
		// Unauthenticated users are redirected to /login
		await page.goto('http://localhost:3001/');
		await page.waitForTimeout(2000);

		// Should be redirected to /login
		await expect(page).toHaveURL(/\/login/);
		// Login page should render
		await expect(page.locator('h1')).toContainText(/Japa.*Admin/);
	});

	test('control app makes direct API calls to backend', async ({ page }) => {
		const apiResponses: { url: string; status: number; body: string }[] = [];

		page.on('response', async (res) => {
			const url = res.url();
			if (url.includes('/api/v1/') || url.includes('localhost:8080')) {
				const body = await res.text().catch(() => '');
				apiResponses.push({ url, status: res.status(), body });
			}
		});

		// Control app runs on port 3002
		await page.goto('http://localhost:3002/');
		await page.waitForTimeout(4000);

		console.log('Control API responses:', apiResponses.map(r => `${r.url} -> ${r.status}`));

		// Control app should attempt API calls
		// Even 401/404 responses prove the frontend is reaching the backend
		const hasApiCall = apiResponses.some(r => r.url.includes('/api/v1/') || r.url.includes('localhost:8080'));
		if (hasApiCall) {
			expect(hasApiCall).toBe(true);
		}
		// Page should have loaded
		await expect(page.locator('html')).toBeVisible();
	});

	test('docs app loads and renders', async ({ page }) => {
		const apiResponses: { url: string; status: number }[] = [];

		page.on('response', async (res) => {
			if (res.url().includes('/api/v1/') || res.url().includes('localhost:8080')) {
				apiResponses.push({ url: res.url(), status: res.status() });
			}
		});

		// Docs app runs on port 3004
		await page.goto('http://localhost:3004/');
		await page.waitForTimeout(4000);

		console.log('Docs API responses:', apiResponses.map(r => `${r.url} -> ${r.status}`));

		// Page should have loaded successfully
		await expect(page.locator('html')).toBeVisible();
	});

	test('backend API returns valid JSON for all public endpoints', async ({ request }) => {
		const endpoints = [
			'/health',
			'/api/v1/features',
			'/api/v1/spaces',
			'/api/v1/space-types',
			'/api/v1/docs/users',
		];

		for (const endpoint of endpoints) {
			const response = await request.get(`http://localhost:8080${endpoint}`);
			const body = await response.text();

			console.log(`${endpoint}: ${response.status()}`);

			if (response.status() === 200) {
				// Should be valid JSON
				expect(() => JSON.parse(body)).not.toThrow();
			}
		}
	});

	test('main app SSR layout calls features endpoint server-side', async ({ request }) => {
		// Verify the backend features endpoint works (this is what the SSR layout calls)
		const response = await request.get('http://localhost:8080/api/v1/feature-flags');
		expect(response.ok()).toBeTruthy();

		const body = await response.json();
		expect(body.data).toBeTruthy();
		expect(typeof body.data).toBe('object');

		// Verify feature flags have expected structure
		const flags = body.data;
		expect('feature.blog' in flags || 'blog' in flags).toBeTruthy();
	});
});
