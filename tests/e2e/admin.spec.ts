import { test, expect } from '@playwright/test';

test.describe('Admin App — Smoke Tests', () => {
	test('unauthenticated root redirects to /login', async ({ page }) => {
		await page.goto('/');
		await expect(page).toHaveURL(/\/login/);
		await expect(page.locator('h1')).toContainText('JapaAdmin');
	});

	test('login page loads with form', async ({ page }) => {
		await page.goto('/login');
		await expect(page.locator('h1')).toContainText('JapaAdmin');
		await expect(page.locator('input[type="email"]')).toBeVisible();
		await expect(page.locator('input[type="password"]')).toBeVisible();
		await expect(page.locator('button[type="submit"]')).toBeVisible();
	});

	test('unauthenticated users page redirects to /login', async ({ page }) => {
		await page.goto('/users');
		await expect(page).toHaveURL(/\/login/);
	});

	test('unauthenticated moderation page redirects to /login', async ({ page }) => {
		await page.goto('/moderation');
		await expect(page).toHaveURL(/\/login/);
	});

	test('unauthenticated settings page redirects to /login', async ({ page }) => {
		await page.goto('/settings');
		await expect(page).toHaveURL(/\/login/);
	});

	test('unauthenticated tickets page redirects to /login', async ({ page }) => {
		await page.goto('/tickets');
		await expect(page).toHaveURL(/\/login/);
	});

	test('unauthenticated broadcast page redirects to /login', async ({ page }) => {
		await page.goto('/broadcast');
		await expect(page).toHaveURL(/\/login/);
	});
});
