import { test, expect } from '@playwright/test';

/**
 * JCE nested-comparison and geo source smoke tests.
 * Verifies that /compare renders the new source selector, geo hierarchy
 * (country/state/city) is selectable, and the nested comparison source
 * filters by nested type.
 */

test.describe('JCE Compare — nested and geo sources', () => {
	test('/compare?type=countries renders source selector', async ({ page }) => {
		const response = await page.goto('/compare?type=countries');
		expect(response?.status()).toBeLessThan(500);
		await expect(page.locator('text=Select source')).toBeVisible();
		// Source selector buttons sit directly under the "Select source" heading
		const sourceSection = page.locator('section:has-text("Select source")').first();
		await expect(sourceSection.locator('button:has-text("Country")').first()).toBeVisible();
		await expect(sourceSection.locator('button:has-text("Existing comparison")').first()).toBeVisible();
	});

	test('/compare?type=countries&source=country lists countries', async ({ page }) => {
		await page.goto('/compare?type=countries&source=country');
		await expect(page.locator('text=Countries available')).toBeVisible();
		// Argentina (alphabetically first) should be visible as a selectable card
		await expect(page.getByText('Argentina').first()).toBeVisible();
	});

	test('/compare?type=countries&source=comparison shows nested type filter', async ({ page }) => {
		await page.goto('/compare?type=countries&source=comparison');
		await expect(page.locator('text=Comparisons available')).toBeVisible();
		// The nested-type dropdown is present when allowed nested types exist
		await expect(page.locator('select').first()).toBeVisible();
	});

	test('/comparisons renders the public directory', async ({ page }) => {
		const response = await page.goto('/comparisons');
		expect(response?.status()).toBeLessThan(500);
		// At least one known published detailed comparison should appear
		await expect(page.locator('text=Healthcare Systems').first()).toBeVisible();
	});
});
