import { test, expect } from '@playwright/test';

// Public roadmap surface smoke (§113) — board, changelog, releases, and the
// submit auth gate. roadmap.enabled is an operator flag, so every page may
// legitimately 404 (feature_disabled) — the smoke asserts each route
// responds cleanly and renders its heading when the feature is on.
test.describe('Main App — Roadmap', () => {
	for (const path of ['/roadmap', '/roadmap/changelog', '/roadmap/releases']) {
		test(`${path} responds without server error`, async ({ page }) => {
			const res = await page.goto(path);
			expect(res?.status() ?? 0).toBeLessThan(500);
			if (res?.status() === 200) {
				await expect(page.locator('h1, h2').first()).toBeVisible();
			}
		});
	}

	test('submit redirects to /login (or 404s while disabled)', async ({ page }) => {
		const res = await page.goto('/roadmap/submit');
		if (res?.status() === 404) return; // feature_disabled takes precedence
		await expect(page).toHaveURL(/\/login/);
	});

	test('board sort syncs to the URL when the board renders', async ({ page }) => {
		const res = await page.goto('/roadmap');
		test.skip(res?.status() === 404, 'roadmap feature disabled');
		const select = page.getByRole('combobox').first();
		if (await select.isVisible()) {
			await select.selectOption('recent');
			await expect(page).toHaveURL(/sort=recent/);
		}
	});
});
