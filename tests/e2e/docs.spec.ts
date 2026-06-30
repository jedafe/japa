import { test, expect } from '@playwright/test';

test.describe('Docs App — Smoke Tests', () => {
	test('docs home loads', async ({ page }) => {
		await page.goto('/');
		await expect(page).toHaveTitle(/JapaConnect Documentation/);
	});

	test('audience page loads', async ({ page }) => {
		// The docs app uses [audience] dynamic route
		await page.goto('/users');
		await expect(page.locator('h1, h2').first()).toBeVisible();
	});
});
