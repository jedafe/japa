import { test, expect } from '@playwright/test';

test.describe('Docs App — Smoke Tests', () => {
	test.beforeEach(({ page }) => {
		// Collect console messages so we can fail on unexpected errors/warnings.
		(page as any).__console = [] as { type: string; text: string }[];
		page.on('console', (msg) => {
			((page as any).__console as { type: string; text: string }[]).push({
				type: msg.type(),
				text: msg.text()
			});
		});
		page.on('pageerror', (err) => {
			((page as any).__console as { type: string; text: string }[]).push({
				type: 'pageerror',
				text: err.message
			});
		});
	});

	test('docs home loads without console errors', async ({ page }) => {
		await page.goto('/');
		await expect(page).toHaveTitle(/JapaConnect Documentation/);
		assertNoErrors(page);
	});

	test('audience page loads and lists pages', async ({ page }) => {
		await page.goto('/docs/users');
		await expect(page.locator('h1, h2').first()).toBeVisible();
		assertNoErrors(page);
	});

	test('api audience landing loads', async ({ page }) => {
		await page.goto('/docs/api');
		await expect(page.locator('h1').first()).toBeVisible();
		assertNoErrors(page);
	});

	test('access denied page loads', async ({ page }) => {
		await page.goto('/docs/superadmin');
		await expect(page.locator('h1')).toContainText(/access denied/i);
		assertNoErrors(page);
	});

	test('search page loads', async ({ page }) => {
		await page.goto('/docs/search?q=test');
		await expect(page.locator('h1')).toBeVisible();
		assertNoErrors(page);
	});

	test('search palette can be opened by clicking the search button', async ({ page }) => {
		await page.goto('/docs');
		await page.waitForLoadState('networkidle');
		const launcher = page.locator('.launcher:visible');
		await launcher.waitFor({ state: 'visible' });
		await launcher.click();
		await expect(page.locator('[role="dialog"] input')).toBeVisible();
		assertNoErrors(page);
	});

	test('search palette can be opened with keyboard shortcut', async ({ page }) => {
		await page.goto('/docs');
		await page.waitForLoadState('networkidle');
		await page.click('body');
		await page.keyboard.press('Control+k');
		await expect(page.locator('[role="dialog"] input')).toBeVisible();
		assertNoErrors(page);
	});

	test('search palette can query and return results', async ({ page }) => {
		await page.goto('/docs');
		await page.waitForLoadState('networkidle');
		await page.locator('.launcher:visible').click();
		const input = page.locator('[role="dialog"] input');
		await input.fill('test');
		await page.waitForTimeout(600);
		// The backend may return results or the empty state; either is fine,
		// but the input value must be reflected and no errors must occur.
		await expect(input).toHaveValue('test');
		assertNoErrors(page);
	});

	test('auth-only playground redirects guests to login', async ({ page }) => {
		await page.goto('/docs/api/playground');
		await page.waitForLoadState('networkidle');
		// The server returns a 307 to the main app login page.
		expect(page.url()).toMatch(/\/login/);
	});

	test('authenticated user can load API keys and playground', async ({ page }) => {
		await login(page, { email: 'user1@japa.to', password: 'Demo1234!' });

		await page.goto('/docs/api/keys');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h1').first()).toBeVisible();
		await expect(page).toHaveTitle(/API Keys/);
		assertNoErrors(page);

		await page.goto('/docs/api/playground');
		await page.waitForLoadState('networkidle');
		await expect(page.locator('h1').first()).toBeVisible();
		await expect(page).toHaveTitle(/API Playground/);
		assertNoErrors(page);
	});
});

function assertNoErrors(page: any) {
	const logs: { type: string; text: string }[] = (page as any).__console ?? [];
	const bad = logs.filter((l) => {
		if (l.type !== 'error' && l.type !== 'pageerror' && l.type !== 'warn') return false;
		// Ignore transient Vite dev-server optimization noise (CodeMirror chunks
		// can 504 on first load while being re-optimized; production builds do not).
		if (l.text.includes('Outdated Optimize Dep')) return false;
		if (l.text.includes('Failed to fetch dynamically imported module')) return false;
		return true;
	});
	if (bad.length > 0) {
		throw new Error(`Console errors/warnings:\n${bad.map((l) => `[${l.type}] ${l.text}`).join('\n')}`);
	}
}

async function login(page: any, user: { email: string; password: string }) {
	// Call the Go backend directly to obtain a session token, then seed the
	// jc_token cookie so the docs SSR shell resolves the user.
	const res = await page.context().request.post('http://localhost:8080/api/v1/auth/login', {
		data: { email: user.email, password: user.password, website: '' }
	});
	expect(res.ok(), `login API failed: ${await res.text()}`).toBeTruthy();
	const payload = await res.json();
	const token = payload?.data?.access_token;
	expect(token, 'no access_token in login response').toBeTruthy();
	await page.context().addCookies([
		{ name: 'jc_token', value: token, domain: 'localhost', path: '/', httpOnly: true, sameSite: 'Lax' }
	]);
}
