import { test, expect } from '@playwright/test';

test.describe('Control App — Smoke Tests', () => {
	test('health dashboard loads', async ({ page }) => {
		await page.goto('/');
		// SPA renders immediately; API call fails gracefully
		await expect(page.locator('h1, h2').first()).toBeVisible();
	});

	test('queue page loads', async ({ page }) => {
		await page.goto('/queue');
		await expect(page.locator('h1, h2').first()).toBeVisible();
	});

	test('database page loads', async ({ page }) => {
		await page.goto('/db');
		await expect(page.locator('h1, h2').first()).toBeVisible();
	});

	test('audit page loads', async ({ page }) => {
		await page.goto('/audit');
		await expect(page.locator('h1, h2').first()).toBeVisible();
	});

	test('security page loads', async ({ page }) => {
		await page.goto('/security');
		await expect(page.locator('h1, h2').first()).toBeVisible();
	});

	test('cache page loads', async ({ page }) => {
		await page.goto('/cache');
		await expect(page.locator('h1, h2').first()).toBeVisible();
	});
});
