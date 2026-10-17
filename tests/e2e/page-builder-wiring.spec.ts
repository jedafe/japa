import { test, expect } from '@playwright/test';

const BASE = 'http://localhost:3001';
const API_BASE = 'http://localhost:8080/api/v1';
const EMAIL = 'admin@japa.to';
const PASSWORD = 'Admin123!';

async function login(page: import('@playwright/test').Page) {
	await page.goto(`${BASE}/login`);
	await page.waitForLoadState('networkidle');
	await page.waitForTimeout(1000);
	await page.evaluate(
		({ email, password }) => {
			const eInput = document.querySelector('input[type="email"]') as HTMLInputElement;
			const pInput = document.querySelector('input[type="password"]') as HTMLInputElement;
			const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
			setter.call(eInput, email);
			eInput.dispatchEvent(new Event('input', { bubbles: true }));
			setter.call(pInput, password);
			pInput.dispatchEvent(new Event('input', { bubbles: true }));
		},
		{ email: EMAIL, password: PASSWORD }
	);
	await page.waitForTimeout(500);
	await page.getByRole('button', { name: 'Sign In' }).click();
	await page.waitForURL('**/');
	await page.waitForLoadState('networkidle');
	await page.waitForTimeout(2000);
	await expect(page.locator('text=Dashboard').first()).toBeVisible({ timeout: 15_000 });
}

test.describe('Page Builder Wiring', () => {
	test.beforeEach(async ({ page }) => {
		await login(page);
	});

	test('sidebar has all page-builder links', async ({ page }) => {
		const btn = page.locator('button').filter({ hasText: 'Customization' });
		await expect(btn).toBeVisible({ timeout: 10_000 });
		await btn.click();
		await page.waitForTimeout(500);
		await expect(page.locator('a[href="/page-builder"]').first()).toBeVisible();
		await expect(page.locator('a[href="/page-builder/pages"]').first()).toBeVisible();
		await expect(page.locator('a[href="/page-builder/themes"]').first()).toBeVisible();
		await expect(page.locator('a[href="/page-templates"]').first()).toBeVisible();
	});

	test('/page-builder loads blocks', async ({ page }) => {
		await page.goto(`${BASE}/page-builder`);
		await page.waitForLoadState('networkidle');
		await expect(page.locator('text=Page Builder').first()).toBeVisible({ timeout: 10_000 });
	});

	test('/page-builder/pages loads', async ({ page }) => {
		await page.goto(`${BASE}/page-builder/pages`);
		await page.waitForLoadState('networkidle');
		await page.waitForTimeout(2000);
		await expect(
			page.locator('button, a').filter({ hasText: /create page/i }).first()
		).toBeVisible({ timeout: 10_000 });
	});

	test('/page-builder/themes loads', async ({ page }) => {
		await page.goto(`${BASE}/page-builder/themes`);
		await page.waitForLoadState('networkidle');
		await page.waitForTimeout(2000);
		await expect(
			page.locator('button, a').filter({ hasText: /create theme/i }).first()
		).toBeVisible({ timeout: 10_000 });
	});

	test('/page-templates loads', async ({ page }) => {
		await page.goto(`${BASE}/page-templates`);
		await page.waitForLoadState('networkidle');
		await page.waitForTimeout(2000);
		await expect(
			page.locator('button, a').filter({ hasText: /create/i }).first()
		).toBeVisible({ timeout: 10_000 });
	});

	test('page editor loads with save button', async ({ page }) => {
		const resp = await page.request.post(`${API_BASE}/auth/login`, {
			data: { email: EMAIL, password: PASSWORD },
		});
		const token = (await resp.json()).data?.access_token ?? '';
		const createRes = await page.request.post(`${API_BASE}/admin/pages`, {
			headers: { Authorization: `Bearer ${token}` },
			data: { title: `E2E ${Date.now()}`, slug: `e2e-${Date.now()}` },
		});
		const pageId = (await createRes.json()).data?.id ?? '';
		expect(pageId).toBeTruthy();

		await page.goto(`${BASE}/page-builder/pages/${pageId}`);
		await page.waitForLoadState('networkidle');
		await page.waitForTimeout(3000);
		await expect(
			page.locator('button').filter({ hasText: /save/i }).first()
		).toBeVisible({ timeout: 15_000 });
	});
});
