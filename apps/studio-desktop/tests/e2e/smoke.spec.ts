import { test, expect } from '@playwright/test';

test.describe('studio desktop routes', () => {
	test('queue page renders with navigation', async ({ page }) => {
		const errors: string[] = [];
		page.on('pageerror', (err) => errors.push(err.message));

		await page.goto('/');
		await expect(page.locator('nav')).toContainText('Queue');
		await expect(page.getByRole('heading', { name: 'Content Queue' })).toBeVisible();

		expect(errors).toHaveLength(0);
	});

	test('prompt page renders', async ({ page }) => {
		const errors: string[] = [];
		page.on('pageerror', (err) => errors.push(err.message));

		await page.goto('/prompt');
		await expect(page.getByRole('heading', { name: 'Prompt Builder' })).toBeVisible();

		expect(errors).toHaveLength(0);
	});

	test('batch page renders', async ({ page }) => {
		const errors: string[] = [];
		page.on('pageerror', (err) => errors.push(err.message));

		await page.goto('/batch');
		await expect(page.getByRole('heading', { name: 'Batch Prompt' })).toBeVisible();

		expect(errors).toHaveLength(0);
	});

	test('sync log page renders', async ({ page }) => {
		const errors: string[] = [];
		page.on('pageerror', (err) => errors.push(err.message));

		await page.goto('/sync');
		await expect(page.getByRole('heading', { name: 'Sync Log' })).toBeVisible();

		expect(errors).toHaveLength(0);
	});

	test('settings page renders', async ({ page }) => {
		const errors: string[] = [];
		page.on('pageerror', (err) => errors.push(err.message));

		await page.goto('/settings');
		await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();

		expect(errors).toHaveLength(0);
	});
});
